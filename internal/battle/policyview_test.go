package battle

import (
	"reflect"
	"testing"
)

func TestPolicyViewDoesNotExposeHiddenOpponentState(t *testing.T) {
	set := testSet()
	makeView := func(move string, reserveHP int) PolicyView {
		t.Helper()
		bt := newBattle(t, set,
			partyOf("a", mon("a1", "spark", "jab")),
			partyOf("b", mon("b1", "spark", move), mon("b2", "tank", "leaf")),
			Seeded(1),
		)
		bt.sides[1].members[1].hp = reserveHP
		view, ok := bt.PolicyViewFor("a")
		if !ok {
			t.Fatal("missing policy view")
		}
		return view
	}
	first, second := makeView("jab", 60), makeView("bolt", 7)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("private loadout or reserve HP changed public inputs: %+v vs %+v", first, second)
	}
	if first.FoeRoster[1].HP != 0 || len(first.FoeActive.RevealedMoves) != 0 {
		t.Fatalf("unrevealed opponent information: %+v", first)
	}
}

func TestPolicyViewRevealsOnlyResolvedMoves(t *testing.T) {
	bt := soloBattle(t, testSet(), mon("a1", "spark", "jab"), mon("b1", "spark", "bolt"), Seeded(1))
	if err := bt.Select("b", moveAct("bolt")); err != nil {
		t.Fatal(err)
	}
	before, _ := bt.PolicyViewFor("a")
	if len(before.FoeActive.RevealedMoves) != 0 {
		t.Fatal("locked action leaked before resolution")
	}
	if err := bt.Select("a", moveAct("jab")); err != nil {
		t.Fatal(err)
	}
	after, _ := bt.PolicyViewFor("a")
	if len(after.FoeActive.RevealedMoves) != 1 || after.FoeActive.RevealedMoves[0] != "bolt" {
		t.Fatalf("resolved Move missing: %+v", after.FoeActive)
	}
	after.FoeActive.RevealedMoves[0] = "modified"
	again, _ := bt.PolicyViewFor("a")
	if again.FoeActive.RevealedMoves[0] != "bolt" {
		t.Fatal("caller mutated Battle through policy view")
	}
}
