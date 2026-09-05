package server

import (
	"slices"
	"testing"
	"time"

	"termon.sh/internal/battle"
	"termon.sh/internal/dojo"
	"termon.sh/internal/game"
	"termon.sh/internal/store"
)

func TestSparringRosterMatchesPlayerLevels(t *testing.T) {
	set := testHub(t).set
	party := []game.Monster{
		{Species: "rootkit", Level: 3},
		{Species: "emberbyte", Level: 7},
		{Species: "aquabit", Level: 11},
	}
	roster, err := dojo.BuildSparringRoster(set, party, 100)
	if err != nil {
		t.Fatal(err)
	}
	for i := range party {
		if roster[i].Level != party[i].Level {
			t.Fatalf("slot %d level = %d want %d", i+1, roster[i].Level, party[i].Level)
		}
	}
}

func TestSparringRosterSharedAcrossTiersAndRemixesAfterClear(t *testing.T) {
	h := testHub(t)
	id := "sparring-roster"
	onboardTrainerFull(t, h, id, "rootkit")
	day := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	species := func(p SparringPreviewMsg) []string {
		out := make([]string, len(p.Slots))
		for i, slot := range p.Slots {
			out[i] = slot.Species
		}
		return out
	}

	apprentice, err := h.sparringPreview(id, dojo.TierApprentice, day, 0)
	if err != nil {
		t.Fatal(err)
	}
	rival, err := h.sparringPreview(id, dojo.TierRival, day, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(species(apprentice), species(rival)) {
		t.Fatalf("daily roster differs by tier: %v vs %v", species(apprentice), species(rival))
	}
	locked, err := h.sparringPreview(id, dojo.TierApprentice, day, 3)
	if err != nil {
		t.Fatal(err)
	}
	if locked.Remix != 0 || !slices.Equal(species(locked), species(apprentice)) {
		t.Fatalf("uncleared tier accepted remix: remix=%d roster=%v", locked.Remix, species(locked))
	}

	dayString := dojo.ServerDayString(day)
	_, err = h.saves.RecordActivityResult(store.ActivityRecord{
		Kind: store.KindSparring, NaturalKey: id + ":sparring:apprentice:" + dayString,
		TrainerID: id, Outcome: store.OutcomeCleared, SparringTier: dojo.TierApprentice,
		CompletedAt: day,
	})
	if err != nil {
		t.Fatal(err)
	}
	remix, err := h.sparringPreview(id, dojo.TierApprentice, day, 1)
	if err != nil {
		t.Fatal(err)
	}
	if remix.Remix != 1 || slices.Equal(species(remix), species(apprentice)) {
		t.Fatalf("cleared tier did not remix: remix=%d roster=%v original=%v", remix.Remix, species(remix), species(apprentice))
	}
	rivalLocked, err := h.sparringPreview(id, dojo.TierRival, day, 1)
	if err != nil {
		t.Fatal(err)
	}
	if rivalLocked.Remix != 0 || !slices.Equal(species(rivalLocked), species(rival)) {
		t.Fatalf("uncleared Rival accepted remix: remix=%d roster=%v", rivalLocked.Remix, species(rivalLocked))
	}
}

func TestStartSparringUsesPreviewedRemix(t *testing.T) {
	h := testHub(t)
	id := "sparring-remix"
	onboardTrainerFull(t, h, id, "rootkit")
	placeNearMaster(t, h, id)
	day := dojo.ServerDay(time.Now().UTC())
	_, err := h.saves.RecordActivityResult(store.ActivityRecord{
		Kind: store.KindSparring, NaturalKey: id + ":sparring:apprentice:" + dojo.ServerDayString(day),
		TrainerID: id, Outcome: store.OutcomeCleared, SparringTier: dojo.TierApprentice,
		CompletedAt: day,
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := h.PreviewSparring(id, dojo.TierApprentice, 1)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Remix != 1 {
		t.Fatalf("preview remix = %d, want 1", preview.Remix)
	}
	if err := h.StartSparring(id, dojo.TierApprentice, preview.Remix); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	match := h.matches[id]
	h.mu.Unlock()
	if match == nil {
		t.Fatal("sparring match was not installed")
	}
	foes := match.bt.Snapshot(id).FoeRoster
	for i := range preview.Slots {
		if foes[i].Species != preview.Slots[i].Species {
			t.Fatalf("slot %d started as %q, preview showed %q", i+1, foes[i].Species, preview.Slots[i].Species)
		}
	}
}

func TestTwoTrainersSameDailyFixture(t *testing.T) {
	h := testHub(t)
	day := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	a := h.dailyMenuInfo("trainer-a", day)
	b := h.dailyMenuInfo("trainer-b", day)
	if a.ID != b.ID {
		t.Fatalf("daily id %q vs %q", a.ID, b.ID)
	}
	fx := dojo.FixtureForDay(day)
	p1, o1, err := dojo.DailyParties(h.set, fx)
	if err != nil {
		t.Fatal(err)
	}
	p2, o2, err := dojo.DailyParties(h.set, fx)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if p1.Members[i].Monster.Species != p2.Members[i].Monster.Species {
			t.Fatal("daily player teams differ")
		}
		if o1.Members[i].Monster.Species != o2.Members[i].Monster.Species {
			t.Fatal("daily opponent teams differ")
		}
	}
}

func TestSparringFirstClearThenReplayNoXP(t *testing.T) {
	h := testHub(t)
	id := "sparring-xp"
	onboardTrainerFull(t, h, id, "rootkit")
	sv, _ := h.Load(id)
	xpBefore := totalPartyXP(sv)
	dayStr := dojo.ServerDayString(time.Now().UTC())
	key := id + ":sparring:apprentice:" + dayStr
	active := []string{sv.Party[0]}
	first, err := h.saves.RecordActivityResult(store.ActivityRecord{
		Kind: "sparring", NaturalKey: key, TrainerID: id,
		ActiveIDs: active, Outcome: "cleared", SparringTier: dojo.TierApprentice,
		CompletedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if totalPartyXP(first) <= xpBefore {
		t.Fatal("first sparring clear should pay XP")
	}
	xpMid := totalPartyXP(first)
	second, err := h.saves.RecordActivityResult(store.ActivityRecord{
		Kind: "sparring", NaturalKey: key, TrainerID: id,
		ActiveIDs: active, Outcome: "cleared", SparringTier: dojo.TierApprentice,
		CompletedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if totalPartyXP(second) != xpMid {
		t.Fatalf("replay paid XP again: %d -> %d", xpMid, totalPartyXP(second))
	}
	exists, _ := h.saves.ActivityExists(id, key)
	if !exists {
		t.Fatal("expected sparring activity row")
	}
}

func TestDailyFirstClearXPThenMasteryOnly(t *testing.T) {
	h := testHub(t)
	id := "daily-xp"
	onboardTrainerFull(t, h, id, "rootkit")
	dayStr := dojo.ServerDayString(time.Now().UTC())
	sv, _ := h.Load(id)
	xpBefore := totalPartyXP(sv)
	active := []string{sv.Party[0]}
	first, err := h.saves.RecordActivityResult(store.ActivityRecord{
		Kind: "daily_xp", NaturalKey: id + ":daily:" + dayStr, TrainerID: id,
		ActiveIDs: active, Outcome: "cleared", CompletedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if totalPartyXP(first) <= xpBefore {
		t.Fatal("daily first clear should pay XP")
	}
	xpMid := totalPartyXP(first)
	second, err := h.saves.RecordActivityResult(store.ActivityRecord{
		Kind: "daily_mastery", NaturalKey: id + ":daily-mastery:" + dayStr, TrainerID: id,
		ActiveIDs: active, Outcome: "mastery", MasteryOnly: true, CompletedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if totalPartyXP(second) != xpMid {
		t.Fatal("mastery-only record should not pay XP")
	}
}

func TestSparringDailyNeverChangeWL(t *testing.T) {
	h := testHub(t)
	id := "dojo-wl"
	onboardTrainerFull(t, h, id, "rootkit")
	sv, _ := h.Load(id)
	w, l := sv.Wins, sv.Losses
	dayStr := dojo.ServerDayString(time.Now().UTC())
	active := []string{sv.Party[0]}
	if _, err := h.saves.RecordActivityResult(store.ActivityRecord{
		Kind: "sparring", NaturalKey: id + ":sparring:apprentice:" + dayStr, TrainerID: id,
		ActiveIDs: active, Outcome: "cleared", SparringTier: dojo.TierApprentice,
		CompletedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.saves.RecordActivityResult(store.ActivityRecord{
		Kind: "daily_xp", NaturalKey: id + ":daily:" + dayStr, TrainerID: id,
		ActiveIDs: active, Outcome: "cleared", CompletedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	sv, _ = h.Load(id)
	if sv.Wins != w || sv.Losses != l {
		t.Fatal("dojo activities changed W/L")
	}
}

func TestPolicyIgnoresPendingHiddenAction(t *testing.T) {
	h := testHub(t)
	set := h.set
	partyA := battle.Party{Trainer: "a", Members: []battle.PartyMember{{Monster: game.Monster{
		ID: "a1", Species: "rootkit", Level: 20, BattleLoadout: []string{"root_access", "chmod", "sudo", "setuid"},
	}}}}
	partyB := battle.Party{Trainer: dojo.BotTrainer, Members: []battle.PartyMember{{Monster: game.Monster{
		ID: "b1", Species: "scorchip", Level: 20, BattleLoadout: []string{"bit_flip", "reflow", "latch_up", "bus_error"},
	}}}}
	bt, err := battle.New(set, partyA, partyB, battle.Seeded(99))
	if err != nil {
		t.Fatal(err)
	}
	if err := bt.Select("a", battle.Action{Kind: battle.ActionMove, Move: "sudo"}); err != nil {
		t.Fatal(err)
	}
	view, _ := bt.PolicyViewFor(dojo.BotTrainer)
	act, _, err := dojo.ChoosePolicyAction(set, view, dojo.TierConfig(dojo.TierRival), battle.Seeded(1))
	if err != nil {
		t.Fatal(err)
	}
	if act.Kind == battle.ActionMove && act.Move == "sudo" {
		t.Fatal("policy chose trainer hidden move")
	}
}

func TestOpenDojoMenuIncludesSparringDaily(t *testing.T) {
	h := testHub(t)
	id := "dojo-full-menu"
	onboardTrainerFull(t, h, id, "rootkit")
	placeNearMaster(t, h, id)
	menu, err := h.OpenDojoMenu(id)
	if err != nil {
		t.Fatal(err)
	}
	if menu.ServerDay == "" || menu.Daily.ID == "" {
		t.Fatalf("menu missing daily: %+v", menu)
	}
}

func totalPartyXP(sv *game.Save) int64 {
	var n int64
	for _, id := range sv.Party {
		if m, ok := game.MonsterByID(sv, id); ok {
			n += m.XP
		}
	}
	return n
}
