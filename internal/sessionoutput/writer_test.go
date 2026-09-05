package sessionoutput

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// gateWriter emulates exhausted SSH channel credit, with transport closure
// releasing the blocked Write. No wall-clock sleeps or goroutines are leaked.
type gateWriter struct {
	bytes.Buffer
	gate   chan struct{}
	closed chan struct{}
	once   sync.Once
}

func (w *gateWriter) Write(p []byte) (int, error) {
	select {
	case <-w.gate:
		return w.Buffer.Write(p)
	case <-w.closed:
		return 0, io.ErrClosedPipe
	}
}
func (w *gateWriter) close() { w.once.Do(func() { close(w.closed) }) }

func TestWriterOrderedAndCopiesInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		out := &gateWriter{gate: make(chan struct{}), closed: make(chan struct{})}
		w := New(ctx, out, out.close, 16, time.Second, nil)
		p := []byte("first")
		if _, err := w.Write(p); err != nil {
			t.Fatal(err)
		}
		synctest.Wait() // first write now waits for channel credit
		p[0] = 'X'
		if _, err := w.Write([]byte("second")); err != nil {
			t.Fatal(err)
		}
		if got := w.Pending(); got != 11 {
			t.Fatalf("pending = %d", got)
		}
		close(out.gate)
		synctest.Wait()
		if got := out.String(); got != "firstsecond" {
			t.Fatalf("stream = %q", got)
		}
		if w.Pending() != 0 {
			t.Fatal("pending bytes survived flush")
		}
		cancel()
		w.Wait()
	})
}

func TestWriterOverflowIncludesBlockedWrite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		out := &gateWriter{gate: make(chan struct{}), closed: make(chan struct{})}
		w := New(t.Context(), out, out.close, 8, time.Second, nil)
		if _, err := w.Write([]byte("123456")); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if _, err := w.Write([]byte("78")); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("9")); !errors.Is(err, ErrOverflow) {
			t.Fatalf("overflow = %v", err)
		}
		w.Wait()
		if w.Pending() != 0 {
			t.Fatal("buffers retained on overflow")
		}
		if _, err := w.Write([]byte("later")); !errors.Is(err, ErrOverflow) {
			t.Fatalf("later write = %v", err)
		}
	})
}

func TestWriterStallDisconnectAndCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		out := &gateWriter{gate: make(chan struct{}), closed: make(chan struct{})}
		w := New(t.Context(), out, out.close, 8, time.Second, nil)
		if _, err := w.Write([]byte("blocked")); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		time.Sleep(time.Second - time.Nanosecond)
		if w.Err() != nil {
			t.Fatal("disconnected before deadline")
		}
		time.Sleep(time.Nanosecond)
		w.Wait()
		if !errors.Is(w.Err(), ErrStalled) {
			t.Fatalf("reason = %v", w.Err())
		}
		if w.Pending() != 0 {
			t.Fatal("buffers retained on timeout")
		}
	})
}

func TestWriterCancellationReleasesBlockedWrite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		out := &gateWriter{gate: make(chan struct{}), closed: make(chan struct{})}
		w := New(ctx, out, out.close, 8, time.Hour, nil)
		if _, err := w.Write([]byte("blocked")); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		cancel()
		w.Wait()
		if !errors.Is(w.Err(), context.Canceled) {
			t.Fatalf("reason = %v", w.Err())
		}
		if w.Pending() != 0 {
			t.Fatal("buffers retained after cancellation")
		}
	})
}

// net.Pipe has no transport buffering: a peer that never reads blocks the
// underlying connection write itself, not merely an application-level queue.
func TestWriterClosesTransportWhenPeerNeverReads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		local, peer := net.Pipe()
		defer func() { _ = peer.Close() }()
		w := New(t.Context(), local, func() { _ = local.Close() }, 1024, time.Second, nil)
		if _, err := w.Write([]byte("frame")); err != nil {
			t.Fatal(err)
		}
		w.Wait()
		if !errors.Is(w.Err(), ErrStalled) || w.Pending() != 0 {
			t.Fatalf("err=%v pending=%d", w.Err(), w.Pending())
		}
		if _, err := peer.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
			t.Fatalf("peer was not disconnected: %v", err)
		}
	})
}

type shortWriter struct{}

func (shortWriter) Write([]byte) (int, error) { return 0, nil }

func TestWriterShortWriteClosesTransport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		closed := false
		w := New(t.Context(), shortWriter{}, func() { closed = true }, 8, time.Second, nil)
		if _, err := w.Write([]byte("data")); err != nil {
			t.Fatal(err)
		}
		w.Wait()
		if !closed || !errors.Is(w.Err(), io.ErrShortWrite) {
			t.Fatalf("closed=%v err=%v", closed, w.Err())
		}
	})
}
