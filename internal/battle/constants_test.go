package battle

import (
	"math"
	"testing"
)

func TestExpectedDamageMatchesEngineRolls(t *testing.T) {
	// Numerical quadrature of the engine's actual hit/crit/variance order is an
	// independent check of the closed-form integral, including low-damage floors.
	for _, base := range []float64{0.2, 1, 3, 16.5, 42, 137.25} {
		for _, accuracy := range []float64{0, 80, 100} {
			const samples = 10000
			want := 0.0
			for i := range samples {
				variance := VarianceMin + (VarianceMax-VarianceMin)*(float64(i)+0.5)/samples
				ordinary := float64(max(MinDamage, int(base*variance)))
				critical := float64(max(MinDamage, int(base*CritMultiplier*variance)))
				want += (ordinary*(CritChance-1) + critical) / CritChance / samples
			}
			want *= accuracy / 100
			if got := ExpectedDamage(base, accuracy); math.Abs(got-want) > 0.001 {
				t.Errorf("base=%v accuracy=%v: expectation=%v, integrated rolls=%v", base, accuracy, got, want)
			}
			for _, hp := range []int{1, 5, 40, 200} {
				capped := 0.0
				for threshold := 1; threshold <= hp; threshold++ {
					capped += KOProbability(base, accuracy, threshold)
				}
				if got := ExpectedHPLoss(base, accuracy, hp); math.Abs(got-capped) > 1e-9 {
					t.Errorf("base=%v accuracy=%v hp=%d: HP loss=%v, tail sum=%v", base, accuracy, hp, got, capped)
				}
			}
		}
	}
}

func TestKOProbabilityMatchesEngineRolls(t *testing.T) {
	for _, base := range []float64{0.2, 3, 16.5, 42} {
		for _, hp := range []int{1, 2, 15, 42, 60} {
			const samples = 10000
			want := 0.0
			for i := range samples {
				variance := VarianceMin + (VarianceMax-VarianceMin)*(float64(i)+0.5)/samples
				if max(MinDamage, int(base*variance)) >= hp {
					want += float64(CritChance-1) / CritChance / samples
				}
				if max(MinDamage, int(base*CritMultiplier*variance)) >= hp {
					want += 1.0 / CritChance / samples
				}
			}
			want *= 0.8
			if got := KOProbability(base, 80, hp); math.Abs(got-want) > 0.001 {
				t.Errorf("base=%v hp=%v: probability=%v, integrated rolls=%v", base, hp, got, want)
			}
		}
	}
}
