package balance

import (
	"testing"

	"termon.sh/internal/battle"
	"termon.sh/internal/dojo"
)

func TestMoveChoiceDominanceSeventyPercentBoundary(t *testing.T) {
	tracker := &MoveChoiceTracker{}
	for range 7 {
		tracker.Observe(moveEvidence("spec", "a", 10, 9))
	}
	for range 3 {
		tracker.Observe(moveEvidence("spec", "b", 10, 9))
	}
	if gate := moveGate(t, tracker.Gates(), "spec", "a"); !gate.Passed || gate.Value != 70 {
		t.Fatalf("70%% gate = %+v, want pass", gate)
	}
	tracker.Observe(moveEvidence("spec", "a", 10, 9))
	if gate := moveGate(t, tracker.Gates(), "spec", "a"); gate.Passed || gate.Value <= 70 {
		t.Fatalf("above-70%% gate = %+v, want failure", gate)
	}
}

func TestMoveChoiceSignedNegativeUtility(t *testing.T) {
	tracker := &MoveChoiceTracker{}
	tracker.Observe(moveEvidence("negative", "a", -10, -11))
	if gate := moveGate(t, tracker.Gates(), "negative", "a"); gate.Choices != 1 {
		t.Fatalf("negative near-best choices = %d, want 1", gate.Choices)
	}
	tracker.Observe(moveEvidence("negative", "b", -10, -12))
	if gate := moveGate(t, tracker.Gates(), "negative", "b"); gate.Choices != 1 {
		t.Fatalf("out-of-band negative selection entered denominator: %+v", gate)
	}
}

func TestMoveChoiceReliableFinisherIsReportedButExcluded(t *testing.T) {
	tracker := &MoveChoiceTracker{}
	evidence := moveEvidence("finisher", "a", 10, 9)
	evidence.ReliableKO = true
	tracker.Observe(evidence)
	rows := tracker.Rows()
	if len(rows) != 2 || rows[0].ReliableKO+rows[1].ReliableKO != 1 {
		t.Fatalf("reliable KO rows = %+v", rows)
	}
	if len(tracker.Gates()) != 0 {
		t.Fatalf("reliable finisher created a conditional dominance gate: %+v", tracker.Gates())
	}
}

func TestMoveChoiceSwitchDoesNotEnterMoveDenominator(t *testing.T) {
	tracker := &MoveChoiceTracker{}
	evidence := moveEvidence("switcher", "a", 10, 9)
	evidence.Selected = battle.Action{Kind: battle.ActionSwitch, SwitchTo: "reserve"}
	tracker.Observe(evidence)
	if tracker.VoluntarySwitches != 1 || len(tracker.Gates()) != 0 {
		t.Fatalf("switch accounting = switches %d gates %+v", tracker.VoluntarySwitches, tracker.Gates())
	}
}

func TestMoveChoicePreservationTupleTie(t *testing.T) {
	ko, reserves, lossA, lossB := .2, 2, 10.0, 11.0
	tracker := &MoveChoiceTracker{}
	tracker.Observe(DecisionEvidence{
		Species: "preserver", Selected: battle.Action{Kind: battle.ActionMove, Move: "a"},
		Considered: []dojo.ScoredActionSummary{
			{Kind: battle.ActionMove, Move: "a", KOProbability: &ko, HealthyReserves: &reserves, ExpectedHPLoss: &lossA},
			{Kind: battle.ActionMove, Move: "b", KOProbability: &ko, HealthyReserves: &reserves, ExpectedHPLoss: &lossB},
		},
	})
	if gate := moveGate(t, tracker.Gates(), "preserver", "a"); gate.Choices != 1 {
		t.Fatalf("Preservation tuple tie not conditional: %+v", gate)
	}
}

func moveEvidence(species, selected string, first, second float64) DecisionEvidence {
	return DecisionEvidence{
		Species: species, Selected: battle.Action{Kind: battle.ActionMove, Move: selected},
		Considered: []dojo.ScoredActionSummary{
			{Kind: battle.ActionMove, Move: "a", Score: first},
			{Kind: battle.ActionMove, Move: "b", Score: second},
		},
	}
}

func moveGate(t *testing.T, gates []MoveChoiceGate, species, move string) MoveChoiceGate {
	t.Helper()
	for _, gate := range gates {
		if gate.Species == species && gate.Move == move {
			return gate
		}
	}
	t.Fatalf("missing gate for %s/%s: %+v", species, move, gates)
	return MoveChoiceGate{}
}
