package tui

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/game"
	"termon.sh/internal/server"
)

const (
	playWidth  = 100
	playHeight = 32
	// cmdWait must comfortably exceed real hub/save command latency under
	// -race on a loaded machine; a timeout here is a test failure, never a
	// silently dropped command.
	cmdWait = 2 * time.Second
)

// player is one in-process trainer session: keys, ticks, and hub messages
// through Model.Update, the same path a live Bubble Tea program uses.
type player struct {
	t        *testing.T
	hub      *server.Hub
	set      *content.Set
	m        Model
	inbox    []tea.Msg
	usedMove map[string]struct{}
	switched bool
	detach   func()
}

// attachPlayer wires one in-process player to the hub: initial view size,
// Init command, and an inbox that mirrors every hub message.
func (p *player) attachPlayer(width, height int) {
	p.detach = p.hub.Attach(p.m.hash, func(msg any) {
		if msg != nil {
			p.inbox = append(p.inbox, msg)
		}
	}, func() {})
	p.t.Cleanup(p.detach)
	p.apply(tea.WindowSizeMsg{Width: width, Height: height})
	p.runCmd(p.m.Init())
	p.flush()
}

func joinNew(t *testing.T, hub *server.Hub, set *content.Set, cred string) *player {
	t.Helper()
	trainer, err := hub.Authenticate(cred, "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	p := &player{t: t, hub: hub, set: set, m: New(trainer.ID, trainer.Save, set, hub)}
	p.attachPlayer(playWidth, playHeight)
	return p
}

func (p *player) apply(msg tea.Msg) {
	p.t.Helper()
	if msg == nil {
		return
	}
	if _, isTick := msg.(tickMsg); isTick {
		p.tick()
		return
	}
	next, cmd := p.m.Update(msg)
	model, ok := next.(Model)
	if !ok {
		p.t.Fatalf("update returned %T", next)
	}
	p.m = model
	p.runCmd(cmd)
}

func (p *player) runCmd(cmd tea.Cmd) {
	p.t.Helper()
	if cmd == nil {
		return
	}
	msg := p.invokeCmd(cmd)
	p.flush()
	switch typed := msg.(type) {
	case nil:
		return
	case tea.BatchMsg:
		for _, inner := range typed {
			p.runCmd(inner)
		}
	default:
		p.apply(typed)
	}
	p.flush()
}

// invokeCmd runs one command to completion. Commands that only produce the
// model's own ticker message return nil (the driver pumps ticks itself);
// anything that does not settle within cmdWait fails the test, because
// dropping a live command here surfaces as a baffling failure many steps
// later.
func (p *player) invokeCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	for {
		select {
		case msg := <-done:
			if _, isTick := msg.(tickMsg); isTick {
				return nil
			}
			return msg
		case <-time.After(cmdWait):
			p.t.Fatalf("command did not settle within %v; refusing to drop it", cmdWait)
			return nil
		}
	}
}

func (p *player) flush() {
	for len(p.inbox) > 0 {
		msg := p.inbox[0]
		p.inbox = p.inbox[1:]
		p.apply(msg)
	}
}

func (p *player) press(key string) {
	p.t.Helper()
	p.flush()
	p.apply(press(key))
	p.flush()
}

func (p *player) tick() {
	next, cmd := p.m.Update(tickMsg{})
	model, ok := next.(Model)
	if !ok {
		p.t.Fatalf("tick returned %T", next)
	}
	p.m = model
	// Update on tickMsg always returns clockCmd (sometimes batched with a hub
	// reveal command). The driver owns ticking, so the clock is never invoked;
	// a revealing battle is unblocked directly against the hub instead.
	_ = cmd
	if p.m.battle.session.battle != nil && !p.m.battle.playing &&
		p.m.battle.session.battle.State() == battle.StateRevealing {
		if err := p.hub.AdvanceReveal(p.m.hash); err == nil {
			p.flush()
		}
	}
	p.flush()
}

func (p *player) view() string {
	return ansi.Strip(p.m.View().Content)
}

func (p *player) save() *game.Save {
	p.t.Helper()
	sv, err := p.hub.Load(p.m.hash)
	if err != nil {
		p.t.Fatal(err)
	}
	return sv
}

func (p *player) dump(why string) string {
	return fmt.Sprintf("%s: screen=%s status=%q pos=(%d,%d)\n%s",
		why, screenName(p.m.screen), p.m.status, p.m.snap.You.X, p.m.snap.You.Y, p.view())
}

func screenName(s screen) string {
	names := []string{
		"onboard", "lobby", "dojo", "queue-editor", "progression",
		"queue", "battle", "workbench", "signal", "expedition", "displaced",
	}
	if int(s) >= 0 && int(s) < len(names) {
		return names[s]
	}
	return strconv.Itoa(int(s))
}
