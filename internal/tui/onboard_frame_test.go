package tui

import "testing"

func TestOnboardTickGateMatchesFullRepaint(t *testing.T) {
	set := loadSet(t)
	for stage := stageWelcome; stage <= stageLesson; stage++ {
		m := New("probe", nil, set, nil)
		m.width, m.height = 120, 40
		m.onboard.stage = stage
		m.buildFrame()
		for tick := range 120 {
			m = drive(m, tickMsg{})
			if m.onboard.age != tick+1 || m.onboard.lineAge != tick+1 {
				t.Fatalf("stage %d tick %d: presentation gate altered animation clocks", stage, tick)
			}
			reference := m
			if actual, full := m.View().Content, reference.buildFrame(); actual != full {
				t.Fatalf("stage %d tick %d: skipped a visible change", stage, tick)
			}
		}
	}
}

func TestStarterInputBypassesIdleTickGate(t *testing.T) {
	m := New("probe", nil, loadSet(t), nil)
	m.width, m.height = 120, 40
	m.onboard.stage = stageStarter
	m.buildFrame()
	before := m.frameBuilds
	m = drive(m, tickMsg{})
	if m.frameBuilds != before {
		t.Fatal("rebuilt an unchanged idle pose")
	}
	previous := m.View().Content
	m = drive(m, press("2"))
	if m.frameBuilds != before+1 || m.View().Content == previous {
		t.Fatal("selection waited for an animation tick")
	}
}
