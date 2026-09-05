package dojo

import (
	"errors"
	"fmt"
	"slices"

	"termon.sh/internal/content"
)

// SparringPool maps each Type slug to the Evolution Families available to the
// daily Sparring roster rotation.
var SparringPool = map[string][]string{
	"organic": {"mossmuff", "rootanami", "rootkit", "sproutware", "thornpatch"},
	"thermal": {"cindernode", "emberbyte", "scorchip", "wickware"},
	"coolant": {"aquabit", "flowcell", "gushkit", "mistcache", "splashscreen"},
	"current": {"amperent", "joulpup", "surgetail", "zaplet"},
	"virus":   {"bloatware", "spamlet", "wormate"},
	"silicon": {"chippunk", "coghound", "servoboar"},
}

// ValidateSparringPool ensures every Type has known, unique pool Families.
func ValidateSparringPool(set *content.Set) error {
	if set == nil {
		return errors.New("dojo: nil content")
	}
	seen := map[string]string{}
	for t := range set.Types {
		families := SparringPool[t]
		if len(families) == 0 {
			return fmt.Errorf("dojo: sparring pool missing type %q", t)
		}
		for _, fam := range families {
			if _, ok := set.Species[fam]; !ok {
				return fmt.Errorf("dojo: sparring pool family %q for type %q unknown", fam, t)
			}
			if FamilyBase(set, fam) != fam {
				return fmt.Errorf("dojo: sparring pool species %q is not a Family base", fam)
			}
			if prior, dup := seen[fam]; dup {
				return fmt.Errorf("dojo: sparring pool family %q belongs to both %q and %q", fam, prior, t)
			}
			seen[fam] = t
			if set.Species[fam].Type != t {
				return fmt.Errorf("dojo: sparring pool family %q has type %q, want %q", fam, set.Species[fam].Type, t)
			}
		}
	}
	for t := range SparringPool {
		if _, ok := set.Types[t]; !ok {
			return fmt.Errorf("dojo: sparring pool has unknown type %q", t)
		}
	}
	for slug := range set.Species {
		if FamilyBase(set, slug) == slug {
			if _, ok := seen[slug]; !ok {
				return fmt.Errorf("dojo: sparring pool missing Family %q", slug)
			}
		}
	}
	return ValidateAllTypeTriples(set)
}

// ValidateAllTypeTriples rejects a pool that cannot build F/N/U rosters.
func ValidateAllTypeTriples(set *content.Set) error {
	types := make([]string, 0, len(set.Types))
	for t := range set.Types {
		types = append(types, t)
	}
	slices.Sort(types)
	for _, a := range types {
		for _, b := range types {
			for _, c := range types {
				if _, err := buildRosterFamilies(set, []string{a, b, c}, 0); err != nil {
					return fmt.Errorf("dojo: pool cannot build roster for types %q/%q/%q: %w", a, b, c, err)
				}
			}
		}
	}
	return nil
}
