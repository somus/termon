package server

import "time"

// Outcome label enums for Instruments. These are bounded by design — never
// add trainer hashes or source addresses as labels.
const (
	RegCreated        = "created"
	RegDeniedQuota    = "denied_quota"
	RegDeniedDisabled = "denied_disabled"
	RegDeniedError    = "denied_error"

	ChallengeIssued   = "issued"
	ChallengeAccepted = "accepted"
	ChallengeDeclined = "declined"
	ChallengeExpired  = "expired"

	QueueJoined    = "joined"
	QueuePaired    = "paired"
	QueueCancelled = "cancelled"
)

// Instruments counts admission and matchmaking events so operators can see
// registration pressure, challenge flow, and queue health. All methods must
// be safe for concurrent use; a nil Instruments disables instrumentation.
type Instruments interface {
	ObserveRegistration(outcome string)
	ObserveDisplacement()
	ObserveChallenge(outcome string)
	ObserveQueueJoin(outcome string)
	ObserveQueueWait(wait time.Duration)
}

// observeRegistration records a registration outcome (nil-safe).
func (h *Hub) observeRegistration(outcome string) {
	if h.instruments != nil {
		h.instruments.ObserveRegistration(outcome)
	}
}

// observeDisplacement records a seat takeover by a newer connection.
func (h *Hub) observeDisplacement(hash string) {
	if h.instruments != nil {
		h.instruments.ObserveDisplacement()
	}
	if h.logger != nil {
		h.logger.Warn("session displaced", "trainer", hash)
	}
}

func (h *Hub) observeChallenge(outcome string) {
	if h.instruments != nil {
		h.instruments.ObserveChallenge(outcome)
	}
}

func (h *Hub) observeQueueJoin(outcome string) {
	if h.instruments != nil {
		h.instruments.ObserveQueueJoin(outcome)
	}
}

func (h *Hub) observeQueueWait(wait time.Duration) {
	if h.instruments != nil && wait >= 0 {
		h.instruments.ObserveQueueWait(wait)
	}
}

// logWarn emits a server-side warning when a logger is attached.
func (h *Hub) logWarn(msg string, args ...any) {
	if h.logger != nil {
		h.logger.Warn(msg, args...)
	}
}
