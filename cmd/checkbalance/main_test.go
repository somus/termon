package main

import (
	"path/filepath"
	"testing"

	"termon.sh/internal/balance"
)

func TestBaselineRegressions(t *testing.T) {
	tests := []struct {
		name      string
		edit      func(*balance.RunOutput)
		wantError bool
	}{
		{name: "known failures accepted"},
		{name: "improved matchup", edit: func(r *balance.RunOutput) { r.Gates[1].Wins++ }},
		{name: "worse matchup", wantError: true, edit: func(r *balance.RunOutput) { r.Gates[1].Wins-- }},
		{name: "matchup reversed", wantError: true, edit: func(r *balance.RunOutput) { r.Gates[1].Wins = r.Gates[1].Total }},
		{name: "new matchup failure", wantError: true, edit: func(r *balance.RunOutput) {
			for i := range r.Gates {
				if r.Gates[i].Name == balance.GateNonMirrorMatchup && r.Gates[i].Passed {
					r.Gates[i].Wins = 0
					return
				}
			}
		}},
		{name: "worse minimum", wantError: true, edit: func(r *balance.RunOutput) { r.Gates[0].Value -= 0.00001 }},
		{name: "worse maximum", wantError: true, edit: func(r *balance.RunOutput) { r.Gates[0].Maximum += 0.00001 }},
		{name: "improved team range", edit: func(r *balance.RunOutput) {
			r.Gates[0].Value += 0.01
			r.Gates[0].Maximum -= 0.01
		}},
		{name: "missing maximum", wantError: true, edit: func(r *balance.RunOutput) { r.Gates[0].Maximum = 0 }},
		{name: "missing capture", wantError: true, edit: func(r *balance.RunOutput) { r.Gates = r.Gates[:len(r.Gates)-1] }},
		{name: "duplicate gate", wantError: true, edit: func(r *balance.RunOutput) { r.Gates = append(r.Gates, r.Gates[0]) }},
		{name: "partial corpus", wantError: true, edit: func(r *balance.RunOutput) { r.BattlesRun-- }},
		{name: "partial matchup", wantError: true, edit: func(r *balance.RunOutput) { r.Gates[1].Total-- }},
		{name: "different seeds", wantError: true, edit: func(r *balance.RunOutput) { r.Snapshot.SeedBase++ }},
		{name: "different rules", wantError: true, edit: func(r *balance.RunOutput) { r.Snapshot.RulesRevision = "changed" }},
		{name: "different policy", wantError: true, edit: func(r *balance.RunOutput) { r.Snapshot.Policy.NearBestBand = 0 }},
		{name: "different teams", wantError: true, edit: func(r *balance.RunOutput) { r.Snapshot.ReferenceTeams[0].Families[0] = "changed" }},
		{name: "content tuning allowed", edit: func(r *balance.RunOutput) { r.Snapshot.ContentRevision = "changed" }},
		{name: "changed threshold", wantError: true, edit: func(r *balance.RunOutput) { r.Gates[1].Threshold = "0-100%" }},
	}
	for _, name := range []string{
		balance.GateMirrorWinRate, balance.GateEngineSideAdvantage,
		balance.GateNeutralKOPace, balance.GateBattlePace, balance.GateIllegalActions, balance.GateCaptureSmoke,
	} {
		tests = append(tests, struct {
			name      string
			edit      func(*balance.RunOutput)
			wantError bool
		}{name: name, wantError: true, edit: func(r *balance.RunOutput) {
			for i := range r.Gates {
				if r.Gates[i].Name == name {
					r.Gates[i].Passed = false
				}
			}
		}})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join("..", "..", ".github", "balance-baseline.json")
			baseline, err := readReport(path)
			if err != nil {
				t.Fatal(err)
			}
			report, err := readReport(path)
			if err != nil {
				t.Fatal(err)
			}
			if tt.edit != nil {
				tt.edit(&report)
			}
			if err := compare(report, baseline); (err != nil) != tt.wantError {
				t.Fatalf("compare error = %v, want error %v", err, tt.wantError)
			}
		})
	}
}
