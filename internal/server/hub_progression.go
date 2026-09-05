package server

import (
	"fmt"
	"time"

	"termon.sh/internal/content"
	"termon.sh/internal/game"
	"termon.sh/internal/store"
)

// ProgressionEntry is one Monster's reward line in the summary screen.
type ProgressionEntry struct {
	Slot             int
	MonsterID        string
	Name             string
	Species          string
	XPGained         int64
	TotalXP          int64
	Level            int
	Unlocked         []string
	EvolutionPending bool
	Share            string // active | reserve | ""
}

// ProgressionMsg follows a persisted reward packet.
type ProgressionMsg struct {
	Entries []ProgressionEntry
}

func progressionDiff(before, after *game.Save, active, reserve []string, set *content.Set) ProgressionMsg {
	activeSet := map[string]struct{}{}
	for _, id := range active {
		activeSet[id] = struct{}{}
	}
	reserveSet := map[string]struct{}{}
	for _, id := range reserve {
		if _, ok := activeSet[id]; !ok {
			reserveSet[id] = struct{}{}
		}
	}
	var msg ProgressionMsg
	for slot, id := range after.Party {
		if id == "" {
			continue
		}
		bm, bok := game.MonsterByID(before, id)
		am, aok := game.MonsterByID(after, id)
		if !aok {
			continue
		}
		entry := ProgressionEntry{
			Slot: slot + 1, MonsterID: id, Species: am.Species,
			TotalXP: am.XP, Level: am.Level, EvolutionPending: am.EvolutionPending,
		}
		if am.Nickname != "" {
			entry.Name = am.Nickname
		} else if set != nil {
			if sp, ok := set.Species[am.Species]; ok {
				entry.Name = sp.Name
			}
		}
		if bok {
			entry.XPGained = am.XP - bm.XP
			entry.Unlocked = diffUnlocks(bm.MoveLibrary, am.MoveLibrary)
		}
		if _, ok := activeSet[id]; ok {
			entry.Share = "active"
		} else if _, ok := reserveSet[id]; ok {
			entry.Share = "reserve"
		}
		if entry.XPGained > 0 || len(entry.Unlocked) > 0 || entry.EvolutionPending {
			msg.Entries = append(msg.Entries, entry)
		}
	}
	return msg
}

func diffUnlocks(before, after []string) []string {
	seen := map[string]struct{}{}
	for _, s := range before {
		seen[s] = struct{}{}
	}
	var out []string
	for _, s := range after {
		if _, ok := seen[s]; !ok {
			out = append(out, s)
		}
	}
	return out
}

func cloneSaveXPView(save *game.Save) *game.Save {
	if save == nil {
		return nil
	}
	cp := *save
	cp.Collection = make([]game.Monster, len(save.Collection))
	copy(cp.Collection, save.Collection)
	return &cp
}

func (h *Hub) persistLessonCapture(mode *lessonMode, hash string, species string, active, reserve []string) error {
	before := cloneSaveXPView(mode.saveBefore)
	key := fmt.Sprintf("%s:lesson:%d", hash, mode.lesson)
	rec := store.ActivityRecord{
		Kind: store.KindLesson, NaturalKey: key, TrainerID: hash,
		ActiveIDs: active, ReserveIDs: reserve, Outcome: store.OutcomeCaptured,
		Capture:     &store.CaptureSpec{Species: species, FillParty: true},
		CompletedAt: time.Now(),
	}
	after, err := h.recordActivityResult(rec)
	if err != nil {
		return err
	}
	mode.saveBefore = after
	var out outbox
	h.mu.Lock()
	h.note(&out, hash, SaveMsg{Save: after})
	h.note(&out, hash, progressionDiff(before, after, active, reserve, h.set))
	h.mu.Unlock()
	out.flush()
	return nil
}
