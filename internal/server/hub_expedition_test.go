package server

import (
	"errors"
	"strings"
	"testing"
	"time"

	"termon.sh/internal/capture"
	"termon.sh/internal/expedition"
	"termon.sh/internal/game"
	"termon.sh/internal/lobby"
	"termon.sh/internal/store"
)

func placeNearNoticeBoard(t testing.TB, h *Hub, hash string) {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	room := trainerRoom(h, hash)
	if err := room.Place(lobby.Presence{
		Hash: hash, X: lobby.NoticeBoardX - 1, Y: lobby.NoticeBoardY,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestNewExpeditionModeRejectsInvalidState(t *testing.T) {
	tests := []struct {
		name       string
		phase      string
		objectives []capture.Objective
	}{
		{name: "unknown phase", phase: "recovery"},
		{name: "prep with objectives", phase: "prep1", objectives: []capture.Objective{{}}},
		{name: "target without objectives", phase: "target"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := newExpeditionMode(tt.phase, "run", "mistcache", &game.Save{}, tt.objectives); err == nil {
				t.Fatal("newExpeditionMode accepted invalid state")
			}
		})
	}
}

func TestExpeditionModeDoesNotPersistAgainstReplacementRun(t *testing.T) {
	h := testHub(t)
	id := "exp-replaced-run"
	onboardTrainer(t, h, id, "rootkit")
	family := expedition.FamiliesForDay(time.Now().UTC())[0]
	if err := h.LaunchExpedition(id, family); err != nil {
		t.Fatal(err)
	}

	h.mu.Lock()
	m := h.matches[id]
	if m == nil {
		h.mu.Unlock()
		t.Fatal("missing expedition match")
	}
	mode, ok := m.mode.(*expeditionMode)
	if !ok {
		h.mu.Unlock()
		t.Fatalf("mode = %T, want expeditionMode", m.mode)
	}
	replacement := &expeditionRun{runID: "replacement", trainer: id, family: family, phase: "recovery"}
	h.expeditions[id] = replacement
	h.mu.Unlock()

	h.finishExpeditionPrepWin(m, mode, id)

	exists, err := h.saves.ActivityExists(id, mode.runID+":prep1")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("stale expedition match persisted against a replacement run")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.expeditions[id] != replacement {
		t.Fatal("stale expedition match replaced or cleared the owning run")
	}
}

// A daily challenge cannot start while an expedition run is open: the two
// modes share the battle seat, and a recovery-phase run holds no match, so the
// match check alone would let the daily through and wedge the run.
func TestStartDailyRefusesDuringExpedition(t *testing.T) {
	h := testHub(t)
	id := "daily-exp"
	onboardTrainer(t, h, id, "rootkit")
	placeNearMaster(t, h, id)
	h.mu.Lock()
	h.expeditions[id] = &expeditionRun{
		runID: "r-guard", trainer: id,
		family: expedition.FamiliesForDay(time.Now().UTC())[0],
		phase:  "recovery", recoveryNext: "target",
	}
	h.mu.Unlock()
	err := h.StartDaily(id)
	if err == nil || !strings.Contains(err.Error(), "expedition") {
		t.Fatalf("err = %v, want expedition-in-progress refusal", err)
	}
}

func TestLaunchExpeditionRefusesShortLoadout(t *testing.T) {
	h := testHub(t)
	id := "exp-short"
	onboardTrainer(t, h, id, "rootkit")
	sv, err := h.Load(id)
	if err != nil || sv == nil {
		t.Fatal(err)
	}
	monID := sv.Party[0]
	if _, err := h.saves.SetBattleLoadout(id, monID, sv.Collection[0].BattleLoadout[:3], nil); err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC()
	family := expedition.FamiliesForDay(day)[0]
	if err := h.LaunchExpedition(id, family); err == nil {
		t.Fatal("expected launch error for <4 moves")
	}
}

func TestOpenSignalBoardRequiresNearNoticeBoard(t *testing.T) {
	h := testHub(t)
	id := "exp-board"
	onboardTrainer(t, h, id, "rootkit")
	if _, err := h.OpenSignalBoard(id); err == nil {
		t.Fatal("expected error away from board")
	}
	placeNearNoticeBoard(t, h, id)
	board, err := h.OpenSignalBoard(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(board.Families) != 3 {
		t.Fatalf("families = %d, want 3", len(board.Families))
	}
}

func TestOpenDojoMenuStillRequiresNearMaster(t *testing.T) {
	h := testHub(t)
	id := "exp-master-menu"
	onboardTrainer(t, h, id, "rootkit")
	placeNearNoticeBoard(t, h, id)
	if _, err := h.OpenDojoMenu(id); err == nil {
		t.Fatal("notice board adjacency must not open lessons")
	}
	placeNearMaster(t, h, id)
	if _, err := h.OpenDojoMenu(id); err != nil {
		t.Fatal(err)
	}
}

func TestExpeditionRouteCaptured(t *testing.T) {
	h := testHub(t)
	id := "exp-captured"
	onboardTrainerFull(t, h, id, "rootkit")
	sv, _ := h.Load(id)
	partyBefore := sv.Party
	collBefore := len(sv.Collection)

	runID, err := newMatchID()
	if err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC()
	family := expedition.FamiliesForDay(day)[0]
	h.mu.Lock()
	h.expeditions[id] = &expeditionRun{
		runID: runID, trainer: id, family: family,
		prep1: "sproutware", prep2: "emberbyte",
		serverDay: expeditionServerDay(day),
	}
	h.mu.Unlock()

	if err := h.commitExpeditionEncounterForTest(id, "prep1", "prep1", ""); err != nil {
		t.Fatal(err)
	}
	if err := h.commitExpeditionEncounterForTest(id, "prep2", "prep2", ""); err != nil {
		t.Fatal(err)
	}
	if err := h.commitExpeditionEncounterForTest(id, "target", "captured", family); err != nil {
		t.Fatal(err)
	}
	sv, err = h.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if sv.Party != partyBefore {
		t.Fatalf("party changed: %+v vs %+v", sv.Party, partyBefore)
	}
	if len(sv.Collection) != collBefore+1 {
		t.Fatalf("collection = %d, want %d", len(sv.Collection), collBefore+1)
	}
	if sv.Collection[len(sv.Collection)-1].Species != family {
		t.Fatalf("captured species = %q, want %q", sv.Collection[len(sv.Collection)-1].Species, family)
	}
}

func TestExpeditionHuntFailedPaysXPNoMonster(t *testing.T) {
	h := testHub(t)
	id := "exp-hunt-fail"
	onboardTrainer(t, h, id, "rootkit")
	sv, _ := h.Load(id)
	xpBefore := sv.Collection[0].XP
	collBefore := len(sv.Collection)

	runID, err := newMatchID()
	if err != nil {
		t.Fatal(err)
	}
	family := expedition.FamiliesForDay(time.Now().UTC())[0]
	h.mu.Lock()
	h.expeditions[id] = &expeditionRun{runID: runID, trainer: id, family: family}
	h.mu.Unlock()

	if err := h.commitExpeditionEncounterForTest(id, "target", "hunt_failed", ""); err != nil {
		t.Fatal(err)
	}
	sv, err = h.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(sv.Collection) != collBefore {
		t.Fatalf("collection grew to %d", len(sv.Collection))
	}
	if sv.Collection[0].XP <= xpBefore {
		t.Fatalf("xp = %d, want > %d", sv.Collection[0].XP, xpBefore)
	}
}

func TestExpeditionTargetConflictAfterHuntFailed(t *testing.T) {
	h := testHub(t)
	id := "exp-conflict"
	onboardTrainer(t, h, id, "rootkit")
	runID, err := newMatchID()
	if err != nil {
		t.Fatal(err)
	}
	key := runID + ":target"
	active := mustLoadPartyLead(t, h, id)
	if _, err := h.saves.RecordActivityResult(store.ActivityRecord{
		Kind: "expedition", NaturalKey: key, TrainerID: id,
		ActiveIDs: []string{active}, Outcome: "hunt_failed",
		CompletedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	captured := store.ActivityRecord{
		Kind: "expedition", NaturalKey: key, TrainerID: id,
		ActiveIDs: []string{active}, Outcome: "captured",
		Capture:     &store.CaptureSpec{Species: "zaplet", FillParty: false},
		CompletedAt: time.Now(),
	}
	if _, err := h.saves.RecordActivityResult(captured); !errors.Is(err, store.ErrResultConflict) {
		t.Fatalf("conflict = %v, want ErrResultConflict", err)
	}
}

func expeditionServerDay(day time.Time) time.Time {
	d := day.UTC()
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}

func TestLaunchExpeditionStartsRun(t *testing.T) {
	h := testHub(t)
	id := "exp-launch"
	onboardTrainer(t, h, id, "rootkit")
	placeNearNoticeBoard(t, h, id)
	board, err := h.OpenSignalBoard(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.LaunchExpedition(id, board.Families[0].Slug); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	run := h.expeditions[id]
	match := h.matches[id]
	h.mu.Unlock()
	if run == nil {
		t.Fatal("missing expedition run")
	}
	if match == nil {
		t.Fatal("missing expedition match")
	}
	mode, ok := match.mode.(*expeditionMode)
	if !ok || mode.phase != expeditionPrep1 {
		t.Fatalf("match = %+v", match)
	}
}

// commitExpeditionEncounterForTest drives the reward-commit path for one
// encounter without playing a battle, so tests exercise RecordActivityResult
// on the real Store rather than hand-injected run state.
func (h *Hub) commitExpeditionEncounterForTest(hash string, phase, outcome string, captureSpecies string) error {
	run := h.expeditionRunLocked(hash)
	if run == nil {
		return errors.New("no expedition")
	}
	sv, err := h.Load(hash)
	if err != nil || sv == nil {
		return err
	}
	active := occupiedPartyIDs(sv)
	rec := store.ActivityRecord{
		Kind: store.KindExpedition, NaturalKey: run.runID + ":" + phase, TrainerID: hash,
		ActiveIDs: active, Outcome: outcome, CompletedAt: time.Now(),
	}
	if outcome == store.OutcomeCaptured && captureSpecies != "" {
		rec.Capture = &store.CaptureSpec{Species: captureSpecies, FillParty: false}
	}
	_, err = h.saves.RecordActivityResult(rec)
	return err
}

func occupiedPartyIDs(sv *game.Save) []string {
	var ids []string
	for _, id := range sv.Party {
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
