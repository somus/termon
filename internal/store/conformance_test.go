package store_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"termon.sh/internal/content"
	"termon.sh/internal/game"
	"termon.sh/internal/store"
)

// factory builds a fresh Store backend for one conformance run.
type factory func(t *testing.T) store.Store

func sqliteFactory(t *testing.T) store.Store {
	t.Helper()
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "termon.db"))
	if err != nil {
		t.Fatal(err)
	}
	s.UseContent(set)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func memoryFactory(t *testing.T) store.Store {
	t.Helper()
	mem := store.NewMemoryStore()
	mem.UseContent(testContent(t))
	return mem
}

// TestStoreConformance pins the backend-neutral Store contract on every
// backend so the in-memory test double cannot drift from SQLite again.
func TestStoreConformance(t *testing.T) {
	for name, build := range map[string]factory{
		"sqlite": sqliteFactory,
		"memory": memoryFactory,
	} {
		t.Run(name, func(t *testing.T) {
			runConformance(t, build(t))
		})
	}
}

func runConformance(t *testing.T, s store.Store) {
	t.Helper()
	scenarios := map[string]func(t *testing.T, s store.Store){
		"resolve unknown credential is not found":     resolveUnknownCredential,
		"create trainer is idempotent per credential": createTrainerIsIdempotent,
		"complete onboarding persists the save":       completeOnboardingPersistsSave,
		"complete onboarding refuses replay":          completeOnboardingRefusesReplay,
		"complete onboarding requires known trainer":  completeOnboardingRequiresKnownTrainer,
		"reset trainer clears save keeps identity":    resetTrainerKeepsIdentity,
		"battle result applies once then idempotent":  battleResultApplyRetryConflict,
		"battle result requires both parties":         battleResultRequiresBothParties,
		"activity lesson capture fills party":         activityLessonCaptureFillsParty,
		"activity expedition capture keeps party":     activityExpeditionCaptureKeepsParty,
		"activity capture pays reserve share":         activityCapturePaysReserveShare,
		"activity result conflicts on outcome change": activityResultConflictsOnOutcomeChange,
		"activity result is idempotent":               activityResultIsIdempotent,
		"loadout rejects empty duplicate unknown":     loadoutRejectsEmptyDuplicateUnknown,
		"accept evolution keeps identity":             acceptEvolutionKeepsIdentity,
	}
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			scenario(t, s)
		})
	}
}

func resolveUnknownCredential(t *testing.T, s store.Store) {
	t.Helper()
	if _, err := s.ResolveCredential("conformance-missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("resolve unknown credential = %v, want ErrNotFound", err)
	}
}

func createTrainerIsIdempotent(t *testing.T, s store.Store) {
	t.Helper()
	first, err := s.CreateTrainer("conformance-idempotent")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateTrainer("conformance-idempotent")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.ID != second.ID {
		t.Fatalf("repeated creation IDs = %q and %q, want one stable ID", first.ID, second.ID)
	}
}

func completeOnboardingPersistsSave(t *testing.T, s store.Store) {
	t.Helper()
	trainer, err := s.CreateTrainer("conformance-happy")
	if err != nil {
		t.Fatal(err)
	}
	want := onboardStarterSave("alpha", "rootkit")
	if err := s.CompleteOnboarding(trainer.ID, want); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadTrainer(trainer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Save == nil || loaded.Save.Handle != "alpha" ||
		len(loaded.Save.Collection) != 1 || loaded.Save.Collection[0].Species != "rootkit" ||
		len(loaded.Save.Collection[0].BattleLoadout) != 4 {
		t.Fatalf("loaded save = %+v, want handle alpha with starter collection", loaded.Save)
	}
}

func completeOnboardingRefusesReplay(t *testing.T, s store.Store) {
	t.Helper()
	trainer, err := s.CreateTrainer("conformance-replay")
	if err != nil {
		t.Fatal(err)
	}
	first := onboardStarterSave("alpha", "rootkit")
	if err := s.CompleteOnboarding(trainer.ID, first); err != nil {
		t.Fatal(err)
	}
	replay := onboardStarterSave("bravo", "emberbyte")
	if err := s.CompleteOnboarding(trainer.ID, replay); !errors.Is(err, store.ErrAlreadyOnboarded) {
		t.Fatalf("replayed onboarding error = %v, want ErrAlreadyOnboarded", err)
	}
	loaded, err := s.LoadTrainer(trainer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Save == nil || loaded.Save.Handle != "alpha" || loaded.Save.Collection[0].Species != "rootkit" {
		t.Fatalf("save after refused replay = %+v, want the original untouched", loaded.Save)
	}
}

func completeOnboardingRequiresKnownTrainer(t *testing.T, s store.Store) {
	t.Helper()
	err := s.CompleteOnboarding("conformance-ghost", onboardStarterSave("alpha", "rootkit"))
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("onboarding unknown Trainer = %v, want ErrNotFound", err)
	}
}

func resetTrainerKeepsIdentity(t *testing.T, s store.Store) {
	t.Helper()
	trainer, err := s.CreateTrainer("conformance-reset")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteOnboarding(trainer.ID, onboardStarterSave("alpha", "rootkit")); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetTrainer(trainer.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadTrainer(trainer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != trainer.ID || loaded.Save != nil {
		t.Fatalf("trainer after reset = %+v, want same ID and no Save", loaded)
	}
	resolved, err := s.ResolveCredential("conformance-reset")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ID != trainer.ID {
		t.Fatalf("credential after reset resolves %q, want %q", resolved.ID, trainer.ID)
	}
	if err := s.ResetTrainer("conformance-ghost"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("reset unknown Trainer = %v, want ErrNotFound", err)
	}
}

func battleResultApplyRetryConflict(t *testing.T, s store.Store) {
	t.Helper()
	winner := onboardConformanceTrainer(t, s, "conformance-winner", "alpha")
	loser := onboardConformanceTrainer(t, s, "conformance-loser", "bravo")
	rec := store.BattleRecord{Result: store.BattleResult{
		ID: "conformance-battle-1", Winner: winner.ID, Loser: loser.ID,
		Reason: "ko", CompletedAt: time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC),
	}}

	records, err := s.RecordBattleResult(rec)
	if err != nil {
		t.Fatal(err)
	}
	if !records.Applied || records.Winner.Wins != 1 || records.Loser.Losses != 1 {
		t.Fatalf("records = %+v, want winner 1-0 and loser 0-1 applied", records)
	}

	retry, err := s.RecordBattleResult(rec)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Applied || retry.Winner.Wins != 1 || retry.Loser.Losses != 1 {
		t.Fatalf("retry records = %+v, want unchanged 1-0 and 0-1 without reapply", retry)
	}

	conflict := rec
	conflict.Result.Winner, conflict.Result.Loser = conflict.Result.Loser, conflict.Result.Winner
	if _, err := s.RecordBattleResult(conflict); !errors.Is(err, store.ErrResultConflict) {
		t.Fatalf("conflicting retry error = %v, want ErrResultConflict", err)
	}
}

func battleResultRequiresBothParties(t *testing.T, s store.Store) {
	t.Helper()
	winner := onboardConformanceTrainer(t, s, "conformance-solo", "alpha")
	_, err := s.RecordBattleResult(store.BattleRecord{Result: store.BattleResult{
		ID: "conformance-battle-2", Winner: winner.ID, Loser: "conformance-ghost",
		Reason: "ko", CompletedAt: time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC),
	}})
	if err == nil {
		t.Fatal("result with unknown loser succeeded, want an error")
	}

	unonboarded, err := s.CreateTrainer("conformance-unonboarded")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.RecordBattleResult(store.BattleRecord{Result: store.BattleResult{
		ID: "conformance-battle-3", Winner: winner.ID, Loser: unonboarded.ID,
		Reason: "ko", CompletedAt: time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC),
	}})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("result with un-onboarded loser = %v, want ErrNotFound family", err)
	}
}

func onboardConformanceTrainer(t *testing.T, s store.Store, credential, handle string) *game.Trainer {
	t.Helper()
	return onboardTrainerGeneric(t, s, credential, handle, "rootkit")
}

// activityLessonCaptureFillsParty pins the lesson-capture contract: the
// captured Monster joins the Collection and takes the first vacant Party slot.
func activityLessonCaptureFillsParty(t *testing.T, s store.Store) {
	t.Helper()
	tr := onboardTrainerGeneric(t, s, "conf-lesson-capture", "alpha", "rootkit")
	save, err := s.RecordActivityResult(store.ActivityRecord{
		Kind: store.KindLesson, NaturalKey: tr.ID + ":lesson:1", TrainerID: tr.ID,
		ActiveIDs: []string{tr.Save.Party[0]}, Outcome: store.OutcomeCaptured,
		Capture:     &store.CaptureSpec{Species: "zaplet", FillParty: true},
		CompletedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(save.Collection) != 2 {
		t.Fatalf("collection = %d, want 2", len(save.Collection))
	}
	if save.Party[1] == "" {
		t.Fatal("lesson capture did not fill party slot")
	}
}

// activityExpeditionCaptureKeepsParty pins FillParty=false: an expedition
// capture mints the Monster but never rewrites the Party.
func activityExpeditionCaptureKeepsParty(t *testing.T, s store.Store) {
	t.Helper()
	tr := onboardTrainerGeneric(t, s, "conf-exp-keep", "alpha", "rootkit")
	partyBefore := tr.Save.Party
	save, err := s.RecordActivityResult(store.ActivityRecord{
		Kind: store.KindExpedition, NaturalKey: tr.ID + ":run1:target", TrainerID: tr.ID,
		ActiveIDs: []string{partyBefore[0]}, Outcome: store.OutcomeCaptured,
		Capture:     &store.CaptureSpec{Species: "zaplet", FillParty: false},
		CompletedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if save.Party != partyBefore {
		t.Fatalf("party changed: %+v vs %+v", save.Party, partyBefore)
	}
}

// activityCapturePaysReserveShare pins the xp-progression.md math on both
// backends: actives get the 65 base plus the 35 capture bonus, reserves get
// 40% of the base alone.
func activityCapturePaysReserveShare(t *testing.T, s store.Store) {
	t.Helper()
	tr := onboardTrainerGeneric(t, s, "conf-reserve", "alpha", "rootkit")
	// Fill Party slot 1 so the record has both an active and a reserve.
	if _, err := s.RecordActivityResult(store.ActivityRecord{
		Kind: store.KindLesson, NaturalKey: tr.ID + ":lesson:setup", TrainerID: tr.ID,
		ActiveIDs: []string{tr.Save.Party[0]}, Outcome: store.OutcomeCaptured,
		Capture:     &store.CaptureSpec{Species: "zaplet", FillParty: true},
		CompletedAt: time.Date(2026, 8, 28, 11, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	tr, err := s.LoadTrainer(tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	activeID, reserveID := tr.Save.Party[0], tr.Save.Party[1]
	xpBefore := collectionXP(tr.Save)

	record := func(key, outcome string) {
		t.Helper()
		_, err := s.RecordActivityResult(store.ActivityRecord{
			Kind: store.KindExpedition, NaturalKey: key, TrainerID: tr.ID,
			ActiveIDs: []string{activeID}, ReserveIDs: []string{reserveID},
			Outcome:     outcome,
			Capture:     &store.CaptureSpec{Species: "mistcache", FillParty: false},
			CompletedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	record("run1:target", store.OutcomeCaptured)
	loaded, err := s.LoadTrainer(tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := collectionXP(loaded.Save)
	if d := got[activeID] - xpBefore[activeID]; d != 100 {
		t.Fatalf("active XP delta = %d, want 100", d)
	}
	if d := got[reserveID] - xpBefore[reserveID]; d != 26 {
		t.Fatalf("reserve XP delta = %d, want 26", d)
	}

	// hunt_failed pays the 65 base with no completion bonus.
	record("run2:target", store.OutcomeHuntFailed)
	loaded, err = s.LoadTrainer(tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	got = collectionXP(loaded.Save)
	if d := got[activeID] - xpBefore[activeID]; d != 165 {
		t.Fatalf("active XP delta after hunt_failed = %d, want 165", d)
	}
	if d := got[reserveID] - xpBefore[reserveID]; d != 52 {
		t.Fatalf("reserve XP delta after hunt_failed = %d, want 52", d)
	}
}

// activityResultConflictsOnOutcomeChange pins ErrResultConflict: a committed
// NaturalKey cannot be recommitted with a different outcome.
func activityResultConflictsOnOutcomeChange(t *testing.T, s store.Store) {
	t.Helper()
	tr := onboardTrainerGeneric(t, s, "conf-conflict", "alpha", "rootkit")
	base := store.ActivityRecord{
		Kind: store.KindExpedition, NaturalKey: tr.ID + ":run2:target", TrainerID: tr.ID,
		ActiveIDs: []string{tr.Save.Party[0]}, Outcome: store.OutcomeHuntFailed,
		CompletedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
	}
	if _, err := s.RecordActivityResult(base); err != nil {
		t.Fatal(err)
	}
	captured := base
	captured.Outcome = store.OutcomeCaptured
	captured.Capture = &store.CaptureSpec{Species: "zaplet"}
	if _, err := s.RecordActivityResult(captured); !errors.Is(err, store.ErrResultConflict) {
		t.Fatalf("conflict = %v, want ErrResultConflict", err)
	}
}

// activityResultIsIdempotent pins replay safety: the same record twice pays once.
func activityResultIsIdempotent(t *testing.T, s store.Store) {
	t.Helper()
	tr := onboardTrainerGeneric(t, s, "conf-idem", "alpha", "rootkit")
	rec := store.ActivityRecord{
		Kind: store.KindLesson, NaturalKey: tr.ID + ":lesson:2", TrainerID: tr.ID,
		ActiveIDs: []string{tr.Save.Party[0]}, Outcome: store.OutcomeCleared,
		CompletedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
	}
	first, err := s.RecordActivityResult(rec)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.RecordActivityResult(rec)
	if err != nil {
		t.Fatal(err)
	}
	if first.Collection[0].XP != second.Collection[0].XP {
		t.Fatal("duplicate activity paid twice")
	}
}

// loadoutRejectsEmptyDuplicateUnknown pins SetBattleLoadout validation.
func loadoutRejectsEmptyDuplicateUnknown(t *testing.T, s store.Store) {
	t.Helper()
	tr := onboardTrainerGeneric(t, s, "conf-loadout", "alpha", "rootkit")
	id := tr.Save.Collection[0].ID
	lib := tr.Save.Collection[0].MoveLibrary
	if _, err := s.SetBattleLoadout(tr.ID, id, nil, nil); !errors.Is(err, store.ErrInvalidLoadout) {
		t.Fatalf("empty loadout = %v", err)
	}
	if _, err := s.SetBattleLoadout(tr.ID, id, []string{lib[0], lib[0]}, nil); !errors.Is(err, store.ErrInvalidLoadout) {
		t.Fatalf("duplicate loadout = %v", err)
	}
	if _, err := s.SetBattleLoadout(tr.ID, id, []string{"not-a-move"}, nil); !errors.Is(err, store.ErrInvalidLoadout) {
		t.Fatalf("unknown move = %v", err)
	}
}

// acceptEvolutionKeepsIdentity pins that AcceptEvolution changes the species
// along the Family chain while keeping the Monster ID stable.
func acceptEvolutionKeepsIdentity(t *testing.T, s store.Store) {
	t.Helper()
	tr := onboardTrainerGeneric(t, s, "conf-evo", "alpha", "rootkit")
	id := tr.Save.Party[0]
	for i := range 20 {
		if _, err := s.RecordActivityResult(store.ActivityRecord{
			Kind: store.KindLesson, NaturalKey: fmt.Sprintf("%s:lesson:evo:%d", tr.ID, i), TrainerID: tr.ID,
			ActiveIDs: []string{id}, Outcome: store.OutcomeCleared,
			CompletedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
		}); err != nil {
			t.Fatal(err)
		}
		loaded, _ := s.LoadTrainer(tr.ID)
		if loaded.Save.Collection[0].EvolutionPending {
			id = loaded.Save.Collection[0].ID
			break
		}
	}
	evolved, err := s.AcceptEvolution(tr.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	if evolved.Collection[0].Species == "rootkit" {
		t.Fatalf("species = %q, want the evolved form", evolved.Collection[0].Species)
	}
	if evolved.Collection[0].ID != id {
		t.Fatal("evolution changed monster id")
	}
}
