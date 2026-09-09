package dojo_test

import (
	"fmt"
	"slices"
	"testing"

	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
	"termon.sh/internal/game"
)

func TestLoadoutSelectionSurvivesMoveRenames(t *testing.T) {
	tests := []struct {
		name        string
		selectMoves func(*content.Set) ([]string, error)
	}{
		{
			name: "reference",
			selectMoves: func(set *content.Set) ([]string, error) {
				return dojo.ReferenceLoadout(set, "aquabit", 20)
			},
		},
		{
			name: "normalized",
			selectMoves: func(set *content.Set) ([]string, error) {
				return game.DefaultQueueMoveSet(set, game.Monster{Species: "aquabit"})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := testContentSet(t)
			before, err := tt.selectMoves(set)
			if err != nil {
				t.Fatal(err)
			}
			sp := set.Species["aquabit"]
			maxOrder := 0
			for _, move := range set.Moves {
				maxOrder = max(maxOrder, move.Order)
			}
			original := map[string]string{}
			for i, entry := range sp.Movepool {
				move := set.Moves[entry.Move]
				delete(set.Moves, entry.Move)
				move.Slug = fmt.Sprintf("renamed_%03d", maxOrder-move.Order)
				move.Name = "Renamed " + move.Slug
				set.Moves[move.Slug] = move
				sp.Movepool[i].Move = move.Slug
				original[move.Slug] = entry.Move
			}
			set.Species[sp.Slug] = sp
			after, err := tt.selectMoves(set)
			if err != nil {
				t.Fatal(err)
			}
			for i, slug := range after {
				after[i] = original[slug]
			}
			if !slices.Equal(before, after) {
				t.Fatalf("renaming changed loadout order: before %v, after %v", before, after)
			}
		})
	}
}

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
