package balance

import (
	"math"
	"testing"

	"termon.sh/internal/battle"
)

func TestStreamingGatesMatchBatchGates(t *testing.T) {
	accumulator := newOutcomeAccumulator()
	var outcomes []*BattleOutcome
	for i, a := range ReferenceTeams {
		for _, b := range ReferenceTeams[i:] {
			for sample := range 12 {
				out := &BattleOutcome{
					TeamA: a, TeamB: b, EngineSideA: true, Turns: 6 + sample,
					SideA: battle.Party{Trainer: "left"}, SideB: battle.Party{Trainer: "right"}, Winner: "left",
					FaintPaces: []FaintPace{{Hits: sample % 7, Critical: sample%2 == 0}, {Hits: 1, SuperEffective: true}},
				}
				if sample%3 == 0 {
					out.Winner = "right"
				}
				outcomes = append(outcomes, out)
				accumulator.Observe(out)
			}
		}
	}
	streamed := map[string]GateResult{}
	for _, gate := range accumulator.Gates() {
		streamed[gate.Name+pairKey(gate.TeamA, gate.TeamB)] = gate
	}
	for _, want := range EvaluateGates(outcomes, nil) {
		got, ok := streamed[want.Name+pairKey(want.TeamA, want.TeamB)]
		if want.Name == GateNonMirrorMatchup && got.TeamA != want.TeamA {
			want.Value = 1 - want.Value
		}
		if !ok || got.Passed != want.Passed || math.Abs(got.Value-want.Value) > 1e-9 || math.Abs(got.Maximum-want.Maximum) > 1e-9 {
			t.Errorf("streamed gate %+v differs from batch %+v", got, want)
		}
	}
}

func TestStreamingGatesRejectMissingDenominators(t *testing.T) {
	for _, gate := range newOutcomeAccumulator().Gates() {
		if gate.Name != GateIllegalActions && gate.Passed {
			t.Errorf("empty corpus passed %s", gate.Name)
		}
	}
}
