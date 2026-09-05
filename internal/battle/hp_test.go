package battle

import "testing"

// TestFieldedHPAfterProjectsFieldedMonsters switches a damaged lead out and
// back in under scripted rolls, then checks the viewer-scoped projection
// against exact ledger arithmetic at each playhead position.
func TestFieldedHPAfterProjectsFieldedMonsters(t *testing.T) {
	set := testSet()
	bt := newBattle(t, set,
		Party{Trainer: "a", Members: []PartyMember{
			{Monster: mon("a1", "spark", "jab")},
			{Monster: mon("a2", "twin", "poke")},
		}},
		partyOf("b", mon("b1", "moss", "leaf")),
		&scriptRand{vals: concat(hitNoCritMinVar(), hitNoCritMinVar(), hitNoCritMinVar(), hitNoCritMinVar())},
	)

	// Turn 1: spark's jab deals 25 to b1; moss's leaf deals 12 to a1.
	// Turn 2: a switches to a2, the incoming Monster, and leaf deals 12.
	// Turn 3: a switches the damaged a1 back in, and leaf deals 12 again.
	resolveTurn(t, bt, moveAct("jab"), moveAct("leaf"))
	resolveTurn(t, bt, switchAct("a2"), moveAct("leaf"))
	resolveTurn(t, bt, switchAct("a1"), moveAct("leaf"))
	events := bt.Events()
	if len(events) != 16 {
		t.Fatalf("event log has %d events, want 16: %v", len(events), kinds(events))
	}

	platesAt := func(n int, wantYou, wantFoe FieldedHP) {
		t.Helper()
		you, foe := bt.FieldedHPAfter("a", n)
		if you != wantYou {
			t.Fatalf("FieldedHPAfter(a, %d) you = %+v, want %+v", n, you, wantYou)
		}
		if foe != wantFoe {
			t.Fatalf("FieldedHPAfter(a, %d) foe = %+v, want %+v", n, foe, wantFoe)
		}
	}

	// Before the send-out beats, the opening leads are the fielded Monsters.
	platesAt(0, FieldedHP{MonsterID: "a1", HP: 50, MaxHP: 50}, FieldedHP{MonsterID: "b1", HP: 50, MaxHP: 50})
	// After turn 1 the lead carries its damage and the foe has taken 25.
	platesAt(8, FieldedHP{MonsterID: "a1", HP: 38, MaxHP: 50}, FieldedHP{MonsterID: "b1", HP: 25, MaxHP: 50})
	// At the switch-to-a2 beat the incoming reserve is named at full HP; the
	// benched a1's damage stays off the return path — the projection only
	// ever names fielded Monsters, so it cannot leak bench HP.
	platesAt(10, FieldedHP{MonsterID: "a2", HP: 50, MaxHP: 50}, FieldedHP{MonsterID: "b1", HP: 25, MaxHP: 50})
	// At the re-entry beat a1 returns damaged, not reset.
	platesAt(14, FieldedHP{MonsterID: "a1", HP: 38, MaxHP: 50}, FieldedHP{MonsterID: "b1", HP: 25, MaxHP: 50})
	// The live position equals the engine state, even past the log's end.
	platesAt(16, FieldedHP{MonsterID: "a1", HP: 26, MaxHP: 50}, FieldedHP{MonsterID: "b1", HP: 25, MaxHP: 50})
	platesAt(21, FieldedHP{MonsterID: "a1", HP: 26, MaxHP: 50}, FieldedHP{MonsterID: "b1", HP: 25, MaxHP: 50})

	// The projection is scoped to the viewer: from b's side the same position
	// names b's fielded Monster as you and a's as foe.
	you, foe := bt.FieldedHPAfter("b", 10)
	if you != (FieldedHP{MonsterID: "b1", HP: 25, MaxHP: 50}) || foe != (FieldedHP{MonsterID: "a2", HP: 50, MaxHP: 50}) {
		t.Fatalf("FieldedHPAfter(b, 10) = %+v, %+v", you, foe)
	}
	// An unknown viewer gets nothing.
	you, foe = bt.FieldedHPAfter("nobody", 10)
	if you != (FieldedHP{}) || foe != (FieldedHP{}) {
		t.Fatalf("unknown viewer = %+v, %+v, want zero values", you, foe)
	}
}
