package website

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/crypto/ssh"

	"termon.sh/internal/telemetry"
)

type eventRecorder struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (r *eventRecorder) Record(event telemetry.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func newTestWebsite(t *testing.T, recorder telemetry.Recorder) http.Handler {
	t.Helper()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := New(key, func() int { return 0 }, recorder)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func TestWebsiteEvents(t *testing.T) {
	visitID := telemetry.NewID()
	payload := func(name, outcome string) string {
		return fmt.Sprintf(`{"visit_id":%q,"event":%q,"outcome":%q}`, visitID, name, outcome)
	}
	valid := payload(telemetry.EventWebsitePageViewed, "")
	for _, tc := range []struct {
		name, body, contentType, origin, fetchSite string
		status                                     int
	}{
		{name: "page", body: valid, status: 204},
		{name: "copy success", body: payload(telemetry.EventWebsiteCommandCopied, "success"), status: 204},
		{name: "copy fallback", body: payload(telemetry.EventWebsiteCommandCopied, "fallback"), status: 204},
		{name: "demo play", body: payload(telemetry.EventWebsiteDemoToggled, "play"), status: 204},
		{name: "demo pause", body: payload(telemetry.EventWebsiteDemoToggled, "pause"), status: 204},
		{name: "instructions", body: payload(telemetry.EventWebsiteInstructionsOpened, ""), status: 204},
		{name: "same origin", body: valid, origin: "https://termon.sh", status: 204},
		{name: "fetch same origin", body: valid, fetchSite: "same-origin", status: 204},
		{name: "charset", body: valid, contentType: "application/json; charset=utf-8", status: 204},
		{name: "unknown field", body: strings.TrimSuffix(valid, "}") + `,"ip":"secret"}`, status: 400},
		{name: "game event", body: payload(telemetry.EventSessionStarted, ""), status: 400},
		{name: "unknown event", body: payload("unknown", ""), status: 400},
		{name: "missing copy outcome", body: payload(telemetry.EventWebsiteCommandCopied, ""), status: 400},
		{name: "invalid outcome", body: payload(telemetry.EventWebsiteCommandCopied, "play"), status: 400},
		{name: "unexpected outcome", body: payload(telemetry.EventWebsitePageViewed, "success"), status: 400},
		{name: "invalid visit", body: strings.Replace(valid, visitID, "trainer-1", 1), status: 400},
		{name: "missing visit", body: `{"event":"website:page_view"}`, status: 400},
		{name: "null", body: `null`, status: 400},
		{name: "array", body: `[]`, status: 400},
		{name: "malformed", body: `{`, status: 400},
		{name: "empty", status: 400},
		{name: "trailing JSON", body: valid + `{}`, status: 400},
		{name: "wrong type", body: valid, contentType: "text/plain", status: 400},
		{name: "missing type", body: valid, contentType: "missing", status: 400},
		{name: "at limit", body: valid + strings.Repeat(" ", 1024-len(valid)), status: 204},
		{name: "over limit", body: valid + strings.Repeat(" ", 1025-len(valid)), status: 413},
		{name: "cross origin", body: valid, origin: "https://example.com", status: 403},
		{name: "null origin", body: valid, origin: "null", status: 403},
		{name: "cross site", body: valid, fetchSite: "cross-site", status: 403},
		{name: "same site other origin", body: valid, fetchSite: "same-site", status: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := &eventRecorder{}
			handler := newTestWebsite(t, recorder)
			request := httptest.NewRequest(http.MethodPost, "https://termon.sh/api/events?secret=query", strings.NewReader(tc.body))
			contentType := tc.contentType
			if contentType == "" {
				contentType = "application/json"
			}
			if contentType != "missing" {
				request.Header.Set("Content-Type", contentType)
			}
			request.Header.Set("Origin", tc.origin)
			request.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			request.Header.Set("Referer", "https://private.example/secret")
			request.Header.Set("User-Agent", "private-agent")
			request.RemoteAddr = "203.0.113.8:1234"
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d", response.Code, tc.status)
			}
			if response.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("event endpoint grants CORS access")
			}
			if tc.status != http.StatusNoContent {
				if len(recorder.events) != 0 {
					t.Fatal("rejected event was recorded")
				}
				return
			}
			if len(recorder.events) != 1 {
				t.Fatalf("recorded %d events", len(recorder.events))
			}
			event := recorder.events[0]
			if event.VisitID != visitID || event.TrainerID != "" || event.Properties["page"] != "/" {
				t.Fatalf("unexpected event: %+v", event)
			}
			for key := range event.Properties {
				if key != "page" && key != "outcome" {
					t.Errorf("unexpected property %q", key)
				}
			}
		})
	}
}

func TestWebsiteAnalyticsEnabled(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(strconv.FormatBool(enabled), func(t *testing.T) {
			var recorder telemetry.Recorder
			if enabled {
				recorder = &eventRecorder{}
			}
			handler := newTestWebsite(t, recorder)
			page := httptest.NewRecorder()
			handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
			if !strings.Contains(page.Body.String(), fmt.Sprintf(`data-analytics-enabled="%t"`, enabled)) {
				t.Fatal("page has wrong analytics flag")
			}
			if strings.Contains(page.Body.String(), "We use PostHog") != enabled {
				t.Fatal("privacy notice does not match analytics state")
			}
			if got := page.Header().Get("Content-Security-Policy"); got != "default-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'" {
				t.Fatalf("CSP changed: %s", got)
			}
			for _, method := range []string{http.MethodPost, http.MethodGet, http.MethodOptions} {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(method, "/api/events", nil))
				if !enabled && response.Code != http.StatusNotFound {
					t.Errorf("disabled endpoint returned %d", response.Code)
				}
				if enabled && method != http.MethodPost && response.Code != http.StatusMethodNotAllowed {
					t.Errorf("%s returned %d", method, response.Code)
				}
			}
		})
	}
}

func TestWebsiteEventsRateLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		recorder := &eventRecorder{}
		handler := newTestWebsite(t, recorder)
		post := func() int {
			body := fmt.Sprintf(`{"visit_id":%q,"event":"website:page_view"}`, telemetry.NewID())
			r := httptest.NewRequest(http.MethodPost, "/api/events", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			return w.Code
		}
		var wg sync.WaitGroup
		for range 40 {
			wg.Go(func() {
				if code := post(); code != http.StatusNoContent {
					t.Errorf("burst request = %d", code)
				}
			})
		}
		wg.Wait()
		if code := post(); code != http.StatusTooManyRequests {
			t.Fatalf("exhausted limit = %d", code)
		}
		time.Sleep(50 * time.Millisecond)
		if code := post(); code != http.StatusNoContent {
			t.Fatalf("refilled token = %d", code)
		}
		if code := post(); code != http.StatusTooManyRequests {
			t.Fatalf("only one token should refill, got %d", code)
		}
		if len(recorder.events) != 41 {
			t.Fatalf("recorded %d events", len(recorder.events))
		}
	})
}
