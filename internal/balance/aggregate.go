package balance

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// outcomeAccumulator retains aggregate gate inputs and bounded replay samples.
type outcomeAccumulator struct {
	team    map[string]record
	pairs   map[string]record
	mirror  record
	side    record
	turns   map[int]int
	hits    map[int]int
	ohko    bool
	illegal int
	samples map[string]*BattleOutcome
	choices MoveChoiceTracker
}

type record struct{ wins, total int }

func newOutcomeAccumulator() *outcomeAccumulator {
	return &outcomeAccumulator{team: map[string]record{}, pairs: map[string]record{}, samples: map[string]*BattleOutcome{}, turns: map[int]int{}, hits: map[int]int{}}
}

func (a *outcomeAccumulator) Observe(out *BattleOutcome) {
	if out == nil {
		return
	}
	winner := WinningTeam(out)
	if out.TeamA.Name != out.TeamB.Name {
		for _, team := range []ReferenceTeam{out.TeamA, out.TeamB} {
			r := a.team[team.Name]
			r.total++
			if winner.Name == team.Name {
				r.wins++
			}
			a.team[team.Name] = r
		}
		key := pairKey(out.TeamA.Name, out.TeamB.Name)
		r := a.pairs[key]
		r.total++
		if winner.Name == min(out.TeamA.Name, out.TeamB.Name) {
			r.wins++
		}
		a.pairs[key] = r
		if _, ok := a.samples[key]; !ok {
			a.samples[key] = out
		}
		a.side.total++
		if EngineSideWin(out) {
			a.side.wins++
		}
	} else {
		a.mirror.total++
		if EngineSideWin(out) {
			a.mirror.wins++
		}
	}
	a.turns[out.Turns]++
	for _, pace := range out.FaintPaces {
		if pace.SuperEffective || pace.StageMismatch {
			continue
		}
		a.hits[pace.Hits]++
		a.ohko = a.ohko || (pace.Hits <= 1 && !pace.Critical)
	}
	a.illegal += out.IllegalActions
	for _, decision := range out.Decisions {
		a.choices.Observe(decision)
	}
}

func (a *outcomeAccumulator) Gates() []GateResult {
	minimum, maximum := 1.0, 0.0
	for _, r := range a.team {
		rate := float64(r.wins) / float64(r.total)
		minimum, maximum = min(minimum, rate), max(maximum, rate)
	}
	gates := []GateResult{{
		Name: GateReferenceTeamWinRate, Passed: len(a.team) > 0 && minimum >= 0.40 && maximum <= 0.60,
		Value: minimum, Maximum: maximum, Threshold: "40-60%", Detail: fmt.Sprintf("team win-rate band %.1f%%-%.1f%%", minimum*100, maximum*100),
	}}
	keys := make([]string, 0, len(a.pairs))
	for key := range a.pairs {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		r := a.pairs[key]
		names := strings.SplitN(key, "/", 2)
		rate := float64(r.wins) / float64(r.total)
		gates = append(gates, GateResult{
			Name: GateNonMirrorMatchup, TeamA: names[0], TeamB: names[1], Wins: r.wins, Total: r.total,
			Passed: rate >= 0.25 && rate <= 0.75, Value: rate, Threshold: "25-75%", Detail: fmt.Sprintf("%s vs %s: %d/%d", names[0], names[1], r.wins, r.total),
		})
	}
	mirrorRate := float64(a.mirror.wins) / float64(max(1, a.mirror.total))
	sideRate := float64(a.side.wins) / float64(max(1, a.side.total))
	advantage := math.Abs(sideRate - 0.5)
	hitMedian, hitTotal := histogramMedian(a.hits)
	turnMedian, turnTotal := histogramMedian(a.turns)
	p90 := histogramValue(a.turns, max(0, (turnTotal*90+99)/100-1))
	paceDetail := fmt.Sprintf("%d neutral faints", hitTotal)
	if a.ohko {
		paceDetail += "; non-crit OHKO"
	}
	return append(gates,
		GateResult{Name: GateMirrorWinRate, Passed: a.mirror.total > 0 && mirrorRate >= 0.47 && mirrorRate <= 0.53, Value: mirrorRate, Threshold: "47-53%"},
		GateResult{Name: GateEngineSideAdvantage, Passed: a.side.total > 0 && advantage <= 0.03, Value: advantage * 100, Threshold: "<=3pp", Detail: fmt.Sprintf("engine-side win %.1f%%", sideRate*100)},
		GateResult{Name: GateNeutralKOPace, Passed: hitTotal > 0 && hitMedian >= 3 && hitMedian <= 5 && !a.ohko, Value: float64(hitMedian), Threshold: "median 3-5 hits/faint, no non-crit OHKO", Detail: paceDetail},
		GateResult{Name: GateBattlePace, Passed: turnTotal > 0 && turnMedian >= 6 && turnMedian <= 15 && p90 <= 24, Value: float64(turnMedian), Threshold: "median 6-15 turns, p90 <= 24", Detail: fmt.Sprintf("p90=%d", p90)},
		GateResult{Name: GateIllegalActions, Passed: a.illegal == 0, Value: float64(a.illegal), Threshold: "0"},
	)
}

func histogramMedian(histogram map[int]int) (int, int) {
	total := 0
	for _, count := range histogram {
		total += count
	}
	if total == 0 {
		return 0, 0
	}
	return (histogramValue(histogram, (total-1)/2) + histogramValue(histogram, total/2)) / 2, total
}

func histogramValue(histogram map[int]int, index int) int {
	keys := make([]int, 0, len(histogram))
	for value := range histogram {
		keys = append(keys, value)
	}
	slices.Sort(keys)
	for _, value := range keys {
		if index < histogram[value] {
			return value
		}
		index -= histogram[value]
	}
	return 0
}

func pairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "/" + b
}

func (a *outcomeAccumulator) FailedSamples() []*BattleOutcome {
	out := make([]*BattleOutcome, 0, len(a.samples))
	for _, sample := range a.samples {
		out = append(out, sample)
	}
	slices.SortFunc(out, func(x, y *BattleOutcome) int {
		if x.Scenario < y.Scenario {
			return -1
		}
		if x.Scenario > y.Scenario {
			return 1
		}
		return 0
	})
	return out
}
