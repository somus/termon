package queue

import (
	"testing"
	"time"
)

func TestFIFOPairing(t *testing.T) {
	var q Queue
	now := time.Now()
	if _, _, err := q.Join("a", now); err != nil {
		t.Fatal(err)
	}
	pos, wait, err := q.Join("b", now)
	if err != nil || pos != 2 || wait != 2 {
		t.Fatalf("join b: pos=%d wait=%d err=%v", pos, wait, err)
	}
	if _, _, err := q.Join("c", now); err != nil {
		t.Fatal(err)
	}
	a, b, _, _, ok := q.Pair(now.Add(time.Minute))
	if !ok || a != "a" || b != "b" {
		t.Fatalf("pair = %s %s ok=%v, want a b", a, b, ok)
	}
	pos, wait, in := q.Position("c")
	if !in || pos != 1 || wait != 1 {
		t.Fatalf("c after pair: pos=%d wait=%d in=%v", pos, wait, in)
	}
}

func TestCancelAndDuplicateJoin(t *testing.T) {
	var q Queue
	now := time.Now()
	_, _, _ = q.Join("a", now)
	_, _, _ = q.Join("b", now)
	pos, wait, _ := q.Join("a", now)
	if pos != 1 || wait != 2 {
		t.Fatalf("duplicate join moved a: pos=%d wait=%d", pos, wait)
	}
	if !q.Cancel("a") {
		t.Fatal("cancel of waiting trainer reported not removed")
	}
	if q.Cancel("a") {
		t.Fatal("second cancel reported removed")
	}
	if _, _, ok := q.Position("a"); ok {
		t.Fatal("canceled a still waiting")
	}
	if q.Waiting() != 1 {
		t.Fatalf("waiting = %d, want 1", q.Waiting())
	}
}

func TestPairReportsWaitTimes(t *testing.T) {
	var q Queue
	start := time.Now()
	_, _, _ = q.Join("early", start)
	_, _, _ = q.Join("late", start.Add(10*time.Second))
	_, _, waitA, waitB, ok := q.Pair(start.Add(30 * time.Second))
	if !ok {
		t.Fatal("pair failed")
	}
	if waitA != 30*time.Second {
		t.Fatalf("waitA = %v, want 30s", waitA)
	}
	if waitB != 20*time.Second {
		t.Fatalf("waitB = %v, want 20s", waitB)
	}
}
