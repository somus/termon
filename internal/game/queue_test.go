package game_test

import (
	"slices"
	"testing"

	"termon.sh/internal/content"
	"termon.sh/internal/game"
)

func loadContent(t *testing.T) *content.Set {
	t.Helper()
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestQueueStatsSum320(t *testing.T) {
	set := loadContent(t)
	for slug := range set.Species {
		sum := 0
		stats := game.QueueStats(set.Species[slug])
		for _, v := range stats {
			if v < 1 {
				t.Fatalf("%s stat below 1: %v", slug, stats)
			}
			sum += v
		}
		if sum != game.QueueStatBudget {
			t.Fatalf("%s stats sum %d, want %d", slug, sum, game.QueueStatBudget)
		}
	}
}

func TestRequireFullParty(t *testing.T) {
	set := loadContent(t)
	save := &game.Save{
		Collection: []game.Monster{{
			ID: "a", Species: "rootkit", Level: 1,
			MoveLibrary:   []string{"m1", "m2", "m3", "m4"},
			BattleLoadout: []string{"m1", "m2", "m3", "m4"},
		}},
		Party: [3]string{"a", "", ""},
	}
	if err := game.RequireFullParty(save); err == nil {
		t.Fatal("partial party should fail")
	}
	_ = set
}

func TestDefaultQueueMoveSet(t *testing.T) {
	set := loadContent(t)
	m := game.Monster{
		Species: "rootkit", Level: 5,
		MoveLibrary:   []string{"rootkit-vine-lash", "rootkit-leaf-slap", "rootkit-needle-jab", "rootkit-moss-wrap"},
		BattleLoadout: []string{"rootkit-vine-lash", "rootkit-leaf-slap", "rootkit-needle-jab", "rootkit-moss-wrap"},
	}
	moves, err := game.DefaultQueueMoveSet(set, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(moves) < 1 || len(moves) > 4 {
		t.Fatalf("moves = %v", moves)
	}
	if err := game.ValidateQueueMoveSet(set, m, moves); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultQueueMoveSetStaysOnLevelOneRung(t *testing.T) {
	set := loadContent(t)
	for slug, sp := range set.Species {
		mon := game.Monster{Species: slug, Level: game.QueueLevel}
		got, err := game.DefaultQueueMoveSet(set, mon)
		if err != nil {
			t.Fatalf("%s: %v", slug, err)
		}
		if len(got) != 4 {
			t.Fatalf("%s DefaultQueueMoveSet = %v, want 4 Moves", slug, got)
		}
		var want []string
		for _, e := range sp.Movepool {
			if e.Level == 1 {
				want = append(want, e.Move)
			}
		}
		slices.Sort(want)
		if len(want) > 4 {
			want = want[:4]
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s DefaultQueueMoveSet = %v, want Level-1 slugs %v", slug, got, want)
		}
		for _, mvSlug := range got {
			mv := set.Moves[mvSlug]
			if mv.Power > 75 {
				t.Errorf("%s Queue default includes %s at power %v; Evolution-rung Moves must stay opt-in", slug, mvSlug, mv.Power)
			}
		}
	}
}

func TestQueueMovePoolIncludesEvolutionRung(t *testing.T) {
	set := loadContent(t)
	for slug := range set.Species {
		mon := game.Monster{Species: slug, Level: game.QueueLevel}
		pool, err := game.QueueMovePool(set, mon)
		if err != nil {
			t.Fatalf("%s: %v", slug, err)
		}
		if len(pool) < 5 {
			t.Errorf("%s Queue pool length %d, want at least 5 so the 90-power Move is legal", slug, len(pool))
		}
		var maxPower float64
		for _, mvSlug := range pool {
			if p := set.Moves[mvSlug].Power; p > maxPower {
				maxPower = p
			}
		}
		if maxPower < 90 {
			t.Errorf("%s Queue pool max power %v, want >= 90", slug, maxPower)
		}
	}
}
