package balance

import (
	"fmt"
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
	HiddenInfoReads   int                 `json:"hidden_info_reads"`
}

// FaintPace is how one Monster fainted: landed hits against it, and whether
// the killing blow was a critical or super-effective.
type FaintPace struct {
	Hits           int  `json:"hits"`
	Critical       bool `json:"critical,omitempty"`
	SuperEffective bool `json:"super_effective,omitempty"`
}

// Simulate runs one battle to completion using Dojo public-state policies.
func Simulate(set *content.Set, a, b battle.Party, seed uint64, policy dojo.PolicyConfig, maxTurns int) (*BattleOutcome, error) {
	if maxTurns < 1 {
		maxTurns = DefaultMaxTurns
	}
	bt, err := battle.New(set, a, b, battle.Seeded(seed))
	if err != nil {
		return nil, err
	}
	out := &BattleOutcome{
		Seed: seed, SideA: a, SideB: b,
		Loadouts: mergeLoadouts(a, b),
	}
	for bt.State() != battle.StateOver {
		switch bt.State() {
		case battle.StateRevealing:
			if err := bt.AdvanceReveal(); err != nil {
				return nil, err
			}
		case battle.StateAwaitingReplacement:
			for _, trainer := range []string{a.Trainer, b.Trainer} {
				snap := bt.Snapshot(trainer)
				if !snap.ReplacementRequired {
					continue
				}
				view, ok := bt.PolicyViewFor(trainer)
				if !ok {
					out.IllegalActions++
					continue
				}
				rng := policyRNG(seed, bt.Turn(), trainer+"-replace")
				id, _, err := dojo.ChooseReplacement(set, view, policy, rng)
				if err != nil {
					out.IllegalActions++
					continue
				}
				act := battle.Action{Kind: battle.ActionSwitch, SwitchTo: id}
				if err := bt.Replace(trainer, id); err != nil {
					out.IllegalActions++
					continue
				}
				out.ActionLog = append(out.ActionLog, actionEntry(bt.Turn(), trainer, act))
			}
		case battle.StateAwaitingActions:
			for _, trainer := range []string{a.Trainer, b.Trainer} {
				if bt.Locked(trainer) {
					continue
				}
				view, ok := bt.PolicyViewFor(trainer)
				if !ok {
					out.IllegalActions++
					continue
				}
				rng := policyRNG(seed, bt.Turn(), trainer)
				act, _, err := dojo.ChoosePolicyAction(set, view, policy, rng)
				if err != nil {
					out.IllegalActions++
					continue
				}
				turn := bt.Turn() + 1
				if err := bt.Select(trainer, act); err != nil {
					out.IllegalActions++
					continue
				}
				out.ActionLog = append(out.ActionLog, actionEntry(turn, trainer, act))
			}
		default:
			if bt.State() == battle.StateOver {
				break
			}
			return nil, fmt.Errorf("balance: unexpected battle state %q", bt.State())
		}
		if bt.Turn() >= maxTurns && bt.State() != battle.StateOver {
			break
		}
	}
	events := bt.Events()
	out.EventKinds = eventKinds(events)
	out.LandedHits = countLandedHits(events)
	out.FaintPaces = CollectFaintPaces(events)
	out.Winner = bt.Winner()
	out.Reason = bt.Reason()
	out.Turns = bt.Turn()
	return out, nil
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
	hits := map[string]int{}
	killingCrit := map[string]bool{}
	killingSE := map[string]bool{}
	moveCrit, moveSE := false, false
	var out []FaintPace
	for _, e := range events {
		switch e.Kind {
		case battle.EventMoveUsed:
			moveCrit, moveSE = false, false
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
			killingSE[id] = moveSE
		case battle.EventFainted:
			id := e.MonsterID
			if id == "" {
				continue
			}
			out = append(out, FaintPace{
				Hits:           hits[id],
				Critical:       killingCrit[id],
				SuperEffective: killingSE[id],
			})
		}
	}
	return out
}
