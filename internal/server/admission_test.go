package server

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"termon.sh/internal/content"
	"termon.sh/internal/store"
)

func TestRegistrationLimiterQuotaPerSource(t *testing.T) {
	r := newRegistrationLimiter(Admission{RegistrationsPerIP: 3, RegistrationWindow: time.Hour})
	now := time.Now()

	for i := range 3 {
		if !r.allow("203.0.113.1", now) {
			t.Fatalf("creation %d within quota rejected", i+1)
		}
	}
	if r.allow("203.0.113.1", now) {
		t.Fatal("creation over quota allowed")
	}
	if !r.allow("203.0.113.2", now) {
		t.Fatal("independent source blocked by another source's quota")
	}
}

func TestRegistrationLimiterWindowReset(t *testing.T) {
	r := newRegistrationLimiter(Admission{RegistrationsPerIP: 1, RegistrationWindow: time.Hour})
	start := time.Now()

	if !r.allow("203.0.113.1", start) {
		t.Fatal("first creation rejected")
	}
	if r.allow("203.0.113.1", start.Add(time.Minute)) {
		t.Fatal("second creation allowed inside window")
	}
	if !r.allow("203.0.113.1", start.Add(time.Hour)) {
		t.Fatal("creation after window expiry rejected")
	}
}

func TestAuthenticateRegistrationGates(t *testing.T) {
	set := testSet(t)
	shared := store.NewMemoryStore()

	seeder := NewHub(set, shared, Admission{OpenAccess: true})
	created, err := seeder.Authenticate("credential-a", "203.0.113.9")
	if err != nil {
		t.Fatalf("seed Authenticate: %v", err)
	}

	closed := NewHub(set, shared, Admission{})
	got, err := closed.Authenticate("credential-a", "203.0.113.9")
	if err != nil {
		t.Fatalf("existing Trainer resolve under closed registration: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("resolved Trainer = %s, want %s", got.ID, created.ID)
	}
	if _, err := closed.Authenticate("unknown-credential", "203.0.113.9"); !errors.Is(err, ErrRegistrationDisabled) {
		t.Fatalf("closed registration error = %v, want ErrRegistrationDisabled", err)
	}

	limited := NewHub(set, shared, Admission{
		OpenAccess:         true,
		RegistrationsPerIP: 1,
		RegistrationWindow: time.Hour,
	})
	if _, err := limited.Authenticate("fresh-credential", "203.0.113.9"); err != nil {
		t.Fatalf("first creation within quota failed: %v", err)
	}
	if _, err := limited.Authenticate("another-credential", "203.0.113.9"); !errors.Is(err, ErrTooManyRegistrations) {
		t.Fatalf("over-quota creation error = %v, want ErrTooManyRegistrations", err)
	}
	if _, err := limited.Authenticate("other-credential", "203.0.113.10"); err != nil {
		t.Fatalf("creation from other source blocked: %v", err)
	}
}

func testSet(t *testing.T) *content.Set {
	t.Helper()
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestRegistrationLimiterUnlimitedMode(t *testing.T) {
	r := newRegistrationLimiter(Admission{RegistrationsPerIP: -1})
	now := time.Now()
	for i := range 1000 {
		if !r.allow("203.0.113.1", now) {
			t.Fatalf("unlimited limiter rejected creation %d", i+1)
		}
	}
}

func TestRegistrationLimiterCapsTrackedSources(t *testing.T) {
	r := newRegistrationLimiter(Admission{RegistrationsPerIP: 3, RegistrationWindow: time.Hour})
	start := time.Now()

	for i := range maxTrackedSources {
		source := fmt.Sprintf("203.0.%d.%d", i/256, i%256)
		if !r.allow(source, start) {
			t.Fatalf("source %d denied before cap", i)
		}
	}
	if r.allow("198.51.100.1", start.Add(time.Minute)) {
		t.Fatal("new source admitted while cap full of warm entries")
	}
	// An existing tracked source still counts normally toward its quota.
	if !r.allow("203.0.0.0", start.Add(time.Minute)) {
		t.Fatal("tracked source denied while under its own quota")
	}
	// After the window lapses, prune makes room for new sources.
	if !r.allow("198.51.100.1", start.Add(2*time.Hour)) {
		t.Fatal("new source denied after expired entries pruned")
	}
}
