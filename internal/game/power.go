package game

import "math"

// MovePower grows from 40% of a Move's authored ceiling at Level 1 to
// the full ceiling at Level 50. Round down before applying combat modifiers.
func MovePower(ceiling float64, level int) float64 {
	level = min(50, max(1, level))
	return max(1, math.Floor(ceiling*(0.4+0.6*float64(level-1)/49)))
}
