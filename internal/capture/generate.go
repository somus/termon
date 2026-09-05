package capture

import (
	"fmt"
	"slices"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
)

// Generate selects three distinct eligible objective IDs for an Expedition Target Encounter.
func Generate(set *content.Set, party []PartyFighter, wildFamily string) ([]Objective, error) {
	wildSpec, ok := set.Species[wildFamily]
	if !ok {
		return nil, fmt.Errorf("capture: unknown wild family %q", wildFamily)
	}
	wildLevel := WildLevel(party)
	identity := IdentityForFamily(wildFamily)
	order := []ObjectiveID{identity, ReadTheMatchup, SafeSwitch, complement(identity)}
	var ids []ObjectiveID
	ids = append(ids, ShowMoveVariety)
	for _, id := range order {
		if len(ids) == 3 {
			break
		}
		if containsID(ids, id) {
			continue
		}
		if eligible(set, id, party, wildSpec, wildLevel) {
			ids = append(ids, id)
		}
	}
	if len(ids) != 3 {
		return nil, fmt.Errorf("capture: could not generate three objectives for %q", wildFamily)
	}
	out := make([]Objective, len(ids))
	for i, id := range ids {
		o, ok := ObjectiveByID(id)
		if !ok {
			return nil, fmt.Errorf("capture: unknown objective %q", id)
		}
		out[i] = o
	}
	return out, nil
}

func containsID(ids []ObjectiveID, id ObjectiveID) bool {
	return slices.Contains(ids, id)
}

func eligible(set *content.Set, id ObjectiveID, party []PartyFighter, wild content.Species, wildLevel int) bool {
	switch id {
	case ShowMoveVariety:
		for _, p := range party {
			slugs := map[string]bool{}
			for _, slug := range p.Loadout {
				mv, ok := set.Moves[slug]
				if ok && mv.Power > 0 {
					slugs[slug] = true
				}
			}
			if len(slugs) < 3 {
				return false
			}
		}
		return true
	case ReadTheMatchup:
		for _, p := range party {
			for _, slug := range p.Loadout {
				mv, ok := set.Moves[slug]
				if ok && set.Effectiveness(mv.Type, wild.Type) >= battle.SuperEffectiveAt {
					return true
				}
			}
		}
		return false
	case SafeSwitch:
		return len(party) >= 2
	case MeasuredPressure:
		weakMin := weakestMinHit(set, party, wild, wildLevel)
		hp := TargetHP(set, party, wild, wildLevel)
		if weakMin < 1 {
			return false
		}
		return 2*weakMin < (hp*3)/4
	case HoldTheLine:
		wmove := wildStrike(set, wild)
		for _, p := range party {
			pSpec := set.Species[p.Species]
			dmg := WildHit(set, wild, wildLevel, wmove, pSpec, p.Level)
			maxHP := gameNaturalHP(pSpec, p.Level)
			if dmg*2 < maxHP {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func gameNaturalHP(spec content.Species, level int) int {
	return max(1, spec.BaseStats.HP*(100+2*(level-1))/100)
}

func weakestMinHit(set *content.Set, party []PartyFighter, wild content.Species, wildLevel int) int {
	weakMin := int(^uint(0) >> 1)
	for _, p := range party {
		pSpec := set.Species[p.Species]
		for _, slug := range p.Loadout {
			mv, ok := set.Moves[slug]
			if !ok || mv.Power <= 0 {
				continue
			}
			mn := partyHit(set, pSpec, p.Level, mv, wild, wildLevel, varianceMin)
			if mn < weakMin {
				weakMin = mn
			}
		}
	}
	if weakMin == int(^uint(0)>>1) {
		return 1
	}
	return weakMin
}

func wildStrike(set *content.Set, wild content.Species) content.Move {
	var best content.Move
	bestScore := -1.0
	for i := range 4 {
		if i >= len(wild.Movepool) {
			break
		}
		mv := set.Moves[wild.Movepool[i].Move]
		if mv.Power <= 0 {
			continue
		}
		score := mv.Accuracy*1000 + mv.Power
		if score > bestScore {
			bestScore = score
			best = mv
		}
	}
	return best
}

// ObjectivesFromIDs builds Objective values for authored lesson lists.
func ObjectivesFromIDs(ids []ObjectiveID) ([]Objective, error) {
	out := make([]Objective, len(ids))
	for i, id := range ids {
		o, ok := ObjectiveByID(id)
		if !ok {
			return nil, fmt.Errorf("capture: unknown objective %q", id)
		}
		out[i] = o
	}
	return out, nil
}
