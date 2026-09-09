package balance

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
)

const (
	dailyMatrixBeamWidth = 32
	dailyMatrixNodeCap   = 4096
	dailyMatrixTurnLimit = 32
)

// DailyMatrix records concrete, replayable evidence for each authored Daily
// Challenge. A proof is accepted only after the engine replay, DailyTracker,
// and published par all agree; a simulator win by itself is not evidence.
type DailyMatrix struct {
	BeamWidth            int                 `json:"beam_width"`
	NodeCap              int                 `json:"node_cap"`
	TurnLimit            int                 `json:"turn_limit"`
	PolicyTieBreakSource string              `json:"policy_tie_break_source"`
	Fixtures             []DailyFixtureProof `json:"fixtures"`
	Passed               bool                `json:"passed"`
}

// DailyFixtureProof records the two required lines for one fixed-seed fixture.
type DailyFixtureProof struct {
	ID        string          `json:"id"`
	Seed      uint64          `json:"seed"`
	Par       int             `json:"par"`
	WithinPar *DailyProofLine `json:"within_par,omitempty"`
	OverPar   *DailyProofLine `json:"over_par,omitempty"`
	Failure   string          `json:"failure,omitempty"`
}

// DailyProofLine is one fully resolved Daily replay.
type DailyProofLine struct {
	Actions          []ActionEntry  `json:"actions"`
	Events           []battle.Event `json:"events"`
	Winner           string         `json:"winner"`
	Turns            int            `json:"turns"`
	ObjectiveMet     bool           `json:"objective_met"`
	ParMet           bool           `json:"par_met"`
	PlayerCritical   bool           `json:"player_critical"`
	PlayerMiss       bool           `json:"player_miss"`
	OpponentCritical bool           `json:"opponent_critical"`
	OpponentMiss     bool           `json:"opponent_miss"`
	Participation    []string       `json:"participation,omitempty"`
}

// RunDailyMatrix finds and verifies a successful-within-par line and a
// won-over-par line for every authoritative Daily fixture. Search considers
// only actions in the player's public Snapshot. Opponent choices use the
// published tier policy with its Daily 0% near-best band.
func RunDailyMatrix(set *content.Set) (DailyMatrix, error) {
	matrix := DailyMatrix{
		BeamWidth: dailyMatrixBeamWidth, NodeCap: dailyMatrixNodeCap, TurnLimit: dailyMatrixTurnLimit,
		PolicyTieBreakSource: "dojo.DailyPolicyRNG shared with the live Daily mode",
		Passed:               true,
	}
	if set == nil {
		return matrix, errors.New("daily matrix: nil content")
	}
	var failed []string
	for _, fixture := range dojo.DailyFixtures {
		proof := DailyFixtureProof{ID: fixture.ID, Seed: fixture.Seed, Par: fixture.Par}
		within, withinErr := findDailyProof(set, fixture, true)
		if withinErr == nil {
			proof.WithinPar = &within
		}
		over, overErr := findDailyProof(set, fixture, false)
		if overErr == nil {
			proof.OverPar = &over
		}
		if withinErr != nil || overErr != nil {
			matrix.Passed = false
			proof.Failure = dailyProofFailure(withinErr, overErr)
			failed = append(failed, fixture.ID+": "+proof.Failure)
		}
		matrix.Fixtures = append(matrix.Fixtures, proof)
	}
	if len(failed) > 0 {
		return matrix, fmt.Errorf("daily matrix: %d unproven fixtures: %s", len(failed), failed[0])
	}
	return matrix, nil
}

func dailyProofFailure(withinErr, overErr error) string {
	switch {
	case withinErr != nil && overErr != nil:
		return "within-par: " + withinErr.Error() + "; over-par: " + overErr.Error()
	case withinErr != nil:
		return "within-par: " + withinErr.Error()
	default:
		return "over-par: " + overErr.Error()
	}
}

type dailySearchNode struct {
	choices []battle.Action
	replay  dailyReplay
}

type dailyReplay struct {
	pending      []battle.Action
	line         DailyProofLine
	tracker      *dojo.DailyTracker
	snapshot     battle.Snapshot
	terminal     bool
	playerWon    bool
	objectiveMet bool
	parMet       bool
}

func findDailyProof(set *content.Set, fixture dojo.DailyFixture, withinPar bool) (DailyProofLine, error) {
	if line, ok, err := recordedDailyProof(set, fixture, withinPar); err != nil || ok {
		return line, err
	}
	root, err := replayDaily(set, fixture, nil)
	if err != nil {
		return DailyProofLine{}, err
	}
	nodes := 1
	queue := []dailySearchNode{{replay: root}}
	for len(queue) > 0 && nodes <= dailyMatrixNodeCap {
		next := make([]dailySearchNode, 0, dailyMatrixBeamWidth*4)
		for _, node := range queue {
			if dailyLineMatches(node.replay, withinPar) {
				return node.replay.line, nil
			}
			if node.replay.terminal || node.replay.line.Turns >= dailyMatrixTurnLimit || (withinPar && dailyClearImpossible(node.replay, fixture)) {
				continue
			}
			for _, action := range node.replay.pending {
				if nodes >= dailyMatrixNodeCap {
					break
				}
				choices := append(append([]battle.Action(nil), node.choices...), action)
				replay, err := replayDaily(set, fixture, choices)
				nodes++
				if err != nil {
					return DailyProofLine{}, fmt.Errorf("daily %s replay: %w", fixture.ID, err)
				}
				child := dailySearchNode{choices: choices, replay: replay}
				if dailyLineMatches(replay, withinPar) {
					return replay.line, nil
				}
				if !replay.terminal && replay.line.Turns < dailyMatrixTurnLimit && (!withinPar || !dailyClearImpossible(replay, fixture)) {
					next = append(next, child)
				}
			}
		}
		slices.SortFunc(next, func(a, b dailySearchNode) int {
			return dailySearchCompare(a, b, fixture, withinPar)
		})
		if len(next) > dailyMatrixBeamWidth {
			next = next[:dailyMatrixBeamWidth]
		}
		queue = next
	}
	mode := "won over par"
	if withinPar {
		mode = "objective clear within par"
	}
	return DailyProofLine{}, fmt.Errorf("%s not found within beam %d/node cap %d", mode, dailyMatrixBeamWidth, dailyMatrixNodeCap)
}

func dailyLineMatches(replay dailyReplay, withinPar bool) bool {
	if !replay.terminal || !replay.playerWon {
		return false
	}
	if withinPar {
		return replay.objectiveMet && replay.parMet
	}
	return replay.objectiveMet && !replay.parMet
}

func dailySearchCompare(a, b dailySearchNode, fixture dojo.DailyFixture, withinPar bool) int {
	as := dailySearchScore(a.replay, fixture, withinPar)
	bs := dailySearchScore(b.replay, fixture, withinPar)
	if as > bs {
		return -1
	}
	if as < bs {
		return 1
	}
	return slices.CompareFunc(a.choices, b.choices, compareDailyAction)
}

func compareDailyAction(a, b battle.Action) int {
	if a.Kind != b.Kind {
		return strings.Compare(string(a.Kind), string(b.Kind))
	}
	if a.Move != b.Move {
		return strings.Compare(a.Move, b.Move)
	}
	if a.SwitchTo != b.SwitchTo {
		return strings.Compare(a.SwitchTo, b.SwitchTo)
	}
	return 0
}

func dailySearchScore(replay dailyReplay, fixture dojo.DailyFixture, withinPar bool) float64 {
	score := dailyObjectiveProgress(replay, fixture)
	for _, member := range replay.snapshot.YourParty {
		if member.MaxHP > 0 && !member.Fainted && member.HP > 0 {
			score += float64(member.HP) / float64(member.MaxHP) * 20
		}
	}
	foeDamage, fainted := 0.0, 0
	for _, foe := range replay.snapshot.FoeRoster {
		if foe.Fainted {
			fainted++
		}
		if foe.Active && foe.MaxHP > 0 {
			foeDamage += float64(foe.MaxHP-foe.HP) / float64(foe.MaxHP)
		}
	}
	if withinPar {
		score += float64(fainted)*100 + foeDamage*40 - float64(replay.line.Turns)
	} else if replay.line.Turns <= fixture.Par {
		// Before par, preserve a live battle long enough to demonstrate a
		// legal over-par win. After par, damage and fainting take priority.
		score += float64(replay.line.Turns)*25 - foeDamage*20 - float64(fainted)*40
	} else {
		score += float64(fainted)*100 + foeDamage*40 - float64(replay.line.Turns)
	}
	if replay.terminal && replay.playerWon {
		score += 1000
	}
	return score
}

func dailyObjectiveProgress(replay dailyReplay, fixture dojo.DailyFixture) float64 {
	switch fixture.Objective {
	case "type_read":
		if replay.tracker.SEMove {
			return 180
		}
	case "safe_switch":
		if replay.tracker.SafeSwitch {
			return 180
		}
	case "full_rotation":
		return float64(len(replay.tracker.MonTurn)) * 70
	case "preservation":
		return float64(dailyHealthyCount(replay.snapshot) * 60)
	case "limited_toolkit":
		if !replay.tracker.UsedOver65 {
			return 80
		}
	case "master_trial":
		score := float64(len(replay.tracker.MonTurn)) * 70
		if replay.tracker.SEMove {
			score += 120
		}
		if replay.tracker.SafeSwitch {
			score += 120
		}
		return score
	}
	return 0
}

func dailyHealthyCount(snap battle.Snapshot) int {
	count := 0
	for _, member := range snap.YourParty {
		if !member.Fainted && member.HP > 0 {
			count++
		}
	}
	return count
}

// replayDaily replays a public player action prefix. It stops before the next
// player decision so the search cannot inspect an opponent's hidden selection.
func replayDaily(set *content.Set, fixture dojo.DailyFixture, choices []battle.Action) (dailyReplay, error) {
	player, opponent, err := dojo.DailyParties(set, fixture)
	if err != nil {
		return dailyReplay{}, err
	}
	bt, err := battle.New(set, player, opponent, battle.Seeded(fixture.Seed))
	if err != nil {
		return dailyReplay{}, err
	}
	tracker := dojo.NewDailyTracker(fixture)
	result := dailyReplay{tracker: tracker}
	choiceIndex := 0
	transitions := 0
	policy := dojo.TierConfig(fixture.PolicyTier)
	policy.NearBestBand = 0
	for bt.State() != battle.StateOver {
		beforeState, beforeTurn := bt.State(), bt.Turn()
		switch bt.State() {
		case battle.StateRevealing:
			if err := bt.AdvanceReveal(); err != nil {
				return result, err
			}
		case battle.StateAwaitingReplacement:
			snap := bt.Snapshot(player.Trainer)
			if snap.ReplacementRequired {
				if choiceIndex == len(choices) {
					result.pending = dailyReplacementActions(snap)
					return dailyReplayResult(set, bt, player, tracker, result), nil
				}
				action := choices[choiceIndex]
				choiceIndex++
				if action.Kind != battle.ActionSwitch {
					return result, fmt.Errorf("daily %s: replacement action %q", fixture.ID, action.Kind)
				}
				if err := bt.Replace(player.Trainer, action.SwitchTo); err != nil {
					return result, err
				}
				result.line.Actions = append(result.line.Actions, actionEntry(bt.Turn(), player.Trainer, action))
				continue
			}
			view, ok := bt.PolicyViewFor(opponent.Trainer)
			if !ok {
				return result, fmt.Errorf("daily %s: opponent replacement view unavailable", fixture.ID)
			}
			id, _, err := dojo.ChooseReplacement(set, view, policy, dojo.DailyPolicyRNG(fixture.Seed, bt.Turn(), true))
			if err != nil {
				return result, err
			}
			action := battle.Action{Kind: battle.ActionSwitch, SwitchTo: id}
			if err := bt.Replace(opponent.Trainer, id); err != nil {
				return result, err
			}
			result.line.Actions = append(result.line.Actions, actionEntry(bt.Turn(), opponent.Trainer, action))
		case battle.StateAwaitingActions:
			if choiceIndex == len(choices) {
				result.pending = dailyPublicActions(bt.Snapshot(player.Trainer))
				return dailyReplayResult(set, bt, player, tracker, result), nil
			}
			action := choices[choiceIndex]
			choiceIndex++
			before := bt.Snapshot(player.Trainer)
			if err := bt.Select(player.Trainer, action); err != nil {
				return result, err
			}
			result.line.Actions = append(result.line.Actions, actionEntry(bt.Turn()+1, player.Trainer, action))
			view, ok := bt.PolicyViewFor(opponent.Trainer)
			if !ok {
				return result, fmt.Errorf("daily %s: opponent policy view unavailable", fixture.ID)
			}
			botAction, _, err := dojo.ChoosePolicyAction(set, view, policy, dojo.DailyPolicyRNG(fixture.Seed, bt.Turn(), false))
			if err != nil {
				return result, err
			}
			if err := bt.Select(opponent.Trainer, botAction); err != nil {
				return result, err
			}
			result.line.Actions = append(result.line.Actions, actionEntry(bt.Turn(), opponent.Trainer, botAction))
			observeDailyDecision(set, bt, tracker, player.Trainer, action, before)
		default:
			return result, fmt.Errorf("daily %s: unexpected state %q", fixture.ID, bt.State())
		}
		if bt.Turn() >= dailyMatrixTurnLimit && bt.State() != battle.StateOver {
			return dailyReplayResult(set, bt, player, tracker, result), nil
		}
		if bt.State() == beforeState && bt.Turn() == beforeTurn {
			transitions++
			if transitions >= 4 {
				return result, fmt.Errorf("daily %s: nonadvancing %s", fixture.ID, bt.State())
			}
		} else {
			transitions = 0
		}
	}
	if choiceIndex != len(choices) {
		return result, fmt.Errorf("daily %s: unused player actions", fixture.ID)
	}
	result.terminal = true
	return dailyReplayResult(set, bt, player, tracker, result), nil
}

func dailyReplayResult(set *content.Set, bt *battle.Battle, player battle.Party, tracker *dojo.DailyTracker, result dailyReplay) dailyReplay {
	result.snapshot = bt.Snapshot(player.Trainer)
	result.line.Events = bt.Events()
	result.line.Winner = bt.Winner()
	result.line.Turns = bt.Turn()
	result.playerWon = result.line.Winner == player.Trainer
	result.objectiveMet = tracker.ObjectiveMet(set, result.playerWon, result.line.Turns, result.snapshot)
	result.parMet = tracker.ParMet(result.playerWon, result.line.Turns)
	result.line.ObjectiveMet = result.objectiveMet
	result.line.ParMet = result.parMet
	for _, event := range result.line.Events {
		if event.Actor == player.Trainer {
			if event.Kind == battle.EventCriticalHit {
				result.line.PlayerCritical = true
			}
			if event.Kind == battle.EventMissed {
				result.line.PlayerMiss = true
			}
			continue
		}
		if event.Kind == battle.EventCriticalHit {
			result.line.OpponentCritical = true
		}
		if event.Kind == battle.EventMissed {
			result.line.OpponentMiss = true
		}
	}
	for id := range tracker.MonTurn {
		result.line.Participation = append(result.line.Participation, id)
	}
	slices.Sort(result.line.Participation)
	return result
}

func dailyReplacementActions(snap battle.Snapshot) []battle.Action {
	var actions []battle.Action
	for _, reserve := range snap.HealthyReserves() {
		actions = append(actions, battle.Action{Kind: battle.ActionSwitch, SwitchTo: reserve.ID})
	}
	return actions
}

func dailyPublicActions(snap battle.Snapshot) []battle.Action {
	var actions []battle.Action
	for _, move := range snap.YourPartyActiveLoadout() {
		actions = append(actions, battle.Action{Kind: battle.ActionMove, Move: move})
	}
	return append(actions, dailyReplacementActions(snap)...)
}

func dailySwitchTypes(set *content.Set, snap battle.Snapshot, action battle.Action) (from, to, foe string) {
	if action.Kind != battle.ActionSwitch {
		return "", "", ""
	}
	for _, member := range snap.YourParty {
		if member.Active {
			from = set.Species[member.Species].Type
		}
		if member.ID == action.SwitchTo {
			to = set.Species[member.Species].Type
		}
	}
	for _, member := range snap.FoeRoster {
		if member.Active {
			foe = set.Species[member.Species].Type
		}
	}
	return from, to, foe
}

func observeDailyDecision(set *content.Set, bt *battle.Battle, tracker *dojo.DailyTracker, trainer string, action battle.Action, before battle.Snapshot) {
	tracker.ObserveTurn(set, bt.Events(), bt.Turn(), trainer, action.Move, bt.Snapshot(trainer))
	if action.Kind == battle.ActionSwitch {
		from, to, foe := dailySwitchTypes(set, before, action)
		tracker.RecordSafeSwitch(set, from, to, foe)
	}
}

// dailyClearImpossible prunes states that cannot satisfy the authored objective,
// rather than rewarding damage on a branch that already forfeited the clear.
func dailyClearImpossible(replay dailyReplay, fixture dojo.DailyFixture) bool {
	if replay.line.Turns > fixture.Par || replay.tracker.UsedOver65 {
		return true
	}
	switch fixture.Objective {
	case "preservation":
		return dailyHealthyCount(replay.snapshot) < 2
	case "full_rotation", "master_trial":
		for _, member := range replay.snapshot.YourParty {
			if member.Fainted && !replay.tracker.MonTurn[member.ID] {
				return true
			}
		}
	}
	return false
}
