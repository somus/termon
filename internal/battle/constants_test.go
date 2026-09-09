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
		}
	}
}
