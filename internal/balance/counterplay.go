package balance

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
	"termon.sh/internal/game"
)

const counterplayTurnCap = 200

// CounterplayMatrix compares staying with each legal first-turn Switch from a
// Type-disadvantaged lead. All later choices use the named public reference
// policy, so the only intervention is the opening choice.
type CounterplayMatrix struct {
	ReferencePolicyRevision string            `json:"reference_policy_revision"`
	Cases                   []CounterplayCase `json:"cases"`
	Passed                  bool              `json:"passed"`
}

// CounterplayCase records one Family's deterministic normalized fixture.
type CounterplayCase struct {
	Family      string              `json:"family"`
	FoeTeam     string              `json:"foe_team"`
	PlayerParty battle.Party        `json:"player_party"`
	FoeParty    battle.Party        `json:"foe_party"`
	Policies    []CounterplayPolicy `json:"policies"`
	Failure     string              `json:"failure,omitempty"`
}

// CounterplayPolicy aggregates paired physical-side runs for one policy.
type CounterplayPolicy struct {
	Policy   string              `json:"policy"`
	Stay     CounterplayRecord   `json:"stay"`
	Switches []CounterplayRecord `json:"switches"`
	Best     string              `json:"best_switch,omitempty"`
	Passed   bool                `json:"passed"`
	Failure  string              `json:"failure,omitempty"`
}

// CounterplayRecord is one opening alternative's completed denominator and a
// single representative replay. Wins belong to the player Party.
type CounterplayRecord struct {
	SwitchTo    string            `json:"switch_to,omitempty"`
	Wins        int               `json:"wins"`
	Count       int               `json:"count"`
	Failed      int               `json:"failed"`
	Rate        float64           `json:"rate"`
	Improvement float64           `json:"improvement"`
	Sample      *CounterplayTrace `json:"sample,omitempty"`
	Failure     *CounterplayTrace `json:"failure,omitempty"`
}

// CounterplayTrace is a bounded replay artifact for one opening line.
type CounterplayTrace struct {
	Scenario    string         `json:"scenario"`
	Seed        uint64         `json:"seed"`
	EngineSideA bool           `json:"engine_side_a"`
	Winner      string         `json:"winner,omitempty"`
	Turns       int            `json:"turns"`
	Error       string         `json:"error,omitempty"`
	Actions     []ActionEntry  `json:"actions,omitempty"`
	Events      []battle.Event `json:"events,omitempty"`
}

// RunCounterplay evaluates all 24 Family fixtures using the supplied paired
// seed corpus. A missing/invalid scenario or capped run is excluded from its
// denominator and makes the corresponding policy result fail.
func RunCounterplay(set *content.Set, seeds []uint64) (CounterplayMatrix, error) {
	matrix := CounterplayMatrix{Passed: true, ReferencePolicyRevision: dojo.ReferencePolicyRevision}
	if set == nil {
		return matrix, errors.New("counterplay: nil content")
	}
	if len(seeds) == 0 {
		return matrix, errors.New("counterplay: empty seed corpus")
	}
	cases, err := buildCounterplayCases(set)
	if err != nil {
		return matrix, err
	}
	for _, c := range cases {
		result := CounterplayCase{Family: c.family, FoeTeam: c.foeTeam, PlayerParty: c.player, FoeParty: c.foe}
		for _, policy := range []string{dojo.ReferencePressure, dojo.ReferencePivot, dojo.ReferencePreservation} {
			entry := runCounterplayPolicy(set, c, policy, seeds)
			if !entry.Passed {
				matrix.Passed = false
			}
			result.Policies = append(result.Policies, entry)
		}
		matrix.Cases = append(matrix.Cases, result)
	}
	return matrix, nil
}

type counterplayFixture struct {
	family, foeTeam string
	player, foe     battle.Party
}

func buildCounterplayCases(set *content.Set) ([]counterplayFixture, error) {
	families := anchorFamilies()
	var fixtures []counterplayFixture
	for _, family := range families {
		leadSpecies, err := dojo.SpeciesAtStage(set, family, 1)
		if err != nil {
			return nil, err
		}
		leadType := set.Species[leadSpecies].Type
		foeTeam, foeFamily, ok := counterplayFoeTeam(set, leadType)
		if !ok {
			return nil, fmt.Errorf("counterplay: %s has no anchor Type-disadvantage foe", family)
		}
		reserves, ok := counterplayReserves(set, families, family, foeFamily)
		if !ok {
			return nil, fmt.Errorf("counterplay: %s has no two legal reserves", family)
		}
		player, err := counterplayParty(set, "counterplay:player:"+family, []string{family, reserves[0], reserves[1]})
		if err != nil {
			return nil, err
		}
		foeFamilies := counterplayFoeOrder(foeTeam, foeFamily)
		foe, err := counterplayParty(set, "counterplay:foe:"+family, foeFamilies)
		if err != nil {
			return nil, err
		}
		fixtures = append(fixtures, counterplayFixture{family: family, foeTeam: foeTeam.Name, player: player, foe: foe})
	}
	return fixtures, nil
}

func counterplayFoeOrder(team ReferenceTeam, lead string) []string {
	out := []string{lead}
	for _, family := range team.Families {
		if family != lead {
			out = append(out, family)
		}
	}
	return out
}

func anchorFamilies() []string {
	seen := map[string]bool{}
	var out []string
	for _, team := range ReferenceTeams {
		for _, family := range team.Families {
			if !seen[family] {
				seen[family] = true
				out = append(out, family)
			}
		}
	}
	slices.Sort(out)
	return out
}

func counterplayFoeTeam(set *content.Set, leadType string) (ReferenceTeam, string, bool) {
	teams := append([]ReferenceTeam(nil), ReferenceTeams...)
	slices.SortFunc(teams, func(a, b ReferenceTeam) int { return strings.Compare(a.Name, b.Name) })
	for _, team := range teams {
		families := append([]string(nil), team.Families[:]...)
		slices.Sort(families)
		for _, family := range families {
			species, err := dojo.SpeciesAtStage(set, family, 1)
			if err == nil && set.Effectiveness(set.Species[species].Type, leadType) >= battle.SuperEffectiveAt {
				return team, family, true
			}
		}
	}
	return ReferenceTeam{}, "", false
}

func counterplayReserves(set *content.Set, families []string, lead, foe string) ([2]string, bool) {
	var out [2]string
	foeSpecies, err := dojo.SpeciesAtStage(set, foe, 1)
	if err != nil {
		return out, false
	}
	foeType := set.Species[foeSpecies].Type
	var favorable, safe []string
	for _, family := range families {
		if family == lead || family == foe {
			continue
		}
		species, err := dojo.SpeciesAtStage(set, family, 1)
		if err != nil || set.Effectiveness(foeType, set.Species[species].Type) >= battle.SuperEffectiveAt {
			continue
		}
		reserveType := set.Species[species].Type
		if set.Effectiveness(reserveType, foeType) >= battle.SuperEffectiveAt {
			favorable = append(favorable, family)
		} else {
			safe = append(safe, family)
		}
	}
	if len(favorable) == 0 {
		return out, false
	}
	out[0] = favorable[0]
	for _, family := range append(favorable[1:], safe...) {
		if family != out[0] {
			out[1] = family
			return out, true
		}
	}
	return out, false
}

func counterplayParty(set *content.Set, trainer string, families []string) (battle.Party, error) {
	party := battle.Party{Trainer: trainer}
	for i, family := range families {
		species, err := dojo.SpeciesAtStage(set, family, 1)
		if err != nil {
			return battle.Party{}, err
		}
		loadout, err := dojo.ReferenceLoadout(set, species, game.QueueLevel)
		if err != nil {
			return battle.Party{}, err
		}
		monster, err := game.NormalizedMonster(set, game.Monster{ID: fmt.Sprintf("%s-%d", trainer, i), Species: species, Level: game.QueueLevel}, loadout)
		if err != nil {
			return battle.Party{}, err
		}
		stats := game.QueueStats(set.Species[species])
		party.Members = append(party.Members, battle.PartyMember{Monster: monster, Stats: &stats})
	}
	return party, nil
}

func runCounterplayPolicy(set *content.Set, fixture counterplayFixture, policy string, seeds []uint64) CounterplayPolicy {
	result := CounterplayPolicy{Policy: policy}
	for _, seed := range seeds {
		for _, sideA := range []bool{true, false} {
			stay, err := counterplayStayAction(set, fixture.player, fixture.foe, seed, sideA)
			if err != nil {
				result.Stay.Failed++
				counterplayRememberFailure(&result.Stay, err, seed, sideA)
				continue
			}
			trace, won, err := runCounterplayBattle(set, fixture, policy, seed, sideA, stay)
			counterplayRecord(&result.Stay, trace, won, err)
		}
	}
	for _, reserve := range fixture.player.Members[1:] {
		record := CounterplayRecord{SwitchTo: reserve.Monster.ID}
		forced := battle.Action{Kind: battle.ActionSwitch, SwitchTo: reserve.Monster.ID}
		for _, seed := range seeds {
			for _, sideA := range []bool{true, false} {
				trace, won, err := runCounterplayBattle(set, fixture, policy, seed, sideA, forced)
				counterplayRecord(&record, trace, won, err)
			}
		}
		if record.Count > 0 {
			record.Rate = float64(record.Wins) / float64(record.Count)
		}
		result.Switches = append(result.Switches, record)
	}
	if result.Stay.Count > 0 {
		result.Stay.Rate = float64(result.Stay.Wins) / float64(result.Stay.Count)
	}
	best := -1.0
	for i := range result.Switches {
		record := &result.Switches[i]
		record.Improvement = record.Rate - result.Stay.Rate
		if record.Count > 0 && record.Improvement > best {
			best, result.Best = record.Improvement, record.SwitchTo
		}
	}
	result.Passed = result.Stay.Count > 0 && result.Stay.Failed == 0 && best >= .10
	for _, record := range result.Switches {
		result.Passed = result.Passed && record.Count > 0 && record.Failed == 0
	}
	if !result.Passed {
		result.Failure = fmt.Sprintf("best switch improvement %.1fpp, want >=10.0pp", best*100)
	}
	return result
}

func counterplayStayAction(set *content.Set, player, foe battle.Party, seed uint64, playerSideA bool) (battle.Action, error) {
	left, right := foe, player
	if playerSideA {
		left, right = player, foe
	}
	bt, err := battle.New(set, left, right, battle.Seeded(seed))
	if err != nil {
		return battle.Action{}, err
	}
	view, ok := bt.PolicyViewFor(player.Trainer)
	if !ok {
		return battle.Action{}, errors.New("counterplay: player policy view unavailable")
	}
	act, _, err := dojo.ChooseReferenceAction(set, view, dojo.ReferencePressure, policyRNG(seed, 0, player.Trainer+"-pressure"))
	if err != nil {
		return battle.Action{}, err
	}
	if act.Kind != battle.ActionMove {
		return battle.Action{}, errors.New("counterplay: Pressure did not select a Move")
	}
	return act, nil
}

func counterplayRecord(record *CounterplayRecord, trace CounterplayTrace, won bool, err error) {
	if err != nil {
		record.Failed++
		trace.Error = err.Error()
		if record.Failure == nil {
			record.Failure = &trace
		}
		return
	}
	record.Count++
	if won {
		record.Wins++
	}
	if record.Sample == nil {
		record.Sample = &trace
	}
}

func counterplayRememberFailure(record *CounterplayRecord, err error, seed uint64, sideA bool) {
	if record.Failure == nil {
		record.Failure = &CounterplayTrace{Seed: seed, EngineSideA: sideA, Error: err.Error()}
	}
}

func runCounterplayBattle(set *content.Set, fixture counterplayFixture, policy string, seed uint64, playerSideA bool, opening battle.Action) (CounterplayTrace, bool, error) {
	left, right := fixture.foe, fixture.player
	if playerSideA {
		left, right = fixture.player, fixture.foe
	}
	trace := CounterplayTrace{Seed: seed, EngineSideA: playerSideA, Scenario: fmt.Sprintf("counterplay/%s/%s/%s", fixture.family, policy, opening.Kind)}
	bt, err := battle.New(set, left, right, battle.Seeded(seed))
	if err != nil {
		return trace, false, err
	}
	firstTurn := true
	transitions := 0
	for bt.State() != battle.StateOver {
		beforeState, beforeTurn := bt.State(), bt.Turn()
		switch bt.State() {
		case battle.StateRevealing:
			if err := bt.AdvanceReveal(); err != nil {
				return counterplayTrace(bt, trace), false, err
			}
		case battle.StateAwaitingReplacement:
			for _, trainer := range []string{fixture.player.Trainer, fixture.foe.Trainer} {
				snap := bt.Snapshot(trainer)
				if !snap.ReplacementRequired {
					continue
				}
				view, ok := bt.PolicyViewFor(trainer)
				if !ok {
					return counterplayTrace(bt, trace), false, fmt.Errorf("counterplay: replacement view for %q unavailable", trainer)
				}
				id, _, err := dojo.ChooseReplacement(set, view, dojo.TierConfig(dojo.TierRival), policyRNG(seed, bt.Turn(), trainer+"-replace"))
				if err != nil {
					return counterplayTrace(bt, trace), false, err
				}
				action := battle.Action{Kind: battle.ActionSwitch, SwitchTo: id}
				if err := bt.Replace(trainer, id); err != nil {
					return counterplayTrace(bt, trace), false, err
				}
				trace.Actions = append(trace.Actions, actionEntry(bt.Turn(), trainer, action))
			}
		case battle.StateAwaitingActions:
			actionTurn := bt.Turn() + 1
			for _, trainer := range []string{fixture.player.Trainer, fixture.foe.Trainer} {
				if bt.Locked(trainer) {
					continue
				}
				var action battle.Action
				if trainer == fixture.player.Trainer && firstTurn {
					action = opening
				} else {
					var err error
					action, err = counterplayReferenceAction(set, bt, trainer, policy, seed)
					if err != nil {
						return counterplayTrace(bt, trace), false, err
					}
				}
				if err := bt.Select(trainer, action); err != nil {
					return counterplayTrace(bt, trace), false, err
				}
				trace.Actions = append(trace.Actions, actionEntry(actionTurn, trainer, action))
			}
			firstTurn = false
		default:
			return counterplayTrace(bt, trace), false, fmt.Errorf("counterplay: unexpected state %q", bt.State())
		}
		if bt.Turn() >= counterplayTurnCap && bt.State() != battle.StateOver {
			return counterplayTrace(bt, trace), false, fmt.Errorf("counterplay: turn cap %d reached", counterplayTurnCap)
		}
		if bt.State() == beforeState && bt.Turn() == beforeTurn {
			transitions++
			if transitions >= 4 {
				return counterplayTrace(bt, trace), false, fmt.Errorf("counterplay: nonadvancing %s", bt.State())
			}
		} else {
			transitions = 0
		}
	}
	trace = counterplayTrace(bt, trace)
	return trace, trace.Winner == fixture.player.Trainer, nil
}

func counterplayReferenceAction(set *content.Set, bt *battle.Battle, trainer, policy string, seed uint64) (battle.Action, error) {
	view, ok := bt.PolicyViewFor(trainer)
	if !ok {
		return battle.Action{}, fmt.Errorf("counterplay: policy view for %q unavailable", trainer)
	}
	action, _, err := dojo.ChooseReferenceAction(set, view, policy, policyRNG(seed, bt.Turn(), trainer))
	return action, err
}

func counterplayTrace(bt *battle.Battle, trace CounterplayTrace) CounterplayTrace {
	trace.Winner = bt.Winner()
	trace.Turns = bt.Turn()
	trace.Events = bt.Events()
	return trace
}
