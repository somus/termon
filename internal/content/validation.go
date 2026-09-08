package content

import "fmt"

const (
	maxMoveLearningLevel = 50
	queueLevel           = 30
	minimumLoadoutMoves  = 4
	stageTwoStatTotal    = 320
	stageThreeStatTotal  = 400
)

// validateSpeciesProgression validates progression constraints after the
// evolution graph has established each Species' stage depth.
func validateSpeciesProgression(species map[string]Species) error {
	predecessors := make(map[string]string, len(species))
	for slug, s := range species {
		if s.EvolvesTo != nil {
			predecessors[s.EvolvesTo.Species] = slug
		}
	}

	for slug, s := range species {
		queueMoves := 0
		levelOneMoves := 0
		for _, entry := range s.Movepool {
			if entry.Level < 1 || entry.Level > maxMoveLearningLevel {
				return fmt.Errorf(
					"species %s: movepool move %s level %d must be between 1 and %d",
					slug,
					entry.Move,
					entry.Level,
					maxMoveLearningLevel,
				)
			}
			if entry.Level <= queueLevel {
				queueMoves++
			}
			if entry.Level == 1 {
				levelOneMoves++
			}
		}
		if queueMoves < minimumLoadoutMoves {
			return fmt.Errorf(
				"species %s: movepool has %d moves available by level %d, want at least %d",
				slug,
				queueMoves,
				queueLevel,
				minimumLoadoutMoves,
			)
		}
		if levelOneMoves < minimumLoadoutMoves {
			return fmt.Errorf(
				"species %s: movepool has %d level-1 moves, want at least %d",
				slug,
				levelOneMoves,
				minimumLoadoutMoves,
			)
		}

		depth := evolutionDepth(slug, predecessors)
		statTotal := baseStatTotal(s.BaseStats)
		switch depth {
		case 2:
			if statTotal != stageTwoStatTotal {
				return fmt.Errorf(
					"species %s: stage 2 base stat total %d, want %d",
					slug,
					statTotal,
					stageTwoStatTotal,
				)
			}
		case 3:
			if statTotal != stageThreeStatTotal {
				return fmt.Errorf(
					"species %s: stage 3 base stat total %d, want %d",
					slug,
					statTotal,
					stageThreeStatTotal,
				)
			}
		}
	}

	return nil
}

func evolutionDepth(slug string, predecessors map[string]string) int {
	depth := 1
	for predecessors[slug] != "" {
		depth++
		slug = predecessors[slug]
	}
	return depth
}

func baseStatTotal(stats Stats) int {
	return stats.HP + stats.Attack + stats.Defense + stats.SpAttack + stats.Speed
}
