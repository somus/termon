package tui

import (
	"testing"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/gametest"
	"termon.sh/internal/onboard"
)

func liveBattle(t *testing.T, set *content.Set) *battle.Battle {
	t.Helper()
	you, err := onboard.DefaultLoadout(set, "rootkit")
	if err != nil {
		t.Fatal(err)
	}
	foe, err := onboard.DefaultLoadout(set, "emberbyte")
	if err != nil {
		t.Fatal(err)
	}
	you.ID = "aaa-lead"
	foe.ID = "bbb-lead"
	bt, err := battle.New(set,
		battle.Party{Trainer: "aaa", Members: []battle.PartyMember{{Monster: you}}},
		battle.Party{Trainer: "bbb", Members: []battle.PartyMember{{Monster: foe}}},
		battle.Seeded(1),
	)
	if err != nil {
		t.Fatal(err)
	}
	return bt
}

func battleModel(t *testing.T, bt *battle.Battle, w, h int) Model {
	t.Helper()
	set := loadSet(t)
	if bt == nil {
		bt = liveBattle(t, set)
	}
	you, _ := bt.Fighter("aaa")
	m := Model{
		width: w, height: h, hash: "aaa", set: set,
		save:   gametest.SaveWithStarter("aaa-lead", "rootkit", you.Moves),
		screen: screenBattle,
		battle: battleScreenModel{session: battleSession{battle: bt, you: "aaa", foe: "bravo", foeHash: "bbb"}},
	}
	m.syncBattleAnims()
	return m
}

func commitTurn(t *testing.T, bt *battle.Battle, youMove, foeMove string) {
	t.Helper()
	if err := bt.Select("aaa", battle.Action{Kind: battle.ActionMove, Move: youMove}); err != nil {
		t.Fatal(err)
	}
	if err := bt.Select("bbb", battle.Action{Kind: battle.ActionMove, Move: foeMove}); err != nil {
		t.Fatal(err)
	}
	if bt.State() == battle.StateRevealing {
		if err := bt.AdvanceReveal(); err != nil {
			t.Fatal(err)
		}
	}
}
