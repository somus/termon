package game

import "testing"

func TestMovePower(t *testing.T) {
	for _, tc := range []struct {
		level int
		want  float64
	}{{0, 30}, {1, 30}, {3, 31}, {25, 52}, {50, 75}, {60, 75}} {
		if got := MovePower(75, tc.level); got != tc.want {
			t.Errorf("MovePower(75, %d) = %v, want %v", tc.level, got, tc.want)
		}
	}
	for _, ceiling := range []float64{1, 40, 55, 65, 75, 90, 100} {
		previous := 0.0
		for level := 1; level <= 50; level++ {
			power := MovePower(ceiling, level)
			if power < previous || power > ceiling {
				t.Fatalf("ceiling %v at level %d: power %v, previous %v", ceiling, level, power, previous)
			}
			previous = power
		}
	}
}
