package main

import (
	"sync"

	"charm.land/ssh"
	wishratelimiter "charm.land/wish/v2/ratelimiter"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"golang.org/x/time/rate"
)

// sessionLimiter keeps source addresses in memory only. Wish's built-in
// limiter logs its address keys at debug level.
type sessionLimiter struct {
	mu    sync.Mutex
	cache *simplelru.LRU[string, *rate.Limiter]
}

func newSessionLimiter() *sessionLimiter {
	// The fixed capacity is positive, so constructing the LRU cannot fail.
	cache, _ := simplelru.NewLRU[string, *rate.Limiter](sessionRateIPs, nil)
	return &sessionLimiter{cache: cache}
}

func (l *sessionLimiter) Allow(sess ssh.Session) error {
	source := sourceAddr(sess)
	l.mu.Lock()
	defer l.mu.Unlock()
	limiter, ok := l.cache.Get(source)
	if !ok {
		limiter = rate.NewLimiter(sessionRateLimit, sessionRateBurst)
		l.cache.Add(source, limiter)
	}
	if !limiter.Allow() {
		return wishratelimiter.ErrRateLimitExceeded
	}
	return nil
}
