package tui

import (
	"testing"

	"termon.sh/internal/content"
)

// The same tick advances the same model state with and without output pressure.
// Only expensive cosmetic frame construction is omitted in the pressured case.
func BenchmarkAnimatedOutput(b *testing.B) {
	set, err := content.Load("../../content")
	if err != nil {
		b.Fatal(err)
	}
	for _, pressured := range []bool{false, true} {
		name := "reading"
		if pressured {
			name = "blocked"
		}
		b.Run(name, func(b *testing.B) {
			m := New("probe", nil, set, nil)
			m.width, m.height = 120, 40
			m.onboard.stage = stageStarter
			m.buildFrame()
			m = m.WithOutputPressure(func() bool { return pressured }, nil)
			b.ReportAllocs()
			for b.Loop() {
				next, _ := m.Update(tickMsg{})
				m = next.(Model)
				_ = m.View()
			}
		})
	}
}
