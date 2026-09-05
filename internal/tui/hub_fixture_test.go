package tui

import (
	"testing"

	"termon.sh/internal/content"
	"termon.sh/internal/game"
	"termon.sh/internal/gametest"
	"termon.sh/internal/lobby"
	"termon.sh/internal/server"
	"termon.sh/internal/store"
)

func lobbyPresence(inBattle bool) lobby.Presence {
	return lobby.Presence{Hash: "aaa", Handle: "alpha", InBattle: inBattle}
}

func newTestStore(t *testing.T) *store.MemoryStore {
	t.Helper()
	s := store.NewMemoryStore()
	s.UseContent(loadSet(t))
	return s
}

func testHub(t *testing.T) (*server.Hub, *content.Set, *store.MemoryStore) {
	t.Helper()
	set := loadSet(t)
	s := newTestStore(t)
	return server.NewHub(set, s, server.Admission{OpenAccess: true, RegistrationsPerIP: -1}), set, s
}

// onboard creates the Trainer behind credential id, then completes
// onboarding with the given handle and starter.
func onboardTrainer(t *testing.T, h *server.Hub, s *store.MemoryStore, id, handle, starter string) *game.Save {
	t.Helper()
	trainer, err := h.Authenticate(id, "10.0.0.1")
	if err != nil {
		t.Fatalf("authenticate %s: %v", id, err)
	}
	sv, err := h.CompleteOnboard(trainer.ID, handle, starter)
	if err != nil {
		t.Fatalf("onboard %s: %v", id, err)
	}
	fillTUIParty(t, s, id)
	return sv
}

// fillTUIParty fills the trainer's Party through the shared gametest fixture.
func fillTUIParty(t *testing.T, s *store.MemoryStore, hash string) {
	t.Helper()
	gametest.FillParty(t, s, hash)
}
