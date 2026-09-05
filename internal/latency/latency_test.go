package latency

import (
	"testing"
	"time"
)

func TestPercentile(t *testing.T) {
	sorted20 := make([]time.Duration, 20)
	for i := range sorted20 {
		sorted20[i] = time.Duration(i+1) * time.Millisecond
	}
	unsorted := []time.Duration{
		7 * time.Millisecond,
		time.Millisecond,
		5 * time.Millisecond,
		3 * time.Millisecond,
	}

	tests := []struct {
		name     string
		samples  []time.Duration
		quantile float64
		want     time.Duration
	}{
		{name: "empty samples yield zero", samples: nil, quantile: 0.95, want: 0},
		{
			name:     "single sample is every quantile",
			samples:  []time.Duration{250 * time.Millisecond},
			quantile: 0.95,
			want:     250 * time.Millisecond,
		},
		{
			name:     "n=20 at q=0.95 pins the second-largest sample",
			samples:  sorted20,
			quantile: 0.95,
			want:     19 * time.Millisecond,
		},
		{
			name:     "q=0 clamps up to the smallest sample",
			samples:  sorted20,
			quantile: 0,
			want:     time.Millisecond,
		},
		{
			name:     "input is sorted internally",
			samples:  unsorted,
			quantile: 0.50,
			want:     3 * time.Millisecond,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Percentile(test.samples, test.quantile)
			if got != test.want {
				t.Fatalf("Percentile(q=%v) = %v, want %v", test.quantile, got, test.want)
			}
		})
	}
}
