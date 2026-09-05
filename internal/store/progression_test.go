package store_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"termon.sh/internal/game"
	"termon.sh/internal/store"
)

func TestSchemaVersion3CreatesActivityResults(t *testing.T) {
	database := filepath.Join(t.TempDir(), "termon.db")
	s, err := store.NewSQLiteStore(database)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	db, err := sql.Open("sqlite", database)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var name string
	if err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='activity_results'`,
	).Scan(&name); err != nil {
		t.Fatalf("activity_results table: %v", err)
	}
}

func TestCompleteOnboardingCollectionParty(t *testing.T) {
	s := testStoreWithContent(t)
	trainer, err := s.CreateTrainer("onboard-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteOnboarding(trainer.ID, onboardStarterSave("alpha", "rootkit")); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadTrainer(trainer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Save.Collection) != 1 {
		t.Fatalf("collection = %d, want 1", len(loaded.Save.Collection))
	}
	m := loaded.Save.Collection[0]
	if m.Level != 1 || m.XP != 0 {
		t.Fatalf("starter level/xp = %d/%d, want 1/0", m.Level, m.XP)
	}
	if len(m.MoveLibrary) != 4 || len(m.BattleLoadout) != 4 {
		t.Fatalf("starter moves = lib %d loadout %d, want 4/4", len(m.MoveLibrary), len(m.BattleLoadout))
	}
	if loaded.Save.Party != [3]string{m.ID, "", ""} {
		t.Fatalf("party = %v, want [%q,\"\",\"\"]", loaded.Save.Party, m.ID)
	}
	if len(loaded.Save.Notices) != 0 {
		t.Fatalf("notices = %d, want 0", len(loaded.Save.Notices))
	}
}

func TestSetPartyValidation(t *testing.T) {
	s := testStoreWithContent(t)
	tr := onboardTrainerGeneric(t, s, "party-a", "alpha", "rootkit")
	id := tr.Save.Collection[0].ID
	if _, err := s.SetParty(tr.ID, [3]string{id, id, ""}); !errors.Is(err, store.ErrInvalidParty) {
		t.Fatalf("duplicate party = %v, want ErrInvalidParty", err)
	}
	if _, err := s.SetParty(tr.ID, [3]string{"missing", "", ""}); !errors.Is(err, store.ErrInvalidParty) {
		t.Fatalf("unknown party id = %v, want ErrInvalidParty", err)
	}
	if _, err := s.SetParty(tr.ID, [3]string{"", id, ""}); err != nil {
		t.Fatal(err)
	}
}

func TestSetNicknameValidation(t *testing.T) {
	s := testStoreWithContent(t)
	tr := onboardTrainerGeneric(t, s, "nick-a", "alpha", "rootkit")
	id := tr.Save.Collection[0].ID
	if _, err := s.SetNickname(tr.ID, id, "  Moss  "); err != nil {
		t.Fatal(err)
	}
	loaded, _ := s.LoadTrainer(tr.ID)
	if loaded.Save.Collection[0].Nickname != "Moss" {
		t.Fatalf("nickname = %q", loaded.Save.Collection[0].Nickname)
	}
	if _, err := s.SetNickname(tr.ID, id, "   "); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetNickname(tr.ID, id, "bad!"); err == nil {
		t.Fatal("invalid nickname accepted")
	}
}

// collectionXP maps each Monster ID to its current XP total.
func collectionXP(save *game.Save) map[string]int64 {
	xp := make(map[string]int64, len(save.Collection))
	for _, m := range save.Collection {
		xp[m.ID] = m.XP
	}
	return xp
}

func TestRecordBattleResultForfeitNoXP(t *testing.T) {
	s := testStoreWithContent(t)
	w := onboardTrainerGeneric(t, s, "forfeit-w", "alpha", "rootkit")
	l := onboardTrainerGeneric(t, s, "forfeit-l", "bravo", "emberbyte")
	wID := w.Save.Party[0]
	lID := l.Save.Party[0]
	_, err := s.RecordBattleResult(store.BattleRecord{
		Result: store.BattleResult{
			ID: "forfeit-1", Winner: w.ID, Loser: l.ID,
			Reason: "forfeit", CompletedAt: time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC),
		},
		WinnerActive: []string{wID}, LoserActive: []string{lID},
		ApplyRewards: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	wLoaded, _ := s.LoadTrainer(w.ID)
	lLoaded, _ := s.LoadTrainer(l.ID)
	if wLoaded.Save.Wins != 1 || lLoaded.Save.Losses != 1 {
		t.Fatal("w/l not updated")
	}
	if wLoaded.Save.Collection[0].XP != 0 || lLoaded.Save.Collection[0].XP != 0 {
		t.Fatal("forfeit paid xp")
	}
}

func TestRecordBattleResultApplyRewardsXP(t *testing.T) {
	s := testStoreWithContent(t)
	w := onboardTrainerGeneric(t, s, "xp-w", "alpha", "rootkit")
	l := onboardTrainerGeneric(t, s, "xp-l", "bravo", "emberbyte")
	_, err := s.RecordBattleResult(store.BattleRecord{
		Result: store.BattleResult{
			ID: "xp-1", Winner: w.ID, Loser: l.ID,
			Reason: "ko", CompletedAt: time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC),
		},
		WinnerActive: []string{w.Save.Party[0]}, LoserActive: []string{l.Save.Party[0]},
		ApplyRewards: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	wLoaded, _ := s.LoadTrainer(w.ID)
	if wLoaded.Save.Collection[0].XP == 0 {
		t.Fatal("winner received no xp")
	}
}

func TestResetTrainerClearsActivityResults(t *testing.T) {
	s := testStoreWithContent(t)
	tr := onboardTrainerGeneric(t, s, "reset-act", "alpha", "rootkit")
	key := tr.ID + ":lesson:3"
	if _, err := s.RecordActivityResult(store.ActivityRecord{
		Kind: "lesson", NaturalKey: key, TrainerID: tr.ID,
		ActiveIDs: []string{tr.Save.Party[0]}, Outcome: "cleared",
		CompletedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetTrainer(tr.ID); err != nil {
		t.Fatal(err)
	}
	ok, err := s.ActivityExists(tr.ID, key)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("activity_results survived reset")
	}
}

func onboardTrainerGeneric(t *testing.T, s store.Store, credential, handle, species string) *game.Trainer {
	t.Helper()
	trainer, err := s.CreateTrainer(credential)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteOnboarding(trainer.ID, onboardStarterSave(handle, species)); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadTrainer(trainer.ID)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}
