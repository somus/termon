package server

import (
	"fmt"
	"testing"
	"time"

	"termon.sh/internal/capture"
	"termon.sh/internal/dojo"
	"termon.sh/internal/game"
	"termon.sh/internal/lobby"
	"termon.sh/internal/store"
)

func placeNearMaster(t testing.TB, h *Hub, hash string) {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	room := trainerRoom(h, hash)
	if err := room.Place(lobby.Presence{
		Hash: hash, X: lobby.MasterX - 1, Y: lobby.MasterY,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestNewLessonModeRejectsIncompleteState(t *testing.T) {
	tests := []struct {
		name        string
		lesson      int
		session     *capture.Session
		wildSpecies string
	}{
		{name: "unknown lesson", lesson: 3, session: capture.NewSession(nil), wildSpecies: "mistcache"},
		{name: "missing capture session", lesson: 1, wildSpecies: "mistcache"},
		{name: "empty capture session", lesson: 1, session: capture.NewSession(nil), wildSpecies: "mistcache"},
		{name: "missing target", lesson: 1, session: capture.NewSession([]capture.Objective{{}})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := newLessonMode(tt.lesson, &game.Save{}, tt.session, tt.wildSpecies); err == nil {
				t.Fatal("newLessonMode accepted incomplete state")
			}
		})
	}
}

func TestLessonTargetsForAllStarters(t *testing.T) {
	want := map[string][2]string{
		"rootkit":   {"mistcache", "chippunk"},
		"emberbyte": {"sproutware", "zaplet"},
		"aquabit":   {"wickware", "spamlet"},
	}
	for starter, targets := range want {
		for lesson := 1; lesson <= 2; lesson++ {
			got, err := dojo.LessonTarget(starter, lesson)
			if err != nil {
				t.Fatalf("%s lesson %d: %v", starter, lesson, err)
			}
			if got != targets[lesson-1] {
				t.Fatalf("%s lesson %d = %q, want %q", starter, lesson, got, targets[lesson-1])
			}
		}
	}
}

func TestLessonsFillPartyForAllStarters(t *testing.T) {
	starters := []string{"rootkit", "emberbyte", "aquabit"}
	for _, starter := range starters {
		t.Run(starter, func(t *testing.T) {
			h := testHub(t)
			id := "lesson-" + starter
			onboardTrainer(t, h, id, starter)
			target1, _ := dojo.LessonTarget(starter, 1)
			target2, _ := dojo.LessonTarget(starter, 2)
			active := mustLoadPartyLead(t, h, id)
			if _, err := h.saves.RecordActivityResult(store.ActivityRecord{
				Kind: "lesson", NaturalKey: id + ":lesson:1", TrainerID: id,
				ActiveIDs: []string{active}, Outcome: "captured",
				Capture:     &store.CaptureSpec{Species: target1, FillParty: true},
				CompletedAt: time.Now(),
			}); err != nil {
				t.Fatal(err)
			}
			sv, err := h.Load(id)
			if err != nil || sv == nil {
				t.Fatal(err)
			}
			if sv.Party[1] == "" {
				t.Fatal("lesson 1 did not fill party slot 2")
			}
			activeIDs := []string{sv.Party[0], sv.Party[1]}
			if _, err := h.saves.RecordActivityResult(store.ActivityRecord{
				Kind: "lesson", NaturalKey: id + ":lesson:2", TrainerID: id,
				ActiveIDs: activeIDs, Outcome: "captured",
				Capture:     &store.CaptureSpec{Species: target2, FillParty: true},
				CompletedAt: time.Now(),
			}); err != nil {
				t.Fatal(err)
			}
			sv, err = h.Load(id)
			if err != nil {
				t.Fatal(err)
			}
			if !game.FullParty(sv) {
				t.Fatalf("party after lesson 2 = %+v, want full", sv.Party)
			}
		})
	}
}

func TestDuplicateLessonNaturalKeyNoRecapture(t *testing.T) {
	h := testHub(t)
	id := "lesson-dup"
	onboardTrainer(t, h, id, "rootkit")
	active := mustLoadPartyLead(t, h, id)
	key := id + ":lesson:1"
	first, err := h.saves.RecordActivityResult(store.ActivityRecord{
		Kind: "lesson", NaturalKey: key, TrainerID: id,
		ActiveIDs: []string{active}, Outcome: "captured",
		Capture:     &store.CaptureSpec{Species: "mistcache", FillParty: true},
		CompletedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.saves.RecordActivityResult(store.ActivityRecord{
		Kind: "lesson", NaturalKey: key, TrainerID: id,
		ActiveIDs: []string{active}, Outcome: "captured",
		Capture:     &store.CaptureSpec{Species: "mistcache", FillParty: true},
		CompletedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Collection) != len(second.Collection) {
		t.Fatalf("duplicate capture grew collection: %d -> %d", len(first.Collection), len(second.Collection))
	}
}

func TestLessonPersistSendsProgressionSummary(t *testing.T) {
	h := testHub(t)
	id := "lesson-prog"
	var msgs []any
	h.Attach(id, func(msg any) { msgs = append(msgs, msg) }, func() {})
	onboardTrainer(t, h, id, "rootkit")
	sv, err := h.Load(id)
	if err != nil || sv == nil {
		t.Fatal(err)
	}
	solo := soloMode{saveBefore: cloneSaveXPView(sv)}
	mode := &lessonMode{
		soloMode: solo,
		lesson:   1,
	}
	active := sv.Party[0]
	if err := h.persistLessonCapture(mode, id, "mistcache", []string{active}, nil); err != nil {
		t.Fatal(err)
	}
	var prog ProgressionMsg
	for _, msg := range msgs {
		if p, ok := msg.(ProgressionMsg); ok {
			prog = p
		}
	}
	if len(prog.Entries) == 0 {
		t.Fatal("expected progression summary after lesson capture")
	}
	if prog.Entries[0].XPGained <= 0 {
		t.Fatalf("progression = %+v, want XP gain", prog.Entries[0])
	}
}

func TestLessonCaptureSendsBattleBeforeProgression(t *testing.T) {
	h := testHub(t)
	id := "lesson-order"
	var msgs []any
	h.Attach(id, func(msg any) { msgs = append(msgs, msg) }, func() {})
	onboardTrainer(t, h, id, "rootkit")
	if err := h.StartRequiredLesson(id, 1); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	m := h.matches[id]
	h.mu.Unlock()
	if m == nil || m.bt == nil {
		t.Fatal("lesson should be live")
	}
	mode, ok := m.mode.(*lessonMode)
	if !ok {
		t.Fatalf("mode = %T, want lessonMode", m.mode)
	}
	msgs = nil
	h.finishLessonCaptured(m, mode, id)

	battleAt, progAt := -1, -1
	var kinds []string
	for i, msg := range msgs {
		kinds = append(kinds, fmt.Sprintf("%T", msg))
		switch msg.(type) {
		case BattleMsg:
			if battleAt < 0 {
				battleAt = i
			}
		case ProgressionMsg:
			if progAt < 0 {
				progAt = i
			}
		}
	}
	if battleAt < 0 {
		t.Fatalf("captured lesson never pushed the Battle, so the KO cannot play; msgs=%v", kinds)
	}
	if progAt < 0 {
		t.Fatalf("expected ProgressionMsg after capture; msgs=%v", kinds)
	}
	if battleAt > progAt {
		t.Fatalf("ProgressionMsg at %d arrived before BattleMsg at %d; msgs=%v", progAt, battleAt, kinds)
	}
}

func TestOpenDojoMenuRequiresNearMaster(t *testing.T) {
	h := testHub(t)
	id := "dojo-menu"
	onboardTrainer(t, h, id, "rootkit")
	if _, err := h.OpenDojoMenu(id); err == nil {
		t.Fatal("expected error away from master")
	}
	placeNearMaster(t, h, id)
	menu, err := h.OpenDojoMenu(id)
	if err != nil {
		t.Fatal(err)
	}
	if menu.Lesson1Done || menu.Lesson2Done {
		t.Fatalf("new trainer menu = %+v", menu)
	}
}

func TestLessonForfeitRetriesRequiredLesson(t *testing.T) {
	h := testHub(t)
	id := "forfeit-lesson"
	onboardTrainer(t, h, id, "rootkit")
	if err := h.StartRequiredLesson(id, 1); err != nil {
		t.Fatal(err)
	}
	if err := h.Forfeit(id); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	m := h.matches[id]
	if m == nil {
		t.Fatal("forfeit on a required Lesson should retry the same Lesson")
	}
	mode, ok := m.mode.(*lessonMode)
	if !ok || mode.lesson != 1 {
		t.Fatal("forfeit on a required Lesson should retry the same Lesson")
	}
}

func TestStartRequiredLessonFromSpawn(t *testing.T) {
	h := testHub(t)
	id := "required-lesson"
	onboardTrainer(t, h, id, "rootkit")
	if err := h.StartLesson(id, 1); err == nil {
		t.Fatal("menu StartLesson should need Master adjacency")
	}
	if err := h.StartRequiredLesson(id, 1); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	m := h.matches[id]
	if m == nil {
		t.Fatal("required lesson 1 should be live")
	}
	mode, ok := m.mode.(*lessonMode)
	if !ok || mode.lesson != 1 {
		t.Fatal("required lesson 1 should be live")
	}
	if !trainerRoom(h, id).NearMaster(id) {
		t.Fatal("required lesson should seat beside Master Sable")
	}
}

func TestPvPForfeitLeavesPersistentLoadoutUnchanged(t *testing.T) {
	h := testHub(t)
	onboardTrainerFull(t, h, "a", "rootkit")
	onboardTrainerFull(t, h, "b", "emberbyte")
	beforeA := cloneMonsterLoadouts(t, h, "a")
	beforeB := cloneMonsterLoadouts(t, h, "b")
	if err := h.startMatch("a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := h.Forfeit("a"); err != nil {
		t.Fatal(err)
	}
	afterA := cloneMonsterLoadouts(t, h, "a")
	afterB := cloneMonsterLoadouts(t, h, "b")
	assertLoadoutsEqual(t, beforeA, afterA)
	assertLoadoutsEqual(t, beforeB, afterB)
	svA, _ := h.Load("a")
	svB, _ := h.Load("b")
	if svA.Collection[0].XP != beforeA[0].XP || svB.Collection[0].XP != beforeB[0].XP {
		t.Fatal("forfeit changed persistent XP")
	}
}

type loadoutSnapshot struct {
	ID            string
	Level         int
	XP            int64
	BattleLoadout []string
	MoveLibrary   []string
}

func cloneMonsterLoadouts(t testing.TB, h *Hub, hash string) []loadoutSnapshot {
	t.Helper()
	sv, err := h.Load(hash)
	if err != nil || sv == nil {
		t.Fatal(err)
	}
	var out []loadoutSnapshot
	for _, m := range sv.Collection {
		out = append(out, loadoutSnapshot{
			ID: m.ID, Level: m.Level, XP: m.XP,
			BattleLoadout: append([]string(nil), m.BattleLoadout...),
			MoveLibrary:   append([]string(nil), m.MoveLibrary...),
		})
	}
	return out
}

func assertLoadoutsEqual(t testing.TB, before, after []loadoutSnapshot) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("collection size %d -> %d", len(before), len(after))
	}
	for i := range before {
		if before[i].ID != after[i].ID ||
			before[i].Level != after[i].Level ||
			before[i].XP != after[i].XP {
			t.Fatalf("monster %d changed: %+v -> %+v", i, before[i], after[i])
		}
		if len(before[i].BattleLoadout) != len(after[i].BattleLoadout) {
			t.Fatalf("loadout len changed for %s", before[i].ID)
		}
		for j := range before[i].BattleLoadout {
			if before[i].BattleLoadout[j] != after[i].BattleLoadout[j] {
				t.Fatalf("loadout changed for %s", before[i].ID)
			}
		}
	}
}

func mustLoadPartyLead(t testing.TB, h *Hub, hash string) string {
	t.Helper()
	sv, err := h.Load(hash)
	if err != nil || sv == nil || sv.Party[0] == "" {
		t.Fatal("missing party lead")
	}
	return sv.Party[0]
}
