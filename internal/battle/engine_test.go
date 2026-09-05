// Content-pack smoke coverage; deterministic mechanics live in battle_test.go.

package battle

import (
	"testing"

	"termon.sh/internal/content"
	"termon.sh/internal/game"
)

func loadSet(t *testing.T) *content.Set {
	t.Helper()
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	return set
}

func trainerParty(hash, speciesSlug string, set *content.Set) Party {
	spec := set.Species[speciesSlug]
	moves := make([]string, 0, 4)
	for _, e := range spec.Movepool {
		if len(moves) == 4 {
			break
		}
		moves = append(moves, e.Move)
	}
	return partyOf(hash, game.Monster{
		ID: hash + "-lead", Species: speciesSlug, Nickname: spec.Name, Level: 1,
		MoveLibrary: moves, BattleLoadout: moves,
	})
}

func TestContentPackBattleRunsToKO(t *testing.T) {
	set := loadSet(t)
	bt, err := New(set, trainerParty("a", "rootkit", set), trainerParty("b", "aquabit", set), Seeded(42))
	if err != nil {
		t.Fatal(err)
	}

	a, _ := bt.Fighter("a")
	b, _ := bt.Fighter("b")
	mv := b.Moves[0]
	resolveTurn(t, bt, moveAct(a.Moves[0]), moveAct(mv))
	if len(bt.Events()) == 0 {
		t.Fatal("expected events after the first turn")
	}
	if bt.State() == StateOver {
		t.Fatal("battle should not be over after one turn")
	}
	for range 100 {
		if bt.State() == StateOver {
			break
		}
		if bt.State() == StateAwaitingReplacement {
			snap := bt.Snapshot("b")
			for _, m := range snap.YourParty {
				if !m.Fainted && !m.Active {
					if err := bt.Replace("b", m.ID); err != nil {
						t.Fatalf("replace: %v", err)
					}
					advanceIfRevealing(t, bt)
					break
				}
			}
			continue
		}
		if bt.State() != StateAwaitingActions {
			advanceIfRevealing(t, bt)
			continue
		}
		resolveTurn(t, bt, moveAct(a.Moves[0]), moveAct(mv))
	}
	if bt.State() != StateOver {
		t.Fatal("battle did not end within 100 turns")
	}
	if bt.Winner() == "" {
		t.Error("winner is empty after KO")
	}
	if bt.Reason() != EndKO {
		t.Errorf("reason = %v, want ko", bt.Reason())
	}
}

func TestDamageBounds(t *testing.T) {
	set := loadSet(t)
	bt, err := New(set, trainerParty("a", "rootkit", set), trainerParty("b", "emberbyte", set), Seeded(7))
	if err != nil {
		t.Fatal(err)
	}
	startHP, _ := bt.HP("a")
	a, _ := bt.Fighter("a")
	b, _ := bt.Fighter("b")
	mv := b.Moves[0]
	ended := false
	for range 50 {
		if bt.State() == StateOver {
			ended = true
			break
		}
		if bt.State() == StateAwaitingReplacement {
			advanceIfRevealing(t, bt)
			continue
		}
		if bt.State() != StateAwaitingActions {
			advanceIfRevealing(t, bt)
			continue
		}
		resolveTurn(t, bt, moveAct(a.Moves[0]), moveAct(mv))
	}
	hp, _ := bt.HP("a")
	if hp < 0 {
		t.Errorf("hp below zero: %d", hp)
	}
	if hp == startHP && !ended {
		t.Error("no damage dealt across 50 turns")
	}
}

func TestEventsVersionAndCount(t *testing.T) {
	set := loadSet(t)
	bt, err := New(set, trainerParty("a", "rootkit", set), trainerParty("b", "aquabit", set), Seeded(42))
	if err != nil {
		t.Fatal(err)
	}
	if bt.EventCount() == 0 || bt.EventsVersion() == 0 {
		t.Fatalf("fresh battle: count=%d version=%d, want send-out events", bt.EventCount(), bt.EventsVersion())
	}
	a, _ := bt.Fighter("a")
	b, _ := bt.Fighter("b")
	mvA := a.Moves[0]
	mvB := b.Moves[0]
	for range 200 {
		if bt.State() == StateOver {
			break
		}
		if bt.State() == StateAwaitingReplacement {
			advanceIfRevealing(t, bt)
			continue
		}
		if bt.State() != StateAwaitingActions {
			advanceIfRevealing(t, bt)
			continue
		}
		resolveTurn(t, bt, moveAct(mvA), moveAct(mvB))
	}
	events := bt.Events()
	if bt.EventCount() != len(events) {
		t.Fatalf("EventCount=%d, len(Events())=%d", bt.EventCount(), len(events))
	}
	if bt.EventsVersion() == 0 {
		t.Fatal("version never advanced despite appended events")
	}
	snap1 := bt.Events()
	before := bt.EventsVersion()
	countBefore := bt.EventCount()
	if bt.State() != StateOver {
		if err := bt.Select("a", moveAct(mvA)); err == nil {
			t.Fatal("select after over should fail or battle not over")
		}
		_ = before
		_ = countBefore
	}
	snap2 := bt.Events()
	if len(snap1) != len(snap2) {
		t.Fatal("snapshots diverged")
	}
}

func TestKnowsMoveUnknownSlug(t *testing.T) {
	set := loadSet(t)
	bt, err := New(set, trainerParty("a", "rootkit", set), trainerParty("b", "aquabit", set), Seeded(42))
	if err != nil {
		t.Fatal(err)
	}
	if err := bt.Select("a", moveAct("no_such_move")); err == nil {
		t.Fatal("unknown slug should be rejected")
	}
}
