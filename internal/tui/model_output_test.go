package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"termon.sh/internal/server"
)

func TestOutputPressureCoalescesCosmeticsWithoutPausingState(t *testing.T) {
	busy := false
	skipped := 0
	m := memoLobbyModel().WithOutputPressure(func() bool { return busy }, func() { skipped++ })
	m.status, m.statusHold = "temporary notice", 3
	m = drive(m, tickMsg{})
	frame, builds, age := m.View().Content, m.frameBuilds, m.lobbyAge
	busy = true
	for range 20 {
		m = drive(m, tickMsg{})
	}
	if m.View().Content != frame || m.frameBuilds != builds {
		t.Fatal("cosmetic frames advanced under pressure")
	}
	if m.status != "" || m.lobbyAge != age+20 {
		t.Fatal("output pressure paused state advancement")
	}
	if skipped != 20 {
		t.Fatalf("skipped = %d", skipped)
	}
	busy = false
	m = drive(m, tickMsg{})
	if m.frameBuilds != builds+1 {
		t.Fatal("obsolete render requests were not coalesced into one frame")
	}
	if strings.Contains(ansi.Strip(m.View().Content), "temporary notice") {
		t.Fatal("expired notice survived catch-up")
	}
}

func TestOutputPressurePrioritizesInputAndHubFeedback(t *testing.T) {
	m := memoLobbyModel().WithOutputPressure(func() bool { return true }, nil)
	m = drive(m, tickMsg{})
	builds := m.frameBuilds
	m = drive(m, press("e"))
	if m.frameBuilds != builds+1 || !strings.Contains(ansi.Strip(m.View().Content), "gl hf") {
		t.Fatal("direct input was deferred")
	}
	m = drive(m, server.ErrorMsg{Text: "action rejected"})
	if m.frameBuilds != builds+2 || !strings.Contains(ansi.Strip(m.View().Content), "action rejected") {
		t.Fatal("Hub feedback was deferred")
	}
	save := fullPartyTestSave()
	m = drive(m, server.SaveMsg{Save: save})
	if m.save != save {
		t.Fatal("Save event was lost")
	}
}
