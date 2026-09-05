package dojo

import (
	"fmt"

	"termon.sh/internal/content"
)

// StageIndex returns 0 base, 1 middle, or 2 final for a Species slug.
func StageIndex(set *content.Set, species string) int {
	base := FamilyBase(set, species)
	fam := loadFamily(set, base)
	switch species {
	case fam.final:
		return 2
	case fam.middle:
		return 1
	default:
		return 0
	}
}

type evoFamily struct {
	base, middle, final string
	midLevel, finLevel  int
}

func loadFamily(set *content.Set, base string) evoFamily {
	sp, ok := set.Species[base]
	if !ok {
		return evoFamily{base: base}
	}
	out := evoFamily{base: base}
	if sp.EvolvesTo == nil {
		return out
	}
	out.middle = sp.EvolvesTo.Species
	out.midLevel = sp.EvolvesTo.Level
	mid, ok := set.Species[out.middle]
	if !ok || mid.EvolvesTo == nil {
		return out
	}
	out.final = mid.EvolvesTo.Species
	out.finLevel = mid.EvolvesTo.Level
	return out
}

// FamilyBase walks evolution predecessors to the Family root slug.
func FamilyBase(set *content.Set, species string) string {
	predecessors := map[string]string{}
	for slug, sp := range set.Species {
		if sp.EvolvesTo != nil {
			predecessors[sp.EvolvesTo.Species] = slug
		}
	}
	current := species
	for {
		p, ok := predecessors[current]
		if !ok {
			return current
		}
		current = p
	}
}

// SpeciesAtStage returns the pool Family Species at the requested stage index.
func SpeciesAtStage(set *content.Set, familyBase string, stage int) (string, error) {
	if _, ok := set.Species[familyBase]; !ok {
		return "", fmt.Errorf("dojo: unknown family %q", familyBase)
	}
	fam := loadFamily(set, familyBase)
	switch stage {
	case 2:
		if fam.final != "" {
			return fam.final, nil
		}
		return fam.base, nil
	case 1:
		if fam.middle != "" {
			return fam.middle, nil
		}
		return fam.base, nil
	default:
		return fam.base, nil
	}
}

// DailySpeciesAtLevel picks middle stage when mid threshold <= level else base.
func DailySpeciesAtLevel(set *content.Set, familyBase string, level int) (string, error) {
	base, ok := set.Species[familyBase]
	if !ok {
		return "", fmt.Errorf("dojo: unknown family %q", familyBase)
	}
	if base.EvolvesTo != nil && base.EvolvesTo.Level <= level {
		return base.EvolvesTo.Species, nil
	}
	return familyBase, nil
}
