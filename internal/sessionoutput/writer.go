// Package sessionoutput isolates terminal rendering from SSH flow control.
package sessionoutput

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

// Limits include the active write as well as all waiting bytes. They do not
// include the SSH peer's advertised window or kernel socket buffers.
const (
	DefaultLimit = 256 << 10
	DefaultStall = 10 * time.Second
)

var (
	// ErrOverflow means accepting a write would exceed the session byte budget.
	ErrOverflow = errors.New("session output budget exhausted")
	// ErrStalled means a channel write exceeded the output-progress deadline.
	ErrStalled = errors.New("session output write stalled")
)

// Observer receives bounded, local measurements. Implementations must not do I/O.
type Observer interface {
	OutputPending(delta int)
	OutputWritten(bytes int, elapsed time.Duration)
	OutputClosed(reason string)
}

// Writer accepts ordered output without waiting for the network. A single
// worker owns the underlying stream. ANSI deltas are never coalesced or dropped
// on a live connection; exhausting the budget closes the transport instead.
// abort must close the underlying transport and unblock Write, not send an SSH
// channel-close message (which can itself block on a stalled TCP connection).
type Writer struct {
	mu        sync.Mutex
	queue     [][]byte
	pending   int
	finishing bool
	err       error
	limit     int
	stall     time.Duration
	out       io.Writer
	abort     func()
	observer  Observer
	wake      chan struct{}
	stopped   chan struct{}
	done      chan struct{}
}

// New starts one output worker. Cancel ctx to release it; Wait joins cleanup.
func New(ctx context.Context, out io.Writer, abort func(), limit int, stall time.Duration, observer Observer) *Writer {
	if limit <= 0 {
		limit = DefaultLimit
	}
	if stall <= 0 {
		stall = DefaultStall
	}
	w := &Writer{
		out: out, abort: abort, limit: limit, stall: stall, observer: observer,
		wake: make(chan struct{}, 1), stopped: make(chan struct{}), done: make(chan struct{}),
	}
	canceled := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(canceled)
		w.fail(ctx.Err(), "canceled")
	})
	go func() {
		defer close(w.done)
		defer func() {
			if !stop() {
				<-canceled
			}
		}()
		w.run()
	}()
	return w
}

func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	if w.err != nil || w.finishing {
		err := w.err
		if err == nil {
			err = io.ErrClosedPipe
		}
		w.mu.Unlock()
		return 0, err
	}
	if len(p) > w.limit-w.pending {
		w.mu.Unlock()
		w.fail(ErrOverflow, "overflow")
		return 0, ErrOverflow
	}
	if len(p) != 0 {
		w.queue = append(w.queue, append([]byte(nil), p...))
		w.pending += len(p)
		if w.observer != nil {
			w.observer.OutputPending(len(p))
		}
		select {
		case w.wake <- struct{}{}:
		default:
		}
	}
	w.mu.Unlock()
	return len(p), nil
}

// Pending reports owned bytes, including a blocked underlying write.
func (w *Writer) Pending() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.pending
}

// Err reports why output stopped, or nil while the worker is healthy.
func (w *Writer) Err() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.err
}

// Finish stops accepting writes, drains accepted output, and joins the worker.
// The write deadline still applies while draining. A healthy finish does not
// close the shared SSH transport, which may host another channel.
func (w *Writer) Finish() {
	w.mu.Lock()
	w.finishing = true
	select {
	case w.wake <- struct{}{}:
	default:
	}
	w.mu.Unlock()
	w.Wait()
}

// Wait waits until the output worker has released its buffers and exited.
func (w *Writer) Wait() { <-w.done }

func (w *Writer) run() {
	for {
		select {
		case <-w.stopped:
			return
		case <-w.wake:
		}
		for {
			w.mu.Lock()
			if w.err != nil {
				w.mu.Unlock()
				return
			}
			if len(w.queue) == 0 {
				finished := w.finishing
				if finished && w.observer != nil {
					w.observer.OutputClosed("finished")
				}
				w.mu.Unlock()
				if finished {
					return
				}
				break
			}
			p := w.queue[0]
			w.queue[0] = nil
			w.queue = w.queue[1:]
			w.mu.Unlock()
			started := time.Now()
			expired := make(chan struct{})
			timer := time.AfterFunc(w.stall, func() {
				defer close(expired)
				w.fail(ErrStalled, "stalled")
			})
			n, err := w.out.Write(p)
			if !timer.Stop() {
				<-expired
			}
			if n != len(p) && err == nil {
				err = io.ErrShortWrite
			}
			w.mu.Lock()
			w.pending -= len(p)
			if w.observer != nil {
				w.observer.OutputPending(-len(p))
				w.observer.OutputWritten(n, time.Since(started))
			}
			w.mu.Unlock()
			if err != nil {
				w.fail(err, "write_error")
				return
			}
		}
	}
}

func (w *Writer) fail(err error, reason string) {
	w.mu.Lock()
	if w.err != nil {
		w.mu.Unlock()
		return
	}
	w.err = err
	for _, p := range w.queue {
		w.pending -= len(p)
		if w.observer != nil {
			w.observer.OutputPending(-len(p))
		}
	}
	w.queue = nil
	close(w.stopped)
	if w.observer != nil {
		w.observer.OutputClosed(reason)
	}
	w.mu.Unlock()
	w.abort()
}
