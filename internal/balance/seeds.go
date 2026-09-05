package balance

// CorpusSeed returns the nth seed (1-indexed) from the recorded splitmix corpus.
func CorpusSeed(base uint64, n int) uint64 {
	if n < 1 {
		return splitmix64(base)
	}
	return splitmix64(base + uint64(n))
}

// CorpusSeeds builds count seeds starting at index 1.
func CorpusSeeds(base uint64, count int) []uint64 {
	out := make([]uint64, count)
	for i := range count {
		out[i] = CorpusSeed(base, i+1)
	}
	return out
}

func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}
