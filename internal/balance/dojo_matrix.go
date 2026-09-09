package balance

import (
	"errors"
	"fmt"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
	"termon.sh/internal/game"
)

const (
	dojoMatrixPlayer = "dojo-matrix:player"
	dojoMatrixBot    = "dojo-matrix:bot"
)

// DojoMatrix records the natural Sparring evidence required by the balance
// methodology. Counts include only completed battles; failed scenarios remain
// visible in their cell and make the matrix fail.
type DojoMatrix struct {
	Cells  []DojoMatrixCell `json:"cells"`
	Gates  []DojoMatrixGate `json:"gates"`
	Passed bool             `json:"passed"`
}

// DojoMatrixCell is one anchor team, natural checkpoint, and Dojo tier. Wins
// and Rate belong to the Dojo opponent, matching the published tier bands.
type DojoMatrixCell struct {
	Team       string            `json:"team"`
	Checkpoint int               `json:"checkpoint"`
	Tier       string            `json:"tier"`
	Wins       int               `json:"wins"`
	Count      int               `json:"count"`
	Failed     int               `json:"failed"`
	Rate       float64           `json:"rate"`
	Failures   []DojoMatrixTrace `json:"failures,omitempty"`
	Sample     *DojoMatrixTrace  `json:"sample,omitempty"`
}

// DojoMatrixTrace is a bounded replay artifact. The matrix intentionally
// keeps one failed trace and one completed sample per cell, never its corpus.
type DojoMatrixTrace struct {
	Scenario    string           `json:"scenario"`
	Seed        uint64           `json:"seed"`
	EngineSideA bool             `json:"engine_side_a"`
	Winner      string           `json:"winner,omitempty"`
	Reason      battle.EndReason `json:"reason,omitempty"`
	Turns       int              `json:"turns"`
	Error       string           `json:"error,omitempty"`
	Actions     []ActionEntry    `json:"actions,omitempty"`
	Events      []battle.Event   `json:"events,omitempty"`
}

// DojoMatrixGate is one published Sparring band or ordering check.
type DojoMatrixGate struct {
	Name       string  `json:"name"`
	Team       string  `json:"team"`
	Checkpoint int     `json:"checkpoint"`
	Tier       string  `json:"tier,omitempty"`
	Passed     bool    `json:"passed"`
	Value      float64 `json:"value"`
	Threshold  string  `json:"threshold"`
	Detail     string  `json:"detail,omitempty"`
}

// RunDojoMatrix runs each anchor team and natural checkpoint against the
// single authoritative Sparring roster built for that player party. The player
// uses the Pivot reference policy; the bot uses the published tier policy.
// Every seed is played with the player on both physical engine sides.
func RunDojoMatrix(set *content.Set, seeds []uint64) (DojoMatrix, error) {
	matrix := DojoMatrix{Passed: true}
	if set == nil {
		return matrix, errors.New("dojo matrix: nil content")
	}
	if len(seeds) == 0 {
		return matrix, errors.New("dojo matrix: empty seed corpus")
	}

	index := make(map[string]int)
	for _, team := range ReferenceTeams {
		for _, checkpoint := range NaturalCheckpoints {
			for _, tier := range dojoMatrixTiers() {
				index[dojoMatrixKey(team.Name, checkpoint, tier)] = len(matrix.Cells)
				matrix.Cells = append(matrix.Cells, DojoMatrixCell{
					Team: team.Name, Checkpoint: checkpoint, Tier: tier,
				})
			}
		}
	}

	failed := 0
	var firstFailure DojoMatrixTrace
	for _, team := range ReferenceTeams {
		for _, checkpoint := range NaturalCheckpoints {
			player, err := BuildNaturalParty(set, team, 0, checkpoint, dojoMatrixPlayer, false)
			if err != nil {
				return matrix, fmt.Errorf("dojo matrix: build %s level %d: %w", team.Name, checkpoint, err)
			}
			bot, err := buildDojoMatrixRoster(set, player)
			if err != nil {
				return matrix, fmt.Errorf("dojo matrix: sparring roster %s level %d: %w", team.Name, checkpoint, err)
			}

			for _, tier := range dojoMatrixTiers() {
				cell := &matrix.Cells[index[dojoMatrixKey(team.Name, checkpoint, tier)]]
				runDojoCell(set, player, bot, seeds, cell)
				failed += cell.Failed
				if firstFailure.Scenario == "" && len(cell.Failures) > 0 {
					firstFailure = cell.Failures[0]
				}
			}
		}
	}
	for i := range matrix.Cells {
		if matrix.Cells[i].Count > 0 {
			matrix.Cells[i].Rate = float64(matrix.Cells[i].Wins) / float64(matrix.Cells[i].Count)
		}
	}
	matrix.Gates = evaluateDojoMatrix(matrix.Cells)
	for _, gate := range matrix.Gates {
		if !gate.Passed {
			matrix.Passed = false
		}
	}
	if failed == 0 {
		return matrix, nil
	}
	matrix.Passed = false
	return matrix, fmt.Errorf(
		"dojo matrix: %d failed scenarios; first %s seed %d: %s",
		failed, firstFailure.Scenario, firstFailure.Seed, firstFailure.Error,
	)
}

func dojoMatrixTiers() []string {
	return []string{dojo.TierApprentice, dojo.TierRival, dojo.TierMaster}
}

func dojoMatrixKey(team string, checkpoint int, tier string) string {
	return fmt.Sprintf("%s/%d/%s", team, checkpoint, tier)
}

func dojoMatrixSideName(playerSideA bool) string {
	if playerSideA {
		return "a"
	}
	return "b"
}

func buildDojoMatrixRoster(set *content.Set, player battle.Party) (battle.Party, error) {
	monsters := make([]game.Monster, len(player.Members))
	for i, member := range player.Members {
		monsters[i] = member.Monster
	}
	slots, err := dojo.BuildSparringRoster(set, monsters, 0)
	if err != nil {
		return battle.Party{}, err
	}
	members := make([]battle.PartyMember, len(slots))
	for i, slot := range slots {
		members[i] = battle.PartyMember{Monster: game.Monster{
			ID:            fmt.Sprintf("%s-%d", dojoMatrixBot, i),
			Species:       slot.Species,
			Level:         slot.Level,
			BattleLoadout: append([]string(nil), slot.Loadout...),
		}}
	}
	return battle.Party{Trainer: dojoMatrixBot, Members: members}, nil
}

func runDojoMatrixScenario(
	set *content.Set,
	player, bot battle.Party,
	tier string,
	seed uint64,
	playerSideA bool,
	scenario string,
) (DojoMatrixTrace, bool, error) {
	left, right := bot, player
	if playerSideA {
		left, right = player, bot
	}
	bt, err := battle.New(set, left, right, battle.Seeded(seed))
	if err != nil {
		return DojoMatrixTrace{Scenario: scenario, Seed: seed, EngineSideA: playerSideA}, false, err
	}
	trace := DojoMatrixTrace{Scenario: scenario, Seed: seed, EngineSideA: playerSideA}
	transitions := 0
	for bt.State() != battle.StateOver {
		beforeState, beforeTurn := bt.State(), bt.Turn()
		switch bt.State() {
		case battle.StateRevealing:
			if err := bt.AdvanceReveal(); err != nil {
				return dojoMatrixFinishedTrace(bt, trace), false, fmt.Errorf("advance reveal: %w", err)
			}
		case battle.StateAwaitingReplacement:
			for _, trainer := range []string{player.Trainer, bot.Trainer} {
				snap := bt.Snapshot(trainer)
				if !snap.ReplacementRequired {
					continue
				}
				view, ok := bt.PolicyViewFor(trainer)
				if !ok {
					return dojoMatrixFinishedTrace(bt, trace), false, fmt.Errorf("replacement view for %q unavailable", trainer)
				}
				cfg := dojo.TierConfig(tier)
				if trainer == player.Trainer {
					// Pivot defines voluntary actions. Forced replacement is resolved
					// through the same published Rival reserve scorer as Simulate.
					cfg = dojo.TierConfig(dojo.TierRival)
				}
				id, _, err := dojo.ChooseReplacement(set, view, cfg, dojoMatrixRNG(seed, bt.Turn(), trainer+"-replace"))
				if err != nil {
					return dojoMatrixFinishedTrace(bt, trace), false, fmt.Errorf("replacement policy for %q: %w", trainer, err)
				}
				act := battle.Action{Kind: battle.ActionSwitch, SwitchTo: id}
				if err := bt.Replace(trainer, id); err != nil {
					return dojoMatrixFinishedTrace(bt, trace), false, fmt.Errorf("replace %q: %w", trainer, err)
				}
				trace.Actions = append(trace.Actions, actionEntry(bt.Turn(), trainer, act))
			}
		case battle.StateAwaitingActions:
			actionTurn := bt.Turn() + 1
			for _, trainer := range []string{player.Trainer, bot.Trainer} {
				if bt.Locked(trainer) {
					continue
				}
				act, err := dojoMatrixAction(set, bt, trainer, player.Trainer, tier, seed)
				if err != nil {
					return dojoMatrixFinishedTrace(bt, trace), false, fmt.Errorf("action policy for %q: %w", trainer, err)
				}
				if err := bt.Select(trainer, act); err != nil {
					return dojoMatrixFinishedTrace(bt, trace), false, fmt.Errorf("select %q: %w", trainer, err)
				}
				trace.Actions = append(trace.Actions, actionEntry(actionTurn, trainer, act))
			}
		default:
			return dojoMatrixFinishedTrace(bt, trace), false, fmt.Errorf("unexpected battle state %q", bt.State())
		}
		if bt.Turn() >= DefaultMaxTurns && bt.State() != battle.StateOver {
			return dojoMatrixFinishedTrace(bt, trace), false, fmt.Errorf("turn cap %d reached", DefaultMaxTurns)
		}
		if bt.State() == beforeState && bt.Turn() == beforeTurn {
			transitions++
			if transitions >= 4 {
				return dojoMatrixFinishedTrace(bt, trace), false, fmt.Errorf("nonadvancing state %q at turn %d", bt.State(), bt.Turn())
			}
		} else {
			transitions = 0
		}
	}
	trace = dojoMatrixFinishedTrace(bt, trace)
	return trace, trace.Winner == player.Trainer, nil
}

func dojoMatrixAction(set *content.Set, bt *battle.Battle, trainer, playerTrainer, tier string, seed uint64) (battle.Action, error) {
	view, ok := bt.PolicyViewFor(trainer)
	if !ok {
		return battle.Action{}, fmt.Errorf("policy view for %q unavailable", trainer)
	}
	if trainer == playerTrainer {
		action, _, err := dojo.ChooseReferenceAction(set, view, dojo.ReferencePivot, dojoMatrixRNG(seed, bt.Turn(), trainer))
		return action, err
	}
	action, _, err := dojo.ChoosePolicyAction(set, view, dojo.TierConfig(tier), dojoMatrixRNG(seed, bt.Turn(), trainer))
	return action, err
}

func dojoMatrixFinishedTrace(bt *battle.Battle, trace DojoMatrixTrace) DojoMatrixTrace {
	trace.Winner = bt.Winner()
	trace.Reason = bt.Reason()
	trace.Turns = bt.Turn()
	trace.Events = bt.Events()
	return trace
}

func dojoMatrixRNG(seed uint64, turn int, tag string) battle.Rand {
	mix := seed ^ 0x517cc1b727220a95
	if turn > 0 {
		mix ^= uint64(turn) * 0x9e3779b97f4a7c15 //nolint:gosec // the capped turn count is non-negative
	}
	for _, char := range []byte(tag) {
		mix = mix*0x100000001b3 ^ uint64(char)
	}
	return battle.Seeded(mix)
}

func evaluateDojoMatrix(cells []DojoMatrixCell) []DojoMatrixGate {
	gates := make([]DojoMatrixGate, 0, len(cells)+len(cells)/3)
	byScenario := make(map[string]map[string]DojoMatrixCell)
	for _, cell := range cells {
		low, high := dojoMatrixBand(cell.Tier)
		passed := cell.Count > 0 && cell.Failed == 0 && cell.Rate >= low && cell.Rate <= high
		detail := fmt.Sprintf("%d/%d Dojo wins", cell.Wins, cell.Count)
		if cell.Failed > 0 {
			detail += fmt.Sprintf("; %d failed scenarios", cell.Failed)
		}
		gates = append(gates, DojoMatrixGate{
			Name: "dojo_tier_band", Team: cell.Team, Checkpoint: cell.Checkpoint,
			Tier: cell.Tier, Passed: passed, Value: cell.Rate * 100,
			Threshold: fmt.Sprintf("Dojo win %.0f-%.0f%%", low*100, high*100), Detail: detail,
		})
		key := fmt.Sprintf("%s/%d", cell.Team, cell.Checkpoint)
		if byScenario[key] == nil {
			byScenario[key] = make(map[string]DojoMatrixCell)
		}
		byScenario[key][cell.Tier] = cell
	}
	for _, team := range ReferenceTeams {
		for _, checkpoint := range NaturalCheckpoints {
			cellsByTier := byScenario[fmt.Sprintf("%s/%d", team.Name, checkpoint)]
			apprentice, aok := cellsByTier[dojo.TierApprentice]
			rival, rok := cellsByTier[dojo.TierRival]
			master, mok := cellsByTier[dojo.TierMaster]
			firstGap := rival.Rate - apprentice.Rate
			secondGap := master.Rate - rival.Rate
			minGap := min(firstGap, secondGap)
			complete := aok && rok && mok && apprentice.Count > 0 && rival.Count > 0 && master.Count > 0 && apprentice.Failed == 0 && rival.Failed == 0 && master.Failed == 0
			passed := complete && apprentice.Rate < rival.Rate && rival.Rate < master.Rate && firstGap >= .07 && secondGap >= .07
			gates = append(gates, DojoMatrixGate{
				Name: "dojo_tier_ordering", Team: team.Name, Checkpoint: checkpoint,
				Passed: passed, Value: minGap * 100, Threshold: "Apprentice < Rival < Master; adjacent >=7pp",
				Detail: fmt.Sprintf("Apprentice %.1f%%, Rival %.1f%%, Master %.1f%%", apprentice.Rate*100, rival.Rate*100, master.Rate*100),
			})
		}
	}
	return gates
}

func dojoMatrixBand(tier string) (float64, float64) {
	switch tier {
	case dojo.TierApprentice:
		return .35, .45
	case dojo.TierRival:
		return .45, .55
	case dojo.TierMaster:
		return .55, .65
	default:
		return 1, 0
	}
}

func runDojoCell(set *content.Set, player, bot battle.Party, seeds []uint64, cell *DojoMatrixCell) {
	for _, seed := range seeds {
		for _, playerSideA := range []bool{true, false} {
			scenario := fmt.Sprintf(
				"dojo/%s/level-%d/%s/seed-%d/player-side-%s",
				cell.Team, cell.Checkpoint, cell.Tier, seed, dojoMatrixSideName(playerSideA),
			)
			trace, playerWon, err := runDojoMatrixScenario(set, player, bot, cell.Tier, seed, playerSideA, scenario)
			if err != nil {
				cell.Failed++
				trace.Error = err.Error()
				if len(cell.Failures) == 0 {
					cell.Failures = append(cell.Failures, trace)
				}
				continue
			}
			cell.Count++
			if !playerWon {
				cell.Wins++
			}
			if cell.Sample == nil {
				cell.Sample = &trace
			}
		}
	}
}
