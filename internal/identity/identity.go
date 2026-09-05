// Package identity derives a Trainer's durable Store key from an SSH
// public key. The raw fingerprint never reaches disk paths.
package identity

import (
	"crypto/sha256"
	"encoding/hex"
)

// Hash returns the hex SHA-256 of a public key's wire encoding.
func Hash(pub []byte) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])
}
