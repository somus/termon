// Package onboard creates a Trainer Save from first-run choices.
package onboard

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"termon.sh/internal/content"
	"termon.sh/internal/game"
)

// StarterSlugs is the v1 starter trio, left-to-right in the first-run picker.
var StarterSlugs = []string{"rootkit", "emberbyte", "aquabit"}

const maxHandle = 16

// ValidHandle reports whether a typed handle may be persisted.
func ValidHandle(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len([]rune(s)) > maxHandle {
		return false
	}
	for _, r := range s {
		if r != '-' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// DefaultLoadout is the first four Movepool entries for a Species.
func DefaultLoadout(set *content.Set, slug string) (game.Monster, error) {
	if set == nil {
		return game.Monster{}, errors.New("onboard: nil content")
	}
	sp, ok := set.Species[slug]
	if !ok {
		return game.Monster{}, fmt.Errorf("onboard: unknown species %q", slug)
	}
	if len(sp.Movepool) < 4 {
		return game.Monster{}, fmt.Errorf("onboard: %s movepool too short", slug)
	}
	moves := make([]string, 4)
	for i := range 4 {
		moves[i] = sp.Movepool[i].Move
	}
	return game.Monster{
		Species:       slug,
		Level:         1,
		MoveLibrary:   append([]string(nil), moves...),
		BattleLoadout: append([]string(nil), moves...),
	}, nil
}

// NewSave builds a persisted Trainer record after first-run completes.
func NewSave(set *content.Set, handle, starter string) (*game.Save, error) {
	handle = strings.TrimSpace(handle)
	if !ValidHandle(handle) {
		return nil, errors.New("onboard: invalid handle")
	}
	mon, err := DefaultLoadout(set, starter)
	if err != nil {
		return nil, err
	}
	return &game.Save{
		Handle:     handle,
		Collection: []game.Monster{mon},
	}, nil
}
