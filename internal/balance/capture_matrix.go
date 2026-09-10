package balance

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"termon.sh/internal/battle"
	"termon.sh/internal/capture"
	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
	"termon.sh/internal/game"
)

const (
	captureTrainer = "capture-matrix:trainer"
	captureWild    = "capture-matrix:wild"
	captureTurns   = 12
)

// CaptureMatrix records deterministic Target Encounter trajectories.
type CaptureMatrix struct {
	Cases  []CaptureMatrixCase `json:"cases"`
	Passed bool                `json:"passed"`
}

// CaptureMatrixCase preserves successful, failed, and unproven evidence.
type CaptureMatrixCase struct {
	Team       string               `json:"team"`
	Checkpoint int                  `json:"checkpoint"`
	Party      []CapturePartyMember `json:"party,omitempty"`
	Target     string               `json:"target"`
	Variant    string               `json:"variant"`
	Objectives []capture.Objective  `json:"objectives,omitempty"`
	Actions    []ActionEntry        `json:"actions,omitempty"`
	Gauge      int                  `json:"gauge"`
	Outcome    string               `json:"outcome"`
	Events     []battle.Event       `json:"events,omitempty"`
	Passed     bool                 `json:"passed"`
	Failure    string               `json:"failure,omitempty"`
}

// CapturePartyMember records one concrete Party input to a trajectory.
type CapturePartyMember struct {
	ID      string   `json:"id"`
	Species string   `json:"species"`
	Level   int      `json:"level"`
	Loadout []string `json:"loadout"`
	MaxHP   int      `json:"max_hp,omitempty"`
	Stats   *[5]int  `json:"stats,omitempty"`
}

// RunCaptureMatrix exercises every Family target against every anchor Party
// and natural checkpoint. It mirrors the public Target Encounter setup.
func RunCaptureMatrix(set *content.Set) CaptureMatrix {
	matrix := CaptureMatrix{Passed: true}
	for _, team := range ReferenceTeams {
		for _, level := range NaturalCheckpoints {
			for _, target := range captureTargets(set) {
				for _, variant := range []string{"min_variance", "max_variance", "miss", "overaggressive"} {
					result := runCaptureCase(set, team, level, target, variant)
					matrix.Cases = append(matrix.Cases, result)
					if !result.Passed {
						matrix.Passed = false
					}
				}
			}
		}
	}
	return matrix
}

func captureTargets(set *content.Set) []string {
	predecessor := map[string]bool{}
	for _, species := range set.Species {
		if species.EvolvesTo != nil {
			predecessor[species.EvolvesTo.Species] = true
		}
	}
	targets := make([]string, 0, len(set.Species))
	for slug := range set.Species {
		if !predecessor[slug] {
			targets = append(targets, slug)
		}
	}
	sort.Strings(targets)
	return targets
}

func runCaptureCase(set *content.Set, team ReferenceTeam, level int, target, variant string) CaptureMatrixCase {
	result := CaptureMatrixCase{Team: team.Name, Checkpoint: level, Target: target, Variant: variant}
	party, err := BuildNaturalParty(set, team, 0, level, captureTrainer, false)
	if err != nil {
		result.Failure = err.Error()
		return result
	}
	result.Party = capturePartyIdentity(party)
	fighters := captureFighters(party)
	objectives, err := capture.Generate(set, fighters, target)
	if err != nil {
		result.Failure = err.Error()
		return result
	}
	result.Objectives = objectives
	wild, err := captureWildParty(set, fighters, target)
	if err != nil {
		result.Failure = err.Error()
		return result
	}
	rng := captureRand{}
	bt, err := battle.New(set, party, wild, &rng)
	if err != nil {
		result.Failure = err.Error()
		return result
	}
	session := capture.NewSession(objectives)
	usedMoves := map[string]bool{}
	missTurn := -1
	for turn := 0; turn < captureTurns && bt.State() != battle.StateOver; turn++ {
		if err := advanceCaptureReveal(bt); err != nil {
			result.Failure = err.Error()
			return result
		}
		if bt.State() != battle.StateAwaitingActions {
			break
		}
		trainerView, ok := bt.PolicyViewFor(captureTrainer)
		if !ok {
			result.Failure = "trainer policy view unavailable"
			return result
		}
		trainerAction, trainerMove := captureAction(set, bt.Snapshot(captureTrainer), session, usedMoves, variant, turn, missTurn < 0)
		if variant == "miss" && missTurn < 0 && trainerMove != "" && turn > 0 && usedMoves[trainerMove] && set.Moves[trainerMove].Accuracy < 100 {
			missTurn = turn
		}
		wildView, ok := bt.PolicyViewFor(captureWild)
		if !ok {
			result.Failure = "wild policy view unavailable"
			return result
		}
		wildAction, _, err := dojo.ChoosePolicyAction(set, wildView, dojo.TierConfig(dojo.TierApprentice), policyRNG(0, bt.Turn(), "capture-wild"))
		if err != nil {
			result.Failure = err.Error()
			return result
		}
		rng.Reset(captureTurnRolls(trainerAction, wildAction, activeSpeed(trainerView), activeSpeed(wildView), variant, turn, missTurn))
		if err := bt.Select(captureTrainer, trainerAction); err != nil {
			result.Failure = err.Error()
			return result
		}
		result.Actions = append(result.Actions, actionEntry(bt.Turn()+1, captureTrainer, trainerAction))
		if err := bt.Select(captureWild, wildAction); err != nil {
			result.Failure = err.Error()
			return result
		}
		if rng.exhausted {
			result.Failure = "combat consumed an unexpected RNG roll"
			return result
		}
		result.Actions = append(result.Actions, actionEntry(bt.Turn(), captureWild, wildAction))
		snap := bt.Snapshot(captureTrainer)
		wildHP, wildMax := captureWildHP(snap)
		input := capture.BuildTurnInput(bt.Events(), bt.Turn(), captureTrainer, captureWild, trainerMove, snap, wildHP, wildMax)
		newly := session.AfterTurn(input)
		if trainerMove != "" {
			usedMoves[trainerMove] = true
		}
		if variant == "miss" && turn == missTurn && len(newly) > 0 {
			result.Failure = "miss action completed a capture objective"
			return result
		}
		outcome := session.OutcomeAfterTurn(wildHP == 0)
		if outcome != "" {
			result.Outcome = outcome
			break
		}
	}
	result.Gauge = session.Gauge
	result.Events = bt.Events()
	if result.Outcome == "" && bt.State() == battle.StateOver {
		wildHP, _ := captureWildHP(bt.Snapshot(captureTrainer))
		result.Outcome = session.OutcomeAfterTurn(wildHP == 0)
	}
	if variant == "overaggressive" {
		result.Passed = result.Outcome == "hunt_failed"
		if !result.Passed {
			result.Failure = "overaggressive line did not faint the target before capture"
		}
		return result
	}
	if variant == "miss" {
		result.Passed = trainerMisses(bt.Events()) == 1 && result.Outcome == "captured" && !hasCritical(bt.Events())
		if !result.Passed {
			result.Failure = "miss line did not recover into capture after exactly one trainer miss"
		}
		return result
	}
	result.Passed = result.Outcome == "captured" && !hasCritical(bt.Events())
	if !result.Passed {
		result.Failure = "objective-seeking line did not capture without a critical hit"
	}
	return result
}

func capturePartyIdentity(party battle.Party) []CapturePartyMember {
	identities := make([]CapturePartyMember, len(party.Members))
	for i, member := range party.Members {
		identities[i] = CapturePartyMember{ID: member.Monster.ID, Species: member.Monster.Species, Level: member.Monster.Level, Loadout: append([]string(nil), member.Monster.BattleLoadout...), MaxHP: member.MaxHP, Stats: member.Stats}
	}
	return identities
}

func captureFighters(party battle.Party) []capture.PartyFighter {
	fighters := make([]capture.PartyFighter, 0, len(party.Members))
	for _, member := range party.Members {
		fighters = append(fighters, capture.PartyFighter{Species: member.Monster.Species, Level: member.Monster.Level, Loadout: member.Monster.BattleLoadout})
	}
	return fighters
}

func captureWildParty(set *content.Set, fighters []capture.PartyFighter, target string) (battle.Party, error) {
	spec, ok := set.Species[target]
	if !ok {
		return battle.Party{}, fmt.Errorf("unknown target %q", target)
	}
	loadout, err := dojo.WildLoadout(set, target)
	if err != nil {
		return battle.Party{}, err
	}
	level := capture.WildLevel(fighters)
	return battle.Party{Trainer: captureWild, ClampOutgoingDamage: true, Members: []battle.PartyMember{{Monster: game.Monster{ID: captureWild + "-monster", Species: target, Level: level, BattleLoadout: loadout}, MaxHP: capture.TargetHP(set, fighters, spec, level)}}}, nil
}

func advanceCaptureReveal(bt *battle.Battle) error {
	for bt.State() == battle.StateRevealing || bt.State() == battle.StateAwaitingReplacement {
		if bt.State() == battle.StateRevealing {
			if err := bt.AdvanceReveal(); err != nil {
				return err
			}
			continue
		}
		snap := bt.Snapshot(captureTrainer)
		reserves := snap.HealthyReserves()
		if len(reserves) == 0 {
			return errors.New("capture matrix: no replacement available")
		}
		if err := bt.Replace(captureTrainer, reserves[0].ID); err != nil {
			return err
		}
	}
	return nil
}

func captureAction(set *content.Set, snap battle.Snapshot, session *capture.Session, used map[string]bool, variant string, turn int, canMiss bool) (battle.Action, string) {
	loadout := snap.YourPartyActiveLoadout()
	if len(loadout) == 0 {
		return battle.Action{}, ""
	}
	if variant == "overaggressive" {
		best := loadout[0]
		for _, slug := range loadout[1:] {
			if set.Moves[slug].Power > set.Moves[best].Power {
				best = slug
			}
		}
		return battle.Action{Kind: battle.ActionMove, Move: best}, best
	}
	if variant == "miss" && turn == 0 {
		for _, slug := range weakestMoves(set, loadout) {
			if set.Moves[slug].Accuracy < 100 {
				return battle.Action{Kind: battle.ActionMove, Move: slug}, slug
			}
		}
	}
	// The first resolved turn can award Hold the Line. Defer the deliberate
	// miss until the second turn so it cannot be mistaken for an objective.
	if variant == "miss" && canMiss && turn > 0 {
		for _, slug := range loadout {
			if used[slug] && set.Moves[slug].Accuracy < 100 {
				return battle.Action{Kind: battle.ActionMove, Move: slug}, slug
			}
		}
	}
	if pendingObjective(session, capture.SafeSwitch) {
		reserves := snap.HealthyReserves()
		if len(reserves) > 0 {
			return battle.Action{Kind: battle.ActionSwitch, SwitchTo: reserves[0].ID}, ""
		}
	}
	if pendingObjective(session, capture.ReadTheMatchup) && len(snap.FoeRoster) > 0 {
		foeType := set.Species[snap.FoeRoster[0].Species].Type
		for _, slug := range weakestMoves(set, loadout) {
			if set.Effectiveness(set.Moves[slug].Type, foeType) >= battle.SuperEffectiveAt {
				return battle.Action{Kind: battle.ActionMove, Move: slug}, slug
			}
		}
		for _, reserve := range snap.HealthyReserves() {
			for _, slug := range reserve.Loadout {
				if set.Effectiveness(set.Moves[slug].Type, foeType) >= battle.SuperEffectiveAt {
					return battle.Action{Kind: battle.ActionSwitch, SwitchTo: reserve.ID}, ""
				}
			}
		}
	}
	if pendingObjective(session, capture.ShowMoveVariety) {
		for _, slug := range weakestMoves(set, loadout) {
			if !used[slug] {
				return battle.Action{Kind: battle.ActionMove, Move: slug}, slug
			}
		}
	}
	move := weakestMoves(set, loadout)[0]
	return battle.Action{Kind: battle.ActionMove, Move: move}, move
}

func pendingObjective(session *capture.Session, id capture.ObjectiveID) bool {
	completed, present := session.Completed[id]
	return present && !completed
}

func weakestMoves(set *content.Set, loadout []string) []string {
	moves := append([]string(nil), loadout...)
	sort.SliceStable(moves, func(i, j int) bool { return set.Moves[moves[i]].Power < set.Moves[moves[j]].Power })
	return moves
}

func captureWildHP(snap battle.Snapshot) (int, int) {
	for _, foe := range snap.FoeRoster {
		if foe.Active {
			return foe.HP, foe.MaxHP
		}
	}
	return 0, 0
}

func trainerMisses(events []battle.Event) int {
	misses := 0
	for _, event := range events {
		if event.Kind == battle.EventMissed && event.Actor == captureTrainer {
			misses++
		}
	}
	return misses
}

func hasCritical(events []battle.Event) bool {
	for _, event := range events {
		if event.Kind == battle.EventCriticalHit {
			return true
		}
	}
	return false
}

type captureRand struct {
	rolls     []float64
	position  int
	exhausted bool
}

func (r *captureRand) Float64() float64 {
	if r.position >= len(r.rolls) {
		r.exhausted = true
		return 0
	}
	roll := r.rolls[r.position]
	r.position++
	return roll
}

func (r *captureRand) Reset(rolls []float64) {
	r.rolls = rolls
	r.position = 0
	r.exhausted = false
}

func captureTurnRolls(trainer, wild battle.Action, trainerSpeed, wildSpeed int, variant string, turn, missTurn int) []float64 {
	rolls := []float64{}
	if trainer.Kind == battle.ActionMove && wild.Kind == battle.ActionMove && trainerSpeed == wildSpeed {
		rolls = append(rolls, 0)
	}
	type plannedAction struct {
		action  battle.Action
		trainer bool
	}
	order := []plannedAction{{action: trainer, trainer: true}, {action: wild}}
	if trainer.Kind == battle.ActionMove && wild.Kind == battle.ActionMove && wildSpeed > trainerSpeed {
		order = []plannedAction{{action: wild}, {action: trainer, trainer: true}}
	}
	for _, planned := range order {
		if planned.action.Kind != battle.ActionMove {
			continue
		}
		miss := variant == "miss" && turn == missTurn && planned.trainer
		if miss {
			rolls = append(rolls, 0.999999)
			continue
		}
		variance := 0.0
		if variant == "max_variance" || variant == "overaggressive" {
			variance = math.Nextafter(1, 0)
		}
		rolls = append(rolls, 0, 0.5, variance)
	}
	return rolls
}

func activeSpeed(view battle.PolicyView) int {
	for _, member := range view.Self {
		if member.Active {
			return member.Spe
		}
	}
	return 0
}
