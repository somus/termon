package dojo_test

import (
	"testing"

	"termon.sh/internal/dojo"
	"termon.sh/internal/game"
)

func TestReferenceLoadoutFourUnique(t *testing.T) {
	set := testContentSet(t)
	for slug := range set.Species {
		got, err := dojo.ReferenceLoadout(set, slug, 1)
		if err != nil {
			t.Fatalf("%s L1: %v", slug, err)
		}
		if len(got) != 4 {
			t.Errorf("%s L1 Reference Loadout length %d, want 4", slug, len(got))
		}
		seen := map[string]bool{}
		for _, mv := range got {
			if seen[mv] {
				t.Errorf("%s L1 Reference Loadout duplicate %s", slug, mv)
			}
			seen[mv] = true
			if set.Moves[mv].Power > 75 {
				t.Errorf("%s L1 Reference Loadout includes %s at power %v", slug, mv, set.Moves[mv].Power)
			}
		}
	}
}

func TestReferenceLoadoutIncludesEvolutionRungAtThirty(t *testing.T) {
	set := testContentSet(t)
	for slug := range set.Species {
		got, err := dojo.ReferenceLoadout(set, slug, game.QueueLevel)
		if err != nil {
			t.Fatalf("%s L%d: %v", slug, game.QueueLevel, err)
		}
		if len(got) != 4 {
			t.Errorf("%s L30 Reference Loadout length %d, want 4", slug, len(got))
			continue
		}
		var maxPower float64
		for _, mv := range got {
			if p := set.Moves[mv].Power; p > maxPower {
				maxPower = p
			}
		}
		if maxPower < 90 {
			t.Errorf("%s L30 Reference Loadout %v max power %v, want the 90-power Evolution Move", slug, got, maxPower)
		}
	}
}

func TestReferenceLoadoutUnknownSpecies(t *testing.T) {
	set := testContentSet(t)
	if _, err := dojo.ReferenceLoadout(set, "not-a-species", 1); err == nil {
		t.Fatal("expected error for unknown species")
	}
}
