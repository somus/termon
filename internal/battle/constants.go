package battle

import (
	"math"
	"time"
)

// Combat tunables. Playtesting adjusts these without touching engine logic.
const (
	DamageDivisor    = 2.72
	STABMultiplier   = 1.5
	CritChance       = 16
	CritMultiplier   = 1.5
	VarianceMin      = 0.85
	VarianceMax      = 1.00
	MinDamage        = 1
	SuperEffectiveAt = 1.5
	ResistedBelow    = 1.0

	DisconnectGrace = 60 * time.Second
)

// DamageBase is the single source of the stat-scaled damage formula: Move
// power scaled by attack over defense, then STAB and Type effectiveness. It
// excludes the crit roll, variance, and the wild-damage clamp so callers can
// layer those per context; every damage computation in the repo goes through
// here, and Combat tunables above are the only knobs.
func DamageBase(power float64, atk, def int, moveType, attackerType string, effectiveness float64) float64 {
	base := int(power*float64(atk)/float64(def)/DamageDivisor) + 2
	dmg := float64(base)
	if moveType == attackerType {
		dmg *= STABMultiplier
	}
	dmg *= effectiveness
	return dmg
}

// ExpectedDamage averages hit, critical and variance rolls, including the
// engine's final integer rounding. base comes from DamageBase, before crits.
// This is ordinary direct damage; a Wild damage clamp must be modeled separately.
func ExpectedDamage(base, accuracy float64) float64 {
	criticalChance := 1.0 / CritChance
	onHit := (1-criticalChance)*meanLandedDamage(base) + criticalChance*meanLandedDamage(base*CritMultiplier)
	return accuracy / 100 * onHit
}

// ExpectedHPLoss caps each possible damage result at the target's current HP.
// Capping after averaging would incorrectly reward unreliable overkill.
func ExpectedHPLoss(base, accuracy float64, hp int) float64 {
	if hp <= 0 {
		return 0
	}
	landed := func(damage float64) float64 {
		low, high := damage*VarianceMin, damage*VarianceMax
		if high == low {
			return float64(min(hp, max(MinDamage, int(low))))
		}
		primitive := func(x float64) float64 {
			bounded := min(x, float64(hp))
			n := math.Floor(bounded)
			return n*bounded - n*(n+1)/2 + max(0, x-float64(hp))*float64(hp)
		}
		integral := primitive(high) - primitive(low)
		integral += max(0, min(high, float64(MinDamage))-low)
		return integral / (high - low)
	}
	return accuracy / 100 * ((1-1.0/CritChance)*landed(base) + landed(base*CritMultiplier)/CritChance)
}

// KOProbability returns the probability that an ordinary direct Move removes
// at least hp, including misses, critical hits, variance and integer rounding.
// base comes from DamageBase. Callers model Wild clamps separately.
func KOProbability(base, accuracy float64, hp int) float64 {
	if hp <= 0 {
		return 1
	}
	chance := func(damage float64) float64 {
		if hp <= MinDamage {
			return 1
		}
		if damage <= 0 {
			return 0
		}
		return min(1, max(0, (VarianceMax-float64(hp)/damage)/(VarianceMax-VarianceMin)))
	}
	criticalChance := 1.0 / CritChance
	return accuracy / 100 * ((1-criticalChance)*chance(base) + criticalChance*chance(base*CritMultiplier))
}

func meanLandedDamage(base float64) float64 {
	low, high := base*VarianceMin, base*VarianceMax
	if high == low {
		return float64(max(MinDamage, int(low)))
	}
	// The integral of floor(x) on [0,x] is n*x-n*(n+1)/2, n=floor(x).
	primitive := func(x float64) float64 {
		n := math.Floor(x)
		return n*x - n*(n+1)/2
	}
	integral := primitive(high) - primitive(low)
	// The engine raises floor(x)==0 to its minimum of one on landed hits.
	integral += max(0, min(high, float64(MinDamage))-low)
	return integral / (high - low)
}
