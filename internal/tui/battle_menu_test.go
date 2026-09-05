package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/onboard"
)

func TestFightRunOpensMovesAndEscReturns(t *testing.T) {
	m := battleModel(t, nil, 120, 40)
	m.battle.fightRoot = true
	if !strings.Contains(m.renderBattle(), "FIGHT") {
		t.Fatal("root menu should show FIGHT/RUN")
	}
	if strings.Contains(m.commandRow(0), "SWITCH") {
		t.Fatal("one-Monster root menu should hide SWITCH")
	}
	if !strings.Contains(commandPane(0), "38;2;110;231;240") {
		t.Fatal("FIGHT/RUN should use the same selection color as onboarding")
	}
	next, _ := m.battleKey(press("enter"))
	m = next.(Model)
	if m.battle.fightRoot {
		t.Fatal("FIGHT should open the move grid")
	}
	if !strings.Contains(m.renderBattle(), "ROOT ACCESS") {
		t.Fatal("expected move names after FIGHT")
	}
	if !strings.Contains(m.renderBattle(), "38;2;110;231;240") {
		t.Fatal("move menu should use the same selection color as onboarding")
	}
	next, _ = m.battleKey(press("esc"))
	m = next.(Model)
	if !m.battle.fightRoot {
		t.Fatal("esc should return to FIGHT/RUN")
	}
}

func liveTwoMonsterBattle(t *testing.T, set *content.Set) *battle.Battle {
	t.Helper()
	you, err := onboard.DefaultLoadout(set, "rootkit")
	if err != nil {
		t.Fatal(err)
	}
	reserve, err := onboard.DefaultLoadout(set, "mistcache")
	if err != nil {
		t.Fatal(err)
	}
	foe, err := onboard.DefaultLoadout(set, "emberbyte")
	if err != nil {
		t.Fatal(err)
	}
	you.ID, reserve.ID, foe.ID = "aaa-lead", "aaa-two", "bbb-lead"
	bt, err := battle.New(set,
		battle.Party{Trainer: "aaa", Members: []battle.PartyMember{{Monster: you}, {Monster: reserve}}},
		battle.Party{Trainer: "bbb", Members: []battle.PartyMember{{Monster: foe}}},
		battle.Seeded(1),
	)
	if err != nil {
		t.Fatal(err)
	}
	return bt
}

func TestFightMenuShowsSwitchWithTwoMonsters(t *testing.T) {
	set := loadSet(t)
	m := battleModel(t, liveTwoMonsterBattle(t, set), 120, 40)
	m.battle.fightRoot = true
	m.battle.battleIntro = false
	v := m.renderBattle()
	if !strings.Contains(v, "SWITCH") {
		t.Fatal("two-Monster fight should show SWITCH")
	}
}

func TestSwitchMenuReturnsToRootAfterTurn(t *testing.T) {
	set := loadSet(t)
	m := battleModel(t, liveTwoMonsterBattle(t, set), 120, 40)
	m.battle.battleIntro = false
	m.wipeHold = 0
	m.battle.playing = false
	m.battle.fightRoot = false
	m.battle.switchRoot = true
	m.finishPlayback()
	view := ansi.Strip(m.renderBattle())
	if !strings.Contains(view, "FIGHT") || !strings.Contains(view, "SWITCH") || !strings.Contains(view, "RUN") {
		t.Fatalf("after a Switch, the next turn should open FIGHT/SWITCH/RUN:\n%s", view)
	}
}
