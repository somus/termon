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

func TestBattleWaitsForOpponentReplacement(t *testing.T) {
	bt, _ := liveFoeReserveBattle(t, loadSet(t))
	for range 60 {
		if bt.State() != battle.StateAwaitingActions {
			break
		}
		a, _ := bt.Fighter("aaa")
		b, _ := bt.Fighter("bbb")
		commitTurn(t, bt, a.Moves[0], b.Moves[0])
	}
	if bt.State() != battle.StateAwaitingReplacement {
		t.Fatalf("phase = %s, want replacement", bt.State())
	}
	replacing, waiting := "aaa", "bbb"
	if bt.Snapshot("bbb").ReplacementRequired {
		replacing, waiting = waiting, replacing
	}
	m := battleModel(t, bt, 100, 32)
	m.battle.session.you, m.battle.session.foeHash = waiting, replacing
	m.battle.fightRoot = true
	view := ansi.Strip(m.renderBattle())
	if !strings.Contains(view, "Waiting for opponent to choose a replacement") || strings.Contains(view, "FIGHT") {
		t.Errorf("survivor should see replacement wait, not actions:\n%s", view)
	}
	if footer := ansi.Strip(m.battle.footer()); strings.Contains(footer, "fight") || strings.Contains(footer, "lock") {
		t.Errorf("waiting footer advertises actions: %s", footer)
	}
	for _, menu := range []string{"root", "moves", "switch"} {
		for _, key := range []string{"enter", "1", "2", "s", "f", "right", "esc"} {
			t.Run(menu+"/"+key, func(t *testing.T) {
				before := m.battle
				before.fightRoot = menu == "root"
				before.switchRoot = menu == "switch"
				after, cmd := before.key(press(key))
				if cmd.kind != battleCommandNone || after.fightRoot != before.fightRoot || after.switchRoot != before.switchRoot || after.cursor != before.cursor {
					t.Fatalf("%s accepted while opponent needs replacement: command=%v", key, cmd.kind)
				}
			})
		}
	}
	logged, _ := m.battle.key(press("tab"))
	if !logged.logOpen {
		t.Fatal("battle log should remain available while waiting")
	}
	m.battle.session.you, m.battle.session.foeHash = replacing, waiting
	if view := ansi.Strip(m.renderBattle()); !strings.Contains(view, "Choose a replacement") {
		t.Fatalf("affected trainer must retain replacement menu:\n%s", view)
	}
	_, cmd := m.battle.key(press("enter"))
	if cmd.kind != battleCommandReplace {
		t.Fatalf("affected trainer cannot replace: command=%v", cmd.kind)
	}
	if err := bt.Replace(replacing, cmd.monsterID); err != nil {
		t.Fatal(err)
	}
	if err := bt.AdvanceReveal(); err != nil {
		t.Fatal(err)
	}
	m.battle.session.you, m.battle.session.foeHash = waiting, replacing
	if view := ansi.Strip(m.renderBattle()); !strings.Contains(view, "FIGHT") {
		t.Fatalf("action menu should return after replacement:\n%s", view)
	}
	after, _ := m.battle.key(press("enter"))
	if after.fightRoot {
		t.Fatal("fight menu should open after replacement")
	}
}
