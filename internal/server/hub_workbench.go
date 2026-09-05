package server

import (
	"termon.sh/internal/game"
	"termon.sh/internal/telemetry"
)

func (h *Hub) workbenchSave(hash, changeKind string, op func() (*game.Save, error)) error {
	sv, err := op()
	if err != nil {
		return err
	}
	var out outbox
	h.mu.Lock()
	h.note(&out, hash, SaveMsg{Save: sv})
	h.mu.Unlock()
	out.flush()
	h.recordEvent(telemetry.Event{
		Name: telemetry.EventWorkbenchChanged, TrainerID: hash,
		Properties: map[string]any{"change_kind": changeKind},
	})
	return nil
}

// SetParty persists Party slot order and pushes SaveMsg to the attached client.
func (h *Hub) SetParty(hash string, party [3]string) error {
	return h.workbenchSave(hash, "party", func() (*game.Save, error) {
		return h.saves.SetParty(hash, party)
	})
}

// SetBattleLoadout persists one Monster's Battle Loadout and clears matching
// progression notices when noticeIDs is non-empty.
func (h *Hub) SetBattleLoadout(hash, monsterID string, moves, noticeIDs []string) error {
	return h.workbenchSave(hash, "battle_loadout", func() (*game.Save, error) {
		return h.saves.SetBattleLoadout(hash, monsterID, moves, noticeIDs)
	})
}

// AcknowledgeProgressionNotices removes open progression notices by ID.
func (h *Hub) AcknowledgeProgressionNotices(hash string, noticeIDs []string) error {
	return h.workbenchSave(hash, "notice_acknowledgement", func() (*game.Save, error) {
		return h.saves.AcknowledgeProgressionNotices(hash, noticeIDs)
	})
}

// SetNickname validates and persists a Monster nickname.
func (h *Hub) SetNickname(hash, monsterID, nickname string) error {
	return h.workbenchSave(hash, "nickname", func() (*game.Save, error) {
		return h.saves.SetNickname(hash, monsterID, nickname)
	})
}

// AcceptEvolution applies a pending Evolution for one Monster.
func (h *Hub) AcceptEvolution(hash, monsterID string) error {
	return h.workbenchSave(hash, "evolution", func() (*game.Save, error) {
		return h.saves.AcceptEvolution(hash, monsterID)
	})
}
