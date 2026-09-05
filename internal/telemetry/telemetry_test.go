package telemetry

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type observations map[string]int

func (o observations) ObserveTelemetry(destination, outcome string) {
	o[destination+":"+outcome]++
}

func TestRecordWritesCorrelatedJSON(t *testing.T) {
	var buf bytes.Buffer
	obs := observations{}
	client, err := New(slog.New(slog.NewJSONHandler(&buf, nil)), Config{
		Environment: "test", AppVersion: "abc123",
	}, obs)
	if err != nil {
		t.Fatal(err)
	}
	sessionID := NewID()
	client.Record(Event{
		Name: EventBattleEnded, TrainerID: "trainer-1", SessionID: sessionID,
		BattleID: "battle-1", Properties: map[string]any{"result": "won"},
	})
	out := buf.String()
	for _, want := range []string{EventBattleEnded, "trainer_id", "trainer-1", sessionID, "battle-1", "abc123"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log %q does not contain %q", out, want)
		}
	}
	if obs["log:recorded"] != 1 {
		t.Fatalf("observations = %v", obs)
	}
}

func TestRecordRejectsSensitiveProperty(t *testing.T) {
	var buf bytes.Buffer
	obs := observations{}
	client, err := New(slog.New(slog.NewJSONHandler(&buf, nil)), Config{}, obs)
	if err != nil {
		t.Fatal(err)
	}
	client.Record(Event{
		Name: EventSessionStarted, TrainerID: "trainer-1",
		Properties: map[string]any{"fingerprint_hash": "secret"},
	})
	if obs["telemetry:invalid"] != 1 || strings.Contains(buf.String(), "secret") {
		t.Fatalf("invalid event handling: observations=%v log=%q", obs, buf.String())
	}
}

func TestPostHogCaptureReachesConfiguredEndpoint(t *testing.T) {
	received := make(chan string, 1)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reader io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			compressed, err := gzip.NewReader(r.Body)
			if err != nil {
				t.Errorf("gzip request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer func() { _ = compressed.Close() }()
			reader = compressed
		}
		body, err := io.ReadAll(reader)
		if err != nil {
			t.Errorf("read request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		select {
		case received <- string(body):
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer endpoint.Close()
	client, err := New(nil, Config{
		APIKey: "phc_test", Host: endpoint.URL, Environment: "test", AppVersion: "version-1",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sessionID := NewID()
	client.Record(Event{Name: EventSessionStarted, TrainerID: "trainer-1", SessionID: sessionID})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-received:
		for _, want := range []string{EventSessionStarted, "trainer-1", sessionID, "version-1"} {
			if !strings.Contains(body, want) {
				t.Fatalf("PostHog request %q does not contain %q", body, want)
			}
		}
	case <-ctx.Done():
		t.Fatal("PostHog endpoint did not receive capture")
	}
}

func TestPostHogLogsAndErrorsReachConfiguredEndpoint(t *testing.T) {
	type request struct {
		path, auth, body string
	}
	received := make(chan request, 4)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reader io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			compressed, err := gzip.NewReader(r.Body)
			if err != nil {
				t.Errorf("gzip request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer func() { _ = compressed.Close() }()
			reader = compressed
		}
		body, err := io.ReadAll(reader)
		if err != nil {
			t.Errorf("read request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received <- request{path: r.URL.Path, auth: r.Header.Get("Authorization"), body: string(body)}
		w.WriteHeader(http.StatusOK)
	}))
	defer endpoint.Close()

	var local bytes.Buffer
	obs := observations{}
	client, err := New(slog.New(slog.NewJSONHandler(&local, nil)), Config{
		APIKey: "phc_test", Host: endpoint.URL, Environment: "test",
		AppVersion: "version-1", LogsEnabled: true,
	}, obs)
	if err != nil {
		t.Fatal(err)
	}
	logger := client.WrapLogger(slog.New(slog.NewJSONHandler(&local, nil)))
	logger.Error("player operation failed",
		"operation", "save_party", "trainer_id", "trainer-1",
		"session_id", "session-1", "error_id", "ABCD-1234",
		"source", "203.0.113.8",
		slog.Group("network", "source_ip", "198.51.100.4", "transport", "ssh"),
		"err", errors.New("write failed"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Close(ctx); err != nil {
		t.Fatal(err)
	}

	var logs, exception *request
	for logs == nil || exception == nil {
		select {
		case got := <-received:
			switch got.path {
			case "/i/v1/logs":
				requestCopy := got
				logs = &requestCopy
			default:
				if strings.Contains(got.body, "$exception") {
					requestCopy := got
					exception = &requestCopy
				}
			}
		case <-ctx.Done():
			t.Fatalf("requests incomplete: logs=%v exception=%v", logs != nil, exception != nil)
		}
	}
	if logs.auth != "Bearer phc_test" {
		t.Fatalf("Logs Authorization = %q", logs.auth)
	}
	for _, want := range []string{"player operation failed", "trainer_id", "trainer-1", "session_id", "session-1"} {
		if !strings.Contains(logs.body, want) {
			t.Errorf("Logs body does not contain %q", want)
		}
	}
	for _, forbidden := range []string{"203.0.113.8", "198.51.100.4"} {
		if strings.Contains(logs.body, forbidden) || strings.Contains(exception.body, forbidden) {
			t.Fatalf("remote telemetry contains forbidden source address %q", forbidden)
		}
	}
	if !strings.Contains(logs.body, "transport") || !strings.Contains(logs.body, "ssh") {
		t.Fatal("remote telemetry lost allowed nested attributes")
	}
	for _, want := range []string{"$exception", "trainer-1", "ABCD-1234", "save_party", "version-1"} {
		if !strings.Contains(exception.body, want) {
			t.Errorf("exception body does not contain %q", want)
		}
	}
	if strings.Contains(exception.body, "write failed") {
		t.Fatal("exception body contains raw internal error text")
	}
	if !strings.Contains(local.String(), "203.0.113.8") || !strings.Contains(local.String(), "write failed") {
		t.Fatal("local diagnostic log lost source address or internal error")
	}
	if obs["posthog_logs:recorded"] != 1 || obs["posthog_logs:delivered"] != 1 {
		t.Fatalf("Logs observations = %v", obs)
	}
}

func TestPostHogNeedsExplicitHost(t *testing.T) {
	_, err := New(nil, Config{APIKey: "phc_test"}, nil)
	if err == nil || !strings.Contains(err.Error(), "POSTHOG_HOST") {
		t.Fatalf("New error = %v", err)
	}
}

func TestIDs(t *testing.T) {
	firstRandom, secondRandom := NewID(), NewID()
	if firstRandom == secondRandom {
		t.Fatal("NewID returned a duplicate")
	}
	first := DeterministicID("battle", "one")
	if first != DeterministicID("battle", "one") || first == DeterministicID("battle", "two") {
		t.Fatal("deterministic IDs are not stable and distinct")
	}
}

func TestCloseWithoutPostHog(t *testing.T) {
	client, err := New(nil, Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
