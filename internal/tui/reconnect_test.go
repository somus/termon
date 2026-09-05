package tui

import (
	"strings"
	"testing"

	"termon.sh/internal/server"
)

func TestReconnectingBanner(t *testing.T) {
	m := battleModel(t, nil, 120, 40)
	next, _ := m.Update(server.ReconnectingMsg{Handle: "bravo"})
	m = next.(Model)
	if !strings.Contains(m.chromeMid(), "Reconnecting") {
		t.Fatalf("chrome = %q", m.chromeMid())
	}
	next, _ = m.Update(server.ReconnectingMsg{})
	m = next.(Model)
	if strings.Contains(m.chromeMid(), "Reconnecting") {
		t.Fatal("cleared banner still showing")
	}
}

func TestDeferredBattleActivationClearsReconnectingBanner(t *testing.T) {
	set := loadSet(t)
	current := liveBattle(t, set)
	m := battleModel(t, current, 120, 40)
	m.reconnecting = "bravo"
	m.battle.playing = true

	nextBattle := liveBattle(t, set)
	next, _ := m.Update(server.BattleMsg{
		Battle: nextBattle, You: "aaa", Foe: "bravo", FoeHash: "bbb",
	})
	m = next.(Model)
	if m.battle.session.battle != current {
		t.Fatal("deferred battle replaced the active playback")
	}
	if m.reconnecting == "" {
		t.Fatal("reconnecting banner cleared before the deferred battle activated")
	}

	m.battle.playing = false
	next, _ = m.Update(tickMsg{})
	m = next.(Model)
	if m.battle.session.battle != nextBattle {
		t.Fatal("deferred battle did not activate after playback finished")
	}
	if m.reconnecting != "" {
		t.Fatalf("reconnecting = %q after deferred activation, want empty", m.reconnecting)
	}
}
