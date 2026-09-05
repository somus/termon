// Package session tracks at most one live SSH connection per fingerprint.
package session

import "sync"

// Conn is a live session the registry can displace.
type Conn interface {
	Kill()
}

// Registry maps a fingerprint hash to the current connection.
type Registry struct {
	mu   sync.Mutex
	live map[string]Conn
}

// NewRegistry is empty.
func NewRegistry() *Registry {
	return &Registry{live: map[string]Conn{}}
}

// Claim installs c as the live connection for hash. The previous Conn, if
// any, is returned so the caller can kill it.
func (r *Registry) Claim(hash string, c Conn) (displaced Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	old := r.live[hash]
	r.live[hash] = c
	if old == c {
		return nil
	}
	return old
}

// Drop forgets c only if it is still the live connection for hash.
func (r *Registry) Drop(hash string, c Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.live[hash] == c {
		delete(r.live, hash)
	}
}
