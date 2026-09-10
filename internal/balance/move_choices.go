package balance

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"termon.sh/internal/battle"
	"termon.sh/internal/dojo"
)

// DecisionEvidence is the public policy evidence captured for one voluntary
// decision. Replacements are deliberately never passed to this tracker.
type DecisionEvidence struct {
	Turn       int                        `json:"turn"`
	Trainer    string                     `json:"trainer"`
	Species    string                     `json:"species"`
	Selected   battle.Action              `json:"selected"`
	Considered []dojo.ScoredActionSummary `json:"considered"`
	ReliableKO bool                       `json:"reliable_ko"`
}

// MoveChoiceTracker aggregates conditional choices without reading battle
// state beyond the public DecisionEvidence supplied by the simulator.
type MoveChoiceTracker struct {
	rows              map[string]*MoveChoiceRow
	species           map[string]*moveChoiceSpecies
	VoluntarySwitches int
}

// MoveChoiceRow covers every Species-Move candidate observed in a policy
// explanation, including candidates that were never selected.
type MoveChoiceRow struct {
	Species             string            `json:"species"`
	Move                string            `json:"move"`
	Selected            int               `json:"selected"`
	ConditionalSelected int               `json:"conditional_selected"`
	Conditional         int               `json:"conditional"`
	ReliableKO          int               `json:"reliable_ko"`
	Sample              *DecisionEvidence `json:"sample,omitempty"`
}

type moveChoiceSpecies struct {
	missingEvidence int
	conditional     int
	reliableKO      int
}

// MoveChoiceGate is one Species dominance check. A missing explanation is an
// explicit not-measured failure, never a green zero-denominator result.
type MoveChoiceGate struct {
	Species     string  `json:"species"`
	Move        string  `json:"move,omitempty"`
	Passed      bool    `json:"passed"`
	NotMeasured bool    `json:"not_measured,omitempty"`
	Value       float64 `json:"value"`
	Threshold   string  `json:"threshold"`
	Selected    int     `json:"selected"`
	Choices     int     `json:"choices"`
	ReliableKO  int     `json:"reliable_ko"`
	Detail      string  `json:"detail,omitempty"`
}

// Observe records one voluntary policy decision.
func (t *MoveChoiceTracker) Observe(e DecisionEvidence) {
	if t.rows == nil {
		t.rows = map[string]*MoveChoiceRow{}
		t.species = map[string]*moveChoiceSpecies{}
	}
	state := t.species[e.Species]
	if state == nil {
		state = &moveChoiceSpecies{}
		t.species[e.Species] = state
	}
	if len(e.Considered) == 0 {
		state.missingEvidence++
		return
	}
	for _, candidate := range e.Considered {
		if candidate.Kind != battle.ActionMove || candidate.Move == "" {
			continue
		}
		t.row(e.Species, candidate.Move)
	}
	if e.Selected.Kind == battle.ActionSwitch {
		t.VoluntarySwitches++
		return
	}
	if e.Selected.Kind != battle.ActionMove || e.Selected.Move == "" {
		state.missingEvidence++
		return
	}
	row := t.row(e.Species, e.Selected.Move)
	if row.Sample == nil {
		sample := cloneDecisionEvidence(e)
		row.Sample = &sample
	}
	row.Selected++
	if e.ReliableKO {
		row.ReliableKO++
		state.reliableKO++
		return
	}
	if !eligibleMoveChoice(e) {
		return
	}
	row.ConditionalSelected++
	state.conditional++
}

func (t *MoveChoiceTracker) row(species, move string) *MoveChoiceRow {
	key := species + "\x00" + move
	if row := t.rows[key]; row != nil {
		return row
	}
	row := &MoveChoiceRow{Species: species, Move: move}
	t.rows[key] = row
	return row
}

// Rows returns a stable complete candidate inventory for observed evidence.
func (t *MoveChoiceTracker) Rows() []MoveChoiceRow {
	rows := make([]MoveChoiceRow, 0, len(t.rows))
	for _, row := range t.rows {
		value := *row
		value.Conditional = t.species[row.Species].conditional
		rows = append(rows, value)
	}
	slices.SortFunc(rows, func(a, b MoveChoiceRow) int {
		if a.Species != b.Species {
			return strings.Compare(a.Species, b.Species)
		}
		return strings.Compare(a.Move, b.Move)
	})
	return rows
}

// Gates applies the 70% conditional-choice dominance limit by Species-Move.
func (t *MoveChoiceTracker) Gates() []MoveChoiceGate {
	var gates []MoveChoiceGate
	rows := t.Rows()
	for species, state := range t.species {
		if state.missingEvidence > 0 {
			gates = append(gates, MoveChoiceGate{Species: species, Passed: false, NotMeasured: true, Threshold: "policy explanation required", Detail: fmt.Sprintf("%d decisions without considered actions", state.missingEvidence)})
		}
		if state.conditional == 0 {
			continue
		}
		for _, row := range rows {
			if row.Species != species {
				continue
			}
			rate := float64(row.ConditionalSelected) / float64(state.conditional)
			gates = append(gates, MoveChoiceGate{
				Species: species, Move: row.Move, Passed: rate <= .70, Value: rate * 100,
				Threshold: "<=70% of conditional Move choices", Selected: row.ConditionalSelected,
				Choices: state.conditional, ReliableKO: state.reliableKO,
				Detail: fmt.Sprintf("%d/%d conditional choices", row.ConditionalSelected, state.conditional),
			})
		}
	}
	slices.SortFunc(gates, func(a, b MoveChoiceGate) int {
		if a.Species != b.Species {
			return strings.Compare(a.Species, b.Species)
		}
		return strings.Compare(a.Move, b.Move)
	})
	return gates
}

func eligibleMoveChoice(e DecisionEvidence) bool {
	moves := moveCandidates(e.Considered)
	if len(moves) < 2 {
		return false
	}
	selected, ok := findMove(moves, e.Selected.Move)
	if !ok {
		return false
	}
	if isPreservation(selected) {
		return preservationEligible(selected, moves)
	}
	best := moves[0].Score
	for _, move := range moves[1:] {
		best = max(best, move.Score)
	}
	threshold := best - math.Abs(best)*.15
	if selected.Score < threshold-1e-9 {
		return false
	}
	for _, move := range moves {
		if move.Move != selected.Move && move.Score >= threshold-1e-9 {
			return true
		}
	}
	return false
}

func moveCandidates(candidates []dojo.ScoredActionSummary) []dojo.ScoredActionSummary {
	var moves []dojo.ScoredActionSummary
	for _, candidate := range candidates {
		if candidate.Kind == battle.ActionMove && candidate.Move != "" {
			moves = append(moves, candidate)
		}
	}
	return moves
}

func findMove(moves []dojo.ScoredActionSummary, slug string) (dojo.ScoredActionSummary, bool) {
	for _, move := range moves {
		if move.Move == slug {
			return move, true
		}
	}
	return dojo.ScoredActionSummary{}, false
}

func isPreservation(candidate dojo.ScoredActionSummary) bool {
	return candidate.KOProbability != nil || candidate.HealthyReserves != nil || candidate.ExpectedHPLoss != nil
}

func preservationEligible(selected dojo.ScoredActionSummary, moves []dojo.ScoredActionSummary) bool {
	if selected.KOProbability == nil || selected.HealthyReserves == nil || selected.ExpectedHPLoss == nil {
		return false
	}
	for _, other := range moves {
		if other.Move == selected.Move || other.KOProbability == nil || other.HealthyReserves == nil || other.ExpectedHPLoss == nil {
			continue
		}
		if math.Abs(*selected.KOProbability-*other.KOProbability) > 1e-9 || *selected.HealthyReserves != *other.HealthyReserves {
			continue
		}
		limit := math.Abs(*selected.ExpectedHPLoss) * .15
		if math.Abs(*selected.ExpectedHPLoss-*other.ExpectedHPLoss) <= limit+1e-9 {
			return true
		}
	}
	return false
}

func cloneDecisionEvidence(e DecisionEvidence) DecisionEvidence {
	clone := e
	clone.Considered = append([]dojo.ScoredActionSummary(nil), e.Considered...)
	return clone
}
