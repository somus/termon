package balance

import (
	"fmt"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
	"termon.sh/internal/game"
)

const (
	sideAName = "balance:a"
	sideBName = "balance:b"
)

// SpeciesAtLevel resolves a Family root to the Species required at level.
func SpeciesAtLevel(set *content.Set, familyBase string, level int) (string, error) {
	base, ok := set.Species[familyBase]
	if !ok {
		return "", fmt.Errorf("balance: unknown family %q", familyBase)
	}
	species := familyBase
	current := base
	for current.EvolvesTo != nil && current.EvolvesTo.Level <= level {
		next, ok := set.Species[current.EvolvesTo.Species]
		if !ok {
			return "", fmt.Errorf("balance: unknown evolution %q", current.EvolvesTo.Species)
		}
		species = current.EvolvesTo.Species
		current = next
	}
	return species, nil
}

// BuildNaturalParty builds a three-Monster natural party with lead rotation.
func BuildNaturalParty(set *content.Set, team ReferenceTeam, leadIdx int, level int, trainer string, swapReserves bool) (battle.Party, error) {
	members, err := buildMembers(set, team, leadIdx, level, trainer, false, swapReserves)
	if err != nil {
		return battle.Party{}, err
	}
	return battle.Party{Trainer: trainer, Members: members}, nil
}

// BuildNormalizedParty builds a queue-normalized three-Monster party.
func BuildNormalizedParty(set *content.Set, team ReferenceTeam, leadIdx int, trainer string, swapReserves bool) (battle.Party, error) {
	members, err := buildMembers(set, team, leadIdx, game.QueueLevel, trainer, true, swapReserves)
	if err != nil {
		return battle.Party{}, err
	}
	return battle.Party{Trainer: trainer, Members: members}, nil
}

func buildMembers(set *content.Set, team ReferenceTeam, leadIdx int, level int, trainer string, normalized bool, swapReserves bool) ([]battle.PartyMember, error) {
	if leadIdx < 0 || leadIdx > 2 {
		return nil, fmt.Errorf("balance: lead index %d out of range", leadIdx)
	}
	families := []string{team.Families[0], team.Families[1], team.Families[2]}
	rotated := make([]string, 3)
	for i := range 3 {
		rotated[i] = families[(leadIdx+i)%3]
	}
	out := make([]battle.PartyMember, 3)
	for i, family := range rotated {
		species, err := SpeciesAtLevel(set, family, level)
		if err != nil {
			return nil, err
		}
		id := fmt.Sprintf("%s-%d", trainer, i)
		mon := game.Monster{ID: id, Species: species, Level: level}
		pm := battle.PartyMember{Monster: mon}
		if normalized {
			queueMoves, err := game.DefaultQueueMoveSet(set, mon)
			if err != nil {
				return nil, err
			}
			nm, err := game.NormalizedMonster(set, mon, queueMoves)
			if err != nil {
				return nil, err
			}
			stats := game.QueueStats(set.Species[species])
			pm.Monster = nm
			pm.Stats = &stats
		} else {
			loadout, err := dojo.ReferenceLoadout(set, species, level)
			if err != nil {
				return nil, err
			}
			pm.Monster.BattleLoadout = loadout
		}
		out[i] = pm
	}
	if swapReserves {
		out[1], out[2] = out[2], out[1]
	}
	return out, nil
}

func partyLoadouts(p battle.Party) map[string][]string {
	out := make(map[string][]string, len(p.Members))
	for _, m := range p.Members {
		out[m.Monster.ID] = append([]string(nil), m.Monster.BattleLoadout...)
	}
	return out
}
