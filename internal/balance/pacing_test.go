package balance

import (
	"math"
	"path/filepath"
	"testing"

	"termon.sh/internal/content"
)

func TestNaturalPacingActualContent(t *testing.T) {
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	if roots := pacingRoots(set); len(roots) != 24 {
		t.Fatalf("roots = %d, want 24", len(roots))
	}
	rows, gate := NaturalPacing(set)
	if !gate.Passed {
		t.Fatalf("natural pacing failed: %+v", rows)
	}
	if len(rows) != 50 {
		t.Fatalf("rows = %d, want 50", len(rows))
	}
	if rows[0].Level != 1 || rows[len(rows)-1].Level != 50 {
		t.Fatalf("level range = %d-%d, want 1-50", rows[0].Level, rows[len(rows)-1].Level)
	}
	for _, row := range rows {
		if row.NeutralPairs == 0 {
			t.Fatalf("level %d has no neutral pairs", row.Level)
		}
	}
	if gate.Name != "natural_ko_pace" {
		t.Fatalf("gate name = %q", gate.Name)
	}
	if gate.Threshold != "mean 2.5-3.5 neutral landed hits at every level, no non-crit neutral OHKO" {
		t.Fatalf("unexpected threshold %q", gate.Threshold)
	}
}

func TestExpectedLandedHits(t *testing.T) {
	distribution := map[int]float64{2: 0.5, 4: 0.5}
	if got := expectedLandedHits(5, distribution); math.Abs(got-2.25) > 1e-9 {
		t.Fatalf("expected landed hits = %v, want 2.25", got)
	}
}

func TestNonCriticalOHKOExcludesUpperVarianceEndpoint(t *testing.T) {
	if nonCriticalOHKO(10, 10) {
		t.Fatal("base damage equal to HP must not OHKO with upper-exclusive variance")
	}
	if !nonCriticalOHKO(10.01, 10) {
		t.Fatal("base damage above HP must have a non-critical OHKO chance")
	}
}
