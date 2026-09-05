package store

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"termon.sh/internal/content"
	"termon.sh/internal/game"
)

func TestPlanActivityTransitionCaptureAndRewards(t *testing.T) {
	set := loadTransitionContent(t)
	starter, err := monsterForSpecies(set, "rootkit", "starter")
	if err != nil {
		t.Fatal(err)
	}
	current := &game.Save{
		Handle: "alpha", Collection: []game.Monster{starter}, Party: [3]string{starter.ID, "", ""},
	}
	rec := ActivityRecord{
		Kind: KindLesson, NaturalKey: "trainer:lesson:1", TrainerID: "trainer",
		ActiveIDs: []string{starter.ID}, Outcome: OutcomeCaptured,
		Capture:     &CaptureSpec{Species: "zaplet", FillParty: true},
		CompletedAt: time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC),
	}
	payload, err := canonicalActivityPayload(rec)
	if err != nil {
		t.Fatal(err)
	}

	got, err := planActivityTransition(set, current, rec, "seed")
	if err != nil {
		t.Fatal(err)
	}
	again, err := planActivityTransition(set, current, rec, "seed")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, again) {
		t.Fatal("transition is not deterministic for the same inputs")
	}
	if current.Collection[0].XP != 0 || len(current.Collection) != 1 {
		t.Fatal("transition mutated the input Save")
	}
	if got.save.Collection[0].XP != lessonXP {
		t.Fatalf("active XP = %d, want %d", got.save.Collection[0].XP, lessonXP)
	}
	if len(got.save.Collection) != 2 {
		t.Fatalf("collection = %d, want 2", len(got.save.Collection))
	}
	captured := got.save.Collection[1]
	if got.result.CapturedMonsterID != captured.ID || got.save.Party[1] != captured.ID {
		t.Fatalf("capture result/party does not reference captured Monster %q", captured.ID)
	}
	if got.result.Payload != payload || got.dailyMastery != nil {
		t.Fatalf("planned records = %+v, mastery %+v", got.result, got.dailyMastery)
	}
	foundReview := false
	for _, notice := range got.save.Notices {
		if notice.Kind == "capture_review" && notice.MonsterID == captured.ID && notice.SourceKey == rec.NaturalKey {
			foundReview = true
		}
	}
	if !foundReview {
		t.Fatal("capture review notice was not planned")
	}
}

func TestPlanActivityTransitionDailyMastery(t *testing.T) {
	set := loadTransitionContent(t)
	starter, err := monsterForSpecies(set, "rootkit", "starter")
	if err != nil {
		t.Fatal(err)
	}
	current := &game.Save{Collection: []game.Monster{starter}, Party: [3]string{starter.ID, "", ""}}
	at := time.Date(2026, 9, 4, 23, 30, 0, 0, time.FixedZone("offset", 5*60*60+30*60))
	rec := ActivityRecord{
		Kind: KindDailyXP, NaturalKey: "trainer:daily-xp:2026-09-04", TrainerID: "trainer",
		ActiveIDs: []string{starter.ID}, Outcome: OutcomeCleared, DailyParMet: true, CompletedAt: at,
	}
	got, err := planActivityTransition(set, current, rec, "daily-seed")
	if err != nil {
		t.Fatal(err)
	}
	if got.save.Collection[0].XP != dailyXP {
		t.Fatalf("daily XP = %d, want %d", got.save.Collection[0].XP, dailyXP)
	}
	if got.dailyMastery == nil {
		t.Fatal("Daily Mastery record was not planned")
	}
	if got.dailyMastery.NaturalKey != "trainer:daily-mastery:2026-09-04" || got.dailyMastery.Kind != KindDailyMastery {
		t.Fatalf("Daily Mastery = %+v", got.dailyMastery)
	}
	masteryRec := rec
	masteryRec.Kind = KindDailyMastery
	masteryRec.NaturalKey = got.dailyMastery.NaturalKey
	masteryRec.MasteryOnly = true
	wantPayload, err := canonicalActivityPayload(masteryRec)
	if err != nil {
		t.Fatal(err)
	}
	if got.dailyMastery.Payload != wantPayload {
		t.Fatalf("Daily Mastery payload = %q, want %q", got.dailyMastery.Payload, wantPayload)
	}
}

func TestPlanActivityTransitionValidatesResult(t *testing.T) {
	set := loadTransitionContent(t)
	starter, err := monsterForSpecies(set, "rootkit", "starter")
	if err != nil {
		t.Fatal(err)
	}
	current := &game.Save{
		Collection: []game.Monster{starter},
		Party:      [3]string{"unknown", "", ""},
	}
	rec := ActivityRecord{
		Kind: KindDailyMastery, NaturalKey: "trainer:daily-mastery:2026-09-04",
		TrainerID: "trainer", Outcome: OutcomeMastery, MasteryOnly: true,
		CompletedAt: time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC),
	}
	_, err = planActivityTransition(set, current, rec, "invalid-seed")
	if !errors.Is(err, ErrCorruptSave) {
		t.Fatalf("transition error = %v, want ErrCorruptSave", err)
	}
}

func loadTransitionContent(t *testing.T) *content.Set {
	t.Helper()
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	return set
}
