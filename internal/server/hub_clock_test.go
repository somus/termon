package server

import (
	"testing"
	"time"

	"termon.sh/internal/battle"
)

// An idle trainer must not stall a PvP Battle forever: once their Decision
// Clock bank runs out, the Battle ends with the decision_timeout reason.
func TestDecisionClockExpiresIdleTrainer(t *testing.T) {
	h := testHub(t)
	onboardTrainerFull(t, h, "a", "rootkit")
	onboardTrainerFull(t, h, "b", "emberbyte")
	if err := h.startMatch("a", "b"); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	m := h.matches["a"]
	h.mu.Unlock()
	if m == nil {
		t.Fatal("no match after startMatch")
	}
	if m.bt.State() != battle.StateAwaitingActions {
		t.Fatalf("state = %q, want awaiting_actions", m.bt.State())
	}
	mode, ok := m.mode.(*pvpMode)
	if !ok {
		t.Fatalf("mode = %T, want pvpMode", m.mode)
	}

	// Arm the clock with one Tick, then simulate a bank long spent by
	// backdating trainer a's deadline.
	now := time.Now()
	h.Tick(now)
	h.mu.Lock()
	mode.clockBank[0] = 30 * time.Second
	mode.clockDeadline[0] = now.Add(-time.Second)
	mode.clockDeadline[1] = now.Add(time.Minute)
	h.mu.Unlock()

	h.Tick(now.Add(time.Second))

	if st := m.bt.State(); st != battle.StateOver {
		t.Fatalf("state = %q, want battle_over after Decision Clock expiry", st)
	}
	if reason := m.bt.Reason(); reason != battle.EndDecisionTimeout {
		t.Fatalf("end reason = %q, want decision_timeout", reason)
	}
	if winner := m.bt.Winner(); winner != "b" {
		t.Fatalf("winner = %q, want the responsive trainer b", winner)
	}
}

// A trainer who never acknowledges the reveal cannot hold the Battle in the
// playback phase: the server-owned window force-advances it.
func TestRevealWindowForceAdvances(t *testing.T) {
	h := testHub(t)
	onboardTrainerFull(t, h, "a", "rootkit")
	onboardTrainerFull(t, h, "b", "emberbyte")
	if err := h.startMatch("a", "b"); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	m := h.matches["a"]
	sv, _ := h.Load("a")
	move := sv.Collection[0].BattleLoadout[0]
	h.mu.Unlock()
	if m == nil {
		t.Fatal("no match after startMatch")
	}
	// Resolve one turn so the Battle enters the revealing playback phase.
	act := battle.Action{Kind: battle.ActionMove, Move: move}
	if err := h.SelectAction("a", act); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	svB, _ := h.Load("b")
	moveB := svB.Collection[0].BattleLoadout[0]
	h.mu.Unlock()
	if err := h.SelectAction("b", battle.Action{Kind: battle.ActionMove, Move: moveB}); err != nil {
		t.Fatal(err)
	}
	if m.bt.State() != battle.StateRevealing {
		t.Fatalf("state = %q, want revealing", m.bt.State())
	}

	now := time.Now()
	h.mu.Lock()
	m.revealDeadline = now.Add(-time.Second)
	h.mu.Unlock()

	h.Tick(now)

	if st := m.bt.State(); st == battle.StateRevealing {
		t.Fatal("battle still revealing past the server-owned window")
	}
}
