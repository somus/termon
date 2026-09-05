package balance

import (
	"fmt"
	"math"
	"slices"
)

// Gate name constants.
const (
	GateReferenceTeamWinRate = "reference_team_win_rate"
	GateNonMirrorMatchup     = "non_mirror_matchup"
	GateMirrorWinRate        = "mirror_win_rate"
	GateEngineSideAdvantage  = "engine_side_advantage"
	GateNeutralKOPace        = "neutral_ko_pace"
	GateBattlePace           = "battle_pace"
	GateIllegalActions       = "illegal_actions"
	GateHiddenInfoReads      = "hidden_info_reads"
	GateCaptureSmoke         = "capture_smoke"
)

// GateResult is one acceptance gate evaluation.
type GateResult struct {
	Name      string  `json:"name"`
	Passed    bool    `json:"passed"`
	Value     float64 `json:"value"`
	Threshold string  `json:"threshold"`
	Detail    string  `json:"detail,omitempty"`
}

// EvaluateGates checks methodology acceptance thresholds.
func EvaluateGates(results []*BattleOutcome, capture *CaptureSmoke) []GateResult {
	var gates []GateResult
	gates = append(gates, evalReferenceTeamWinRate(results))
	gates = append(gates, evalNonMirrorMatchup(results))
	gates = append(gates, evalMirrorWinRate(results))
	gates = append(gates, evalEngineSideAdvantage(results))
	gates = append(gates, evalNeutralKOPace(results))
	gates = append(gates, evalBattlePace(results))
	gates = append(gates, evalIllegalActions(results))
	gates = append(gates, evalHiddenInfoReads(results))
	if capture != nil {
		gates = append(gates, evalCaptureSmoke(capture))
	}
	return gates
}

func evalReferenceTeamWinRate(results []*BattleOutcome) GateResult {
	worst := 1.0
	worstTeam := ""
	for _, team := range ReferenceTeams {
		wins, total := teamRecord(results, team)
		if total == 0 {
			continue
		}
		rate := float64(wins) / float64(total)
		if rate < worst {
			worst = rate
		}
		if rate < 0.40 || rate > 0.60 {
			if worstTeam == "" {
				worstTeam = team.Name
			}
		}
		_ = worstTeam
	}
	// Report min/max team win rate band compliance.
	minRate, maxRate := teamWinRateBand(results)
	passed := minRate >= 0.40 && maxRate <= 0.60
	return GateResult{
		Name: GateReferenceTeamWinRate, Passed: passed,
		Value: minRate, Threshold: "40-60%",
		Detail: fmt.Sprintf("team win-rate band %.1f%%-%.1f%%", minRate*100, maxRate*100),
	}
}

func teamWinRateBand(results []*BattleOutcome) (minRate, maxRate float64) {
	minRate = 1
	maxRate = 0
	for _, team := range ReferenceTeams {
		wins, total := teamRecord(results, team)
		if total == 0 {
			continue
		}
		rate := float64(wins) / float64(total)
		if rate < minRate {
			minRate = rate
		}
		if rate > maxRate {
			maxRate = rate
		}
	}
	return minRate, maxRate
}

func teamRecord(results []*BattleOutcome, team ReferenceTeam) (wins, total int) {
	for _, r := range results {
		if r.TeamA.Name != team.Name && r.TeamB.Name != team.Name {
			continue
		}
		total++
		if WinningTeam(r).Name == team.Name {
			wins++
		}
	}
	return wins, total
}

func evalNonMirrorMatchup(results []*BattleOutcome) GateResult {
	if len(ReferenceTeams) < 2 {
		return GateResult{Name: GateNonMirrorMatchup, Passed: true, Threshold: "25-75%"}
	}
	a, b := ReferenceTeams[0], ReferenceTeams[1]
	wins, total := matchupRecord(results, a, b)
	rate := 0.0
	if total > 0 {
		rate = float64(wins) / float64(total)
	}
	passed := total > 0 && rate >= 0.25 && rate <= 0.75
	return GateResult{
		Name: GateNonMirrorMatchup, Passed: passed,
		Value: rate, Threshold: "25-75%",
		Detail: fmt.Sprintf("%s vs %s", a.Name, b.Name),
	}
}

func matchupRecord(results []*BattleOutcome, teamA, teamB ReferenceTeam) (wins, total int) {
	for _, r := range results {
		if !matchupPair(r, teamA, teamB) {
			continue
		}
		total++
		if WinningTeam(r).Name == teamA.Name {
			wins++
		}
	}
	return wins, total
}

func matchupPair(r *BattleOutcome, teamA, teamB ReferenceTeam) bool {
	a := r.TeamA.Name == teamA.Name && r.TeamB.Name == teamB.Name
	b := r.TeamA.Name == teamB.Name && r.TeamB.Name == teamA.Name
	return a || b
}

func evalMirrorWinRate(results []*BattleOutcome) GateResult {
	wins, total := 0, 0
	for _, r := range results {
		if r.TeamA.Name != r.TeamB.Name {
			continue
		}
		total++
		if EngineSideWin(r) {
			wins++
		}
	}
	rate := 0.0
	if total > 0 {
		rate = float64(wins) / float64(total)
	}
	passed := total > 0 && rate >= 0.47 && rate <= 0.53
	return GateResult{
		Name: GateMirrorWinRate, Passed: passed,
		Value: rate, Threshold: "47-53%",
	}
}

func evalEngineSideAdvantage(results []*BattleOutcome) GateResult {
	wins, total := 0, 0
	for _, r := range results {
		if r.TeamA.Name == r.TeamB.Name {
			continue
		}
		total++
		if EngineSideWin(r) {
			wins++
		}
	}
	adv := 0.0
	if total > 0 {
		adv = math.Abs(float64(wins)/float64(total) - 0.5)
	}
	passed := adv <= 0.03
	return GateResult{
		Name: GateEngineSideAdvantage, Passed: passed,
		Value: adv * 100, Threshold: "<=3pp",
		Detail: fmt.Sprintf("engine-side win %.1f%%", float64(wins)/float64(max(total, 1))*100),
	}
}

func evalNeutralKOPace(results []*BattleOutcome) GateResult {
	var hits []int
	ohko := false
	for _, r := range results {
		for _, p := range r.FaintPaces {
			if p.SuperEffective {
				continue
			}
			hits = append(hits, p.Hits)
			if p.Hits <= 1 && !p.Critical {
				ohko = true
			}
		}
	}
	med := medianInt(hits)
	passed := len(hits) > 0 && med >= 3 && med <= 5 && !ohko
	detail := fmt.Sprintf("%d neutral faints", len(hits))
	if ohko {
		detail += "; non-crit OHKO"
	}
	return GateResult{
		Name: GateNeutralKOPace, Passed: passed,
		Value: float64(med), Threshold: "median 3-5 hits/faint, no non-crit OHKO",
		Detail: detail,
	}
}

func evalBattlePace(results []*BattleOutcome) GateResult {
	var turns []int
	for _, r := range results {
		turns = append(turns, r.Turns)
	}
	med := medianInt(turns)
	p90 := percentileInt(turns, 90)
	passed := med >= 6 && med <= 15 && p90 <= 24
	return GateResult{
		Name: GateBattlePace, Passed: passed,
		Value: float64(med), Threshold: "median 6-15 turns, p90 <= 24",
		Detail: fmt.Sprintf("p90=%d", p90),
	}
}

func evalIllegalActions(results []*BattleOutcome) GateResult {
	total := 0
	for _, r := range results {
		total += r.IllegalActions
	}
	return GateResult{
		Name: GateIllegalActions, Passed: total == 0,
		Value: float64(total), Threshold: "0",
	}
}

func evalHiddenInfoReads(results []*BattleOutcome) GateResult {
	total := 0
	for _, r := range results {
		total += r.HiddenInfoReads
	}
	return GateResult{
		Name: GateHiddenInfoReads, Passed: total == 0,
		Value: float64(total), Threshold: "0",
	}
}

func evalCaptureSmoke(smoke *CaptureSmoke) GateResult {
	return GateResult{
		Name: GateCaptureSmoke, Passed: smoke.Passed,
		Value: float64(smoke.Eligible), Threshold: "generator eligibility",
		Detail: fmt.Sprintf("%d/%d families eligible", smoke.Eligible, smoke.Total),
	}
}

func medianInt(vals []int) int {
	if len(vals) == 0 {
		return 0
	}
	cp := append([]int(nil), vals...)
	slices.Sort(cp)
	mid := len(cp) / 2
	if len(cp)%2 == 0 {
		return (cp[mid-1] + cp[mid]) / 2
	}
	return cp[mid]
}

func percentileInt(vals []int, pct int) int {
	if len(vals) == 0 {
		return 0
	}
	cp := append([]int(nil), vals...)
	slices.Sort(cp)
	idx := int(math.Ceil(float64(pct)/100*float64(len(cp)))) - 1
	idx = max(0, min(idx, len(cp)-1))
	return cp[idx]
}
