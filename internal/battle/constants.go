package battle

import "time"

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
