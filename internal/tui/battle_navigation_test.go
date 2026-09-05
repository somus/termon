package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"termon.sh/internal/battle"
	"termon.sh/internal/server"
)

func TestBattleCompletionReturnsToDojo(t *testing.T) {
	h, set, s := testHub(t)
	var inbox []any
	h.Attach("aaa", func(msg any) { inbox = append(inbox, msg) }, func() {})
	h.Attach("bbb", func(any) {}, func() {})
	sv := onboardTrainer(t, h, s, "aaa", "alpha", "rootkit")
	onboardTrainer(t, h, s, "bbb", "bravo", "emberbyte")
	if err := h.StartMatch("aaa", "bbb"); err != nil {
		t.Fatal(err)
	}
	if err := h.Forfeit("aaa"); err != nil {
		t.Fatal(err)
	}
	var msg server.BattleMsg
	for _, item := range inbox {
		if b, ok := item.(server.BattleMsg); ok {
			msg = b
		}
	}
	m := New("aaa", sv, set, h)
	m.width, m.height = 120, 40
	next, _ := m.Update(msg)
	m = next.(Model)
	if m.screen != screenBattle || m.battle.session.battle.State() != battle.StateOver {
		t.Fatal("expected completed battle")
	}
	drainPlayback(&m)
	if m.battle.resultHold == 0 {
		t.Fatal("results should hold before Enter is accepted")
	}
	next, cmd := m.Update(press("enter"))
	m = next.(Model)
	if cmd != nil {
		t.Fatal("enter during the results hold must not leave")
	}
	m.battle.resultHold = 0
	m.battle.playing = false
	m.battle.playPause = false
	m.battle.revealPending = false
	next, _ = m.Update(server.SnapshotMsg{You: lobbyPresence(false)})
	m = next.(Model)
	if m.screen != screenBattle {
		t.Fatal("KO/forfeit snapshot must not dump the trainer before results")
	}
	next, _ = m.Update(tea.KeyReleaseMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.screen != screenBattle {
		t.Fatal("key release must not leave the results screen")
	}
	next, cmd = m.Update(press("enter"))
	if cmd == nil {
		state := "nil"
		if m.battle.session.battle != nil {
			state = string(m.battle.session.battle.State())
		}
		t.Fatalf("expected return-to-dojo command (state=%s playing=%v resultHold=%d intro=%v)",
			state, m.battle.playing, m.battle.resultHold, m.battle.battleIntro)
	}
	out := cmd()
	next, _ = next.(Model).Update(out)
	m = next.(Model)
	if m.screen != screenLobby {
		t.Fatalf("screen %d, want lobby", m.screen)
	}
}
