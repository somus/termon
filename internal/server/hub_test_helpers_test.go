package server

import (
	"errors"
	"strings"
	"testing"

	"termon.sh/internal/gametest"
)

func assertPartialParty(t testing.TB, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected partial party error")
	}
	var player *PlayerError
	if !errors.As(err, &player) || !strings.Contains(player.Msg, "full party") {
		t.Fatalf("err = %v, want full party error", err)
	}
}

func onboardTrainerFull(t testing.TB, h *Hub, id, starter string) {
	t.Helper()
	onboardTrainer(t, h, id, starter)
	fillTestParty(t, h, id)
}

func assertNotAdjacentChallenge(t testing.TB, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected challenge error")
	}
	var player *PlayerError
	if !errors.As(err, &player) || !strings.Contains(player.Msg, "stand next") {
		t.Fatalf("err = %v, want stand next error", err)
	}
}

func fillTestParty(t testing.TB, h *Hub, hash string) {
	t.Helper()
	gametest.FillParty(t, h.saves, hash)
}
