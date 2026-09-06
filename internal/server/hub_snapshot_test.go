package server

import (
	"sync"
	"testing"
	"time"

	"termon.sh/internal/lobby"
)

func TestMoveAndSnapshotDefersBroadcastUntilTick(t *testing.T) {
	h := testHub(t)
	onboardTrainer(t, h, "aaa", "rootkit")
	sends := 0
	t.Cleanup(h.Attach("aaa", func(any) { sends++ }, nil))
	before := h.Snapshot("aaa")
	after, err := h.MoveAndSnapshot("aaa", lobby.West)
	if err != nil {
		t.Fatal(err)
	}
	if after.You.X != before.You.X-1 || after.Sequence <= before.Sequence {
		t.Fatal("movement result does not describe the accepted intent")
	}
	if sends != 0 {
		t.Fatal("movement invoked a callback before returning")
	}
	h.Tick(time.Now())
	if sends == 0 {
		t.Fatal("movement did not mark the Dojo for its normal broadcast")
	}
}

func TestSnapshotCapturesHaveUniqueIncreasingSequences(t *testing.T) {
	h := testHub(t)
	onboardTrainer(t, h, "aaa", "rootkit")
	onboardTrainer(t, h, "bbb", "emberbyte")
	before := h.Snapshot("aaa").Sequence
	if before == 0 {
		t.Fatal("valid snapshot has no sequence")
	}
	const captures = 100
	results := make(chan uint64, captures)
	var workers sync.WaitGroup
	for i := range captures {
		hash := "aaa"
		if i%2 == 1 {
			hash = "bbb"
		}
		workers.Go(func() { results <- h.Snapshot(hash).Sequence })
	}
	workers.Wait()
	close(results)
	seen := make(map[uint64]bool, captures)
	for sequence := range results {
		if sequence <= before || seen[sequence] {
			t.Fatalf("invalid or repeated sequence %d", sequence)
		}
		seen[sequence] = true
	}
	after := h.Snapshot("aaa").Sequence
	for sequence := range seen {
		if sequence >= after {
			t.Fatal("later capture did not advance the sequence")
		}
	}
}
