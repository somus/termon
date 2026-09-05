package game

// XPForLevel returns cumulative XP required to reach level.
func XPForLevel(level int) int64 {
	switch {
	case level <= 1:
		return 0
	case level <= 24:
		return int64(90 * (level - 1))
	case level <= 40:
		return 2070 + int64(240*(level-24))
	default:
		return 5910 + int64(390*(level-40))
	}
}

// LevelForXP returns the greatest level whose threshold is at or below xp.
func LevelForXP(xp int64) int {
	if xp < 0 {
		return 1
	}
	xpCap := XPForLevel(50)
	if xp >= xpCap {
		return 50
	}
	for level := 50; level >= 1; level-- {
		if xp >= XPForLevel(level) {
			return level
		}
	}
	return 1
}

// ClampXP limits xp to the Level 50 threshold.
func ClampXP(xp int64) int64 {
	xpCap := XPForLevel(50)
	if xp > xpCap {
		return xpCap
	}
	if xp < 0 {
		return 0
	}
	return xp
}

// NaturalStat scales a Species base stat for level.
func NaturalStat(base, level int) int {
	if level < 1 {
		level = 1
	}
	v := base * (100 + 2*(level-1)) / 100
	if v < 1 {
		return 1
	}
	return v
}
