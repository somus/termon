package tui

import (
	"testing"

	"termon.sh/internal/battle"
	"termon.sh/internal/server"
)

func TestProgressionWaitsForKOPlayback(t *testing.T) {
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

	next, _ = m.Update(server.ProgressionMsg{Entries: []server.ProgressionEntry{{
		Slot: 1, Name: "Aquabit", XPGained: 90, Level: 2, Share: "active",
	}}})
	m = next.(Model)
	if m.screen != screenBattle {
		t.Fatal("progression must wait until the faint sequence finishes")
	}

	for i := 0; i < 400 && m.battle.playing; i++ {
		next, _ = m.Update(tickMsg{})
		m = next.(Model)
	}
	if m.battle.playing {
		t.Fatal("KO playback did not finish")
	}
	if m.screen != screenBattle {
		t.Fatal("results should stay on the Battle after playback")
	}

	for i := 0; i < holdResult+2 && m.screen == screenBattle; i++ {
		next, _ = m.Update(tickMsg{})
		m = next.(Model)
	}
	if m.screen != screenProgression {
		t.Fatalf("screen=%d, want the XP card on the Dojo after the results hold, without Enter", m.screen)
	}
}

func TestProgressionOpensAfterRevealingCaptureTurn(t *testing.T) {
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
		t.Fatalf("state=%s, want revealing (Gauge can fill without a faint)", bt.State())
	}

	m := battleModel(t, bt, 120, 40)
	m.battle.playSeen = 2
	next, _ := m.Update(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bravo", FoeHash: "bbb"})
	m = next.(Model)
	m.wipeHold = 0
	if !m.battle.playing {
		t.Fatal("expected playback of the capture turn")
	}

	next, _ = m.Update(server.ProgressionMsg{Entries: []server.ProgressionEntry{{
		Slot: 1, Name: "Aquabit", XPGained: 90, Level: 2, Share: "active",
	}}})
	m = next.(Model)
	if m.screen != screenBattle {
		t.Fatal("progression must wait until the capture turn finishes playing")
	}

	for i := 0; i < 400 && m.battle.playing; i++ {
		next, _ = m.Update(tickMsg{})
		m = next.(Model)
	}
	if m.battle.playing {
		t.Fatal("capture-turn playback did not finish")
	}
	if m.screen != screenProgression {
		t.Fatalf("screen=%d, want progression after the revealing capture turn", m.screen)
	}
}

func TestExpeditionRecoveryWaitsForKOPlayback(t *testing.T) {
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

	next, _ = m.Update(server.ProgressionMsg{Entries: []server.ProgressionEntry{{
		Slot: 1, Name: "Rootkit", XPGained: 40, Level: 2, Share: "active",
	}}})
	m = next.(Model)
	next, _ = m.Update(server.ExpeditionMsg{
		Phase: "recovery", Family: "wickware", FamilyName: "Wickware",
		RecoveryNext: "prep2", LastEncounter: "prep1", LastXPGained: 40,
	})
	m = next.(Model)
	if m.screen != screenBattle {
		t.Fatal("recovery must wait until the faint sequence finishes")
	}

	for i := 0; i < 400 && m.battle.playing; i++ {
		next, _ = m.Update(tickMsg{})
		m = next.(Model)
	}
	if m.battle.playing {
		t.Fatal("KO playback did not finish")
	}
	if m.screen != screenBattle {
		t.Fatal("results should stay on the Battle after playback")
	}

	for i := 0; i < holdResult+2 && m.screen == screenBattle; i++ {
		next, _ = m.Update(tickMsg{})
		m = next.(Model)
	}
	if m.screen != screenProgression {
		t.Fatalf("screen=%d, want the XP card after the results hold", m.screen)
	}

	next, _ = m.Update(press("enter"))
	m = next.(Model)
	if m.screen != screenExpedition || m.expeditionFlow.msg.Phase != "recovery" {
		t.Fatalf("screen=%d phase=%q, want expedition recovery after the XP card", m.screen, m.expeditionFlow.msg.Phase)
	}
}

func TestNewBattleDoesNotClobberPendingProgression(t *testing.T) {
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
	next, _ = m.Update(server.ProgressionMsg{Entries: []server.ProgressionEntry{{
		Slot: 1, Name: "Aquabit", XPGained: 90, Level: 2, Share: "active",
	}}})
	m = next.(Model)
	if !m.battle.playing || m.screen != screenBattle {
		t.Fatal("expected playback of the finishing turn")
	}

	next, _ = m.Update(server.BattleMsg{
		Battle: liveBattle(t, set), You: "aaa", Foe: "next", FoeHash: "ccc",
	})
	m = next.(Model)
	if m.battle.session.battle != bt {
		t.Fatal("a new Battle must not replace the faint sequence or skip the XP card")
	}
	if m.wipeHold > 0 || m.battle.battleIntro {
		t.Fatal("a new Battle must not wipe into a fresh send-out")
	}

	for i := 0; i < 400 && m.battle.playing; i++ {
		next, _ = m.Update(tickMsg{})
		m = next.(Model)
	}
	for i := 0; i < holdResult+2 && m.screen == screenBattle; i++ {
		next, _ = m.Update(tickMsg{})
		m = next.(Model)
	}
	if m.screen != screenProgression {
		t.Fatalf("screen=%d, want the XP card after the original faint", m.screen)
	}
}
