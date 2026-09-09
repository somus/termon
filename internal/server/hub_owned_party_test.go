package server

import (
	"reflect"
	"slices"
	"testing"

	"termon.sh/internal/game"
	"termon.sh/internal/lobby"
)

func TestPvPRoutesUseEarnedProgression(t *testing.T) {
	for _, route := range []string{"queue", "challenge"} {
		t.Run(route, func(t *testing.T) {
			h := testHub(t)
			onboardTrainerFull(t, h, "owner", "rootkit")
			onboardTrainerFull(t, h, "foe", "emberbyte")
			before, err := h.Load("owner")
			if err != nil {
				t.Fatal(err)
			}
			if route == "queue" {
				if _, _, err := h.FindBattle("owner"); err != nil {
					t.Fatal(err)
				}
				if _, _, err := h.FindBattle("foe"); err != nil {
					t.Fatal(err)
				}
			} else {
				h.mu.Lock()
				room := trainerRoom(h, "owner")
				owner, _ := room.Get("owner")
				room.Leave("foe")
				err := room.Place(lobby.Presence{Hash: "foe", Handle: "foe", Species: "emberbyte", X: owner.X + 1, Y: owner.Y})
				h.mu.Unlock()
				if err != nil {
					t.Fatal(err)
				}
				if err := h.Challenge("owner"); err != nil {
					t.Fatal(err)
				}
				if err := h.Respond("foe", true); err != nil {
					t.Fatal(err)
				}
			}
			h.mu.Lock()
			match := h.matches["owner"]
			h.mu.Unlock()
			if match == nil {
				t.Fatal("route did not start battle")
			}
			view, ok := match.bt.PolicyViewFor("owner")
			if !ok || len(view.Self) != 3 {
				t.Fatal("missing full owned battle Party")
			}
			for i, member := range view.Self {
				owned, ok := game.MonsterByID(before, before.Party[i])
				if !ok {
					t.Fatal("missing owned Monster")
				}
				if member.ID != owned.ID || member.Species != owned.Species || member.Level != owned.Level || !slices.Equal(member.Loadout, owned.BattleLoadout) {
					t.Fatalf("battle member %+v differs from owned progression %+v", member, owned)
				}
				base := h.set.Species[owned.Species].BaseStats
				want := [5]int{game.NaturalStat(base.HP, owned.Level), game.NaturalStat(base.Attack, owned.Level), game.NaturalStat(base.Defense, owned.Level), game.NaturalStat(base.SpAttack, owned.Level), game.NaturalStat(base.Speed, owned.Level)}
				got := [5]int{member.MaxHP, member.Atk, member.Def, member.SpA, member.Spe}
				if got != want {
					t.Fatalf("stats = %v, want earned stats %v", got, want)
				}
			}
			after, err := h.Load("owner")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.Collection, after.Collection) {
				t.Fatal("entering PvP mutated progression")
			}
		})
	}
}

func TestOwnedBattlePartyDoesNotAliasSaveLoadout(t *testing.T) {
	save := &game.Save{Party: [3]string{"a", "b", "c"}, Collection: []game.Monster{
		{ID: "a", Species: "rootkit", Level: 7, BattleLoadout: []string{"bark_bash"}},
		{ID: "b", Species: "emberbyte", Level: 14, BattleLoadout: []string{"burn_in"}},
		{ID: "c", Species: "aquabit", Level: 24, BattleLoadout: []string{"stream_pulse"}},
	}}
	party, err := ownedBattleParty("owner", save)
	if err != nil {
		t.Fatal(err)
	}
	party.Members[0].Monster.BattleLoadout[0] = "changed"
	if save.Collection[0].BattleLoadout[0] != "bark_bash" {
		t.Fatal("battle loadout aliases Save")
	}
	save.Party[1] = "a"
	if _, err := ownedBattleParty("owner", save); err == nil {
		t.Fatal("duplicate owned Monster accepted")
	}
	save.Party[1] = "missing"
	if _, err := ownedBattleParty("owner", save); err == nil {
		t.Fatal("unowned Monster accepted")
	}
}
