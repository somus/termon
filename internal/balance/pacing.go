package balance

import (
	"fmt"
	"math"
	"sort"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/game"
)

// PacingRow records direct, landed-hit pacing across comparable natural stages.
type PacingRow struct {
	Level                   int     `json:"level"`
	NeutralMeanHits         float64 `json:"neutral_mean_hits"`
	AdvantageMeanHits       float64 `json:"advantage_mean_hits"`
	NeutralPairs            int     `json:"neutral_pairs"`
	AdvantagePairs          int     `json:"advantage_pairs"`
	NonCriticalNeutralOHKOs int     `json:"non_critical_neutral_ohkos"`
}

// NaturalPacing evaluates every natural level with all ordered, equal-stage
// family pairs. It selects each attacker's strongest eligible Move against its
// defender, then calculates hits only after a Move has landed.
func NaturalPacing(set *content.Set) ([]PacingRow, GateResult) {
	rows := make([]PacingRow, 0, 50)
	roots := pacingRoots(set)
	passed := len(roots) > 0
	minimum, maximum, ohkos := math.Inf(1), 0.0, 0
	for level := 1; level <= 50; level++ {
		row := naturalPacingRow(set, roots, level)
		rows = append(rows, row)
		minimum = min(minimum, row.NeutralMeanHits)
		maximum = max(maximum, row.NeutralMeanHits)
		ohkos += row.NonCriticalNeutralOHKOs
		passed = passed && row.NeutralPairs > 0 && row.NeutralMeanHits >= 2.5 && row.NeutralMeanHits <= 3.5 && row.NonCriticalNeutralOHKOs == 0
	}
	return rows, GateResult{
		Name:      GateNaturalKOPace,
		Passed:    passed,
		Value:     minimum,
		Maximum:   maximum,
		Total:     len(rows),
		Detail:    fmt.Sprintf("Levels 1-50: mean %.3f-%.3f neutral landed hits; %d non-crit neutral OHKO pairs", minimum, maximum, ohkos),
		Threshold: "mean 2.5-3.5 neutral landed hits at every level, no non-crit neutral OHKO",
	}
}

func naturalPacingRow(set *content.Set, roots []string, level int) PacingRow {
	row := PacingRow{Level: level}
	if set == nil {
		return row
	}
	neutralTotal, advantageTotal := 0.0, 0.0
	for _, attackerRoot := range roots {
		attacker, attackerStage, ok := pacingSpecies(set, attackerRoot, level)
		if !ok {
			continue
		}
		for _, defenderRoot := range roots {
			defender, defenderStage, ok := pacingSpecies(set, defenderRoot, level)
			if !ok || attackerStage != defenderStage {
				continue
			}
			base, effectiveness, ok := bestPacingAttack(set, attacker, defender, level)
			if !ok {
				continue
			}
			hits := expectedLandedHits(game.NaturalStat(set.Species[defender].BaseStats.HP, level), landedDamageDistribution(base))
			if effectiveness > 1 {
				row.AdvantagePairs++
				advantageTotal += hits
				continue
			}
			row.NeutralPairs++
			neutralTotal += hits
			if nonCriticalOHKO(base, game.NaturalStat(set.Species[defender].BaseStats.HP, level)) {
				row.NonCriticalNeutralOHKOs++
			}
		}
	}
	if row.NeutralPairs > 0 {
		row.NeutralMeanHits = neutralTotal / float64(row.NeutralPairs)
	}
	if row.AdvantagePairs > 0 {
		row.AdvantageMeanHits = advantageTotal / float64(row.AdvantagePairs)
	}
	return row
}

func pacingRoots(set *content.Set) []string {
	if set == nil {
		return nil
	}
	evolved := make(map[string]bool)
	for _, species := range set.Species {
		if species.EvolvesTo != nil {
			evolved[species.EvolvesTo.Species] = true
		}
	}
	roots := make([]string, 0, len(set.Species))
	for slug := range set.Species {
		if !evolved[slug] {
			roots = append(roots, slug)
		}
	}
	sort.Strings(roots)
	return roots
}

func pacingSpecies(set *content.Set, root string, level int) (string, int, bool) {
	species, err := SpeciesAtLevel(set, root, level)
	if err != nil {
		return "", 0, false
	}
	stage := 0
	for current := root; current != species; stage++ {
		next := set.Species[current].EvolvesTo
		if next == nil {
			return "", 0, false
		}
		current = next.Species
	}
	return species, stage, true
}

func bestPacingAttack(set *content.Set, attacker, defender string, level int) (float64, float64, bool) {
	attackerSpecies, attackerOK := set.Species[attacker]
	defenderSpecies, defenderOK := set.Species[defender]
	if !attackerOK || !defenderOK {
		return 0, 0, false
	}
	bestScore, bestBase, bestEffectiveness := -1.0, 0.0, 0.0
	bestOrder := math.MaxInt
	for _, entry := range attackerSpecies.Movepool {
		if entry.Level > level {
			continue
		}
		move, ok := set.Moves[entry.Move]
		if !ok {
			continue
		}
		attack := game.NaturalStat(attackerSpecies.BaseStats.Attack, level)
		if move.Category == "special" {
			attack = game.NaturalStat(attackerSpecies.BaseStats.SpAttack, level)
		}
		effectiveness := set.Effectiveness(move.Type, defenderSpecies.Type)
		base := battle.DamageBase(game.MovePower(move.Power, level), attack, game.NaturalStat(defenderSpecies.BaseStats.Defense, level), move.Type, attackerSpecies.Type, effectiveness)
		score := battle.ExpectedDamage(base, move.Accuracy)
		if score > bestScore || (score == bestScore && move.Order < bestOrder) {
			bestScore, bestBase, bestEffectiveness, bestOrder = score, base, effectiveness, move.Order
		}
	}
	return bestBase, bestEffectiveness, bestScore >= 0
}

func landedDamageDistribution(base float64) map[int]float64 {
	distribution := make(map[int]float64)
	addVarianceDistribution(distribution, base, 1-1.0/battle.CritChance)
	addVarianceDistribution(distribution, base*battle.CritMultiplier, 1.0/battle.CritChance)
	return distribution
}

func addVarianceDistribution(distribution map[int]float64, base, weight float64) {
	if base <= 0 {
		distribution[battle.MinDamage] += weight
		return
	}
	width := battle.VarianceMax - battle.VarianceMin
	for damage := battle.MinDamage; damage <= int(math.Ceil(base*battle.VarianceMax)); damage++ {
		low := float64(damage) / base
		if damage == battle.MinDamage {
			low = battle.VarianceMin
		}
		high := float64(damage+1) / base
		low = max(low, battle.VarianceMin)
		high = min(high, battle.VarianceMax)
		if high > low {
			distribution[damage] += weight * (high - low) / width
		}
	}
}

func expectedLandedHits(hp int, distribution map[int]float64) float64 {
	if hp <= 0 || len(distribution) == 0 {
		return 0
	}
	expected := make([]float64, hp+1)
	damages := make([]int, 0, len(distribution))
	for damage := range distribution {
		damages = append(damages, damage)
	}
	sort.Ints(damages)
	for remaining := 1; remaining <= hp; remaining++ {
		expected[remaining] = 1
		for _, damage := range damages {
			expected[remaining] += distribution[damage] * expected[max(0, remaining-damage)]
		}
	}
	return expected[hp]
}

func nonCriticalOHKO(base float64, hp int) bool {
	if hp <= battle.MinDamage {
		return true
	}
	return base*battle.VarianceMax > float64(hp)
}
