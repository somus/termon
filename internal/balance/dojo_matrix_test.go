package balance

import (
	"math"
	"path/filepath"
	"reflect"
	"testing"

	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
)

func TestRunDojoMatrixIsDeterministicForReducedCorpus(t *testing.T) {
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := RunDojoMatrix(set, []uint64{17})
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunDojoMatrix(set, []uint64{17})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("reduced Dojo matrix is not deterministic")
	}
	wantCells := len(ReferenceTeams) * len(NaturalCheckpoints) * len(dojoMatrixTiers())
	if len(first.Cells) != wantCells {
		t.Fatalf("cells = %d, want %d", len(first.Cells), wantCells)
	}
	for _, cell := range first.Cells {
		if cell.Count != 2 {
			t.Fatalf("%s level %d %s count = %d, want paired physical sides", cell.Team, cell.Checkpoint, cell.Tier, cell.Count)
		}
		if cell.Sample == nil || len(cell.Sample.Events) == 0 {
			t.Fatalf("%s level %d %s missing bounded sample trace", cell.Team, cell.Checkpoint, cell.Tier)
		}
	}
}

func TestEvaluateDojoMatrixUsesCompletedDenominatorAndAdjacentOrdering(t *testing.T) {
	team, checkpoint := ReferenceTeams[0].Name, NaturalCheckpoints[0]
	cells := []DojoMatrixCell{
		{Team: team, Checkpoint: checkpoint, Tier: dojo.TierApprentice, Wins: 2, Count: 5, Rate: .4},
		{Team: team, Checkpoint: checkpoint, Tier: dojo.TierRival, Wins: 5, Count: 10, Rate: .5},
		{Team: team, Checkpoint: checkpoint, Tier: dojo.TierMaster, Wins: 3, Count: 5, Rate: .6},
	}
	gates := evaluateDojoMatrix(cells)
	var apprenticeBand, ordering *DojoMatrixGate
	for i := range gates {
		gate := &gates[i]
		if gate.Team != team || gate.Checkpoint != checkpoint {
			continue
		}
		if gate.Name == "dojo_tier_band" && gate.Tier == dojo.TierApprentice {
			apprenticeBand = gate
		}
		if gate.Name == "dojo_tier_ordering" {
			ordering = gate
		}
	}
	if apprenticeBand == nil || !apprenticeBand.Passed || apprenticeBand.Value != 40 {
		t.Fatalf("Apprentice band gate = %+v, want completed 2/5 Dojo wins = 40%%", apprenticeBand)
	}
	if ordering == nil || !ordering.Passed || math.Abs(ordering.Value-10) > 1e-9 {
		t.Fatalf("ordering gate = %+v, want 10pp minimum adjacent gap", ordering)
	}

	cells[1].Failed = 1
	cells[1].Failures = []DojoMatrixTrace{{Scenario: "failed", Seed: 3, Error: "turn cap"}}
	for i := range cells {
		if cells[i].Count > 0 {
			cells[i].Rate = float64(cells[i].Wins) / float64(cells[i].Count)
		}
	}
	for _, gate := range evaluateDojoMatrix(cells) {
		if gate.Team == team && gate.Checkpoint == checkpoint && gate.Name == "dojo_tier_band" && gate.Tier == dojo.TierRival && gate.Passed {
			t.Fatal("Rival band passed despite a failed scenario outside its denominator")
		}
		if gate.Team == team && gate.Checkpoint == checkpoint && gate.Name == "dojo_tier_ordering" && gate.Passed {
			t.Fatal("ordering passed despite an incomplete tier corpus")
		}
	}
}
