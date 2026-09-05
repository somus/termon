// Package latency holds shared duration-quantile math for load reports.
package latency

import (
	"math"
	"slices"
	"time"
)

// Percentile returns the quantile-th value of samples (0 ≤ quantile ≤ 1),
// sorted internally, using the ceil(len*quantile)-1 index convention
// clamped to index 0. Empty samples yield 0.
func Percentile(samples []time.Duration, quantile float64) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	ordered := append([]time.Duration(nil), samples...)
	slices.Sort(ordered)
	index := max(int(math.Ceil(quantile*float64(len(ordered))))-1, 0)
	return ordered[index]
}
