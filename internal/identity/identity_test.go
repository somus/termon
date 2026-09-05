package identity

import "testing"

func TestHashIsStableAndNotTheRawKey(t *testing.T) {
	key := []byte("ssh-ed25519 AAAA-test-key")
	a := Hash(key)
	b := Hash(key)
	if a != b {
		t.Fatalf("hash not stable: %s vs %s", a, b)
	}
	if len(a) != 64 {
		t.Fatalf("hash length %d, want 64 hex chars", len(a))
	}
	if a == string(key) {
		t.Fatal("hash leaked the raw key material")
	}
	if Hash([]byte("other-key")) == a {
		t.Fatal("different keys hashed the same")
	}
}
