package sessionoutput

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type blockedStream struct {
	entered chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func (s *blockedStream) Write([]byte) (int, error) {
	s.once.Do(func() { close(s.entered) })
	<-s.closed
	return 0, io.ErrClosedPipe
}

type feedbackModel int

func (feedbackModel) Init() tea.Cmd                         { return nil }
func (m feedbackModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m + 1, nil }
func (m feedbackModel) View() tea.View                      { return tea.NewView(fmt.Sprintf("feedback %d", m)) }

// Hub outboxes deliver to callbacks sequentially. A session must keep accepting
// messages while its SSH writer is blocked, so later recipients aren't trapped
// behind that session. This exercises the real Bubble Tea renderer mutex.
func TestBlockedOutputDoesNotBlockBroadcastRecipients(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	stream := &blockedStream{entered: make(chan struct{}), closed: make(chan struct{})}
	var closeOnce sync.Once
	output := New(ctx, stream, func() { closeOnce.Do(func() { close(stream.closed) }) }, 4096, time.Minute, nil)
	p := tea.NewProgram(feedbackModel(0), tea.WithContext(ctx), tea.WithInput(nil),
		tea.WithOutput(output), tea.WithWindowSize(80, 24), tea.WithoutSignalHandler())
	done := make(chan struct{})
	go func() { defer close(done); _, _ = p.Run() }()
	t.Cleanup(func() {
		cancel()
		output.Wait()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("program goroutine did not exit")
		}
	})
	select {
	case <-stream.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("renderer did not write")
	}
	healthy := make(chan struct{})
	go func() {
		// Multiple Sends require the prior Update/render to have completed.
		for range 10 {
			p.Send(struct{}{})
		}
		close(healthy)
	}()
	select {
	case <-healthy:
	case <-time.After(time.Second):
		t.Fatal("blocked client delayed later broadcast recipient")
	}
}
