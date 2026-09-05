// Package queue is the global FIFO that pairs Trainers into Battles.
//
// Not safe for concurrent use: every caller must hold the Hub mutex.
package queue

import (
	"errors"
	"time"
)

// Queue is a single waiting line with per-waiter enqueue times so pairing
// can report how long each Trainer waited.
type Queue struct {
	order      []string
	enqueuedAt map[string]time.Time
}

// Join appends hash unless already waiting. position is 1-based. now stamps
// the waiter's arrival for wait-time observation.
func (q *Queue) Join(hash string, now time.Time) (position, waiting int, err error) {
	if hash == "" {
		return 0, len(q.order), errors.New("queue: empty hash")
	}
	if q.enqueuedAt == nil {
		q.enqueuedAt = map[string]time.Time{}
	}
	for i, h := range q.order {
		if h == hash {
			return i + 1, len(q.order), nil
		}
	}
	q.order = append(q.order, hash)
	q.enqueuedAt[hash] = now
	return len(q.order), len(q.order), nil
}

// Cancel removes hash from the line, reporting whether it was waiting.
func (q *Queue) Cancel(hash string) bool {
	_, wasWaiting := q.enqueuedAt[hash]
	out := q.order[:0]
	for _, h := range q.order {
		if h != hash {
			out = append(out, h)
		}
	}
	q.order = out
	delete(q.enqueuedAt, hash)
	return wasWaiting
}

// Position is 1-based. ok is false when hash is not waiting.
func (q *Queue) Position(hash string) (position, waiting int, ok bool) {
	for i, h := range q.order {
		if h == hash {
			return i + 1, len(q.order), true
		}
	}
	return 0, len(q.order), false
}

// Members reports the hashes currently waiting in the line. Callers that
// need membership for many trainers at once (a broadcast pass over a Dojo)
// pay one pass here instead of one Position scan each.
func (q *Queue) Members() map[string]struct{} {
	members := make(map[string]struct{}, len(q.order))
	for _, hash := range q.order {
		members[hash] = struct{}{}
	}
	return members
}

// Pair pops the two oldest waiters when at least two are waiting, reporting
// how long each waited since joining.
func (q *Queue) Pair(now time.Time) (a, b string, waitA, waitB time.Duration, ok bool) {
	if len(q.order) < 2 {
		return "", "", 0, 0, false
	}
	a, b = q.order[0], q.order[1]
	waitA = max(0, now.Sub(q.enqueuedAt[a]))
	waitB = max(0, now.Sub(q.enqueuedAt[b]))
	q.order = append([]string{}, q.order[2:]...)
	delete(q.enqueuedAt, a)
	delete(q.enqueuedAt, b)
	return a, b, waitA, waitB, true
}

// Waiting is the current line length.
func (q *Queue) Waiting() int { return len(q.order) }
