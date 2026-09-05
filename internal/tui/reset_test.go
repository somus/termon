package tui

import (
	"testing"

	"termon.sh/internal/server"
	"termon.sh/internal/store"
)

func newResetTestModel(t *testing.T) (Model, *server.Hub, string) {
	t.Helper()
	set := loadSet(t)
	saves := store.NewMemoryStore()
	saves.UseContent(set)
	hub := server.NewHub(set, saves, server.Admission{OpenAccess: true})
	trainer, err := hub.Authenticate("reset-credential", "10.9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.CompleteOnboard(trainer.ID, "steady-lynx-7", "rootkit"); err != nil {
		t.Fatal(err)
	}
	save, err := hub.Load(trainer.ID)
	if err != nil || save == nil {
		t.Fatalf("onboarded save missing: %v", err)
	}
	m := New(trainer.ID, save, set, hub)
	m.screen = screenLobby
	return m, hub, trainer.ID
}

func TestResetRequiresSecondPress(t *testing.T) {
	m, hub, id := newResetTestModel(t)

	next, cmd := m.Update(press("n"))
	if cmd != nil {
		t.Fatal("first n must not call ResetTrainer")
	}
	armed := next.(Model)
	if armed.status != "press n again to erase your save" {
		t.Fatalf("armed status = %q", armed.status)
	}
	if save, _ := hub.Load(id); save == nil {
		t.Fatal("save wiped by first press")
	}

	fired, cmd := armed.Update(press("n"))
	if cmd == nil {
		t.Fatal("second n must arm the reset command")
	}
	if msg := cmd(); msg != (resetMsg{}) {
		t.Fatalf("cmd msg = %v, want resetMsg", msg)
	}
	wiped := fired.(Model)
	if save, _ := hub.Load(wiped.hash); save != nil {
		t.Fatal("save survived confirmed reset")
	}
}

func TestResetDisarmsOnOtherKeyAndExpiry(t *testing.T) {
	m, hub, id := newResetTestModel(t)

	next, _ := m.Update(press("n"))
	armed := next.(Model)
	next, _ = armed.Update(press("w"))
	afterMove := next.(Model)
	if afterMove.resetArm != 0 {
		t.Fatal("other key must disarm reset")
	}
	if _, cmd := afterMove.Update(press("n")); cmd != nil {
		t.Fatal("disarmed single n wiped the save")
	}
	if save, _ := hub.Load(id); save == nil {
		t.Fatal("save wiped without confirmation")
	}

	next, _ = afterMove.Update(press("n"))
	armedAgain := next.(Model)
	for range holdStatus {
		next, _ = armedAgain.Update(tickMsg{})
		armedAgain = next.(Model)
	}
	if armedAgain.resetArm != 0 {
		t.Fatal("arm hold did not expire")
	}
	if _, cmd := armedAgain.Update(press("n")); cmd != nil {
		t.Fatal("expired arm still wiped the save")
	}
}
