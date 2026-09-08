package balance

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
)

// ActionEntry records one resolved action for replay artifacts.
type ActionEntry struct {
	Turn     int    `json:"turn"`
	Trainer  string `json:"trainer"`
	Kind     string `json:"kind"`
	Move     string `json:"move,omitempty"`
	SwitchTo string `json:"switch_to,omitempty"`
}

// BattleOutcome is one completed scenario run.
type BattleOutcome struct {
	Scenario          string              `json:"scenario"`
	Kind              string              `json:"kind"`
	Stage             string              `json:"stage,omitempty"`
	LoadoutVariant    string              `json:"loadout_variant,omitempty"`
	Seed              uint64              `json:"seed"`
	Winner            string              `json:"winner"`
	Reason            battle.EndReason    `json:"reason"`
	Turns             int                 `json:"turns"`
	EngineSideA       bool                `json:"engine_side_a"`
	PartyOrderSwapped bool                `json:"party_order_swapped"`
	TeamA             ReferenceTeam       `json:"team_a"`
	TeamB             ReferenceTeam       `json:"team_b"`
	SideA             battle.Party        `json:"-"`
	SideB             battle.Party        `json:"-"`
	Loadouts          map[string][]string `json:"loadouts,omitempty"`
	ActionLog         []ActionEntry       `json:"action_log,omitempty"`
	EventKinds        []battle.EventKind  `json:"event_kinds,omitempty"`
	LandedHits        int                 `json:"landed_hits"`
	FaintPaces        []FaintPace         `json:"faint_paces,omitempty"`
	IllegalActions    int                 `json:"illegal_actions"`
	events            []battle.Event
	stages            map[string]int
}

// FaintPace records all landed hits, the killing critical, and whether any
// contributing hit was super-effective or came from a different stage.
type FaintPace struct {
	Hits           int  `json:"hits"`
	Critical       bool `json:"critical,omitempty"`
	SuperEffective bool `json:"super_effective,omitempty"`
	StageMismatch  bool `json:"stage_mismatch,omitempty"`
}

// Simulate runs one battle to completion using Dojo public-state policies.
func Simulate(set *content.Set, a, b battle.Party, seed uint64, policy dojo.PolicyConfig, maxTurns int) (*BattleOutcome, error) {
	if maxTurns < 1 {
		maxTurns = DefaultMaxTurns
	}
	bt, err := battle.New(set, a, b, battle.Seeded(seed))
	if err != nil {
		return nil, fmt.Errorf("balance: create battle: %w", err)
	}
	out := &BattleOutcome{
		Seed: seed, SideA: a, SideB: b,
		Loadouts: mergeLoadouts(a, b),
		stages:   make(map[string]int),
	}
	for _, party := range []battle.Party{a, b} {
		for _, member := range party.Members {
			out.stages[member.Monster.ID] = dojo.StageIndex(set, member.Monster.Species)
		}
	}
	transitions := 0
	for bt.State() != battle.StateOver {
		beforeState, beforeTurn := bt.State(), bt.Turn()
		switch bt.State() {
		case battle.StateRevealing:
			if err := bt.AdvanceReveal(); err != nil {
				return finishOutcome(bt, out), fmt.Errorf("balance: advance reveal: %w", err)
			}
		case battle.StateAwaitingReplacement:
			for _, trainer := range []string{a.Trainer, b.Trainer} {
				snap := bt.Snapshot(trainer)
				if !snap.ReplacementRequired {
					continue
				}
				view, ok := bt.PolicyViewFor(trainer)
				if !ok {
					return finishOutcome(bt, out), fmt.Errorf("balance: policy view for replacement %q: unavailable", trainer)
				}
				rng := policyRNG(seed, bt.Turn(), trainer+"-replace")
				id, _, err := dojo.ChooseReplacement(set, view, policy, rng)
				if err != nil {
					return finishOutcome(bt, out), fmt.Errorf("balance: replacement policy for %q: %w", trainer, err)
				}
				act := battle.Action{Kind: battle.ActionSwitch, SwitchTo: id}
				if err := bt.Replace(trainer, id); err != nil {
					return finishOutcome(bt, out), fmt.Errorf("balance: replace %q with %q: %w", trainer, id, err)
				}
				out.ActionLog = append(out.ActionLog, actionEntry(bt.Turn(), trainer, act))
			}
		case battle.StateAwaitingActions:
			if err := selectPolicyActions(set, bt, out, policy); err != nil {
				return finishOutcome(bt, out), err
			}
		default:
			if bt.State() == battle.StateOver {
				break
			}
			return finishOutcome(bt, out), fmt.Errorf("balance: unexpected battle state %q", bt.State())
		}
		if bt.Turn() >= maxTurns && bt.State() != battle.StateOver {
			return finishOutcome(bt, out), fmt.Errorf("balance: max turns %d reached", maxTurns)
		}
		if bt.State() == beforeState && bt.Turn() == beforeTurn {
			transitions++
			if transitions >= 4 {
				return finishOutcome(bt, out), fmt.Errorf("balance: nonadvancing state %q at turn %d", bt.State(), bt.Turn())
			}
		} else {
			transitions = 0
		}
	}
	return finishOutcome(bt, out), nil
}

func finishOutcome(bt *battle.Battle, out *BattleOutcome) *BattleOutcome {
	events := bt.Events()
	out.events = events
	out.EventKinds = eventKinds(events)
	out.LandedHits = countLandedHits(events)
	out.FaintPaces = collectFaintPaces(events, out.stages)
	out.Winner = bt.Winner()
	out.Reason = bt.Reason()
	out.Turns = bt.Turn()
	return out
}

type streamedOutcome struct {
	Scenario string            `json:"scenario"`
	Seed     uint64            `json:"seed"`
	Stage    string            `json:"stage"`
	Policy   dojo.PolicyConfig `json:"policy"`
	Outcome  *BattleOutcome    `json:"outcome"`
	SideA    battle.Party      `json:"side_a"`
	SideB    battle.Party      `json:"side_b"`
	Events   []battle.Event    `json:"events"`
}

func writeOutcome(w io.Writer, out *BattleOutcome, policy dojo.PolicyConfig) error {
	if w == nil || out == nil {
		return nil
	}
	return json.NewEncoder(w).Encode(streamedOutcome{Scenario: out.Scenario, Seed: out.Seed, Stage: out.Stage, Policy: policy, Outcome: out, SideA: out.SideA, SideB: out.SideB, Events: out.events})
}

func policyRNG(seed uint64, turn int, tag string) battle.Rand {
	mix := seed ^ 0x517cc1b727220a95
	if turn > 0 {
		mix ^= uint64(turn) * 0x9e3779b97f4a7c15 //nolint:gosec // turn is bounded by DefaultMaxTurns
	}
	for _, b := range []byte(tag) {
		mix = mix*0x100000001b3 ^ uint64(b)
	}
	return battle.Seeded(mix)
}

func actionEntry(turn int, trainer string, act battle.Action) ActionEntry {
	return ActionEntry{
		Turn: turn, Trainer: trainer, Kind: string(act.Kind),
		Move: act.Move, SwitchTo: act.SwitchTo,
	}
}

func mergeLoadouts(a, b battle.Party) map[string][]string {
	out := partyLoadouts(a)
	maps.Copy(out, partyLoadouts(b))
	return out
}

func eventKinds(events []battle.Event) []battle.EventKind {
	out := make([]battle.EventKind, len(events))
	for i, e := range events {
		out[i] = e.Kind
	}
	return out
}

func countLandedHits(events []battle.Event) int {
	n := 0
	for _, e := range events {
		if e.Kind == battle.EventDamageDealt {
			n++
		}
	}
	return n
}

// CollectFaintPaces records landed hits against each Monster until it faints.
func CollectFaintPaces(events []battle.Event) []FaintPace {
	return collectFaintPaces(events, nil)
}

func collectFaintPaces(events []battle.Event, stages map[string]int) []FaintPace {
	hits := map[string]int{}
	killingCrit := map[string]bool{}
	killingSE := map[string]bool{}
	mismatched := map[string]bool{}
	attackerID := ""
	moveCrit, moveSE := false, false
	var out []FaintPace
	for _, e := range events {
		switch e.Kind {
		case battle.EventMoveUsed:
			moveCrit, moveSE = false, false
			attackerID = e.MonsterID
		case battle.EventCriticalHit:
			moveCrit = true
		case battle.EventSuperEffective:
			moveSE = true
		case battle.EventDamageDealt:
			id := e.TargetID
			if id == "" {
				continue
			}
			hits[id]++
			killingCrit[id] = moveCrit
			killingSE[id] = killingSE[id] || moveSE
			if stages != nil {
				mismatched[id] = mismatched[id] || stages[attackerID] != stages[id]
			}
		case battle.EventFainted:
			id := e.MonsterID
			if id == "" {
				continue
			}
			out = append(out, FaintPace{
				Hits:           hits[id],
				Critical:       killingCrit[id],
				SuperEffective: killingSE[id],
				StageMismatch:  mismatched[id],
			})
		}
	}
	return out
}

func selectPolicyActions(set *content.Set, bt *battle.Battle, out *BattleOutcome, policy dojo.PolicyConfig) error {
	for _, trainer := range []string{out.SideA.Trainer, out.SideB.Trainer} {
		if bt.Locked(trainer) {
			continue
		}
		view, ok := bt.PolicyViewFor(trainer)
		if !ok {
			return fmt.Errorf("balance: policy view for %q: unavailable", trainer)
		}
		rng := policyRNG(out.Seed, bt.Turn(), trainer)
		act, _, err := dojo.ChoosePolicyAction(set, view, policy, rng)
		if err != nil {
			return fmt.Errorf("balance: action policy for %q: %w", trainer, err)
		}
		turn := bt.Turn() + 1
		if err := bt.Select(trainer, act); err != nil {
			return fmt.Errorf("balance: select %q: %w", trainer, err)
		}
		out.ActionLog = append(out.ActionLog, actionEntry(turn, trainer, act))
	}
	return nil
}
