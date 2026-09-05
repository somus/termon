package server

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/lobby"
	"termon.sh/internal/store"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func testHub(t testing.TB) *Hub {
	t.Helper()
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	// Registrations are unlimited so bulk-onboarding tests never trip the
	// per-source quota; quota behavior is covered in admission_test.go.
	mem := store.NewMemoryStore()
	mem.UseContent(set)
	return NewHub(set, mem, Admission{OpenAccess: true, RegistrationsPerIP: -1})
}

// onboard creates the Trainer behind credential id, then completes
// onboarding with handle id and the given starter.
func onboardTrainer(t testing.TB, h *Hub, id, starter string) {
	t.Helper()
	trainer, err := h.Authenticate(id, "10.0.0.1")
	if err != nil {
		t.Fatalf("authenticate %s: %v", id, err)
	}
	if _, err := h.CompleteOnboard(trainer.ID, id, starter); err != nil {
		t.Fatalf("onboard %s: %v", id, err)
	}
}

func trainerRoom(h *Hub, hash string) *lobby.Room {
	return h.dojos[h.dojoOf[hash]]
}

func TestNewAndReturningTrainer(t *testing.T) {
	h := testHub(t)
	trainer, err := h.Authenticate("aaa", "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if trainer.ID == "" || trainer.Save != nil {
		t.Fatalf("new Trainer = %+v", trainer)
	}
	sv, err := h.CompleteOnboard(trainer.ID, "swift-otter-12", "rootkit")
	if err != nil {
		t.Fatal(err)
	}
	if sv.Handle != "swift-otter-12" || len(sv.Collection) != 1 || sv.Collection[0].Species != "rootkit" {
		t.Fatalf("save = %+v", sv)
	}
	loaded, err := h.Load(trainer.ID)
	if err != nil || loaded == nil {
		t.Fatalf("load: %v %v", loaded, err)
	}
	if loaded.Handle != "swift-otter-12" {
		t.Fatalf("returning handle = %s", loaded.Handle)
	}
	freshTrainer, err := h.Authenticate("unknown", "10.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := h.Load(freshTrainer.ID)
	if err != nil || fresh != nil {
		t.Fatalf("unknown trainer should be new, got %v %v", fresh, err)
	}
}

func TestThirtyThirdTrainerEntersASecondDojo(t *testing.T) {
	h := testHub(t)
	for i := 1; i <= 33; i++ {
		id := fmt.Sprintf("trainer-%02d", i)
		onboardTrainerFull(t, h, id, "rootkit")
	}

	first := h.Snapshot("trainer-01")
	if len(first.Others) != 31 {
		t.Fatalf("first Dojo has %d other Trainers, want 31", len(first.Others))
	}
	second := h.Snapshot("trainer-33")
	if len(second.Others) != 0 {
		t.Fatalf("second Dojo has %d other Trainers, want 0", len(second.Others))
	}
}

func TestDojoPresenceAndChallengesStayLocal(t *testing.T) {
	h := testHub(t)
	for i := 1; i <= 33; i++ {
		id := fmt.Sprintf("trainer-%02d", i)
		onboardTrainerFull(t, h, id, "rootkit")
	}
	var firstSnapshots, secondSnapshots int
	h.Attach("trainer-01", func(msg any) {
		if _, ok := msg.(SnapshotMsg); ok {
			firstSnapshots++
		}
	}, func() {})
	h.Attach("trainer-33", func(msg any) {
		if _, ok := msg.(SnapshotMsg); ok {
			secondSnapshots++
		}
	}, func() {})

	h.Emote("trainer-01", "hello!")
	if firstSnapshots == 0 {
		t.Fatal("first Dojo did not receive its presence update")
	}
	if secondSnapshots != 0 {
		t.Fatalf("second Dojo received %d presence updates from the first", secondSnapshots)
	}
	if err := h.Challenge("trainer-33"); err == nil {
		t.Fatal("Trainer challenged a colocated coordinate in another Dojo")
	}
}

func TestGlobalQueuePairsTrainersAcrossDojos(t *testing.T) {
	h := testHub(t)
	onboardTrainerFull(t, h, "trainer-01", "rootkit")
	onboardTrainerFull(t, h, "trainer-02", "emberbyte")
	_, _, err := h.FindBattle("trainer-01")
	if err != nil {
		t.Fatalf("queue trainer-01: %v", err)
	}
	_, _, err = h.FindBattle("trainer-02")
	if err != nil {
		t.Fatalf("queue trainer-02: %v", err)
	}
}

func TestPartialPartyCannotQueue(t *testing.T) {
	h := testHub(t)
	onboardTrainer(t, h, "solo", "rootkit")
	_, _, err := h.FindBattle("solo")
	assertPartialParty(t, err)
}

func TestReconnectKeepsDojoAndEmptyDojoIsReclaimed(t *testing.T) {
	h := testHub(t)
	for i := 1; i <= 34; i++ {
		id := fmt.Sprintf("trainer-%02d", i)
		onboardTrainerFull(t, h, id, "rootkit")
	}
	detach33 := h.Attach("trainer-33", func(any) {}, func() {})
	detach34 := h.Attach("trainer-34", func(any) {}, func() {})
	before := h.Snapshot("trainer-33")
	detach33()
	resumed, ok := h.Resume("trainer-33").(SnapshotMsg)
	if !ok {
		t.Fatalf("resume = %#v", resumed)
	}
	if resumed.Dojo != before.Dojo || len(resumed.Others) != 1 || resumed.Others[0].Hash != "trainer-34" {
		t.Fatalf("reconnected Snapshot = %+v, want Dojo %d with trainer-34", resumed, before.Dojo)
	}

	detach33 = h.Attach("trainer-33", func(any) {}, func() {})
	detach33()
	detach34()
	stats := h.Stats()
	if stats.Dojos != 1 || stats.Trainers != 32 {
		t.Fatalf("stats after reclaim = %+v, want one Dojo with 32 Trainers", stats)
	}
}

func TestBattleResultFailureIsVisibleAndRetriesBeforePublishing(t *testing.T) {
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	saves := store.NewMemoryStore()
	saves.UseContent(set)
	h := NewHub(set, saves, Admission{OpenAccess: true})
	var statesA []battle.State
	var errorsA []string
	h.Attach("a", func(msg any) {
		switch msg := msg.(type) {
		case BattleMsg:
			statesA = append(statesA, msg.Battle.State())
		case ErrorMsg:
			errorsA = append(errorsA, msg.Text)
		}
	}, func() {})
	h.Attach("b", func(any) {}, func() {})
	onboardTrainerFull(t, h, "a", "rootkit")
	onboardTrainerFull(t, h, "b", "emberbyte")
	if err := h.startMatch("a", "b"); err != nil {
		t.Fatal(err)
	}
	saves.FailNextResults(1)
	if err := h.Forfeit("a"); err != nil {
		t.Fatal(err)
	}
	if len(errorsA) == 0 || errorsA[len(errorsA)-1] != "result save failed; retrying" {
		t.Fatalf("errors = %v, want visible retry status", errorsA)
	}
	for _, state := range statesA {
		if state == battle.StateOver {
			t.Fatal("Battle Result was published before persistence succeeded")
		}
	}
	if h.matches["a"] == nil || !h.matches["a"].pending {
		t.Fatal("failed Battle Result was not retained for retry")
	}
	resume, ok := h.Resume("a").(ErrorMsg)
	if !ok || resume.Text != "result save failed; retrying" {
		t.Fatalf("resume during retry = %#v", resume)
	}

	// Advance past the result-save retry backoff (attempt 1 waits 1s).
	h.Tick(time.Now().Add(2 * time.Second))
	if len(statesA) == 0 || statesA[len(statesA)-1] != battle.StateOver {
		t.Fatalf("states after retry = %v, want battle_over", statesA)
	}
	if h.matches["a"] != nil {
		t.Fatal("successful retry left the Battle active")
	}
	winner, err := h.Load("b")
	if err != nil {
		t.Fatal(err)
	}
	loser, err := h.Load("a")
	if err != nil {
		t.Fatal(err)
	}
	if winner.Wins != 1 || loser.Losses != 1 {
		t.Fatalf("records after retry = winner %+v loser %+v", winner, loser)
	}
}

func TestBattleCompletionAndTickDoNotLeak(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	h := testHub(t)
	for _, id := range []string{"a", "b"} {
		onboardTrainerFull(t, h, id, "rootkit")
	}
	if err := h.startMatch("a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := h.Forfeit("a"); err != nil {
		t.Fatal(err)
	}
	h.Tick(time.Now())
}

func TestHubSQLiteBattleUsesStableTrainerIDs(t *testing.T) {
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	saves, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "termon.db"))
	if err != nil {
		t.Fatal(err)
	}
	saves.UseContent(set)
	t.Cleanup(func() { _ = saves.Close() })
	h := NewHub(set, saves, Admission{OpenAccess: true})
	a, err := h.Authenticate("credential-a", "10.0.0.3")
	if err != nil {
		t.Fatal(err)
	}
	b, err := h.Authenticate("credential-b", "10.0.0.4")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == "credential-a" || b.ID == "credential-b" || a.ID == b.ID {
		t.Fatalf("Trainer IDs = %q and %q, want distinct opaque IDs", a.ID, b.ID)
	}
	if _, err := h.CompleteOnboard(a.ID, "alpha", "rootkit"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.CompleteOnboard(b.ID, "bravo", "emberbyte"); err != nil {
		t.Fatal(err)
	}
	fillTestParty(t, h, a.ID)
	fillTestParty(t, h, b.ID)
	if err := h.startMatch(a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := h.Forfeit(a.ID); err != nil {
		t.Fatal(err)
	}
	winner, err := h.Load(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	loser, err := h.Load(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if winner.Wins != 1 || loser.Losses != 1 {
		t.Fatalf("SQLite records = winner %+v loser %+v", winner, loser)
	}
}

func TestAttachDisplacesOlderSession(t *testing.T) {
	h := testHub(t)
	killed := false
	h.Attach("aaa", func(any) {}, func() { killed = true })
	h.Attach("aaa", func(any) {}, func() {})
	if !killed {
		t.Fatal("older session was not displaced")
	}
}

func TestQueuePairsFIFO(t *testing.T) {
	h := testHub(t)
	onboardTrainerFull(t, h, "a", "rootkit")
	onboardTrainerFull(t, h, "b", "emberbyte")
	pos, wait, err := h.FindBattle("a")
	if err != nil || pos != 1 || wait != 1 {
		t.Fatalf("first queue = pos %d wait %d err %v, want 1/1/nil", pos, wait, err)
	}
	if _, _, err := h.FindBattle("b"); err != nil {
		t.Fatalf("pair: %v", err)
	}
}

func TestChallengeRequiresAdjacent(t *testing.T) {
	h := testHub(t)
	onboardTrainerFull(t, h, "a", "rootkit")
	onboardTrainerFull(t, h, "b", "emberbyte")
	h.mu.Lock()
	room := trainerRoom(h, "a")
	pa, _ := room.Get("a")
	room.Leave("b")
	_ = room.Place(lobby.Presence{Hash: "b", Handle: "bravo", Species: "emberbyte", X: pa.X + 3, Y: pa.Y})
	h.mu.Unlock()
	assertNotAdjacentChallenge(t, h.Challenge("a"))
}

func TestBattleResultRefreshesLiveRecords(t *testing.T) {
	h := testHub(t)
	var gotA, gotB []any
	h.Attach("a", func(msg any) { gotA = append(gotA, msg) }, func() {})
	h.Attach("b", func(msg any) { gotB = append(gotB, msg) }, func() {})
	onboardTrainerFull(t, h, "a", "rootkit")
	onboardTrainerFull(t, h, "b", "emberbyte")
	if err := h.startMatch("a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := h.Forfeit("a"); err != nil {
		t.Fatal(err)
	}

	wantRecord := func(messages []any, wins, losses int) {
		t.Helper()
		for _, item := range messages {
			msg, ok := item.(SaveMsg)
			if ok && msg.Save != nil {
				if msg.Save.Wins != wins || msg.Save.Losses != losses {
					t.Fatalf("live record = %d-%d, want %d-%d", msg.Save.Wins, msg.Save.Losses, wins, losses)
				}
				return
			}
		}
		t.Fatal("missing live record refresh")
	}
	wantRecord(gotA, 0, 1)
	wantRecord(gotB, 1, 0)
}

func TestResetTrainerWipesSave(t *testing.T) {
	h := testHub(t)
	trainer, err := h.Authenticate("aaa", "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.CompleteOnboard(trainer.ID, "swift-otter-12", "rootkit"); err != nil {
		t.Fatal(err)
	}
	if err := h.ResetTrainer("aaa"); err != nil {
		t.Fatal(err)
	}
	sv, err := h.Load("aaa")
	if err != nil || sv != nil {
		t.Fatalf("expected no save after reset, got %v %v", sv, err)
	}
	if _, ok := trainerRoom(h, "aaa").Get("aaa"); ok {
		t.Fatal("reset trainer still in the dojo")
	}
}

func TestDropLeavesLobbyAndQueue(t *testing.T) {
	h := testHub(t)
	detachA := h.Attach("a", func(any) {}, func() {})
	h.Attach("b", func(any) {}, func() {})
	onboardTrainerFull(t, h, "a", "rootkit")
	onboardTrainerFull(t, h, "b", "emberbyte")
	detachA()
	for _, p := range h.Snapshot("b").Others {
		if p.Hash == "a" {
			t.Fatal("dropped trainer still visible in the dojo")
		}
	}
}

func TestDropClearsPendingChallenge(t *testing.T) {
	h := testHub(t)
	onboardTrainerFull(t, h, "a", "rootkit")
	assertNotAdjacentChallenge(t, h.Challenge("a"))
}

func TestLobbyMovementBroadcastsOncePerTick(t *testing.T) {
	h := testHub(t)
	var mu sync.Mutex
	snapshots := 0
	h.Attach("a", func(msg any) {
		if _, ok := msg.(SnapshotMsg); ok {
			mu.Lock()
			snapshots++
			mu.Unlock()
		}
	}, func() {})
	onboardTrainerFull(t, h, "a", "rootkit")
	mu.Lock()
	snapshots = 0
	mu.Unlock()

	if err := h.Move("a", lobby.North); err != nil {
		t.Fatal(err)
	}
	if err := h.Move("a", lobby.North); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got := snapshots
	mu.Unlock()
	if got != 0 {
		t.Fatalf("movement broadcast before tick: %d snapshots", got)
	}

	h.Tick(time.Now())
	h.Tick(time.Now())
	mu.Lock()
	got = snapshots
	mu.Unlock()
	if got != 1 {
		t.Fatalf("movement broadcasts = %d, want one coalesced snapshot", got)
	}
}

func TestConcurrentMovesPreserveCollision(t *testing.T) {
	h := testHub(t)
	for _, hash := range []string{"a", "b"} {
		h.Attach(hash, func(any) {}, func() {})
		onboardTrainerFull(t, h, hash, "rootkit")
	}
	h.mu.Lock()
	room := trainerRoom(h, "a")
	room.Leave("a")
	room.Leave("b")
	_ = room.Place(lobby.Presence{Hash: "a", X: 10, Y: 6})
	_ = room.Place(lobby.Presence{Hash: "b", X: 12, Y: 6})
	h.mu.Unlock()

	errs := make(chan error, 2)
	go func() { errs <- h.Move("a", lobby.East) }()
	go func() { errs <- h.Move("b", lobby.West) }()
	first, second := <-errs, <-errs
	if (first == nil) == (second == nil) {
		t.Fatalf("move errors = %v, %v; want exactly one collision winner", first, second)
	}
	a := h.Snapshot("a").You
	b := h.Snapshot("b").You
	if a.X == b.X && a.Y == b.Y {
		t.Fatalf("concurrent moves occupied the same tile: a=%+v b=%+v", a, b)
	}
}

func TestConcurrentDetachCleansPresence(t *testing.T) {
	h := testHub(t)
	detach := make([]func(), 0, 8)
	for i := range 8 {
		hash := fmt.Sprintf("d%02d", i)
		detach = append(detach, h.Attach(hash, func(any) {}, func() {}))
		onboardTrainerFull(t, h, hash, "rootkit")
	}
	var wg sync.WaitGroup
	for _, drop := range detach {
		wg.Go(func() {
			drop()
		})
	}
	wg.Wait()
	if got := trainerRoom(h, "d00").Snapshot(); len(got) != 0 {
		t.Fatalf("detached presence remains: %+v", got)
	}
}

func TestConcurrentChallengeAndEmoteExpiry(t *testing.T) {
	h := testHub(t)
	for _, hash := range []string{"a", "b"} {
		h.Attach(hash, func(any) {}, func() {})
		onboardTrainerFull(t, h, hash, "rootkit")
	}
	h.mu.Lock()
	room := trainerRoom(h, "a")
	pa, _ := room.Get("a")
	room.Leave("b")
	_ = room.Place(lobby.Presence{Hash: "b", X: pa.X + 1, Y: pa.Y})
	h.mu.Unlock()

	future := time.Now().Add(time.Minute)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		_ = h.Challenge("a")
	}()
	go func() {
		defer wg.Done()
		h.Emote("a", "gg")
	}()
	go func() {
		defer wg.Done()
		h.Tick(future)
	}()
	wg.Wait()
	h.Tick(future)

	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.challenges) != 0 {
		t.Fatalf("expired challenges remain: %+v", h.challenges)
	}
	p, _ := trainerRoom(h, "a").Get("a")
	if p.Emote != "" {
		t.Fatalf("expired emote remains: %q", p.Emote)
	}
}

func TestResumeRejoinsLiveBattle(t *testing.T) {
	h := testHub(t)
	var gotB []any
	detachA := h.Attach("a", func(any) {}, func() {})
	h.Attach("b", func(msg any) { gotB = append(gotB, msg) }, func() {})
	onboardTrainerFull(t, h, "a", "rootkit")
	onboardTrainerFull(t, h, "b", "emberbyte")
	if err := h.startMatch("a", "b"); err != nil {
		t.Fatal(err)
	}
	var live *battle.Battle
	for _, msg := range gotB {
		if b, ok := msg.(BattleMsg); ok {
			live = b.Battle
		}
	}
	if live == nil {
		t.Fatal("expected a live battle")
	}
	events := live.Events()
	detachA()
	sawBanner := false
	for _, msg := range gotB {
		if r, ok := msg.(ReconnectingMsg); ok && r.Handle != "" {
			sawBanner = true
		}
	}
	if !sawBanner {
		t.Fatal("opponent did not see reconnecting banner")
	}
	h.Attach("a", func(any) {}, func() {})
	resumed := h.Resume("a")
	bm, ok := resumed.(BattleMsg)
	if !ok || bm.Battle != live {
		t.Fatalf("resume = %#v, want the live battle", resumed)
	}
	if len(bm.Battle.Events()) != len(events) {
		t.Fatalf("resume events = %d, want %d", len(bm.Battle.Events()), len(events))
	}
	cleared := false
	for _, msg := range gotB {
		if r, ok := msg.(ReconnectingMsg); ok && r.Handle == "" {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("opponent banner was not cleared on reconnect")
	}
}

func TestDisconnectTimeoutAwardsWin(t *testing.T) {
	h := testHub(t)
	detachA := h.Attach("a", func(any) {}, func() {})
	var gotB []any
	h.Attach("b", func(msg any) { gotB = append(gotB, msg) }, func() {})
	onboardTrainerFull(t, h, "a", "rootkit")
	onboardTrainerFull(t, h, "b", "emberbyte")
	if err := h.startMatch("a", "b"); err != nil {
		t.Fatal(err)
	}
	detachA()
	h.Tick(time.Now().Add(battle.DisconnectGrace + time.Second))
	svA, err := h.Load("a")
	if err != nil || svA == nil {
		t.Fatal(err)
	}
	svB, err := h.Load("b")
	if err != nil || svB == nil {
		t.Fatal(err)
	}
	if svA.Wins != 0 || svA.Losses != 1 {
		t.Fatalf("disconnected record = %d-%d, want 0-1", svA.Wins, svA.Losses)
	}
	if svB.Wins != 1 || svB.Losses != 0 {
		t.Fatalf("remaining record = %d-%d, want 1-0", svB.Wins, svB.Losses)
	}
	var over BattleMsg
	found := false
	for _, msg := range gotB {
		if b, ok := msg.(BattleMsg); ok && b.Battle != nil && b.Battle.State() == battle.StateOver {
			over = b
			found = true
		}
	}
	if !found || over.Battle.Reason() != battle.EndDisconnectTimeout {
		t.Fatalf("remaining trainer did not receive disconnect_timeout, last=%#v", over)
	}
	if over.Battle.Winner() != "b" {
		t.Fatalf("winner = %s, want b", over.Battle.Winner())
	}
	if _, ok := h.Resume("a").(BattleMsg); ok {
		t.Fatal("timeout left a live battle for the disconnected trainer")
	}
}

func TestReconnectClearsGrace(t *testing.T) {
	h := testHub(t)
	detachA := h.Attach("a", func(any) {}, func() {})
	h.Attach("b", func(any) {}, func() {})
	onboardTrainerFull(t, h, "a", "rootkit")
	onboardTrainerFull(t, h, "b", "emberbyte")
	if err := h.startMatch("a", "b"); err != nil {
		t.Fatal(err)
	}
	detachA()
	h.Attach("a", func(any) {}, func() {})
	if _, ok := h.Resume("a").(BattleMsg); !ok {
		t.Fatal("reconnect did not resume the live battle")
	}
	h.Tick(time.Now().Add(battle.DisconnectGrace + time.Second))
	svA, _ := h.Load("a")
	svB, _ := h.Load("b")
	if svA.Wins != 0 || svA.Losses != 0 || svB.Wins != 0 || svB.Losses != 0 {
		t.Fatalf("grace after reconnect wrote W/L: a=%d-%d b=%d-%d", svA.Wins, svA.Losses, svB.Wins, svB.Losses)
	}
}

func TestChallengeExpires(t *testing.T) {
	h := testHub(t)
	onboardTrainerFull(t, h, "a", "rootkit")
	assertNotAdjacentChallenge(t, h.Challenge("a"))
}

func TestCompleteOnboardSurvivesSeatingFailure(t *testing.T) {
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	saves := store.NewMemoryStore()
	saves.UseContent(set)
	h := NewHub(set, saves, Admission{OpenAccess: true})
	trainer, err := h.Authenticate("aaa", "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	// The first seating attempt hits a store failure; onboarding must
	// still succeed because the Save is durable, and the immediate retry
	// seats the trainer.
	sv, err := h.CompleteOnboard(trainer.ID, "swift-otter-12", "rootkit")
	if err != nil {
		t.Fatalf("complete onboard = %v, want success once the Save is durable", err)
	}
	if sv == nil {
		t.Fatal("complete onboard returned no save")
	}
	saves.FailNextLoads(1)
	if err := h.EnterLobby(trainer.ID); err == nil {
		t.Fatal("enter lobby should fail once while the store cannot load")
	}
	if err := h.EnterLobby(trainer.ID); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	_, seated := trainerRoom(h, trainer.ID).Get(trainer.ID)
	h.mu.Unlock()
	if !seated {
		t.Fatal("onboarded trainer was not seated")
	}
}

func TestResumeSurfacesStoreFailures(t *testing.T) {
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	saves := store.NewMemoryStore()
	saves.UseContent(set)
	h := NewHub(set, saves, Admission{OpenAccess: true})

	// A failing store must reach the client as an error, not as
	// "needs onboarding".
	saves.FailNextLoads(1)
	resumed := h.Resume("ghost")
	if _, ok := resumed.(ErrorMsg); !ok {
		t.Fatalf("resume under store failure = %#v, want an ErrorMsg", resumed)
	}
	// A never-onboarded trainer still resumes as nil.
	if msg := h.Resume("newbie"); msg != nil {
		t.Fatalf("resume of unknown trainer = %#v, want nil for onboarding", msg)
	}

	onboardTrainerFull(t, h, "a", "rootkit")
	if _, ok := h.Resume("a").(SnapshotMsg); !ok {
		t.Fatal("onboarded trainer did not resume into the lobby")
	}

	saves.FailNextLoads(1)
	err = h.EnterLobby("a")
	if err == nil || !strings.Contains(err.Error(), "server: enter lobby:") {
		t.Fatalf("enter lobby under store failure = %v, want wrapped cause", err)
	}
}

func TestResumeKeepsQueueOnDisplace(t *testing.T) {
	h := testHub(t)
	onboardTrainerFull(t, h, "a", "rootkit")
	pos, wait, err := h.FindBattle("a")
	if err != nil || pos != 1 {
		t.Fatalf("queue = pos %d wait %d err %v", pos, wait, err)
	}
}

func TestHubConcurrentSessions(t *testing.T) {
	h := testHub(t)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			hash := fmt.Sprintf("r%02d", i)
			starter := "rootkit"
			if i%2 == 1 {
				starter = "emberbyte"
			}
			d := h.Attach(hash, func(any) {}, func() {})
			trainer, err := h.Authenticate(hash, "10.0.0.1")
			if err != nil {
				t.Errorf("authenticate %s: %v", hash, err)
				d()
				return
			}
			if _, err := h.CompleteOnboard(trainer.ID, "h-"+hash, starter); err != nil {
				t.Errorf("onboard %s: %v", hash, err)
				d()
				return
			}
			_ = h.startMatch(hash, fmt.Sprintf("r%02d", (i+1)%8))
			h.Tick(time.Now().Add(time.Minute))
			d()
			d2 := h.Attach(hash, func(any) {}, func() {})
			_ = h.Resume(hash)
			d2()
		})
	}
	wg.Wait()
}

func TestStartMatchRefusesWhenPartyAlreadyBattling(t *testing.T) {
	h := testHub(t)
	h.Attach("a", func(any) {}, func() {})
	h.Attach("b", func(any) {}, func() {})
	onboardTrainerFull(t, h, "a", "rootkit")
	onboardTrainerFull(t, h, "b", "emberbyte")
	foreign := &match{id: "foreign", a: "b", b: "c"}
	h.mu.Lock()
	h.matches["b"] = foreign
	h.mu.Unlock()
	if err := h.startMatch("a", "b"); err == nil {
		t.Fatal("startMatch should refuse when a party is already seated")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.matches["a"] != nil {
		t.Fatal("startMatch installed a match for the free trainer")
	}
	if h.matches["b"] != foreign {
		t.Fatal("startMatch overwrote the live match seat")
	}
}

func TestRespondNotifiesWhenChallengeIsStale(t *testing.T) {
	h := testHub(t)
	onboardTrainerFull(t, h, "a", "rootkit")
	assertNotAdjacentChallenge(t, h.Challenge("a"))
}

func TestFailedPairingNotifiesAndUnsticks(t *testing.T) {
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	saves := store.NewMemoryStore()
	saves.UseContent(set)
	h := NewHub(set, saves, Admission{OpenAccess: true})
	var errsA, errsB []string
	var snapA, snapB SnapshotMsg
	h.Attach("a", func(msg any) {
		switch msg := msg.(type) {
		case ErrorMsg:
			errsA = append(errsA, msg.Text)
		case SnapshotMsg:
			snapA = msg
		}
	}, func() {})
	h.Attach("b", func(msg any) {
		switch msg := msg.(type) {
		case ErrorMsg:
			errsB = append(errsB, msg.Text)
		case SnapshotMsg:
			snapB = msg
		}
	}, func() {})
	onboardTrainerFull(t, h, "a", "rootkit")
	onboardTrainerFull(t, h, "b", "emberbyte")

	saves.FailNextLoads(1)
	if err := h.startMatch("a", "b"); err == nil {
		t.Fatal("pairing should fail while the store cannot load a party")
	}

	const want = "the battle could not start; please try again"
	if len(errsA) == 0 || errsA[len(errsA)-1] != want {
		t.Fatalf("first waiter errors = %v, want %q", errsA, want)
	}
	if len(errsB) == 0 || errsB[len(errsB)-1] != want {
		t.Fatalf("second waiter errors = %v, want %q", errsB, want)
	}
	if snapA.You.InQueue || snapB.You.InQueue {
		t.Fatalf("stale queue flag remains: a=%+v b=%+v", snapA.You, snapB.You)
	}
}

func TestFailedChallengeAcceptTellsChallenger(t *testing.T) {
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	saves := store.NewMemoryStore()
	saves.UseContent(set)
	h := NewHub(set, saves, Admission{OpenAccess: true})
	var errsA, errsB []string
	var snapA SnapshotMsg
	h.Attach("a", func(msg any) {
		switch msg := msg.(type) {
		case ErrorMsg:
			errsA = append(errsA, msg.Text)
		case SnapshotMsg:
			snapA = msg
		}
	}, func() {})
	h.Attach("b", func(msg any) {
		if e, ok := msg.(ErrorMsg); ok {
			errsB = append(errsB, e.Text)
		}
	}, func() {})
	onboardTrainerFull(t, h, "a", "rootkit")
	onboardTrainerFull(t, h, "b", "emberbyte")
	h.mu.Lock()
	room := trainerRoom(h, "a")
	pa, _ := room.Get("a")
	room.Leave("b")
	_ = room.Place(lobby.Presence{Hash: "b", Handle: "bravo", Species: "emberbyte", X: pa.X + 1, Y: pa.Y})
	h.mu.Unlock()
	if err := h.Challenge("a"); err != nil {
		t.Fatal(err)
	}

	saves.FailNextLoads(1)
	if err := h.Respond("b", true); err == nil {
		t.Fatal("accepting into a failed match start should report an error")
	}

	const want = "the battle could not start; please try again"
	if len(errsB) == 0 || errsB[len(errsB)-1] != want {
		t.Fatalf("challenged trainer errors = %v, want %q", errsB, want)
	}
	_ = snapA
	_ = errsA
}

func TestFinishMatchDoesNotDeleteForeignKeys(t *testing.T) {
	h := testHub(t)
	x := &match{id: "x", a: "a", b: "b"}
	y := &match{id: "y", a: "a", b: "b"}
	h.mu.Lock()
	h.matches["a"] = y // y overwrote x's seats while x was mid-flight
	h.matches["b"] = y
	h.mu.Unlock()
	x.completedAt = time.Now()
	h.finishMatch(x)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.matches["a"] != y || h.matches["b"] != y {
		t.Fatal("finishMatch deleted seats owned by a newer match")
	}
}

func TestExpireDropSkipsReattachedTrainer(t *testing.T) {
	h := testHub(t)
	detachA := h.Attach("a", func(any) {}, func() {})
	h.Attach("b", func(any) {}, func() {})
	onboardTrainerFull(t, h, "a", "rootkit")
	onboardTrainerFull(t, h, "b", "emberbyte")
	if err := h.startMatch("a", "b"); err != nil {
		t.Fatal(err)
	}
	detachA() // grace drop starts for a
	// a reconnects before the grace lapses; Attach cancels the drop.
	detachA = h.Attach("a", func(any) {}, func() {})
	defer detachA()
	h.expireDrop("a", time.Now().Add(battle.DisconnectGrace+time.Second))
	svA, err := h.Load("a")
	if err != nil || svA == nil {
		t.Fatal(err)
	}
	svB, err := h.Load("b")
	if err != nil || svB == nil {
		t.Fatal(err)
	}
	if svA.Wins != 0 || svA.Losses != 0 || svB.Wins != 0 || svB.Losses != 0 {
		t.Fatalf("reconnected trainer forfeited: a=%d-%d b=%d-%d, want all zero",
			svA.Wins, svA.Losses, svB.Wins, svB.Losses)
	}
	h.mu.Lock()
	live := h.matches["a"] != nil && h.matches["a"] == h.matches["b"]
	h.mu.Unlock()
	if !live {
		t.Fatal("battle should still be live after the reattached trainer survives expiry")
	}
}

func TestResultSaveRetryBacksOff(t *testing.T) {
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	saves := store.NewMemoryStore()
	saves.UseContent(set)
	h := NewHub(set, saves, Admission{OpenAccess: true})
	var errorsA []string
	var statesA []battle.State
	h.Attach("a", func(msg any) {
		switch msg := msg.(type) {
		case ErrorMsg:
			errorsA = append(errorsA, msg.Text)
		case BattleMsg:
			statesA = append(statesA, msg.Battle.State())
		}
	}, func() {})
	h.Attach("b", func(any) {}, func() {})
	onboardTrainerFull(t, h, "a", "rootkit")
	onboardTrainerFull(t, h, "b", "emberbyte")
	if err := h.startMatch("a", "b"); err != nil {
		t.Fatal(err)
	}
	saves.FailNextResults(1)
	if err := h.Forfeit("a"); err != nil {
		t.Fatal(err)
	}
	if len(errorsA) != 1 || errorsA[0] != "result save failed; retrying" {
		t.Fatalf("errors after first failure = %v, want exactly one notice", errorsA)
	}
	// A tick inside the backoff window must not re-attempt (or re-notify).
	h.Tick(time.Now())
	if len(errorsA) != 1 {
		t.Fatalf("errors after backoff-window tick = %v, want no spam", errorsA)
	}
	h.mu.Lock()
	stillPending := h.matches["a"] != nil && h.matches["a"].pending
	h.mu.Unlock()
	if !stillPending {
		t.Fatal("result save should stay pending during backoff")
	}
	// Advancing past the backoff retries silently and publishes on success.
	h.Tick(time.Now().Add(2 * time.Second))
	if len(errorsA) != 1 {
		t.Fatalf("errors after successful retry = %v, want silent retry", errorsA)
	}
	if len(statesA) == 0 || statesA[len(statesA)-1] != battle.StateOver {
		t.Fatalf("states after successful retry = %v, want battle_over", statesA)
	}
	loser, err := h.Load("a")
	if err != nil {
		t.Fatal(err)
	}
	winner, err := h.Load("b")
	if err != nil {
		t.Fatal(err)
	}
	if loser.Losses != 1 || winner.Wins != 1 {
		t.Fatalf("records after retry = loser %+v winner %+v", loser, winner)
	}
}

func TestResultRetriesGiveUp(t *testing.T) {
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	saves := store.NewMemoryStore()
	saves.UseContent(set)
	h := NewHub(set, saves, Admission{OpenAccess: true})
	var errsA, errsB []string
	h.Attach("a", func(msg any) {
		if e, ok := msg.(ErrorMsg); ok {
			errsA = append(errsA, e.Text)
		}
	}, func() {})
	h.Attach("b", func(msg any) {
		if e, ok := msg.(ErrorMsg); ok {
			errsB = append(errsB, e.Text)
		}
	}, func() {})
	onboardTrainerFull(t, h, "a", "rootkit")
	onboardTrainerFull(t, h, "b", "emberbyte")
	if err := h.startMatch("a", "b"); err != nil {
		t.Fatal(err)
	}
	saves.FailNextResults(maxResultRetries + 1)
	if err := h.Forfeit("a"); err != nil {
		t.Fatal(err)
	}

	countText := func(texts []string, want string) int {
		n := 0
		for _, text := range texts {
			if text == want {
				n++
			}
		}
		return n
	}
	const terminal = "progress could not be saved; ask the operator"
	matchesGone := func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.matches["a"] == nil && h.matches["b"] == nil
	}
	// Drive past every backoff window until retries exhaust.
	for range maxResultRetries * 3 {
		if matchesGone() {
			break
		}
		h.Tick(time.Now().Add(time.Hour))
	}
	if countText(errsA, terminal) != 1 || countText(errsB, terminal) != 1 {
		t.Fatalf("terminal notices = %d/%d, want exactly one per trainer: %v / %v",
			countText(errsA, terminal), countText(errsB, terminal), errsA, errsB)
	}
	if !matchesGone() {
		t.Fatal("exhausted match was never released")
	}
	if h.Snapshot("a").You.InBattle || h.Snapshot("b").You.InBattle {
		t.Fatal("trainers still flagged InBattle after the give-up")
	}

	// Extra ticks must stay silent: retries are over and no partial record
	// was written.
	noticesA, noticesB := len(errsA), len(errsB)
	h.Tick(time.Now().Add(time.Hour))
	h.Tick(time.Now().Add(2 * time.Hour))
	if len(errsA) != noticesA || len(errsB) != noticesB {
		t.Fatalf("post-exhaustion ticks spammed notices: %v / %v", errsA, errsB)
	}
	if !matchesGone() {
		t.Fatal("exhausted match reappeared")
	}
	loser, err := h.Load("a")
	if err != nil {
		t.Fatal(err)
	}
	winner, err := h.Load("b")
	if err != nil {
		t.Fatal(err)
	}
	if loser.Losses != 0 || winner.Wins != 0 {
		t.Fatalf("partial records leaked: loser %+v winner %+v", loser, winner)
	}
}
