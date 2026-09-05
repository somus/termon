package tui

import (
	"strings"
	"testing"

	"termon.sh/internal/battle"
)

func TestBattleLogModalShowsEvents(t *testing.T) {
	set := loadSet(t)
	bt := liveBattle(t, set)
	you, _ := bt.Fighter("aaa")
	foe, _ := bt.Fighter("bbb")
	commitTurn(t, bt, you.Moves[0], foe.Moves[0])
	m := battleModel(t, bt, 120, 40)
	next, _ := m.battleKey(press("tab"))
	m = next.(Model)
	v := m.renderBattle()
	if !strings.Contains(v, "battle log") {
		t.Fatal("expected log modal")
	}
	if !strings.Contains(v, "you") || !strings.Contains(v, "foe") {
		t.Fatal("log should mark your hits vs theirs")
	}
	if !strings.Contains(v, "HP") {
		t.Fatal("log should sit on the battle, not replace it")
	}
}

func TestBattleLogScrollsWithoutClosing(t *testing.T) {
	bt, _ := hpBattle(t)
	for range 2 {
		you, _ := bt.Fighter("aaa")
		foe, _ := bt.Fighter("bbb")
		commitTurn(t, bt, you.Moves[0], foe.Moves[0])
	}
	m := battleModel(t, bt, 80, 7)
	maxTop := m.battleLogMaxTop()
	if maxTop < 1 {
		t.Fatalf("battle log max top = %d, want scrollable history", maxTop)
	}

	next, _ := m.battleKey(press("tab"))
	m = next.(Model)
	if !m.battle.logOpen || !m.battle.logFollow || m.battle.logTop != maxTop {
		t.Fatalf("opened log = {open:%v top:%d follow:%v}, want bottom %d", m.battle.logOpen, m.battle.logTop, m.battle.logFollow, maxTop)
	}

	next, _ = m.battleKey(press("up"))
	m = next.(Model)
	if !m.battle.logOpen || m.battle.logTop != maxTop-1 || m.battle.logFollow {
		t.Fatalf("up = {open:%v top:%d follow:%v}, want top %d", m.battle.logOpen, m.battle.logTop, m.battle.logFollow, maxTop-1)
	}
	next, _ = m.battleKey(press("end"))
	m = next.(Model)
	if m.battle.logTop != maxTop || !m.battle.logFollow {
		t.Fatalf("end = {top:%d follow:%v}, want bottom %d", m.battle.logTop, m.battle.logFollow, maxTop)
	}
	next, _ = m.battleKey(press("pgup"))
	m = next.(Model)
	if m.battle.logTop != 0 || m.battle.logFollow {
		t.Fatalf("page up = {top:%d follow:%v}, want oldest history", m.battle.logTop, m.battle.logFollow)
	}
	next, _ = m.battleKey(press("x"))
	m = next.(Model)
	if !m.battle.logOpen {
		t.Fatal("unbound key closed the battle log")
	}
	next, _ = m.battleKey(press("esc"))
	m = next.(Model)
	if m.battle.logOpen {
		t.Fatal("escape did not close the battle log")
	}
}

func TestBattleLogFormatsBeats(t *testing.T) {
	events := []battle.Event{
		{Turn: 1, Kind: battle.EventTurnStarted, Text: "Turn 1"},
		{Turn: 1, Actor: "aaa", Kind: battle.EventMoveUsed, Text: "Aquabit used Ping Flood!"},
		{Turn: 1, Actor: "aaa", Kind: battle.EventSuperEffective, Text: "It's super effective!"},
		{Turn: 1, Actor: "aaa", Kind: battle.EventDamageDealt, Damage: 12, Text: "Chippunk took 12 damage."},
		{Turn: 1, Actor: "bbb", Kind: battle.EventMoveUsed, Text: "Chippunk used Punch Card!"},
		{Turn: 1, Actor: "bbb", Kind: battle.EventMissed, Text: "The attack missed!"},
		{Turn: 1, Actor: "bbb", Kind: battle.EventFainted, Text: "Chippunk fainted!"},
		{Turn: 1, Actor: "aaa", Kind: battle.EventBattleOver, Text: "Battle over."},
	}
	got := strings.Join(formatBattleLog(events, "aaa", 40), "\n")
	for _, want := range []string{"turn 1", "you", "PING FLOOD", "12", "2×", "foe", "miss", "fainted", "you won"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in\n%s", want, got)
		}
	}
	empty := strings.Join(formatBattleLog(nil, "aaa", 40), "\n")
	if !strings.Contains(empty, "No turns yet") {
		t.Fatal("empty log should explain itself")
	}
}
