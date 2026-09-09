package dojo

import (
	"testing"

	"termon.sh/internal/battle"
)

type fixedRoll float64

func (r fixedRoll) Float64() float64 { return float64(r) }

func TestNearBestSamplingBoundaries(t *testing.T) {
	cases := []struct {
		name   string
		scores []float64
		band   float64
		roll   float64
		want   string
	}{
		{name: "first eligible", scores: []float64{0.9, 1, 0.7}, band: 0.15, roll: 0, want: "a"},
		{name: "second eligible", scores: []float64{0.9, 1, 0.7}, band: 0.15, roll: 0.5, want: "b"},
		{name: "upper boundary", scores: []float64{0.9, 1, 0.7}, band: 0.15, roll: 0.999, want: "b"},
		{name: "negative best", scores: []float64{-1.1, -1, -2}, band: 0.15, roll: 0, want: "a"},
		{name: "zero best", scores: []float64{-0.1, 0, 0}, band: 0.15, roll: 0.9, want: "c"},
		{name: "daily best only", scores: []float64{0.9, 1, 0.7}, band: 0, roll: 0, want: "b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidates := make([]policyCandidate, len(tc.scores))
			for i, score := range tc.scores {
				candidates[i] = policyCandidate{action: battle.Action{Move: string(rune('a' + i))}, score: score}
			}
			got := sampleNearBest(candidates, tc.band, fixedRoll(tc.roll))
			if got.action.Move != tc.want {
				t.Fatalf("picked %q, want %q", got.action.Move, tc.want)
			}
		})
	}
}
