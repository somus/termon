package capture

import (
	"slices"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/game"
)

// PartyFighter is one Monster snapshot for Capture HP computation.
type PartyFighter struct {
	Species string
	Level   int
	Loadout []string
}

// WildLevel returns max(1, min(50, max(party Levels))).
func WildLevel(party []PartyFighter) int {
	maxLv := 1
	for _, p := range party {
		if p.Level > maxLv {
			maxLv = p.Level
		}
	}
	return min(50, max(1, maxLv))
}

// TargetHP computes Target Encounter HP from the snapshotted Party versus wild.
func TargetHP(set *content.Set, party []PartyFighter, wild content.Species, wildLevel int) int {
	globalMax := 0
	low := 1
	for _, p := range party {
		pSpec, ok := set.Species[p.Species]
		if !ok {
			continue
		}
		var mins []int
		mx := 0
		for _, slug := range p.Loadout {
			mv, ok := set.Moves[slug]
			if !ok || mv.Power <= 0 {
				continue
			}
			a := partyHit(set, pSpec, p.Level, mv, wild, wildLevel, varianceMin)
			c := partyHit(set, pSpec, p.Level, mv, wild, wildLevel, varianceMax)
			mins = append(mins, a)
			if c > mx {
				mx = c
			}
		}
		if mx < 1 {
			mx = 1
		}
		if mx > globalMax {
			globalMax = mx
		}
		slices.Sort(mins)
		line := 0
		for i, v := range mins {
			if i >= 3 {
				break
			}
			line += v
		}
		if se := firstSEMove(set, p.Loadout, wild.Type); se != nil && len(mins) >= 3 {
			line = mins[0] + mins[1] + partyHit(set, pSpec, p.Level, *se, wild, wildLevel, varianceMin)
		}
		low = max(low, mx+1, line+1)
	}
	high := max(low, 8*globalMax)
	return min(high, low+globalMax)
}

// ClampWildDamage limits wild outgoing damage to max(1, floor((defenderMaxHP-1)/5)).
func ClampWildDamage(defenderMaxHP, dealt int) int {
	clamp := max(1, (defenderMaxHP-1)/5)
	return min(dealt, clamp)
}

// WildHit computes clamped wild damage against a defender at full natural HP.
func WildHit(set *content.Set, wildSpec content.Species, wildLevel int, move content.Move, defSpec content.Species, defLevel int) int {
	dealt := rawHit(set, wildSpec, wildLevel, move, defSpec, defLevel, varianceMax)
	defMaxHP := game.NaturalStat(defSpec.BaseStats.HP, defLevel)
	return ClampWildDamage(defMaxHP, dealt)
}

func partyHit(set *content.Set, atkSpec content.Species, atkLevel int, move content.Move, defSpec content.Species, defLevel int, variance float64) int {
	return rawHit(set, atkSpec, atkLevel, move, defSpec, defLevel, variance)
}

func rawHit(set *content.Set, atkSpec content.Species, atkLevel int, move content.Move, defSpec content.Species, defLevel int, variance float64) int {
	a := game.NaturalStat(atkSpec.BaseStats.Attack, atkLevel)
	if move.Category == "special" {
		a = game.NaturalStat(atkSpec.BaseStats.SpAttack, atkLevel)
	}
	d := game.NaturalStat(defSpec.BaseStats.Defense, defLevel)
	dmg := battle.DamageBase(move.Power, a, d, move.Type, atkSpec.Type, set.Effectiveness(move.Type, defSpec.Type))
	dmg *= variance
	return max(battle.MinDamage, int(dmg))
}

func firstSEMove(set *content.Set, loadout []string, wildType string) *content.Move {
	for _, slug := range loadout {
		mv, ok := set.Moves[slug]
		if !ok {
			continue
		}
		if set.Effectiveness(mv.Type, wildType) >= battle.SuperEffectiveAt {
			cp := mv
			return &cp
		}
	}
	return nil
}

// IdentityForFamily returns measured_pressure or hold_the_line for a base Family slug.
func IdentityForFamily(family string) ObjectiveID {
	if id, ok := familyIdentity[family]; ok {
		return id
	}
	return MeasuredPressure
}

func complement(id ObjectiveID) ObjectiveID {
	if id == MeasuredPressure {
		return HoldTheLine
	}
	return MeasuredPressure
}

var familyIdentity = map[string]ObjectiveID{
	"rootkit": HoldTheLine, "sproutware": MeasuredPressure, "thornpatch": HoldTheLine,
	"mossmuff": HoldTheLine, "rootanami": HoldTheLine, "emberbyte": MeasuredPressure,
	"cindernode": HoldTheLine, "scorchip": MeasuredPressure, "wickware": MeasuredPressure,
	"aquabit": MeasuredPressure, "flowcell": HoldTheLine, "gushkit": MeasuredPressure,
	"mistcache": MeasuredPressure, "splashscreen": HoldTheLine, "zaplet": MeasuredPressure,
	"joulpup": MeasuredPressure, "amperent": HoldTheLine, "surgetail": HoldTheLine,
	"spamlet": MeasuredPressure, "bloatware": HoldTheLine, "wormate": HoldTheLine,
	"chippunk": MeasuredPressure, "coghound": MeasuredPressure, "servoboar": HoldTheLine,
}

// Variance bounds for PvE band modeling mirror the battle combat tunables.
const (
	varianceMin = battle.VarianceMin
	varianceMax = battle.VarianceMax
)
