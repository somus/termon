package game

import (
	"errors"
	"fmt"
)

// ErrNoPartyLead means the Party has no occupied lead slot.
var ErrNoPartyLead = errors.New("game: no party lead")

// PartyMembers returns occupied Party slots in order (1–3 Monsters).
func PartyMembers(save *Save) ([]Monster, error) {
	if save == nil {
		return nil, ErrNoPartyLead
	}
	var out []Monster
	for _, id := range save.Party {
		if id == "" {
			continue
		}
		m, ok := MonsterByID(save, id)
		if !ok {
			return nil, fmt.Errorf("game: party references missing monster %q", id)
		}
		out = append(out, m)
	}
	if len(out) == 0 {
		return nil, ErrNoPartyLead
	}
	return out, nil
}

// PartyLead returns the Monster in Party slot 1.
func PartyLead(save *Save) (Monster, error) {
	if save == nil || save.Party[0] == "" {
		return Monster{}, ErrNoPartyLead
	}
	for _, m := range save.Collection {
		if m.ID == save.Party[0] {
			return m, nil
		}
	}
	return Monster{}, ErrNoPartyLead
}

// MonsterByID looks up one Collection entry.
func MonsterByID(save *Save, id string) (Monster, bool) {
	for _, m := range save.Collection {
		if m.ID == id {
			return m, true
		}
	}
	return Monster{}, false
}
