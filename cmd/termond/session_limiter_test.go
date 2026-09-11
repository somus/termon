package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	charmlog "charm.land/log/v2"
	"charm.land/ssh"
	"charm.land/wish/v2"
	wishratelimiter "charm.land/wish/v2/ratelimiter"

	"termon.sh/internal/metrics"
)

type addressedSession struct {
	ssh.Session
	address string
}

func (s addressedSession) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP(s.address), Port: 1234}
}

func TestSessionLimiterBurstRefillAndSources(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		limiter := newSessionLimiter()
		for _, address := range []string{"203.0.113.7", "2001:db8::7"} {
			sess := addressedSession{address: address}
			for range sessionRateBurst {
				if err := limiter.Allow(sess); err != nil {
					t.Fatal(err)
				}
			}
			if err := limiter.Allow(sess); !errors.Is(err, wishratelimiter.ErrRateLimitExceeded) {
				t.Fatalf("burst exhausted: %v", err)
			}
		}
		time.Sleep(time.Second)
		if err := limiter.Allow(addressedSession{address: "203.0.113.7"}); err != nil {
			t.Fatalf("refilled token denied: %v", err)
		}
	})
}

func TestSessionLimiterConcurrentBurst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		limiter := newSessionLimiter()
		var admitted atomic.Int32
		var wg sync.WaitGroup
		for range 50 {
			wg.Go(func() {
				if limiter.Allow(addressedSession{address: "203.0.113.7"}) == nil {
					admitted.Add(1)
				}
			})
		}
		wg.Wait()
		if got := admitted.Load(); got != sessionRateBurst {
			t.Fatalf("concurrent admissions = %d, want %d", got, sessionRateBurst)
		}
	})
}

func TestSessionLimiterRetainsRecentSources(t *testing.T) {
	limiter := newSessionLimiter()
	for i := range sessionRateIPs {
		if err := limiter.Allow(addressedSession{address: fmt.Sprintf("2001:db8::%x", i)}); err != nil {
			t.Fatal(err)
		}
	}
	for _, address := range []string{"2001:db8::", "203.0.113.7"} {
		if err := limiter.Allow(addressedSession{address: address}); err != nil {
			t.Fatal(err)
		}
	}
	if limiter.cache.Len() != sessionRateIPs || !limiter.cache.Contains("2001:db8::") || limiter.cache.Contains("2001:db8::1") {
		t.Fatal("limiter did not evict the least recently used source at capacity")
	}
}

func TestSessionMiddlewareDoesNotLogAddresses(t *testing.T) {
	var logs bytes.Buffer
	previous := charmlog.Default()
	charmlog.SetDefault(charmlog.NewWithOptions(&logs, charmlog.Options{Level: charmlog.DebugLevel}))
	t.Cleanup(func() { charmlog.SetDefault(previous) })
	gate := newLoginGate(defaultLoginRatePerSecond, defaultLoginBurst, defaultLoginMaxWait, false, metrics.New())
	var handler ssh.Handler = func(ssh.Session) {}
	program := func(next ssh.Handler) ssh.Handler { return next }
	for _, middleware := range sessionMiddleware(wish.Middleware(program), gate, false) {
		handler = middleware(handler)
	}
	addr := startProxyAwareSSHServer(t, handler)
	for _, address := range []string{"203.0.113.7", "2001:db8::7"} {
		protocol := "TCP4"
		if strings.Contains(address, ":") {
			protocol = "TCP6"
		}
		destination := "198.51.100.2"
		if protocol == "TCP6" {
			destination = "2001:db8::2"
		}
		header := []byte(fmt.Sprintf("PROXY %s %s %s 1234 2222\r\n", protocol, address, destination))
		client := dialSSH(t, addr, header, newTestSigner(t))
		session, err := client.NewSession()
		if err != nil {
			t.Fatal(err)
		}
		if err := session.RequestPty("xterm", 24, 80, nil); err != nil {
			t.Fatal(err)
		}
		if err := session.Run(""); err != nil {
			t.Fatal(err)
		}
		_ = session.Close()
	}
	if logs.Len() != 0 {
		t.Fatalf("session middleware emitted transport logs: %s", logs.String())
	}
}
