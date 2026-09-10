package telemetry

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebsiteEventValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Event)
	}{
		{name: "missing visit", mutate: func(e *Event) { e.VisitID = "" }},
		{name: "invalid visit", mutate: func(e *Event) { e.VisitID = "trainer-1" }},
		{name: "nil UUID", mutate: func(e *Event) { e.VisitID = "00000000-0000-0000-0000-000000000000" }},
		{name: "trainer", mutate: func(e *Event) { e.TrainerID = "trainer-1" }},
		{name: "session", mutate: func(e *Event) { e.SessionID = NewID() }},
		{name: "battle", mutate: func(e *Event) { e.BattleID = "battle-1" }},
		{name: "activity", mutate: func(e *Event) { e.ActivityID = "activity-1" }},
		{name: "error", mutate: func(e *Event) { e.ErrorID = "error-1" }},
		{name: "raw URL", mutate: func(e *Event) { e.Properties["page"] = "/?secret" }},
		{name: "unknown property", mutate: func(e *Event) { e.Properties["referrer"] = "private" }},
		{name: "profile override", mutate: func(e *Event) { e.Properties["$process_person_profile"] = true }},
		{name: "missing page", mutate: func(e *Event) { delete(e.Properties, "page") }},
		{name: "outcome type", mutate: func(e *Event) { e.Properties["outcome"] = 1 }},
		{name: "gameplay visit", mutate: func(e *Event) { e.Name = EventSessionStarted; e.TrainerID = "trainer-1" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event, err := NewWebsiteEvent(NewID(), EventWebsitePageViewed, "")
			if err != nil {
				t.Fatal(err)
			}
			event.ID = NewID()
			if err := validate(event); err != nil {
				t.Fatalf("valid event rejected: %v", err)
			}
			tc.mutate(&event)
			if err := validate(event); err == nil {
				t.Fatal("invalid event accepted")
			}
		})
	}
	if err := validate(Event{ID: NewID(), Name: EventSessionStarted}); err == nil {
		t.Fatal("gameplay event accepted without Trainer ID")
	}
}

func TestWebsitePostHogPayload(t *testing.T) {
	received := make(chan []byte, 1)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reader io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			compressed, err := gzip.NewReader(r.Body)
			if err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer func() { _ = compressed.Close() }()
			reader = compressed
		}
		body, err := io.ReadAll(reader)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer endpoint.Close()
	var logs bytes.Buffer
	client, err := New(slog.New(slog.NewJSONHandler(&logs, nil)), Config{
		APIKey: "phc_test", Host: endpoint.URL, Environment: "development", AppVersion: "test-version",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	visitID := NewID()
	event, err := NewWebsiteEvent(visitID, EventWebsiteCommandCopied, "success")
	if err != nil {
		t.Fatal(err)
	}
	client.Record(event)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := client.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-received:
		var payload struct {
			Batch []struct {
				Event      string         `json:"event"`
				DistinctID string         `json:"distinct_id"`
				UUID       string         `json:"uuid"`
				Timestamp  time.Time      `json:"timestamp"`
				Properties map[string]any `json:"properties"`
			} `json:"batch"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Batch) != 1 {
			t.Fatalf("batch = %s", body)
		}
		got := payload.Batch[0]
		if got.Event != event.Name || got.DistinctID != "website:"+visitID || got.UUID == "" || got.Timestamp.IsZero() {
			t.Fatalf("unexpected capture: %s", body)
		}
		for key, want := range map[string]any{
			"page": "/", "outcome": "success", "visit_id": visitID,
			"$process_person_profile": false, "$geoip_disable": true,
			"environment": "development", "app_version": "test-version",
		} {
			if got.Properties[key] != want {
				t.Errorf("property %s = %v, want %v", key, got.Properties[key], want)
			}
		}
		for _, key := range []string{"trainer_id", "$session_id", "ip", "$ip", "source_ip", "referrer", "$current_url", "user_agent"} {
			if _, ok := got.Properties[key]; ok {
				t.Errorf("unexpected property %q", key)
			}
		}
	case <-ctx.Done():
		t.Fatal("PostHog did not receive website capture")
	}
	if !strings.Contains(logs.String(), visitID) || strings.Contains(logs.String(), "trainer_id") {
		t.Fatalf("incorrect log identity: %s", logs.String())
	}
}
