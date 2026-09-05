// Package gametest provides Monster and Save fixtures for tests in other
// packages, so test-only constructors never ship in production game code.
package gametest

import (
	"testing"
	"time"

	"termon.sh/internal/game"
	"termon.sh/internal/store"
)

// Starter builds a Level 1 Monster with the given library and loadout.
func Starter(id, species string, moves []string) game.Monster {
	lib := append([]string(nil), moves...)
	load := append([]string(nil), moves...)
	return game.Monster{
		ID:            id,
		Species:       species,
		Level:         1,
		MoveLibrary:   lib,
		BattleLoadout: load,
	}
}

// SaveWithStarter builds a Save with one lead Monster in Collection and Party.
func SaveWithStarter(id, species string, moves []string) *game.Save {
	m := Starter(id, species, moves)
	return &game.Save{
		Handle:     "alpha",
		Collection: []game.Monster{m},
		Party:      [3]string{id, "", ""},
	}
}

// FillParty records two lesson captures so a one-starter save becomes a full
// Party of rootkit, emberbyte, and aquabit. It pins the completion date so
// tests are deterministic across UTC midnights, and returns the final save.
func FillParty(t testing.TB, saves gameStore, trainerID string) *game.Save {
	t.Helper()
	pinned := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	sv := LoadSave(t, saves, trainerID)
	for _, species := range []string{"emberbyte", "aquabit"} {
		var err error
		sv, err = saves.RecordActivityResult(store.ActivityRecord{
			Kind: store.KindLesson, NaturalKey: trainerID + ":fill:" + species,
			TrainerID: trainerID, ActiveIDs: []string{sv.Party[0]}, Outcome: store.OutcomeCaptured,
			Capture:     &store.CaptureSpec{Species: species, FillParty: true},
			CompletedAt: pinned,
		})
		if err != nil {
			t.Fatalf("fill party: %v", err)
		}
	}
	if !game.FullParty(sv) {
		t.Fatalf("party still partial after fill: %+v", sv.Party)
	}
	return sv
}

// LoadSave loads a trainer's save or fails the test.
func LoadSave(t testing.TB, saves gameStore, trainerID string) *game.Save {
	t.Helper()
	tr, err := saves.LoadTrainer(trainerID)
	if err != nil {
		t.Fatal(err)
	}
	return tr.Save
}

// gameStore is the Store surface gametest needs; narrowing the interface keeps
// the package honest about what it touches.
type gameStore interface {
	LoadTrainer(id string) (*game.Trainer, error)
	RecordActivityResult(rec store.ActivityRecord) (*game.Save, error)
}
