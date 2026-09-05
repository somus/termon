package sessionoutput

import (
	"context"
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"
)

func TestFinishDrainsWithoutClosingHealthyTransport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		out := &gateWriter{gate: make(chan struct{}), closed: make(chan struct{})}
		w := New(ctx, out, out.close, 16, time.Second, nil)
		if _, err := w.Write([]byte("terminal reset")); err != nil {
			t.Fatal(err)
		}
		finished := make(chan struct{})
		go func() { w.Finish(); close(finished) }()
		synctest.Wait()
		select {
		case <-finished:
			t.Fatal("finish lost pending terminal output")
		default:
		}
		if _, err := w.Write([]byte("late")); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("late write: %v", err)
		}
		close(out.gate)
		<-finished
		cancel()
		synctest.Wait()
		if out.String() != "terminal reset" || w.Pending() != 0 {
			t.Fatal("finish did not drain output")
		}
		select {
		case <-out.closed:
			t.Fatal("normal channel exit closed the shared transport")
		default:
		}
	})
}
