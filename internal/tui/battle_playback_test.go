package tui

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"termon.sh/internal/battle"
	"termon.sh/internal/server"
)

func advancePlaybackUntil(m *Model, match func(battle.Event) bool) bool {
	for range 400 {
		if !m.battle.playing {
			return false
		}
		if event, ok := m.playEvent(); ok && match(event) {
			return true
		}
		m.battle.playGrace = 0
		if m.battle.playPause {
			m.continuePlayback()
			continue
		}
		m.battle.playHold = 0
		m.nextPlayEvent()
	}
	return false
}

func drainPlayback(m *Model) {
	m.wipeHold = 0
	for m.battle.playing {
		advancePlaybackUntil(m, func(battle.Event) bool { return false })
	}
}

func TestPlayGroupsCoverMatchPathways(t *testing.T) {
	cases := []struct {
		name   string
		events []battle.Event
		want   []playGroup
	}{
		{
			name: "two sides",
			events: []battle.Event{
				{Kind: battle.EventTurnStarted, Text: "Turn 1"},
				{Kind: battle.EventMoveUsed, Text: "A used Jab!"},
				{Kind: battle.EventDamageDealt, Text: "B took 4 damage."},
				{Kind: battle.EventMoveUsed, Text: "B used Leaf!"},
				{Kind: battle.EventDamageDealt, Text: "A took 3 damage."},
			},
			want: []playGroup{{0, 1, false}, {1, 3, true}, {3, 5, true}},
		},
		{
			name: "miss then hit",
			events: []battle.Event{
				{Kind: battle.EventTurnStarted, Text: "Turn 1"},
				{Kind: battle.EventMoveUsed, Text: "A used Jab!"},
				{Kind: battle.EventMissed, Text: "The attack missed!"},
				{Kind: battle.EventMoveUsed, Text: "B used Leaf!"},
				{Kind: battle.EventCriticalHit, Text: "A critical hit!"},
				{Kind: battle.EventDamageDealt, Text: "A took 3 damage."},
			},
			want: []playGroup{{0, 1, false}, {1, 3, true}, {3, 6, true}},
		},
		{
			name: "ko first side",
			events: []battle.Event{
				{Kind: battle.EventTurnStarted, Text: "Turn 1"},
				{Kind: battle.EventMoveUsed, Text: "A used Crush!"},
				{Kind: battle.EventSuperEffective, Text: "It's super effective!"},
				{Kind: battle.EventDamageDealt, Text: "B took 40 damage."},
				{Kind: battle.EventFainted, Text: "B fainted!"},
				{Kind: battle.EventBattleOver, Text: "Battle over."},
			},
			want: []playGroup{{0, 1, false}, {1, 6, true}},
		},
		{
			name: "forfeit",
			events: []battle.Event{
				{Kind: battle.EventForfeit, Text: "forfeited"},
				{Kind: battle.EventBattleOver, Text: "Battle over."},
			},
			want: []playGroup{{0, 2, true}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := playGroups(tc.events)
			if len(got) != len(tc.want) {
				t.Fatalf("groups = %+v, want %+v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("group %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestPlaybackPathwaysAutoAdvance(t *testing.T) {
	t.Parallel()
	set := loadSet(t)
	cases := []struct {
		name  string
		build func(*testing.T) *battle.Battle
		over  bool
	}{
		{
			name: "resolved turn",
			build: func(t *testing.T) *battle.Battle {
				bt := liveBattle(t, set)
				you, _ := bt.Fighter("aaa")
				foe, _ := bt.Fighter("bbb")
				commitTurn(t, bt, you.Moves[0], foe.Moves[0])
				return bt
			},
		},
		{
			name: "forfeit",
			build: func(t *testing.T) *battle.Battle {
				bt := liveBattle(t, set)
				if err := bt.Forfeit("aaa"); err != nil {
					t.Fatal(err)
				}
				return bt
			},
			over: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bt := tc.build(t)
			m := battleModel(t, bt, 120, 40)
			next, _ := m.Update(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bravo", FoeHash: "bbb"})
			m = next.(Model)
			for i := 0; i < 400 && m.battle.playing; i++ {
				next, _ = m.Update(tickMsg{})
				m = next.(Model)
			}
			if m.battle.playing {
				t.Fatal("playback did not finish")
			}
			if tc.over {
				if m.battle.session.battle.State() != battle.StateOver {
					t.Fatal("expected battle over")
				}
				if m.battle.resultHold == 0 {
					t.Fatal("results should wait on a timer before Enter")
				}
				v := m.renderBattle()
				if strings.Contains(v, "return to the dojo") && strings.Contains(v, "▼") {
					t.Fatal("results ▼ should wait for the hold")
				}
			}
		})
	}
}

func TestPlaybackEnterRevealsThenAdvances(t *testing.T) {
	set := loadSet(t)
	bt := liveBattle(t, set)
	you, _ := bt.Fighter("aaa")
	foe, _ := bt.Fighter("bbb")
	commitTurn(t, bt, you.Moves[0], foe.Moves[0])
	m := battleModel(t, bt, 120, 40)
	m.battle.playSeen = 2
	next, _ := m.Update(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bravo", FoeHash: "bbb"})
	m = next.(Model)
	m.wipeHold = 0
	m.battle.playGrace = 0
	start := m.battle.playAt

	next, _ = m.Update(press("enter"))
	m = next.(Model)
	if !m.battle.playReveal || m.battle.playAt != start {
		t.Fatalf("first Enter should reveal the current line: reveal=%v at=%d want=%d", m.battle.playReveal, m.battle.playAt, start)
	}

	next, _ = m.Update(press("enter"))
	m = next.(Model)
	if m.battle.playing && m.battle.playAt == start {
		t.Fatal("second Enter should advance playback")
	}
}

func TestPlaybackFooterMarksEnterAsOptional(t *testing.T) {
	m := battleModel(t, nil, 120, 40)
	m.battle.playing = true
	footer := m.battleFooter()
	if !strings.Contains(footer, "faster") || strings.Contains(footer, "continue") {
		t.Fatalf("playback footer = %q, want optional acceleration", footer)
	}
}

func TestRevealAdvanceKeepsBattleClock(t *testing.T) {
	t.Parallel()
	set := loadSet(t)
	bt := liveBattle(t, set)
	you, _ := bt.Fighter("aaa")
	foe, _ := bt.Fighter("bbb")
	if err := bt.Select("aaa", battle.Action{Kind: battle.ActionMove, Move: you.Moves[0]}); err != nil {
		t.Fatal(err)
	}
	if err := bt.Select("bbb", battle.Action{Kind: battle.ActionMove, Move: foe.Moves[0]}); err != nil {
		t.Fatal(err)
	}
	if bt.State() != battle.StateRevealing {
		t.Fatalf("state=%s, want revealing so the TUI still owns AdvanceReveal", bt.State())
	}

	m := battleModel(t, bt, 120, 40)
	m.battle.playSeen = 2
	next, cmd := m.Update(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bravo", FoeHash: "bbb"})
	m = next.(Model)
	m.wipeHold = 0
	for i := 0; i < 400 && (m.battle.playing || m.battle.revealPending); i++ {
		next, cmd = m.Update(tickMsg{})
		m = next.(Model)
	}
	if m.battle.playing {
		t.Fatal("first-turn playback did not finish")
	}
	if bt.State() != battle.StateRevealing {
		t.Fatalf("state=%s, want revealing after playback", bt.State())
	}
	if !cmdSchedulesTick(cmd) {
		t.Fatal("AdvanceReveal replaced the 100ms clock; the next turn's typewriter would freeze on its first frame")
	}
}

func cmdSchedulesTick(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	ch := make(chan tea.Msg, 1)
	go func() {
		defer func() {
			if recover() != nil {
				ch <- nil
			}
		}()
		ch <- cmd()
	}()
	select {
	case msg := <-ch:
		switch msg := msg.(type) {
		case tickMsg:
			return true
		case tea.BatchMsg:
			return slices.ContainsFunc(msg, cmdSchedulesTick)
		}
		return false
	case <-time.After(40 * time.Millisecond):
		// tea.Tick(100ms) has not returned yet — that is the battle clock.
		return true
	}
}

func TestReleasedSnapshotDoesNotAbortSendOut(t *testing.T) {
	set := loadSet(t)
	bt := liveBattle(t, set)
	m := battleModel(t, nil, 120, 40)
	next, _ := m.Update(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bravo", FoeHash: "bbb"})
	m = next.(Model)
	if m.screen != screenBattle || (!m.battle.battleIntro && m.wipeHold == 0) {
		t.Fatal("expected a fresh send-out")
	}
	next, _ = m.Update(server.SnapshotMsg{You: lobbyPresence(false)})
	m = next.(Model)
	if m.screen != screenBattle {
		t.Fatal("an out-of-battle snapshot must not dump the send-out onto the Dojo")
	}
	if m.wipeHold == 0 && !m.battle.battleIntro {
		t.Fatal("the send-out must keep playing")
	}
}

func TestRetryDoesNotClobberKOPlayback(t *testing.T) {
	t.Parallel()
	set := loadSet(t)
	bt := liveBattle(t, set)
	you, _ := bt.Fighter("aaa")
	foe, _ := bt.Fighter("bbb")
	for i := 0; i < 20 && bt.State() != battle.StateOver; i++ {
		commitTurn(t, bt, you.Moves[0], foe.Moves[0])
	}
	if bt.State() != battle.StateOver {
		t.Fatal("need a finished Battle so the faint can play")
	}

	m := battleModel(t, bt, 120, 40)
	m.battle.playSeen = 2
	next, _ := m.Update(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bravo", FoeHash: "bbb"})
	m = next.(Model)
	m.wipeHold = 0
	if !m.battle.playing {
		t.Fatal("expected playback of the finishing turn")
	}

	retry := liveBattle(t, set)
	next, _ = m.Update(server.BattleMsg{Battle: retry, You: "aaa", Foe: "wild", FoeHash: "ccc"})
	m = next.(Model)
	if m.battle.session.battle != bt {
		t.Fatal("a hunt-failed retry must not replace the faint sequence")
	}
	if m.wipeHold > 0 || m.battle.battleIntro {
		t.Fatal("a hunt-failed retry must not wipe into a fresh send-out during the faint")
	}

	for i := 0; i < 400 && m.battle.playing; i++ {
		next, _ = m.Update(tickMsg{})
		m = next.(Model)
	}
	for i := 0; i < holdResult+2 && m.battle.session.battle == bt; i++ {
		next, _ = m.Update(tickMsg{})
		m = next.(Model)
	}
	if m.battle.session.battle != retry {
		t.Fatal("the retry should start after the faint and results hold")
	}
}

func TestBattlePlaysEventsBeforeMenu(t *testing.T) {
	set := loadSet(t)
	bt := liveBattle(t, set)
	you, _ := bt.Fighter("aaa")
	foe, _ := bt.Fighter("bbb")
	commitTurn(t, bt, you.Moves[0], foe.Moves[0])
	m := battleModel(t, bt, 120, 40)
	m.battle.playSeen = 2 // skip send-out replay; play the resolved turn
	next, _ := m.Update(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bravo", FoeHash: "bbb"})
	m = next.(Model)
	if !m.battle.playing {
		t.Fatal("expected event playback after a resolved turn")
	}
	m.wipeHold = 0
	m.battle.battleAge = 8
	v := m.renderBattle()
	if strings.Contains(v, "What will") {
		t.Fatal("menu should wait until playback finishes")
	}
	if !strings.Contains(v, "Turn") && !strings.Contains(v, "used") {
		t.Fatalf("expected a narrated event, got %q", v)
	}
	at := m.battle.playAt
	for range 6 {
		next, _ = m.Update(tickMsg{})
		m = next.(Model)
	}
	if m.battle.playAt != at {
		t.Fatal("action advanced too soon; animations should hold on a timer")
	}
	moves := 0
	for _, e := range bt.Events() {
		if e.Kind == battle.EventMoveUsed {
			moves++
		}
	}
	if moves >= 2 {
		for i := 0; i < 200 && m.battle.playing && !m.battle.playPause; i++ {
			m.battle.playHold = 0
			m.nextPlayEvent()
		}
		if !m.battle.playPause {
			t.Fatal("expected a readable pause after the first side's turn")
		}
		pausedAt := m.battle.playAt
		for i := 0; i < holdGroupPause && m.battle.playing; i++ {
			next, _ = m.Update(tickMsg{})
			m = next.(Model)
		}
		if m.battle.playing && m.battle.playPause && m.battle.playAt == pausedAt {
			t.Fatal("the side pause should auto-advance")
		}
	}
	drainPlayback(&m)
	v = m.renderBattle()
	if !strings.Contains(v, "What will") && m.battle.session.battle.State() != battle.StateOver {
		t.Fatal("expected the move menu after playback")
	}
}

func TestNarrationHidesHintChrome(t *testing.T) {
	set := loadSet(t)
	bt := liveBattle(t, set)
	you, _ := bt.Fighter("aaa")
	foe, _ := bt.Fighter("bbb")
	commitTurn(t, bt, you.Moves[0], foe.Moves[0])
	m := battleModel(t, bt, 120, 40)
	m.battle.playSeen = 2
	next, _ := m.Update(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bravo", FoeHash: "bbb"})
	m = next.(Model)
	m.wipeHold = 0
	m.battle.battleAge = 8
	v := m.renderBattle()
	for _, hint := range []string{"arrows/hjkl", "tab: battle log", "enter: continue", "enter: FIGHT"} {
		if strings.Contains(v, hint) {
			t.Fatalf("narration should hide hint chrome, found %q", hint)
		}
	}
}

func TestDamageBeatDrainsDuringHold(t *testing.T) {
	set := loadSet(t)
	bt := liveBattle(t, set)
	you, _ := bt.Fighter("aaa")
	foe, _ := bt.Fighter("bbb")
	commitTurn(t, bt, you.Moves[0], foe.Moves[0])
	m := battleModel(t, bt, 120, 40)
	m.battle.playSeen = 2
	next, _ := m.Update(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bravo", FoeHash: "bbb"})
	m = next.(Model)
	advancePlaybackUntil(&m, func(event battle.Event) bool {
		return event.Kind == battle.EventDamageDealt
	})
	e, ok := m.playEvent()
	if !ok || e.Kind != battle.EventDamageDealt {
		t.Fatal("expected a damage beat")
	}
	m.battle.playHoldTotal = 10
	m.battle.playHold = 10
	m.battle.playPause = false
	end := m.playheadEnd()
	post := platePost(t, bt, e, end)
	pre := post + e.Damage
	startYou, startFoe := m.arenaFighters()
	start := startYou.HP
	if e.TargetID == startFoe.ID {
		start = startFoe.HP
	}
	if start != pre {
		t.Fatalf("beat opens at %d, want exact pre-hit %d", start, pre)
	}
	m.battle.playHold = 0
	endYou, endFoe := m.arenaFighters()
	endHP := endYou.HP
	if e.TargetID == endFoe.ID {
		endHP = endFoe.HP
	}
	if endHP != post {
		t.Fatalf("beat settles at %d, want exact post-hit %d", endHP, post)
	}
}
