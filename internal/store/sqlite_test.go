package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"termon.sh/internal/content"
	"termon.sh/internal/game"
)

func TestSQLiteDefaultUsesNormalWALProfile(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "termon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	metrics, err := s.Metrics()
	if err != nil {
		t.Fatal(err)
	}
	if metrics.JournalMode != "wal" || metrics.Synchronous != "normal" {
		t.Fatalf("SQLite profile = %s/%s, want wal/normal", metrics.JournalMode, metrics.Synchronous)
	}
}

func TestSQLiteTrainerIdentitySurvivesReopen(t *testing.T) {
	database := filepath.Join(t.TempDir(), "termon.db")
	s, err := NewSQLiteStore(database)
	if err != nil {
		t.Fatal(err)
	}
	trainer, err := s.CreateTrainer("credential-a")
	if err != nil {
		t.Fatal(err)
	}
	if trainer.ID == "" || trainer.Save != nil {
		t.Fatalf("new Trainer = %+v, want stable ID and onboarding-required Save", trainer)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = NewSQLiteStore(database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	resolved, err := s.ResolveCredential("credential-a")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ID != trainer.ID || resolved.Save != nil {
		t.Fatalf("resolved Trainer = %+v, want ID %q and onboarding-required Save", resolved, trainer.ID)
	}
}

func TestSQLiteConcurrentCredentialCreationResolvesOneTrainer(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "termon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	var wg sync.WaitGroup
	trainers := make(chan *game.Trainer, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			trainer, err := s.CreateTrainer("credential-a")
			trainers <- trainer
			errs <- err
		})
	}
	wg.Wait()
	close(trainers)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var id string
	for trainer := range trainers {
		if id == "" {
			id = trainer.ID
		}
		if trainer.ID != id {
			t.Fatalf("Trainer IDs differ: %q and %q", id, trainer.ID)
		}
	}
}

func TestSQLiteRecordsBothSidesOfBattle(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "termon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	winner := onboardTrainer(t, s, "credential-a", "alpha")
	loser := onboardTrainer(t, s, "credential-b", "bravo")

	records, err := s.RecordBattleResult(BattleRecord{
		Result: BattleResult{
			ID: "battle-1", Winner: winner.ID, Loser: loser.ID,
			Reason: "ko", CompletedAt: time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !records.Applied || records.Winner.Wins != 1 || records.Winner.Losses != 0 ||
		records.Loser.Wins != 0 || records.Loser.Losses != 1 {
		t.Fatalf("records = %+v, want winner 1-0 and loser 0-1", records)
	}
	loadedWinner, err := s.LoadTrainer(winner.ID)
	if err != nil {
		t.Fatal(err)
	}
	loadedLoser, err := s.LoadTrainer(loser.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedWinner.Save.Wins != 1 || loadedLoser.Save.Losses != 1 {
		t.Fatalf("stored records = winner %+v loser %+v", loadedWinner.Save, loadedLoser.Save)
	}
}

func TestSQLiteBattleResultRetryIsIdempotent(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "termon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	winner := onboardTrainer(t, s, "credential-a", "alpha")
	loser := onboardTrainer(t, s, "credential-b", "bravo")
	result := BattleRecord{Result: BattleResult{
		ID: "battle-1", Winner: winner.ID, Loser: loser.ID,
		Reason: "ko", CompletedAt: time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC),
	}}

	if _, err := s.RecordBattleResult(result); err != nil {
		t.Fatal(err)
	}
	retry, err := s.RecordBattleResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Applied || retry.Winner.Wins != 1 || retry.Loser.Losses != 1 {
		t.Fatalf("retry records = %+v, want unchanged 1-0 and 0-1", retry)
	}
}

func TestSQLiteBattleResultRetryAfterReopenIsIdempotent(t *testing.T) {
	database := filepath.Join(t.TempDir(), "termon.db")
	s, err := NewSQLiteStore(database)
	if err != nil {
		t.Fatal(err)
	}
	winner := onboardTrainer(t, s, "credential-a", "alpha")
	loser := onboardTrainer(t, s, "credential-b", "bravo")
	result := BattleRecord{Result: BattleResult{
		ID: "battle-1", Winner: winner.ID, Loser: loser.ID,
		Reason: "ko", CompletedAt: time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC),
	}}
	if _, err := s.RecordBattleResult(result); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = NewSQLiteStore(database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	retry, err := s.RecordBattleResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Applied || retry.Winner.Wins != 1 || retry.Loser.Losses != 1 {
		t.Fatalf("retry after reopen = %+v, want unchanged records", retry)
	}
}

func TestSQLiteRejectsConflictingBattleResultRetry(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "termon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	winner := onboardTrainer(t, s, "credential-a", "alpha")
	loser := onboardTrainer(t, s, "credential-b", "bravo")
	result := BattleRecord{Result: BattleResult{
		ID: "battle-1", Winner: winner.ID, Loser: loser.ID,
		Reason: "ko", CompletedAt: time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC),
	}}
	if _, err := s.RecordBattleResult(result); err != nil {
		t.Fatal(err)
	}
	result.Result.Winner, result.Result.Loser = result.Result.Loser, result.Result.Winner

	_, err = s.RecordBattleResult(result)
	if !errors.Is(err, ErrResultConflict) {
		t.Fatalf("conflicting retry error = %v, want ErrResultConflict", err)
	}
}

func TestSQLiteBattleResultFailureRollsBack(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "termon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	winner := onboardTrainer(t, s, "credential-a", "alpha")
	loser := onboardTrainer(t, s, "credential-b", "bravo")
	if _, err := s.db.Exec(`CREATE TRIGGER fail_loser
		BEFORE UPDATE OF losses ON trainers
		BEGIN SELECT RAISE(ABORT, 'injected loser failure'); END`); err != nil {
		t.Fatal(err)
	}
	result := BattleRecord{Result: BattleResult{
		ID: "battle-1", Winner: winner.ID, Loser: loser.ID,
		Reason: "ko", CompletedAt: time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC),
	}}

	if _, err := s.RecordBattleResult(result); err == nil {
		t.Fatal("RecordBattleResult succeeded, want injected failure")
	}
	loadedWinner, err := s.LoadTrainer(winner.ID)
	if err != nil {
		t.Fatal(err)
	}
	loadedLoser, err := s.LoadTrainer(loser.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedWinner.Save.Wins != 0 || loadedLoser.Save.Losses != 0 {
		t.Fatalf("records after rollback = winner %+v loser %+v", loadedWinner.Save, loadedLoser.Save)
	}
	if _, err := s.db.Exec("DROP TRIGGER fail_loser"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordBattleResult(result); err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
}

func TestSQLiteConcurrentBattleResultRetryAppliesOnce(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "termon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	winner := onboardTrainer(t, s, "credential-a", "alpha")
	loser := onboardTrainer(t, s, "credential-b", "bravo")
	result := BattleRecord{Result: BattleResult{
		ID: "battle-1", Winner: winner.ID, Loser: loser.ID,
		Reason: "ko", CompletedAt: time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC),
	}}

	var wg sync.WaitGroup
	responses := make(chan ResultRecords, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			records, err := s.RecordBattleResult(result)
			responses <- records
			errs <- err
		})
	}
	wg.Wait()
	close(responses)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	applied := 0
	for records := range responses {
		if records.Applied {
			applied++
		}
		if records.Winner.Wins != 1 || records.Loser.Losses != 1 {
			t.Fatalf("concurrent records = %+v, want exactly one result", records)
		}
	}
	if applied != 1 {
		t.Fatalf("applied count = %d, want 1", applied)
	}
}

func onboardTrainer(t *testing.T, s *SQLiteStore, credential, handle string) *game.Trainer {
	t.Helper()
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	s.UseContent(set)
	trainer, err := s.CreateTrainer(credential)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteOnboarding(trainer.ID, &game.Save{
		Handle:     handle,
		Collection: []game.Monster{{Species: "rootkit"}},
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadTrainer(trainer.ID)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func TestSQLiteTrainerAndWorldStats(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "termon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	winner := onboardTrainer(t, s, "credential-stats-a", "alpha")
	loser := onboardTrainer(t, s, "credential-stats-b", "bravo")
	started := time.Now().UTC().Add(-10 * time.Minute)
	if err := s.StartSession(SessionRecord{
		ID: "session-one", TrainerID: winner.ID, StartedAt: started, AppVersion: "test",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.EndSession("session-one", started.Add(5*time.Minute), "client_disconnect"); err != nil {
		t.Fatal(err)
	}
	for i, at := range []time.Time{started.Add(time.Minute), started.Add(2 * time.Minute)} {
		_, err := s.RecordBattleResult(BattleRecord{Result: BattleResult{
			ID: fmt.Sprintf("stats-battle-%d", i), Winner: winner.ID, Loser: loser.ID,
			Reason: "ko", CompletedAt: at,
		}})
		if err != nil {
			t.Fatal(err)
		}
	}
	stats, err := s.TrainerStats(winner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Wins != 2 || stats.Losses != 0 || stats.CurrentStreak != 2 || stats.LongestStreak != 2 {
		t.Fatalf("battle stats = %+v", stats)
	}
	if stats.Sessions != 1 || stats.PlayTime != 5*time.Minute || stats.CollectionSize != 1 {
		t.Fatalf("session/collection stats = %+v", stats)
	}
	world, err := s.WorldStats()
	if err != nil {
		t.Fatal(err)
	}
	if world.RegisteredTrainers != 2 || world.CompletedBattles != 2 {
		t.Fatalf("world stats = %+v", world)
	}
}

func TestSQLiteRejectsNewerSchema(t *testing.T) {
	database := filepath.Join(t.TempDir(), "termon.db")
	db, err := sql.Open("sqlite", database)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO schema_migrations(version) VALUES (?)", currentSchemaVersion+1); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = NewSQLiteStore(database)
	if !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("open newer schema error = %v, want ErrSchemaTooNew", err)
	}
}

func TestSQLiteRejectsNewerSaveVersion(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "termon.db"))
	if err != nil {
		t.Fatal(err)
	}
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	s.UseContent(set)
	t.Cleanup(func() { _ = s.Close() })
	trainer, err := s.CreateTrainer("credential-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteOnboarding(trainer.ID, &game.Save{
		Handle:     "alpha",
		Collection: []game.Monster{{Species: "rootkit"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(
		"UPDATE trainers SET save_version = ? WHERE id = ?",
		currentSaveVersion+1, trainer.ID,
	); err != nil {
		t.Fatal(err)
	}

	_, err = s.LoadTrainer(trainer.ID)
	if !errors.Is(err, ErrSaveTooNew) {
		t.Fatalf("load newer Save error = %v, want ErrSaveTooNew", err)
	}
}

func TestSQLiteRejectsNewerSaveVersionAtStartup(t *testing.T) {
	database := filepath.Join(t.TempDir(), "termon.db")
	s, err := NewSQLiteStore(database)
	if err != nil {
		t.Fatal(err)
	}
	trainer, err := s.CreateTrainer("credential-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(
		"UPDATE trainers SET save_version = ?, save_payload = '{}' WHERE id = ?",
		currentSaveVersion+1, trainer.ID,
	); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = NewSQLiteStore(database)
	if !errors.Is(err, ErrSaveTooNew) {
		t.Fatalf("open newer Save error = %v, want ErrSaveTooNew", err)
	}
}

func TestSQLiteReadinessReportsCompatibilityAndIntegrity(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "termon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	health, err := s.Readiness(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if health.SchemaVersion != currentSchemaVersion || health.SupportedSchemaVersion != currentSchemaVersion {
		t.Fatalf("schema versions = %d/%d, want %d/%d", health.SchemaVersion, health.SupportedSchemaVersion, currentSchemaVersion, currentSchemaVersion)
	}
	if health.SupportedSaveVersion != currentSaveVersion || health.Integrity != "ok" || health.JournalMode != "wal" {
		t.Fatalf("readiness = %+v", health)
	}
}

func TestSQLiteRejectsCorruptSavePayload(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "termon.db"))
	if err != nil {
		t.Fatal(err)
	}
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	s.UseContent(set)
	t.Cleanup(func() { _ = s.Close() })
	trainer, err := s.CreateTrainer("credential-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteOnboarding(trainer.ID, &game.Save{
		Handle:     "alpha",
		Collection: []game.Monster{{Species: "rootkit"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(
		"UPDATE trainers SET save_payload = ? WHERE id = ?",
		[]byte("not-json"), trainer.ID,
	); err != nil {
		t.Fatal(err)
	}

	_, err = s.LoadTrainer(trainer.ID)
	if !errors.Is(err, ErrCorruptSave) {
		t.Fatalf("load corrupt Save error = %v, want ErrCorruptSave", err)
	}
}

func TestSQLiteOnboardingSaveSurvivesReopen(t *testing.T) {
	database := filepath.Join(t.TempDir(), "termon.db")
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSQLiteStore(database)
	if err != nil {
		t.Fatal(err)
	}
	s.UseContent(set)
	trainer, err := s.CreateTrainer("credential-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteOnboarding(trainer.ID, &game.Save{
		Handle: "alpha", Wins: 2, Losses: 1,
		Collection: []game.Monster{{Species: "rootkit"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = NewSQLiteStore(database)
	if err != nil {
		t.Fatal(err)
	}
	s.UseContent(set)
	t.Cleanup(func() { _ = s.Close() })
	loaded, err := s.LoadTrainer(trainer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != trainer.ID || loaded.Save.Handle != "alpha" ||
		loaded.Save.Wins != 2 || loaded.Save.Losses != 1 ||
		len(loaded.Save.Collection) != 1 || loaded.Save.Collection[0].Species != "rootkit" {
		t.Fatalf("loaded save = %+v", loaded.Save)
	}
}

func TestSQLiteResetPreservesTrainerIdentity(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "termon.db"))
	if err != nil {
		t.Fatal(err)
	}
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	s.UseContent(set)
	t.Cleanup(func() { _ = s.Close() })
	trainer, err := s.CreateTrainer("credential-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteOnboarding(trainer.ID, &game.Save{
		Handle: "alpha", Wins: 4, Losses: 2,
		Collection: []game.Monster{{Species: "rootkit"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetTrainer(trainer.ID); err != nil {
		t.Fatal(err)
	}

	resolved, err := s.ResolveCredential("credential-a")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ID != trainer.ID || resolved.Save != nil {
		t.Fatalf("reset Trainer = %+v, want preserved ID %q and onboarding-required Save", resolved, trainer.ID)
	}
}

func TestLoadTrainerPreservesNickname(t *testing.T) {
	database := filepath.Join(t.TempDir(), "termon.db")
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSQLiteStore(database)
	if err != nil {
		t.Fatal(err)
	}
	s.UseContent(set)
	defer func() { _ = s.Close() }()
	trainer := onboardTrainer(t, s, "credential-nickname", "nick")
	mid := trainer.Save.Collection[0].ID

	db, err := sql.Open("sqlite", database)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	payload := fmt.Sprintf(`{"collection":[{"id":"%s","species":"rootkit","nickname":"Mossy","xp":0,"level":1,"move_library":["root_access","spaghetti_code","trim","big_bang_deploy"],"battle_loadout":["root_access"]}],"party":["%s","",""],"notices":[]}`, mid, mid)
	if _, err := db.Exec(
		`UPDATE trainers SET save_payload = ? WHERE id = ?`,
		[]byte(payload), trainer.ID,
	); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.LoadTrainer(trainer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Save.Collection[0].Nickname != "Mossy" {
		t.Fatalf("nickname = %q, want Mossy", loaded.Save.Collection[0].Nickname)
	}
	m := loaded.Save.Collection[0]
	if fmt.Sprint(m.MoveLibrary) != fmt.Sprint([]string{"root_access", "chmod", "sudo", "setuid"}) {
		t.Fatalf("retired loadout not reminted: %v", m.MoveLibrary)
	}
}

func TestSQLiteCompleteOnboardingRefusesReplay(t *testing.T) {
	database := filepath.Join(t.TempDir(), "termon.db")
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSQLiteStore(database)
	if err != nil {
		t.Fatal(err)
	}
	s.UseContent(set)
	trainer, err := s.CreateTrainer("credential-a")
	if err != nil {
		t.Fatal(err)
	}
	first := &game.Save{
		Handle: "alpha", Wins: 2, Losses: 1,
		Collection: []game.Monster{{Species: "rootkit"}},
	}
	if err := s.CompleteOnboarding(trainer.ID, first); err != nil {
		t.Fatal(err)
	}
	replayed := &game.Save{
		Handle: "beta", Wins: 99, Losses: 99,
		Collection: []game.Monster{{Species: "emberbyte"}},
	}
	if err := s.CompleteOnboarding(trainer.ID, replayed); !errors.Is(err, ErrAlreadyOnboarded) {
		t.Fatalf("replayed onboarding = %v, want ErrAlreadyOnboarded", err)
	}
	got, err := s.LoadTrainer(trainer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Save == nil {
		t.Fatal("onboarded trainer lost their Save")
	}
	if got.Save.Handle != "alpha" || got.Save.Wins != 2 || got.Save.Losses != 1 {
		t.Fatalf("save after replay = %+v, original Save was clobbered", got.Save)
	}
	if err := s.CompleteOnboarding("missing-id", replayed); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown trainer = %v, want ErrNotFound", err)
	}
}

func TestSQLiteSynchronousModeIsCaseInsensitive(t *testing.T) {
	for _, mode := range []string{"normal", "NORMAL", "Normal", " full ", "FULL"} {
		database := filepath.Join(t.TempDir(), "termon.db")
		s, err := OpenSQLiteStore(database, SQLiteOptions{Synchronous: SQLiteSynchronous(mode)})
		if err != nil {
			t.Fatalf("mode %q: %v", mode, err)
		}
		_ = s.Close()
	}
	if _, err := OpenSQLiteStore(filepath.Join(t.TempDir(), "termon.db"), SQLiteOptions{Synchronous: "extra"}); err == nil {
		t.Fatal("unsupported mode accepted")
	}
}

// Regression: relative database paths must open cleanly (termond's default
// is data/termon.db); the file: URI needs an absolute path.
func TestSQLiteOpensRelativePath(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	s, err := OpenSQLiteStore(filepath.Join("data", "termon.db"), SQLiteOptions{Synchronous: SQLiteSynchronousNormal})
	if err != nil {
		t.Fatalf("relative path: %v", err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateTrainer("credential-a"); err != nil {
		t.Fatal(err)
	}
}

// Pre-collection Saves stored the party as monster objects. Login must still
// load those rows after the payload became collection + party IDs.
func TestSQLiteLoadsPreCollectionPartySave(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "termon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	s.UseContent(set)

	trainer, err := s.CreateTrainer("legacy-key")
	if err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"party":[{"species":"rootkit","nickname":"","moves":["root_access","spaghetti_code","trim","big_bang_deploy"]}]}`)
	if _, err := s.db.Exec(
		`UPDATE trainers SET handle = ?, wins = 1, losses = 0, save_version = 1, save_payload = ? WHERE id = ?`,
		"lucky-mole-80", legacy, trainer.ID,
	); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.LoadTrainer(trainer.ID)
	if err != nil {
		t.Fatalf("load legacy save: %v", err)
	}
	if loaded.Save == nil {
		t.Fatal("legacy save decoded as onboarding-required")
	}
	if loaded.Save.Handle != "lucky-mole-80" || loaded.Save.Wins != 1 || loaded.Save.Losses != 0 {
		t.Fatalf("record = %+v, want lucky-mole-80 1-0", loaded.Save)
	}
	if len(loaded.Save.Collection) != 1 {
		t.Fatalf("collection = %d, want 1", len(loaded.Save.Collection))
	}
	m := loaded.Save.Collection[0]
	if m.ID == "" || m.Species != "rootkit" || m.Level != 1 || m.XP != 0 {
		t.Fatalf("monster = %+v, want minted rootkit at level 1", m)
	}
	wantMoves := []string{"root_access", "chmod", "sudo", "setuid"}
	if fmt.Sprint(m.MoveLibrary) != fmt.Sprint(wantMoves) || fmt.Sprint(m.BattleLoadout) != fmt.Sprint(wantMoves) {
		t.Fatalf("moves lib=%v loadout=%v, want current L1 movepool %v", m.MoveLibrary, m.BattleLoadout, wantMoves)
	}
	if loaded.Save.Party != [3]string{m.ID, "", ""} {
		t.Fatalf("party = %v, want [%q,\"\",\"\"]", loaded.Save.Party, m.ID)
	}

	again, err := s.LoadTrainer(trainer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Save.Collection[0].ID != m.ID {
		t.Fatalf("monster id changed on reload: %q then %q", m.ID, again.Save.Collection[0].ID)
	}

	resolved, err := s.ResolveCredential("legacy-key")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Save.Collection[0].ID != m.ID {
		t.Fatalf("resolve monster id = %q, want %q", resolved.Save.Collection[0].ID, m.ID)
	}
}
