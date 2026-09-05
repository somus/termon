package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"termon.sh/internal/lobby"
	"termon.sh/internal/server"
)

func memoLobbyModel() Model {
	m := New("aaa", fullPartyTestSave(), nil, nil)
	m.width, m.height = 80, 24
	m.screen = screenLobby
	m.snap = server.SnapshotMsg{
		Dojo: 1,
		You:  lobby.Presence{Hash: "aaa", Handle: "alpha", Species: "rootkit", X: 5, Y: 5},
	}
	return m
}

// drive feeds msg through Update, returning the resulting model.
func drive(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestNoOpTicksAreFree(t *testing.T) {
	m := memoLobbyModel()
	m = drive(m, tickMsg{}) // prime the cache through the Update path
	baseline := m.frameBuilds
	if baseline == 0 {
		t.Fatal("expected the priming update to build a frame")
	}
	first := ansi.Strip(m.View().Content)

	for range 50 {
		m = drive(m, tickMsg{})
	}

	if m.frameBuilds != baseline {
		t.Fatalf("frameBuilds = %d after 50 idle ticks, want %d", m.frameBuilds, baseline)
	}
	if got := ansi.Strip(m.View().Content); got != first {
		t.Fatalf("idle ticks changed the frame:\n%s", got)
	}
}

func TestAnyMutationRebuildsFrame(t *testing.T) {
	t.Run("snapshot", func(t *testing.T) {
		m := memoLobbyModel()
		m = drive(m, tickMsg{})
		baseline := m.frameBuilds

		m = drive(m, server.SnapshotMsg{
			Dojo: 1,
			You:  lobby.Presence{Hash: "aaa", Handle: "alpha", Species: "rootkit", X: 6, Y: 5},
			Others: []lobby.Presence{
				{Hash: "bbb", Handle: "bravo", Species: "emberbyte", X: 8, Y: 6},
			},
		})

		if m.frameBuilds <= baseline {
			t.Fatalf("frameBuilds = %d after snapshot, want > %d", m.frameBuilds, baseline)
		}
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "bravo") {
			t.Fatalf("snapshot mutation missing from frame:\n%s", view)
		}
	})

	t.Run("key input", func(t *testing.T) {
		m := memoLobbyModel()
		m = drive(m, tickMsg{})
		baseline := m.frameBuilds

		m = drive(m, press("e")) // opens the emote picker footer

		if m.frameBuilds <= baseline {
			t.Fatalf("frameBuilds = %d after key input, want > %d", m.frameBuilds, baseline)
		}
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "gl hf") || !strings.Contains(view, "emote") {
			t.Fatalf("emote picker missing from frame:\n%s", view)
		}
	})
}

func TestResizeRebuildsFrame(t *testing.T) {
	m := memoLobbyModel()
	m = drive(m, tickMsg{})
	baseline := m.frameBuilds

	m = drive(m, tea.WindowSizeMsg{Width: 100, Height: 30})

	if m.frameBuilds <= baseline {
		t.Fatalf("frameBuilds = %d after resize, want > %d", m.frameBuilds, baseline)
	}
	if m.frameW != 100 || m.frameH != 30 {
		t.Fatalf("cached frame size = %dx%d, want 100x30", m.frameW, m.frameH)
	}
	view := ansi.Strip(m.View().Content)
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) != 30 {
		t.Fatalf("resized frame height = %d, want 30", len(lines))
	}
}

func TestStatusExpiryStillAnimates(t *testing.T) {
	m := memoLobbyModel()
	m.status = "hold tight"
	m.statusHold = 3

	for i := range 3 { // each countdown tick must rebuild until it clears
		before := m.frameBuilds
		m = drive(m, tickMsg{})
		if m.frameBuilds <= before {
			t.Fatalf("tick %d: frameBuilds = %d, want > %d while status visible", i+1, m.frameBuilds, before)
		}
		view := ansi.Strip(m.View().Content)
		if i < 2 && !strings.Contains(view, "hold tight") {
			t.Fatalf("tick %d: status vanished early:\n%s", i+1, view)
		}
	}
	if m.status != "" {
		t.Fatalf("status = %q after hold elapsed, want cleared", m.status)
	}
	if got := ansi.Strip(m.View().Content); strings.Contains(got, "hold tight") {
		t.Fatalf("expired status still rendered:\n%s", got)
	}

	// Once clear, ticks are free again.
	stable := m.frameBuilds
	m = drive(m, tickMsg{})
	if m.frameBuilds != stable {
		t.Fatalf("frameBuilds = %d after expiry, want %d", m.frameBuilds, stable)
	}
}

func TestLobbyWalkAnimationStillRebuilds(t *testing.T) {
	m := memoLobbyModel()
	m = drive(m, tickMsg{})

	// A walker arriving via snapshot starts a lobbyWalk animation.
	m = drive(m, server.SnapshotMsg{
		Dojo: 1,
		You:  lobby.Presence{Hash: "aaa", Handle: "alpha", Species: "rootkit", X: 5, Y: 5},
		Others: []lobby.Presence{
			{Hash: "bbb", Handle: "bravo", Species: "emberbyte", X: 8, Y: 6},
		},
	})
	if len(m.lobbyWalk) == 0 {
		// bravo was new (no previous position), so no walk animation is
		// registered; move him to trigger one.
		m = drive(m, server.SnapshotMsg{
			Dojo: 1,
			You:  lobby.Presence{Hash: "aaa", Handle: "alpha", Species: "rootkit", X: 5, Y: 5},
			Others: []lobby.Presence{
				{Hash: "bbb", Handle: "bravo", Species: "emberbyte", X: 9, Y: 6},
			},
		})
	}
	if len(m.lobbyWalk) == 0 {
		t.Fatal("expected a walk animation after movement")
	}

	before := m.frameBuilds
	m = drive(m, tickMsg{})
	if m.frameBuilds <= before {
		t.Fatalf("frameBuilds = %d during walk animation, want > %d", m.frameBuilds, before)
	}
}
