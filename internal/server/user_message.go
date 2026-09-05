package server

import (
	"errors"
	"strings"

	"termon.sh/internal/battle"
	"termon.sh/internal/game"
	"termon.sh/internal/store"
	"termon.sh/internal/telemetry"
)

// PlayerError marks a message written to be shown to players verbatim.
type PlayerError struct{ Msg string }

func (e *PlayerError) Error() string { return e.Msg }

// playerFacing wraps guidance text as a player-safe error.
func playerFacing(msg string) error { return &PlayerError{Msg: msg} }

// ErrorMessage returns player-safe guidance and adds a searchable reference to
// unexpected failures while logging the same reference with correlation IDs.
func (h *Hub) ErrorMessage(trainerID, operation string, err error) ErrorMsg {
	text := UserMessage(err)
	if text != "something went wrong; try again" {
		return ErrorMsg{Text: text}
	}
	raw := strings.ToUpper(strings.ReplaceAll(telemetry.NewID(), "-", ""))
	reference := raw[:4] + "-" + raw[4:8]
	if h.logger != nil {
		h.logger.Error("player operation failed",
			"operation", operation, "trainer_id", trainerID,
			"session_id", h.sessionID(trainerID), "error_id", reference, "err", err)
	}
	return ErrorMsg{Text: "something went wrong; reference " + reference}
}

// UserMessage translates an error into a short, player-safe status line.
// Errors carrying internal store/engine detail collapse into generic text;
// guidance marked with PlayerError passes through untouched.
func UserMessage(err error) string {
	if err == nil {
		return ""
	}
	var player *PlayerError
	switch {
	case errors.As(err, &player):
		return player.Msg
	case errors.Is(err, store.ErrCorruptSave), errors.Is(err, store.ErrSaveTooNew):
		return "your save could not be loaded; ask the operator for help"
	case errors.Is(err, store.ErrAlreadyOnboarded), errors.Is(err, ErrRegistrationDisabled),
		errors.Is(err, ErrTooManyRegistrations):
		return "this trainer cannot register right now; try reconnecting"
	case errors.Is(err, game.ErrPartialParty):
		return "need a full party of three first"
	case errors.Is(err, store.ErrNotFound):
		return "trainer not found; try reconnecting"
	case errors.Is(err, battle.ErrBattleOver):
		return "that battle is already over"
	case errors.Is(err, battle.ErrAlreadySelected):
		return "your move is already locked in"
	default:
		return "something went wrong; try again"
	}
}
