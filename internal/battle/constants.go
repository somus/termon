package battle

import (
	"math"
	"time"
)

// Combat tunables. Playtesting adjusts these without touching engine logic.
const (
	DamageDivisor    = 5
	STABMultiplier   = 1.5
	CritChance       = 16
	CritMultiplier   = 1.5
	VarianceMin      = 0.85
	VarianceMax      = 1.00
	MinDamage        = 1
	SuperEffectiveAt = 2.0
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
