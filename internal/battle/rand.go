package battle

import (
	crand "crypto/rand"
	"encoding/binary"
	"math/rand/v2"
)

// Rand supplies combat rolls. Each resolution consumes them in a fixed
// order: a speed-tie roll when Speeds match, then per move an accuracy
// roll, and on a hit a crit roll and a variance roll.
type Rand interface {
	// Float64 returns a value in [0.0, 1.0).
	Float64() float64
}

type stdRand struct {
	r *rand.Rand
}

// Seeded returns a deterministic Rand for a given seed.
func Seeded(seed uint64) Rand { //nolint:gosec // gameplay rolls need determinism; the seed comes from crypto/rand
	return &stdRand{r: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))} //nolint:gosec // gameplay rolls need determinism; the seed comes from crypto/rand
}

// RandomSeed draws an unpredictable seed for live Battles.
func RandomSeed() (uint64, error) {
	var raw [8]byte
	if _, err := crand.Read(raw[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(raw[:]), nil
}

func (s *stdRand) Float64() float64 {
	return s.r.Float64()
}
