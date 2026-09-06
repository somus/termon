package tui

import (
	"testing"

	"termon.sh/internal/gametest"
	"termon.sh/internal/lobby"
)

func TestMovementIntentsApplyInInputOrder(t *testing.T) {
	hub, set, saves := testHub(t)
	onboardTrainer(t, hub, saves, "aaa", "alpha", "rootkit")
	for range 7 {
		if err := hub.Move("aaa", lobby.West); err != nil {
			t.Fatal(err)
		}
	}
	m := New("aaa", gametest.LoadSave(t, saves, "aaa"), set, hub)
	m.snap = hub.Snapshot("aaa")
	first, right := m.Update(press("d"))
	_, left := first.(Model).Update(press("a"))
	// Independent Bubble Tea commands may run in either order. Reversing
	// them at the west wall must not reverse the authoritative move intents.
	if left != nil {
		_ = left()
	}
	if right != nil {
		_ = right()
	}
	if got := hub.Snapshot("aaa").You.X; got != 1 {
		t.Fatalf("input d,a ended at x=%d, want 1", got)
	}
}

func TestMovementPaintsInTheInputUpdate(t *testing.T) {
	hub, set, saves := testHub(t)
	onboardTrainer(t, hub, saves, "aaa", "alpha", "rootkit")
	m := New("aaa", gametest.LoadSave(t, saves, "aaa"), set, hub)
	m.width, m.height = 120, 40
	m = drive(m, hub.Snapshot("aaa"))
	before := m.frameBuilds
	m = drive(m, press("d"))
	if m.snap.You.X != 9 || m.frameBuilds != before+1 {
		t.Fatal("move did not paint the authoritative result in the input update")
	}
}

func TestOlderSnapshotCannotRewindMovement(t *testing.T) {
	hub, set, saves := testHub(t)
	save := onboardTrainer(t, hub, saves, "aaa", "alpha", "rootkit")
	older := hub.Snapshot("aaa")
	if err := hub.Move("aaa", lobby.East); err != nil {
		t.Fatal(err)
	}
	newer := hub.Snapshot("aaa")
	m := New("aaa", save, set, hub)
	m.width, m.height = 120, 40
	m = drive(m, newer)
	frame := m.View().Content
	m = drive(m, older)
	if m.snap.You.X != newer.You.X || m.snap.You.Y != newer.You.Y {
		t.Fatalf("late broadcast rewound position: got %d,%d want %d,%d", m.snap.You.X, m.snap.You.Y, newer.You.X, newer.You.Y)
	}
	if m.View().Content != frame {
		t.Fatal("late snapshot changed the visible frame")
	}
}

func TestOlderSnapshotCannotRemoveJoinedTrainer(t *testing.T) {
	hub, set, saves := testHub(t)
	save := onboardTrainer(t, hub, saves, "aaa", "alpha", "rootkit")
	older := hub.Snapshot("aaa")
	onboardTrainer(t, hub, saves, "bbb", "bravo", "emberbyte")
	newer := hub.Snapshot("aaa")
	m := New("aaa", save, set, hub)
	m = drive(m, newer)
	m = drive(m, older)
	if len(m.snap.Others) != len(newer.Others) {
		t.Fatal("late snapshot removed a joined Trainer")
	}
}
