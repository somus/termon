package server

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"termon.sh/internal/battle"
	"termon.sh/internal/game"
	"termon.sh/internal/store"
)

func TestErrorMessageCorrelatesUnexpectedFailure(t *testing.T) {
	var logs bytes.Buffer
	hub := NewHub(nil, store.NewMemoryStore(), Admission{})
	hub.Instrument(nil, slog.New(slog.NewJSONHandler(&logs, nil)))
	msg := hub.ErrorMessage("trainer-123", "save_party", errors.New("disk failed"))
	if !strings.Contains(msg.Text, "reference ") {
		t.Fatalf("ErrorMessage = %q", msg.Text)
	}
	reference := strings.TrimPrefix(msg.Text, "something went wrong; reference ")
	for _, want := range []string{"trainer-123", "save_party", reference, "disk failed"} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("log %q does not contain %q", logs.String(), want)
		}
	}
}

func TestUserMessage(t *testing.T) {
	const trainerHash = "9f2b6c1d8a4e"
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil is empty", err: nil, want: ""},
		{
			name: "corrupt save",
			err:  fmt.Errorf("load: %w", store.ErrCorruptSave),
			want: "your save could not be loaded; ask the operator for help",
		},
		{
			name: "save from a newer version",
			err:  store.ErrSaveTooNew,
			want: "your save could not be loaded; ask the operator for help",
		},
		{
			name: "already onboarded",
			err:  store.ErrAlreadyOnboarded,
			want: "this trainer cannot register right now; try reconnecting",
		},
		{
			name: "registration disabled",
			err:  ErrRegistrationDisabled,
			want: "this trainer cannot register right now; try reconnecting",
		},
		{
			name: "too many registrations",
			err:  fmt.Errorf("quota: %w", ErrTooManyRegistrations),
			want: "this trainer cannot register right now; try reconnecting",
		},
		{
			name: "partial party",
			err:  game.ErrPartialParty,
			want: "need a full party of three first",
		},
		{
			name: "unknown trainer",
			err:  fmt.Errorf("resolve: %w", store.ErrNotFound),
			want: "trainer not found; try reconnecting",
		},
		{
			name: "battle already over",
			err:  fmt.Errorf("select: %w", battle.ErrBattleOver),
			want: "that battle is already over",
		},
		{
			name: "move already locked",
			err:  battle.ErrAlreadySelected,
			want: "your move is already locked in",
		},
		{
			name: "player-facing guidance passes through verbatim",
			err:  fmt.Errorf("challenge: %w", playerFacing("stand next to a trainer and press C")),
			want: "stand next to a trainer and press C",
		},
		{
			name: "missing party wraps a Trainer hash and must stay generic",
			err:  fmt.Errorf("server: missing party for %s: %w", trainerHash, errors.New("store down")),
			want: "something went wrong; try again",
		},
		{
			name: "unrecognized internal error stays generic",
			err:  errors.New("server: enter lobby: disk I/O failure"),
			want: "something went wrong; try again",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := UserMessage(test.err)
			if got != test.want {
				t.Fatalf("UserMessage = %q, want %q", got, test.want)
			}
			if strings.Contains(got, trainerHash) {
				t.Fatalf("UserMessage leaked a Trainer hash: %q", got)
			}
		})
	}
}
