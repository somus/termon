package main

import (
	"bytes"
	"errors"
	"net"
	"sort"
	"strings"
	"testing"
	"time"

	"charm.land/ssh"
	gossh "golang.org/x/crypto/ssh"

	"termon.sh/internal/metrics"
)

// startGatedSSHServer serves a minimal SSH server whose handler chain is
// exactly the login gate followed by handle, mirroring termond's middleware
// placement (gate runs last before the program).
func startGatedSSHServer(t *testing.T, gate *loginGate, handle func(sess ssh.Session)) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var handler ssh.Handler = func(sess ssh.Session) { handle(sess) }
	srv := &ssh.Server{
		Handler:          gate.middleware()(handler),
		PublicKeyHandler: func(ssh.Context, ssh.PublicKey) bool { return true },
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(l) }()
	t.Cleanup(func() {
		_ = srv.Close()
		if err := <-done; err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			t.Errorf("ssh server: %v", err)
		}
	})
	return l.Addr().String()
}

// runCapturedSession runs one session and returns what the server wrote to it.
func runCapturedSession(client *gossh.Client) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer func() { _ = session.Close() }()
	var buf bytes.Buffer
	session.Stdout = &buf
	err = session.Run("")
	return buf.String(), err
}

func TestLoginGateAdmitsBurstThenSpaces(t *testing.T) {
	const (
		admissionsPerSecond = 10.0 // one admission every 100ms after the burst
		burst               = 2
	)
	gate := newLoginGate(admissionsPerSecond, burst, 30*time.Second, false, metrics.New())
	arrivals := make(chan time.Time, 4)
	addr := startGatedSSHServer(t, gate, func(ssh.Session) { arrivals <- time.Now() })

	started := time.Now()
	for range burst + 2 {
		client := dialSSH(t, addr, nil, newTestSigner(t))
		if _, err := runCapturedSession(client); err != nil {
			t.Fatalf("session run: %v", err)
		}
	}
	var times []time.Time
	for len(times) < burst+2 {
		select {
		case at := <-arrivals:
			times = append(times, at)
		case <-time.After(10 * time.Second):
			t.Fatalf("only %d of %d sessions reached the program", len(times), burst+2)
		}
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })

	if got := times[0].Sub(started); got > 3*time.Second {
		t.Fatalf("first burst admission took %v, want near-instant", got)
	}
	// Reservations are made after each SSH handshake, so token refill can
	// legally make adjacent arrivals closer than one full interval. Assert the
	// gate delayed the first post-burst admission and the whole tail instead of
	// requiring wall-clock spacing between every handler invocation.
	if delay := times[burst].Sub(started); delay < 70*time.Millisecond {
		t.Errorf("first post-burst admission arrived after %v, want >= 1/rate apart", delay)
	}
	if tail := times[len(times)-1].Sub(started); tail < 140*time.Millisecond {
		t.Errorf("post-burst admissions completed after %v, want a delayed tail", tail)
	}
}

func TestLoginGateClosesSessionsPastMaxWait(t *testing.T) {
	gate := newLoginGate(1, 1, 250*time.Millisecond, false, metrics.New())
	reached := make(chan struct{}, 2)
	addr := startGatedSSHServer(t, gate, func(ssh.Session) { reached <- struct{}{} })

	// Consume the burst so the next session would have to wait ~1s.
	first := dialSSH(t, addr, nil, newTestSigner(t))
	if _, err := runCapturedSession(first); err != nil {
		t.Fatalf("burst session run: %v", err)
	}
	<-reached

	second := dialSSH(t, addr, nil, newTestSigner(t))
	out, err := runCapturedSession(second)
	if !strings.Contains(out, "very busy") {
		t.Fatalf("dropped session output %q does not contain the busy message", out)
	}
	var exitErr *gossh.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitStatus() != 1 {
		t.Fatalf("dropped session error = %v, want exit status 1", err)
	}
	select {
	case <-reached:
		t.Fatal("dropped session reached the program handler")
	case <-time.After(500 * time.Millisecond):
	}
}

func TestLoginGateTellsWaitingSessionsTheyAreQueued(t *testing.T) {
	gate := newLoginGate(2, 1, 30*time.Second, false, metrics.New()) // ~500ms wait once the burst is spent
	reached := make(chan struct{}, 2)
	addr := startGatedSSHServer(t, gate, func(ssh.Session) { reached <- struct{}{} })

	first := dialSSH(t, addr, nil, newTestSigner(t))
	if _, err := runCapturedSession(first); err != nil {
		t.Fatalf("burst session run: %v", err)
	}

	second := dialSSH(t, addr, nil, newTestSigner(t))
	out, err := runCapturedSession(second)
	if err != nil {
		t.Fatalf("waiting session run: %v", err)
	}
	if !strings.Contains(out, "filling up") {
		t.Fatalf("waiting session output %q does not contain the filling-up line", out)
	}
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("waiting session was never admitted")
	}
}

func TestLoginGateExemptLoopbackPassesInstantly(t *testing.T) {
	// Without the exemption this configuration drops everything: any wait
	// exceeds a nanosecond budget.
	gate := newLoginGate(0.01, 1, time.Nanosecond, true, metrics.New())
	reached := make(chan struct{}, 3)
	addr := startGatedSSHServer(t, gate, func(ssh.Session) { reached <- struct{}{} })

	started := time.Now()
	for range 3 { // 3x burst
		client := dialSSH(t, addr, nil, newTestSigner(t))
		if _, err := runCapturedSession(client); err != nil {
			t.Fatalf("exempt session run: %v", err)
		}
	}
	for range 3 {
		select {
		case <-reached:
		case <-time.After(2 * time.Second):
			t.Fatal("exempt session never reached the program")
		}
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("3 exempt sessions took %v, want instant passage", elapsed)
	}
}
