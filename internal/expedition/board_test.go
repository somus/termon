package expedition_test

import (
	"testing"
	"time"

	"termon.sh/internal/expedition"
)

func TestBoardRotationCoversAllFamilies(t *testing.T) {
	seen := map[string]int{}
	for day := range expedition.CycleDays {
		fams := expedition.FamiliesForDayIndex(day)
		if len(fams) != 3 {
			t.Fatalf("day %d: got %d families, want 3", day, len(fams))
		}
		for _, f := range fams {
			seen[f]++
		}
	}
	if len(seen) != len(expedition.FamilyOrder) {
		t.Fatalf("saw %d distinct families, want %d", len(seen), len(expedition.FamilyOrder))
	}
	for _, slug := range expedition.FamilyOrder {
		if seen[slug] != 1 {
			t.Fatalf("family %q appeared %d times", slug, seen[slug])
		}
	}
}

func TestDayIndexFromUTC(t *testing.T) {
	epoch := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	if expedition.DayIndex(epoch) != 0 {
		t.Fatalf("epoch day index = %d, want 0", expedition.DayIndex(epoch))
	}
	next := epoch.Add(24 * time.Hour)
	if expedition.DayIndex(next) != 1 {
		t.Fatalf("next day index = %d, want 1", expedition.DayIndex(next))
	}
	if expedition.DayIndex(epoch.Add(8*24*time.Hour)) != 0 {
		t.Fatal("expected 8-day cycle wrap")
	}
}

func TestFamiliesForServerDay(t *testing.T) {
	// Pinned expectations computed from FamilyOrder at authoring time, so a
	// rotation or indexing change breaks this test loudly instead of
	// comparing an expression against itself.
	cases := []struct {
		day  string
		idx  int
		want [3]string
	}{
		{"2026-08-28", 5, [3]string{"joulpup", "amperent", "surgetail"}},
		{"2026-08-29", 6, [3]string{"spamlet", "bloatware", "wormate"}},
		{"2026-09-03", 3, [3]string{"aquabit", "flowcell", "gushkit"}},
	}
	for _, tc := range cases {
		day, err := time.Parse("2006-01-02", tc.day)
		if err != nil {
			t.Fatal(err)
		}
		if got := expedition.DayIndex(day); got != tc.idx {
			t.Fatalf("%s: DayIndex = %d, want %d", tc.day, got, tc.idx)
		}
		fams := expedition.FamiliesForDay(day)
		if fams[0] != tc.want[0] || fams[1] != tc.want[1] || fams[2] != tc.want[2] {
			t.Fatalf("%s: board = %v, want %v", tc.day, fams, tc.want)
		}
	}
}

func TestDrawPrepsDistinctAndDeterministic(t *testing.T) {
	pool := expedition.SupportPool("rootkit")
	p1a, p2a, err := expedition.DrawPreps(pool, "rootkit", 42)
	if err != nil {
		t.Fatal(err)
	}
	p1b, p2b, err := expedition.DrawPreps(pool, "rootkit", 42)
	if err != nil {
		t.Fatal(err)
	}
	if p1a != p1b || p2a != p2b {
		t.Fatal("draw should be deterministic for same seed")
	}
	if p1a == p2a {
		t.Fatalf("preps must differ: %q %q", p1a, p2a)
	}
	if p1a == "rootkit" || p2a == "rootkit" {
		t.Fatal("target must not appear in preps")
	}
}

func TestSupportPoolsHaveTwoEntries(t *testing.T) {
	for _, slug := range expedition.FamilyOrder {
		pool := expedition.SupportPool(slug)
		if len(pool) < 2 {
			t.Fatalf("%s pool = %v, want ≥2", slug, pool)
		}
		seen := map[string]struct{}{}
		for _, p := range pool {
			if p == slug {
				t.Fatalf("%s pool contains target", slug)
			}
			seen[p] = struct{}{}
		}
		if len(seen) < 2 {
			t.Fatalf("%s pool has duplicates: %v", slug, pool)
		}
		if expedition.SupportTheme(slug) == "" {
			t.Fatalf("%s missing theme", slug)
		}
	}
}
