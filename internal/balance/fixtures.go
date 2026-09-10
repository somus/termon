package balance

import (
	"cmp"
	"fmt"
	"slices"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
	"termon.sh/internal/game"
	"termon.sh/internal/onboard"
)

// Fixture stage and loadout variants identify reproducible preparation choices.
const (
	FixtureStageReachable   = "reachable"
	FixtureStageBase        = "base"
	FixtureStageMiddle      = "middle"
	FixtureStageFinal       = "final"
	FixtureLoadoutDefault   = "default"
	FixtureLoadoutReference = "reference"
	FixtureLoadoutFrontier  = "frontier"
)

// FixtureParty builds one stage/loadout fixture through the live eligibility
// paths. Normalized fixtures always use queue stats at level 30.
func FixtureParty(set *content.Set, team ReferenceTeam, lead, level int, trainer string, swapped, normalized bool, stage, loadout string) (battle.Party, error) {
	if lead < 0 || lead > 2 {
		return battle.Party{}, fmt.Errorf("balance: lead index %d out of range", lead)
	}
	if level < 1 || level > 50 {
		return battle.Party{}, fmt.Errorf("balance: fixture level %d out of range", level)
	}
	members := make([]battle.PartyMember, 3)
	for slot := range 3 {
		family := team.Families[(lead+slot)%3]
		species, err := fixtureSpecies(set, family, level, normalized, stage)
		if err != nil {
			return battle.Party{}, err
		}
		mon := game.Monster{ID: fmt.Sprintf("%s-%d", trainer, slot), Species: species, Level: level}
		if normalized {
			mon.Level = game.QueueLevel
		}
		moves, err := fixtureLoadout(set, mon, loadout, normalized)
		if err != nil {
			return battle.Party{}, err
		}
		if normalized {
			mon, err = game.NormalizedMonster(set, mon, moves)
			if err != nil {
				return battle.Party{}, err
			}
			stats := game.QueueStats(set.Species[species])
			members[slot] = battle.PartyMember{Monster: mon, Stats: &stats}
		} else {
			mon.BattleLoadout = moves
			members[slot] = battle.PartyMember{Monster: mon}
		}
	}
	if swapped {
		members[1], members[2] = members[2], members[1]
	}
	return battle.Party{Trainer: trainer, Members: members}, nil
}

func fixtureSpecies(set *content.Set, family string, level int, normalized bool, stage string) (string, error) {
	if _, ok := set.Species[family]; !ok {
		return "", fmt.Errorf("balance: unknown family %q", family)
	}
	if stage == "" || stage == FixtureStageReachable {
		return SpeciesAtLevel(set, family, level)
	}
	chain := []string{family}
	for set.Species[chain[len(chain)-1]].EvolvesTo != nil {
		chain = append(chain, set.Species[chain[len(chain)-1]].EvolvesTo.Species)
	}
	index := map[string]int{FixtureStageBase: 0, FixtureStageMiddle: 1, FixtureStageFinal: 2}[stage]
	if stage != FixtureStageBase && stage != FixtureStageMiddle && stage != FixtureStageFinal {
		return "", fmt.Errorf("balance: unknown fixture stage %q", stage)
	}
	if index >= len(chain) {
		return "", fmt.Errorf("balance: family %q has no %s stage", family, stage)
	}
	if !normalized {
		reachable, err := SpeciesAtLevel(set, family, level)
		if err != nil {
			return "", err
		}
		reachableIndex := slices.Index(chain, reachable)
		if index > reachableIndex {
			return "", fmt.Errorf("balance: %s stage %q unreachable at level %d", stage, family, level)
		}
	}
	return chain[index], nil
}

func fixtureLoadout(set *content.Set, mon game.Monster, variant string, normalized bool) ([]string, error) {
	if variant == "" || variant == FixtureLoadoutDefault {
		if normalized {
			return game.DefaultQueueMoveSet(set, mon)
		}
		defaultMonster, err := onboard.DefaultLoadout(set, mon.Species)
		if err != nil {
			return nil, err
		}
		return defaultMonster.BattleLoadout, nil
	}
	if variant == FixtureLoadoutReference {
		level := mon.Level
		return dojo.ReferenceLoadout(set, mon.Species, level)
	}
	if variant != FixtureLoadoutFrontier {
		return nil, fmt.Errorf("balance: unknown fixture loadout %q", variant)
	}
	pool := battle.LevelLegalMovepool(set, mon.Species, mon.Level)
	stats := game.QueueStats(set.Species[mon.Species])
	if !normalized {
		sp := set.Species[mon.Species]
		stats = [5]int{game.NaturalStat(sp.BaseStats.HP, mon.Level), game.NaturalStat(sp.BaseStats.Attack, mon.Level), game.NaturalStat(sp.BaseStats.Defense, mon.Level), game.NaturalStat(sp.BaseStats.SpAttack, mon.Level), game.NaturalStat(sp.BaseStats.Speed, mon.Level)}
	}
	type scored struct {
		slug             string
		damage, accuracy float64
		typ, category    string
	}
	choices := []scored{}
	for _, slug := range pool {
		mv := set.Moves[slug]
		attack := stats[1]
		if mv.Category == "special" {
			attack = stats[3]
		}
		choices = append(choices, scored{slug, battle.ExpectedDamage(battle.DamageBase(game.MovePower(mv.Power, mon.Level), attack, 100, mv.Type, set.Species[mon.Species].Type, 1), mv.Accuracy), mv.Accuracy, mv.Type, mv.Category})
	}
	frontier := []scored{}
	for i, candidate := range choices {
		dominated := false
		for j, other := range choices {
			if i != j && candidate.typ == other.typ && candidate.category == other.category && set.Moves[other.slug].Power >= set.Moves[candidate.slug].Power && other.accuracy >= candidate.accuracy && (set.Moves[other.slug].Power > set.Moves[candidate.slug].Power || other.accuracy > candidate.accuracy) {
				dominated = true
			}
		}
		if !dominated {
			frontier = append(frontier, candidate)
		}
	}
	slices.SortFunc(frontier, func(a, b scored) int {
		if a.damage != b.damage {
			if a.damage > b.damage {
				return -1
			}
			return 1
		}
		if a.accuracy != b.accuracy {
			if a.accuracy > b.accuracy {
				return -1
			}
			return 1
		}
		return cmp.Compare(set.Moves[a.slug].Order, set.Moves[b.slug].Order)
	})
	ordered := append([]scored(nil), frontier...)
	selected := make(map[string]bool, len(frontier))
	for _, choice := range frontier {
		selected[choice.slug] = true
	}
	fillers := []scored{}
	for _, choice := range choices {
		if !selected[choice.slug] {
			fillers = append(fillers, choice)
		}
	}
	slices.SortFunc(fillers, func(a, b scored) int {
		if a.damage != b.damage {
			if a.damage > b.damage {
				return -1
			}
			return 1
		}
		if a.accuracy != b.accuracy {
			if a.accuracy > b.accuracy {
				return -1
			}
			return 1
		}
		return cmp.Compare(set.Moves[a.slug].Order, set.Moves[b.slug].Order)
	})
	ordered = append(ordered, fillers...)
	if len(ordered) > 4 {
		ordered = ordered[:4]
	}
	out := make([]string, len(ordered))
	for i, choice := range ordered {
		out[i] = choice.slug
	}
	if normalized {
		if err := game.ValidateQueueMoveSet(set, mon, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}
