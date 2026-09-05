package balance

// FailedGateReport captures replay context for a gate failure.
type FailedGateReport struct {
	Gate       string              `json:"gate"`
	Scenario   string              `json:"scenario"`
	Seed       uint64              `json:"seed"`
	TeamA      ReferenceTeam       `json:"team_a"`
	TeamB      ReferenceTeam       `json:"team_b"`
	Loadouts   map[string][]string `json:"loadouts"`
	ActionLog  []ActionEntry       `json:"action_log"`
	EventKinds []string            `json:"event_kinds,omitempty"`
}

// BuildFailedReports attaches replay artifacts for failing gates.
func BuildFailedReports(results []*BattleOutcome, gates []GateResult) []FailedGateReport {
	failing := map[string]bool{}
	for _, g := range gates {
		if !g.Passed {
			failing[g.Name] = true
		}
	}
	if len(failing) == 0 {
		return nil
	}
	var out []FailedGateReport
	for _, g := range gates {
		if g.Passed {
			continue
		}
		if rep := firstReportForGate(g.Name, results); rep != nil {
			out = append(out, *rep)
		}
	}
	return out
}

func firstReportForGate(gate string, results []*BattleOutcome) *FailedGateReport {
	switch gate {
	case GateReferenceTeamWinRate:
		return reportForTeamBand(results)
	case GateNonMirrorMatchup:
		return reportForMatchup(results, ReferenceTeams[0], ReferenceTeams[1])
	case GateMirrorWinRate:
		return reportForMirror(results)
	case GateEngineSideAdvantage:
		return reportForEngineSide(results)
	case GateNeutralKOPace:
		return reportForKOPace(results)
	case GateBattlePace:
		return reportForBattlePace(results)
	case GateIllegalActions:
		return reportForIllegal(results)
	default:
		if len(results) > 0 {
			return outcomeReport(gate, results[0])
		}
	}
	return nil
}

func reportForTeamBand(results []*BattleOutcome) *FailedGateReport {
	minRate, _ := teamWinRateBand(results)
	for _, team := range ReferenceTeams {
		wins, total := teamRecord(results, team)
		if total == 0 {
			continue
		}
		rate := float64(wins) / float64(total)
		if rate <= minRate+1e-9 && (rate < 0.40 || rate > 0.60) {
			for _, r := range results {
				if r.TeamA.Name == team.Name || r.TeamB.Name == team.Name {
					return outcomeReport(GateReferenceTeamWinRate, r)
				}
			}
		}
	}
	return nil
}

func reportForMatchup(results []*BattleOutcome, a, b ReferenceTeam) *FailedGateReport {
	for _, r := range results {
		if matchupPair(r, a, b) {
			return outcomeReport(GateNonMirrorMatchup, r)
		}
	}
	return nil
}

func reportForMirror(results []*BattleOutcome) *FailedGateReport {
	for _, r := range results {
		if r.TeamA.Name == r.TeamB.Name {
			return outcomeReport(GateMirrorWinRate, r)
		}
	}
	return nil
}

func reportForEngineSide(results []*BattleOutcome) *FailedGateReport {
	for _, r := range results {
		if r.TeamA.Name != r.TeamB.Name {
			return outcomeReport(GateEngineSideAdvantage, r)
		}
	}
	return nil
}

func reportForKOPace(results []*BattleOutcome) *FailedGateReport {
	if len(results) == 0 {
		return nil
	}
	var ohko *BattleOutcome
	worst := results[0]
	worstHits := 1 << 30
	for _, r := range results {
		for _, p := range r.FaintPaces {
			if p.SuperEffective {
				continue
			}
			if p.Hits <= 1 && !p.Critical {
				ohko = r
			}
			if p.Hits < worstHits {
				worstHits = p.Hits
				worst = r
			}
		}
	}
	if ohko != nil {
		return outcomeReport(GateNeutralKOPace, ohko)
	}
	return outcomeReport(GateNeutralKOPace, worst)
}

func reportForBattlePace(results []*BattleOutcome) *FailedGateReport {
	worst := results[0]
	for _, r := range results {
		if r.Turns > worst.Turns {
			worst = r
		}
	}
	return outcomeReport(GateBattlePace, worst)
}

func reportForIllegal(results []*BattleOutcome) *FailedGateReport {
	for _, r := range results {
		if r.IllegalActions > 0 {
			return outcomeReport(GateIllegalActions, r)
		}
	}
	return nil
}

func outcomeReport(gate string, r *BattleOutcome) *FailedGateReport {
	kinds := make([]string, len(r.EventKinds))
	for i, k := range r.EventKinds {
		kinds[i] = string(k)
	}
	return &FailedGateReport{
		Gate: gate, Scenario: r.Scenario, Seed: r.Seed,
		TeamA: r.TeamA, TeamB: r.TeamB,
		Loadouts: r.Loadouts, ActionLog: r.ActionLog,
		EventKinds: kinds,
	}
}
