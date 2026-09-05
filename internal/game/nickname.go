package game

import (
	"errors"
	"strings"
	"unicode"
)

// ErrInvalidNickname means the nickname failed validation.
var ErrInvalidNickname = errors.New("game: invalid nickname")

// ValidateNickname trims ends; empty clears; otherwise 1–16 allowed runes.
func ValidateNickname(nickname string) (string, error) {
	trimmed := strings.TrimSpace(nickname)
	if trimmed == "" {
		return "", nil
	}
	runes := []rune(trimmed)
	if len(runes) > 16 {
		return "", ErrInvalidNickname
	}
	for _, r := range runes {
		if r == '-' || r == ' ' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) {
			continue
		}
		return "", ErrInvalidNickname
	}
	return trimmed, nil
}
