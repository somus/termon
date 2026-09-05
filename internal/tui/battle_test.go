package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"termon.sh/internal/battle"
	"termon.sh/internal/server"
)

func TestBattleKeyOverEnterReturnsLobby(t *testing.T) {
	bt := liveBattle(t, loadSet(t))
	if err := bt.Forfeit("aaa"); err != nil {
		t.Fatal(err)
	}
	m := battleModel(t, bt, 120, 40)
	m.battle.resultHold = 0
	m.battle.playing = false
	_, cmd := m.battleKey(press("enter"))
	if cmd == nil {
		t.Fatalf("state=%s", bt.State())
	}
}

func TestIntroWaitsForEnter(t *testing.T) {
	m := battleModel(t, nil, 120, 40)
	bt := m.battle.session.battle
	m.battle.session = battleSession{}
	next, _ := m.Update(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bravo", FoeHash: "bbb"})
	m = next.(Model)
	if !m.battle.battleIntro || m.wipeHold == 0 {
		t.Fatal("intro should start after the battle wipe")
	}
	m.wipeHold = 1
	next, _ = m.Update(tickMsg{})
	m = next.(Model)
	if m.battle.introHold == 0 {
		t.Fatal("intro should start on a timer after the wipe")
	}
	next, _ = m.Update(press("enter"))
	m = next.(Model)
	if !m.battle.battleIntro {
		t.Fatal("enter during the intro hold must not skip")
	}
	m.battle.introHold = 0
	next, _ = m.Update(press("enter"))
	m = next.(Model)
	if m.battle.battleIntro {
		t.Fatal("enter after the intro hold should open the menu")
	}
}

func TestFirstSelectionDoesNotReplayBattleIntro(t *testing.T) {
	set := loadSet(t)
	bt := liveBattle(t, set)
	m := battleModel(t, bt, 120, 40)
	m.battle.battleIntro = false
	m.battle.introHold = 0
	m.wipeHold = 0
	m.battle.playSeen = len(bt.Events())

	you, _ := bt.Fighter("aaa")
	if err := bt.Select("aaa", battle.Action{Kind: battle.ActionMove, Move: you.Moves[0]}); err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bravo", FoeHash: "bbb"})
	m = next.(Model)

	if m.battle.battleIntro || m.battle.introHold != 0 {
		t.Fatalf("same-battle selection replayed intro: intro=%v hold=%d", m.battle.battleIntro, m.battle.introHold)
	}
	if youT, foeT := m.introSlide(); youT != 1 || foeT != 1 {
		t.Fatalf("sprites left resting positions: you=%v foe=%v", youT, foeT)
	}
	if got := m.renderBattle(); !strings.Contains(got, "Waiting for opponent") && !strings.Contains(got, "LOCKED") {
		t.Fatalf("battle view = %q, want opponent wait state", got)
	}
}

func TestIntroSendOutOmitsTrainerID(t *testing.T) {
	m := battleModel(t, nil, 120, 40)
	m.battle.battleIntro = true
	m.wipeHold = 0
	m.battle.session.foe = "deadbeefdeadbeef"
	view := ansi.Strip(m.renderBattle())
	if strings.Contains(view, "deadbeef") {
		t.Fatalf("intro leaked trainer id:\n%s", view)
	}
	if !strings.Contains(view, "Foe sent out") {
		t.Fatalf("intro should name a foe send-out:\n%s", view)
	}
}
