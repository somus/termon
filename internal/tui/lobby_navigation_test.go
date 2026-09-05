package tui

import (
	"testing"
)

func TestLobbyOpensWorkbench(t *testing.T) {
	h, set, s := testHub(t)
	sv := onboardTrainer(t, h, s, "aaa", "alpha", "rootkit")
	m := New("aaa", sv, set, h)
	m.width, m.height = 100, 32
	m.screen = screenLobby
	next := driveCmd(m, press("p"))
	if next.screen != screenWorkbench {
		t.Fatalf("screen = %d, want workbench", next.screen)
	}
}
