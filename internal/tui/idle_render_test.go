package tui

import "testing"

// Keep cached output equivalent to a full repaint while measuring how many
// full frame builds actually produce new pixels. The reference rebuild happens
// on a copy and does not affect the measured model's cache or animation state.
func TestIdleFramesMatchFullRepaint(t *testing.T) {
	set := loadSet(t)
	cases := []struct {
		name  string
		model Model
	}{
		{name: "lobby", model: memoLobbyModel()},
		{name: "battle_menu", model: battleModel(t, nil, 120, 40)},
	}
	for _, stage := range []struct {
		name  string
		stage onboardStage
	}{
		{"welcome", stageWelcome},
		{"dialogue", stageTalk},
		{"handle", stageHandle},
		{"handle_input", stageHandleInput},
		{"starter", stageStarter},
		{"confirm", stageConfirm},
	} {
		m := New("probe", nil, set, nil)
		m.width, m.height = 120, 40
		m.onboard.stage = stage.stage
		m.onboard.lineAge = 1000 // settled text, not typewriter playback
		cases = append(cases, struct {
			name  string
			model Model
		}{stage.name, m})
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			m := test.model
			m.buildFrame()
			before := m.frameBuilds
			changed := 0
			for range 120 {
				previous := m.View().Content
				m = drive(m, tickMsg{})
				frame := m.View().Content
				if frame != previous {
					changed++
				}
				reference := m
				if full := reference.buildFrame(); frame != full {
					t.Fatal("cached frame differs from a full repaint")
				}
			}
			t.Logf("120 ticks: %d full builds, %d changed frames", m.frameBuilds-before, changed)
		})
	}
}
