package session

import (
	"sync"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type fakeConn struct{ killed bool }

func (f *fakeConn) Kill() { f.killed = true }

func TestClaimDisplacesOlderConnection(t *testing.T) {
	reg := NewRegistry()
	first := &fakeConn{}
	second := &fakeConn{}
	third := &fakeConn{}
	fourth := &fakeConn{}
	if got := reg.Claim("h", first); got != nil {
		t.Fatal("first claim displaced someone")
	}
	old := reg.Claim("h", second)
	if old != first {
		t.Fatal("second claim did not return the older connection")
	}
	old.Kill()
	if !first.killed {
		t.Fatal("older connection was not killed")
	}
	reg.Drop("h", first)
	if reg.Claim("h", third) != second {
		t.Fatal("drop of displaced conn disturbed the live conn")
	}
	reg.Drop("h", third)
	if got := reg.Claim("h", fourth); got != nil {
		t.Fatal("drop of live conn left it registered")
	}
}

func TestRegistryConcurrentClaimDrop(_ *testing.T) {
	reg := NewRegistry()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			c := &fakeConn{}
			reg.Claim("h", c)
			reg.Drop("h", c)
		})
	}
	wg.Wait()
}
