package game_test

import (
	"testing"

	"termon.sh/internal/game"
)

func TestXPForLevelAnchors(t *testing.T) {
	tests := []struct {
		level int
		want  int64
	}{
		{1, 0},
		{14, 1170},
		{24, 2070},
		{30, 3510},
		{50, 9810},
	}
	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			if got := game.XPForLevel(tc.level); got != tc.want {
				t.Fatalf("XPForLevel(%d) = %d, want %d", tc.level, got, tc.want)
			}
		})
	}
}

func TestLevelForXP(t *testing.T) {
	tests := []struct {
		xp   int64
		want int
	}{
		{0, 1},
		{1169, 13},
		{1170, 14},
		{9810, 50},
		{99999, 50},
	}
	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			if got := game.LevelForXP(tc.xp); got != tc.want {
				t.Fatalf("LevelForXP(%d) = %d, want %d", tc.xp, got, tc.want)
			}
		})
	}
}

func TestNaturalStatLevelOne(t *testing.T) {
	if got := game.NaturalStat(55, 1); got != 55 {
		t.Fatalf("NaturalStat(55, 1) = %d, want 55", got)
	}
}
