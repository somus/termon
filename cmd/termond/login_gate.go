package main

import (
	"io"
	"time"

	"charm.land/ssh"
	"charm.land/wish/v2"
	"golang.org/x/time/rate"

	"termon.sh/internal/metrics"
)

const (
	defaultLoginRatePerSecond = 25
	defaultLoginBurst         = 128
	defaultLoginMaxWait       = 45 * time.Second

	// anticipatedNoticeDelay: sessions expected to wait longer than this are
	// told they are queued before admission; shorter waits finish before a
	// player would notice the line.
	anticipatedNoticeDelay = 250 * time.Millisecond

	loginWaitMessage = "the dojo is filling up; seating you shortly…"
	loginBusyMessage = "the dojo is very busy right now; please try again in a moment"
)

// loginGate admits SSH session starts at a bounded global rate so cold
// bursts queue briefly instead of stampeding startup. Waiting sessions get
// a status line; sessions that would wait past maxWait are closed.
type loginGate struct {
	limiter        *rate.Limiter
	maxWait        time.Duration
	exemptLoopback bool
	metrics        *metrics.Metrics
}

func newLoginGate(admissionsPerSecond float64, burst int, maxWait time.Duration, exemptLoopback bool, m *metrics.Metrics) *loginGate {
	return &loginGate{
		limiter:        rate.NewLimiter(rate.Limit(admissionsPerSecond), burst),
		maxWait:        maxWait,
		exemptLoopback: exemptLoopback,
		metrics:        m,
	}
}

// middleware returns the wish middleware enforcing the gate. Place it so it
// runs after the per-IP connection limiter and immediately before the
// Bubble Tea program starts.
func (g *loginGate) middleware() wish.Middleware {
	return func(next ssh.Handler) ssh.Handler {
		return func(sess ssh.Session) {
			if g.admit(sess) {
				next(sess)
			}
		}
	}
}

// admit blocks until the gate admits sess. It reports whether the session
// may proceed to the program handler.
func (g *loginGate) admit(sess ssh.Session) bool {
	if g.exemptLoopback && isLoopbackAddr(sess.RemoteAddr()) {
		// Local dev/load-test tooling only: measure raw capacity untouched.
		return true
	}
	reservation := g.limiter.Reserve()
	delay := reservation.Delay()
	if delay > g.maxWait {
		reservation.Cancel()
		g.metrics.ObserveLoginDrop()
		_, _ = io.WriteString(sess, loginBusyMessage+"\n")
		_ = sess.Exit(1)
		return false
	}
	if delay > anticipatedNoticeDelay {
		_, _ = io.WriteString(sess, loginWaitMessage+"\n")
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	g.metrics.ObserveLoginWait(delay)
	return true
}
