package balance

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"termon.sh/internal/content"
)

// EnumerateMatrixScenarios streams the stage/loadout and physical-placement
// matrix. Policy and seed are deliberately supplied by the caller.
func EnumerateMatrixScenarios(set *content.Set, teams []ReferenceTeam, yield func(Scenario) error) error {
	if set == nil {
		return errors.New("balance: nil content")
	}
	levels := NaturalMatrixLevels(set)
	for i, teamA := range teams {
		for _, teamB := range teams[i:] {
			for leadA := range 3 {
				for leadB := range 3 {
					for _, stage := range []string{FixtureStageBase, FixtureStageMiddle, FixtureStageFinal} {
						for _, loadout := range []string{FixtureLoadoutDefault, FixtureLoadoutReference, FixtureLoadoutFrontier} {
							sc := NormalizedScenario(fmt.Sprintf("matrix/normalized/%s/%s-vs-%s/%d-%d", stage, teamA.Name, teamB.Name, leadA, leadB), teamA, teamB, leadA)
							sc.LeadB, sc.Stage, sc.Loadout = leadB, stage, loadout
							if err := yieldPlacements(sc, yield); err != nil {
								return err
							}
						}
					}
					if err := enumerateNaturalPlacements(set, teamA, teamB, leadA, leadB, levels, yield); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func naturalStageAllowed(set *content.Set, team ReferenceTeam, level int, stage string) (bool, error) {
	for _, family := range team.Families {
		_, err := fixtureSpecies(set, family, level, false, stage)
		if err == nil {
			continue
		}
		if stage == FixtureStageMiddle && strings.Contains(err.Error(), "unreachable") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func yieldPlacements(sc Scenario, yield func(Scenario) error) error {
	for _, sideA := range []bool{true, false} {
		for _, swapped := range []bool{false, true} {
			run := sc
			run.EngineSideA, run.PartyOrderSwapped = sideA, swapped
			run.Name = fmt.Sprintf("%s/loadout-%s/side-a-%t/reserves-swapped-%t", sc.Name, sc.Loadout, sideA, swapped)
			if err := yield(run); err != nil {
				return err
			}
		}
	}
	return nil
}

// NaturalMatrixLevels returns checkpoints plus every evolution threshold ±1.
func NaturalMatrixLevels(set *content.Set) []int {
	levels := append([]int(nil), NaturalCheckpoints...)
	for _, species := range set.Species {
		if species.EvolvesTo != nil {
			for _, level := range []int{species.EvolvesTo.Level - 1, species.EvolvesTo.Level, species.EvolvesTo.Level + 1} {
				levels = append(levels, min(50, max(1, level)))
			}
		}
	}
	slices.Sort(levels)
	return slices.Compact(levels)
}

func enumerateNaturalPlacements(set *content.Set, teamA, teamB ReferenceTeam, leadA, leadB int, levels []int, yield func(Scenario) error) error {
	for _, level := range levels {
		for _, stage := range []string{FixtureStageReachable, FixtureStageBase, FixtureStageMiddle} {
			for _, loadout := range []string{FixtureLoadoutDefault, FixtureLoadoutReference, FixtureLoadoutFrontier} {
				allowed, err := naturalStageAllowed(set, teamA, level, stage)
				if err != nil {
					return err
				}
				if !allowed {
					continue
				}
				allowed, err = naturalStageAllowed(set, teamB, level, stage)
				if err != nil {
					return err
				}
				if !allowed {
					continue
				}
				sc := NaturalScenario(fmt.Sprintf("matrix/natural/%d/%s/%s-vs-%s/%d-%d", level, stage, teamA.Name, teamB.Name, leadA, leadB), teamA, teamB, leadA, leadB, level)
				sc.Stage, sc.Loadout = stage, loadout
				if err := yieldPlacements(sc, yield); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
