package server

import (
	"errors"
	"sync"
	"time"
)

const (
	defaultRegistrationsPerIP = 10
	defaultRegistrationWindow = time.Hour
	maxTrackedSources         = 4096
)

// ErrRegistrationDisabled means open registration is switched off.
var ErrRegistrationDisabled = errors.New("server: registration disabled")

// ErrTooManyRegistrations means the source created too many new Trainers
// inside the quota window.
var ErrTooManyRegistrations = errors.New("server: too many registrations from this address")

// Admission configures open-registration behavior. The zero value enables
// registration with package-default quotas.
type Admission struct {
	// OpenAccess allows unknown SSH credentials to create a Trainer. When
	// false, only existing credentials can connect.
	OpenAccess bool
	// RegistrationsPerIP caps new Trainer creations per source per window.
	// Zero selects defaultRegistrationsPerIP; negative means unlimited
	// (load-test harnesses).
	RegistrationsPerIP int
	// RegistrationWindow is the fixed quota window. Zero selects
	// defaultRegistrationWindow.
	RegistrationWindow time.Duration
}

func (a Admission) limit() int {
	if a.RegistrationsPerIP != 0 {
		return a.RegistrationsPerIP
	}
	return defaultRegistrationsPerIP
}

func (a Admission) window() time.Duration {
	if a.RegistrationWindow > 0 {
		return a.RegistrationWindow
	}
	return defaultRegistrationWindow
}

type sourceQuota struct {
	count   int
	resetAt time.Time
}

// registrationLimiter is a fixed-window counter of new-Trainer creations
// keyed by source address. Entries expire with their window; when
// maxTrackedSources warm entries survive pruning, new sources are denied so
// memory stays bounded.
type registrationLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	sources map[string]*sourceQuota
}

func newRegistrationLimiter(a Admission) *registrationLimiter {
	return &registrationLimiter{
		limit:   a.limit(),
		window:  a.window(),
		sources: map[string]*sourceQuota{},
	}
}

// allow reports whether source may create a new Trainer at now.
func (r *registrationLimiter) allow(source string, now time.Time) bool {
	if r.limit < 0 {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.sources) >= maxTrackedSources {
		r.pruneLocked(now)
	}
	quota, ok := r.sources[source]
	if !ok || !now.Before(quota.resetAt) {
		if !ok && len(r.sources) >= maxTrackedSources {
			// Cap reached with no expired entries to prune: deny new
			// sources so tracked memory stays bounded.
			return false
		}
		r.sources[source] = &sourceQuota{count: 1, resetAt: now.Add(r.window)}
		return true
	}
	if quota.count >= r.limit {
		return false
	}
	quota.count++
	return true
}

func (r *registrationLimiter) pruneLocked(now time.Time) {
	for source, quota := range r.sources {
		if !now.Before(quota.resetAt) {
			delete(r.sources, source)
		}
	}
}
