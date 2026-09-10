// Command checkbalance rejects regressions against the reviewed balance baseline.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"

	"termon.sh/internal/balance"
)

func main() {
	reportPath := flag.String("report", "", "Balance Run JSON report")
	baselinePath := flag.String("baseline", ".github/balance-baseline.json", "reviewed baseline report")
	flag.Parse()
	if err := run(*reportPath, *baselinePath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("balance: no regressions against reviewed baseline")
}

func run(reportPath, baselinePath string) error {
	report, err := readReport(reportPath)
	if err != nil {
		return err
	}
	baseline, err := readReport(baselinePath)
	if err != nil {
		return err
	}
	return compare(report, baseline)
}

func readReport(path string) (balance.RunOutput, error) {
	var report balance.RunOutput
	data, err := os.ReadFile(path) //nolint:gosec // Local report path is explicitly supplied by the CLI caller.
	if err != nil {
		return report, fmt.Errorf("read balance report: %w", err)
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return report, fmt.Errorf("decode balance report: %w", err)
	}
	return report, nil
}

type gateKey struct {
	name, teamA, teamB string
}

func compare(report, baseline balance.RunOutput) error {
	a, b := report.Snapshot, baseline.Snapshot
	if a.SeedBase != b.SeedBase || a.SeedCount != b.SeedCount ||
		a.RulesRevision != b.RulesRevision || a.Policy != b.Policy ||
		!slices.Equal(a.ReferenceTeams, b.ReferenceTeams) ||
		report.BattlesRun != baseline.BattlesRun || report.BattlesRun == 0 {
		return errors.New("balance corpus differs from baseline; review its seeds, rules, policy, teams and battle count")
	}
	previous := make(map[gateKey]balance.GateResult, len(baseline.Gates))
	for _, gate := range baseline.Gates {
		previous[gateKey{gate.Name, gate.TeamA, gate.TeamB}] = gate
	}
	var failures []error
	for _, gate := range report.Gates {
		key := gateKey{gate.Name, gate.TeamA, gate.TeamB}
		old, ok := previous[key]
		if !ok {
			failures = append(failures, fmt.Errorf("unexpected or duplicate gate: %v", key))
			continue
		}
		delete(previous, key)
		if err := compareGate(gate, old); err != nil {
			failures = append(failures, fmt.Errorf("%v: %w", key, err))
		}
	}
	if len(previous) != 0 || len(baseline.Gates) == 0 {
		failures = append(failures, errors.New("balance report is missing required gates"))
	}
	return errors.Join(failures...)
}

func compareGate(gate, old balance.GateResult) error {
	if gate.Threshold != old.Threshold {
		return errors.New("gate threshold changed; review baseline")
	}
	switch gate.Name {
	case balance.GateNonMirrorMatchup:
		if gate.Total != old.Total || gate.Total <= 0 || gate.Wins < 0 || gate.Wins > gate.Total {
			return errors.New("matchup coverage differs from baseline")
		}
		low, high := 0.25, 0.75
		if !old.Passed {
			low, high = min(low, old.Value), max(high, old.Value)
		}
		rate := float64(gate.Wins) / float64(gate.Total)
		if rate < low || rate > high {
			return fmt.Errorf("win rate %.6f outside baseline range [%.6f, %.6f]", rate, low, high)
		}
	case balance.GateReferenceTeamWinRate:
		if gate.Maximum <= 0 || old.Maximum <= 0 || gate.Value > gate.Maximum {
			return errors.New("missing or invalid team win-rate range")
		}
		low, high := 0.4, 0.6
		if !old.Passed {
			low, high = min(low, old.Value), max(high, old.Maximum)
		}
		if gate.Value < low || gate.Maximum > high {
			return fmt.Errorf("team win-rate range [%.6f, %.6f] outside baseline [%.6f, %.6f]",
				gate.Value, gate.Maximum, low, high)
		}
	default:
		if !gate.Passed {
			return fmt.Errorf("gate failed: %s", gate.Detail)
		}
	}
	return nil
}
