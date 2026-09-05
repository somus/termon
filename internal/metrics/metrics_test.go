package metrics

import (
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"termon.sh/internal/content"
	"termon.sh/internal/game"
	"termon.sh/internal/server"
	"termon.sh/internal/store"
)

func TestMetricsExposeRuntimeAndStoreBehavior(t *testing.T) {
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	raw := store.NewMemoryStore()
	raw.UseContent(set)
	observed := New()
	saves := observed.WrapStore(raw)
	for _, id := range []string{"a", "b"} {
		if _, err := saves.CreateTrainer(id); err != nil {
			t.Fatal(err)
		}
		if err := saves.CompleteOnboarding(id, &game.Save{
			Handle: id, Collection: []game.Monster{{Species: "rootkit"}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	raw.FailNextResults(1)
	result := store.BattleRecord{Result: store.BattleResult{
		ID: "battle-1", Winner: "a", Loser: "b", Reason: "forfeit", CompletedAt: time.Now(),
	}}
	if _, err := saves.RecordBattleResult(result); err == nil {
		t.Fatal("injected Store failure succeeded")
	}
	if _, err := saves.RecordBattleResult(result); err != nil {
		t.Fatal(err)
	}

	hub := server.NewHub(set, saves, server.Admission{OpenAccess: true})
	if err := hub.EnterLobby("a"); err != nil {
		t.Fatal(err)
	}
	if err := hub.EnterLobby("b"); err != nil {
		t.Fatal(err)
	}
	sqlite, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "termon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlite.Close() })
	observed.RegisterRuntime(hub, sqlite)

	recorder := httptest.NewRecorder()
	observed.Handler().ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	body := recorder.Body.String()
	for _, want := range []string{
		`termon_battle_results_total{reason="forfeit"} 1`,
		`termon_store_failures_total{operation="record_battle_result"} 1`,
		`termon_battle_result_persistence_duration_seconds_count 2`,
		`termon_trainers 2`,
		`termon_dojos 1`,
		`termon_active_ssh_sessions 0`,
		`termon_queue_depth 0`,
		`termon_active_battles 0`,
		`termon_sqlite_wait_seconds_total 0`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output does not contain %q", want)
		}
	}
}

func TestLoginGateSignalsExposeWaitAndDrops(t *testing.T) {
	observed := New()
	observed.ObserveLoginWait(750 * time.Millisecond)
	observed.ObserveLoginDrop()
	recorder := httptest.NewRecorder()
	observed.Handler().ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	body := recorder.Body.String()
	for _, want := range []string{
		`termon_login_wait_seconds_bucket{le="0.5"} 0`,
		`termon_login_wait_seconds_bucket{le="1"} 1`,
		`termon_login_wait_seconds_count 1`,
		`termon_login_drops_total 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output does not contain %q", want)
		}
	}
}

func TestExpectedStoreMissIsNotFailure(t *testing.T) {
	observed := New()
	_, err := observed.WrapStore(store.NewMemoryStore()).ResolveCredential("unknown")
	if err == nil {
		t.Fatal("unknown credential resolved")
	}
	recorder := httptest.NewRecorder()
	observed.Handler().ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	if strings.Contains(recorder.Body.String(), "termon_store_failures_total{") {
		t.Fatal("expected Store miss was counted as a failure")
	}
}

func TestInstrumentedHubEmitsAdmissionAndMatchmakingSignals(t *testing.T) {
	raw := store.NewMemoryStore()
	observed := New()
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	raw.UseContent(set)
	hub := server.NewHub(set, observed.WrapStore(raw), server.Admission{
		OpenAccess:         true,
		RegistrationsPerIP: 1,
	})
	hub.Instrument(observed, nil)

	// Registration outcomes: create, deny-by-quota, create from new source.
	first, err := hub.Authenticate("hash-a", "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Authenticate("hash-b", "10.0.0.1"); !errors.Is(err, server.ErrTooManyRegistrations) {
		t.Fatalf("second registration from one source = %v, want ErrTooManyRegistrations", err)
	}
	second, err := hub.Authenticate("hash-b", "10.0.0.2")
	if err != nil {
		t.Fatal(err)
	}

	// Seat both with full parties and pair them through the Queue.
	for _, id := range []string{first.ID, second.ID} {
		if _, err := hub.CompleteOnboard(id, id, "rootkit"); err != nil {
			t.Fatal(err)
		}
		for _, species := range []string{"emberbyte", "aquabit"} {
			sv, _ := hub.Load(id)
			if game.FullParty(sv) {
				break
			}
			if _, err := raw.RecordActivityResult(store.ActivityRecord{
				Kind: "lesson", NaturalKey: id + ":metrics-fill:" + species,
				TrainerID: id, Outcome: "captured",
				Capture:     &store.CaptureSpec{Species: species, FillParty: true},
				CompletedAt: time.Now(),
			}); err != nil {
				t.Fatal(err)
			}
		}
		detach := hub.Attach(id, func(any) {}, func() {})
		defer detach()
	}
	if err := hub.StartMatch(first.ID, second.ID); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	observed.Handler().ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	body := recorder.Body.String()
	for _, want := range []string{
		`termon_registrations_total{outcome="created"} 2`,
		`termon_registrations_total{outcome="denied_quota"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output does not contain %q", want)
		}
	}
}
