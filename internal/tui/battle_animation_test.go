package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/onboard"
	"termon.sh/internal/server"
)

func TestTypeOnRevealsPrefix(t *testing.T) {
	if got := typeOn("ABCDEF", 0); got != "AB" {
		t.Fatalf("first frame typeOn = %q", got)
	}
	if got := typeOn("ABCDEF", 1); got != "ABCD" {
		t.Fatalf("typeOn = %q", got)
	}
	if got := typeOn("ABCDEF", 20); got != "ABCDEF" {
		t.Fatalf("full typeOn = %q", got)
	}
}

func TestTypeMarkShowsCaretWhileTyping(t *testing.T) {
	if !strings.Contains(typeMark(false, true, 0), "▌") {
		t.Fatal("typing should show a block caret")
	}
	if strings.Contains(typeMark(false, true, 4), "▌") {
		t.Fatal("caret should blink off")
	}
	if strings.Contains(typeMark(true, false, 0), "▌") || !strings.Contains(typeMark(true, false, 0), "▼") {
		t.Fatal("finished text should keep the continue arrow, not the caret")
	}
}

func TestWipeOnNewBattle(t *testing.T) {
	m := battleModel(t, nil, 120, 40)
	next, _ := m.Update(server.BattleMsg{
		Battle: liveBattle(t, m.set), You: "aaa", Foe: "bravo", FoeHash: "bbb",
	})
	m = next.(Model)
	if m.wipeHold == 0 {
		t.Fatal("new battle should wipe")
	}
	v := m.renderWipe()
	if !strings.Contains(v, "█") || !strings.Contains(v, " ") {
		t.Fatal("wipe should be a checkerboard, not a solid flash")
	}
}

func TestAttackPoseLoopsDuringHold(t *testing.T) {
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
		return event.Kind == battle.EventMoveUsed
	})
	m.battle.playGrace = 0
	m.battle.playHold = 20
	you1, foe1 := m.battlePoses()
	m.battle.battleAge += 3
	you2, foe2 := m.battlePoses()
	e, _ := m.playEvent()
	looped := you1 != you2
	if e.Actor != m.battle.session.you {
		looped = foe1 != foe2
	}
	if !looped {
		t.Fatal("attack pose should loop during the hold, not freeze on one frame")
	}
}

func TestIntroSlidesSprites(t *testing.T) {
	m := battleModel(t, nil, 120, 40)
	m.battle.battleIntro = true
	m.wipeHold = 0
	m.battle.introHold = holdIntro
	startYou, startFoe := m.placeSprites(20)
	m.battle.introHold = 0
	endYou, endFoe := m.placeSprites(20)
	if startFoe.x <= endFoe.x {
		t.Fatalf("foe should slide in from the right: start %d end %d", startFoe.x, endFoe.x)
	}
	if startYou.x >= endYou.x {
		t.Fatalf("player should slide in from the left: start %d end %d", startYou.x, endYou.x)
	}
}

func TestHitShakeMovesDefender(t *testing.T) {
	set := loadSet(t)
	bt := liveBattle(t, set)
	you, _ := bt.Fighter("aaa")
	foe, _ := bt.Fighter("bbb")
	commitTurn(t, bt, you.Moves[0], foe.Moves[0])
	m := battleModel(t, bt, 120, 40)
	m.battle.playSeen = 2
	next, _ := m.Update(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bravo", FoeHash: "bbb"})
	m = next.(Model)
	if !advancePlaybackUntil(&m, func(event battle.Event) bool {
		return event.Kind == battle.EventDamageDealt
	}) {
		t.Fatal("expected a damage beat")
	}
	m.battle.playGrace = 0
	m.battle.playHold = 20
	m.battle.playPause = false
	e, _ := m.playEvent()
	m.battle.battleAge = 0
	aYou, aFoe := m.placeSprites(20)
	m.battle.battleAge = 1
	bYou, bFoe := m.placeSprites(20)
	moved := aFoe.x != bFoe.x
	if e.Actor != m.battle.session.you {
		moved = aYou.x != bYou.x
	}
	if !moved {
		t.Fatal("defender should shake on the damage hold")
	}
}

func TestFaintSinksThenVanishes(t *testing.T) {
	set := loadSet(t)
	bt := liveBattle(t, set)
	you, _ := bt.Fighter("aaa")
	foe, _ := bt.Fighter("bbb")
	for i := 0; i < 20 && bt.State() != battle.StateOver; i++ {
		commitTurn(t, bt, you.Moves[0], foe.Moves[0])
		if bt.State() == battle.StateOver {
			break
		}
	}
	if bt.State() != battle.StateOver {
		t.Fatal("expected a finished battle so someone faints")
	}
	m := battleModel(t, bt, 120, 40)
	m.battle.playSeen = 2
	next, _ := m.Update(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bravo", FoeHash: "bbb"})
	m = next.(Model)
	if !advancePlaybackUntil(&m, func(event battle.Event) bool {
		return event.Kind == battle.EventFainted
	}) {
		t.Fatal("expected a faint beat")
	}
	m.battle.playGrace = 0
	m.battle.playHold = 20
	m.battle.playPause = false
	e, _ := m.playEvent()
	m.battle.battleAge = 0
	you0, foe0 := m.placeSprites(20)
	m.battle.battleAge = 5
	you1, foe1 := m.placeSprites(20)
	if e.Actor == m.battle.session.you {
		if you0.y >= you1.y {
			t.Fatal("faint should sink downward")
		}
	} else if foe0.y >= foe1.y {
		t.Fatal("faint should sink downward")
	}
	m.battle.battleAge = faintHideAfter
	you2, foe2 := m.placeSprites(20)
	if e.Actor == m.battle.session.you {
		if you2.show {
			t.Fatal("fainted player should vanish")
		}
	} else if foe2.show {
		t.Fatal("fainted foe should vanish")
	}
}

func liveFoeReserveBattle(t *testing.T, set *content.Set) (*battle.Battle, string) {
	t.Helper()
	you, err := onboard.DefaultLoadout(set, "rootkit")
	if err != nil {
		t.Fatal(err)
	}
	youRes, err := onboard.DefaultLoadout(set, "mistcache")
	if err != nil {
		t.Fatal(err)
	}
	lead, err := onboard.DefaultLoadout(set, "emberbyte")
	if err != nil {
		t.Fatal(err)
	}
	reserve, err := onboard.DefaultLoadout(set, "scorchip")
	if err != nil {
		t.Fatal(err)
	}
	you.ID, youRes.ID, lead.ID, reserve.ID = "aaa-lead", "aaa-two", "bbb-lead", "bbb-two"
	bt, err := battle.New(set,
		battle.Party{Trainer: "aaa", Members: []battle.PartyMember{{Monster: you}, {Monster: youRes}}},
		battle.Party{Trainer: "bbb", Members: []battle.PartyMember{{Monster: lead}, {Monster: reserve}}},
		battle.Seeded(1),
	)
	if err != nil {
		t.Fatal(err)
	}
	return bt, reserve.ID
}

func faintFoeThenReplace(t *testing.T, bt *battle.Battle, reserveID string) {
	t.Helper()
	for range 60 {
		if bt.Snapshot("bbb").ReplacementRequired {
			if err := bt.Replace("bbb", reserveID); err != nil {
				t.Fatal(err)
			}
			return
		}
		if bt.Snapshot("aaa").ReplacementRequired {
			if err := bt.Replace("aaa", "aaa-two"); err != nil {
				t.Fatal(err)
			}
			if bt.State() == battle.StateRevealing {
				if err := bt.AdvanceReveal(); err != nil {
					t.Fatal(err)
				}
			}
			continue
		}
		if bt.State() == battle.StateRevealing {
			if err := bt.AdvanceReveal(); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if bt.State() == battle.StateOver {
			t.Fatal("battle ended before the foe could send a replacement")
		}
		you, _ := bt.Fighter("aaa")
		foe, _ := bt.Fighter("bbb")
		commitTurn(t, bt, you.Moves[0], foe.Moves[0])
	}
	t.Fatal("foe never needed a replacement")
}

func TestReplacementSpriteReturnsAfterFaint(t *testing.T) {
	set := loadSet(t)
	bt, reserveID := liveFoeReserveBattle(t, set)
	faintFoeThenReplace(t, bt, reserveID)
	m := battleModel(t, bt, 120, 40)
	m.battle.playSeen = 2
	next, _ := m.Update(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bravo", FoeHash: "bbb"})
	m = next.(Model)
	drainPlayback(&m)
	m.syncBattleAnims()
	_, foe := m.placeSprites(20)
	if !foe.show {
		t.Fatal("replacement should be on the field after the fainted lead vanishes")
	}
	view := strings.ToUpper(ansi.Strip(m.renderBattle()))
	if !strings.Contains(view, "SCORCHIP") {
		t.Fatalf("plate should name the replacement:\n%s", view)
	}
}

func TestFaintPlateKeepsFaintedName(t *testing.T) {
	set := loadSet(t)
	bt, reserveID := liveFoeReserveBattle(t, set)
	faintFoeThenReplace(t, bt, reserveID)
	m := battleModel(t, bt, 120, 40)
	m.battle.playSeen = 2
	next, _ := m.Update(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bravo", FoeHash: "bbb"})
	m = next.(Model)
	found := advancePlaybackUntil(&m, func(event battle.Event) bool {
		return event.Kind == battle.EventFainted && event.Actor == m.battle.session.foeHash
	})
	if !found {
		t.Fatal("expected the foe's faint beat")
	}
	view := strings.ToUpper(ansi.Strip(m.renderArena(20)))
	if strings.Contains(view, "SCORCHIP") {
		t.Fatalf("faint plate should not jump to the replacement:\n%s", view)
	}
	if !strings.Contains(view, "EMBERBYTE") {
		t.Fatalf("faint plate should still name the lead:\n%s", view)
	}
}

func TestReplacementSlidesIn(t *testing.T) {
	set := loadSet(t)
	bt, reserveID := liveFoeReserveBattle(t, set)
	faintFoeThenReplace(t, bt, reserveID)
	m := battleModel(t, bt, 120, 40)
	m.battle.playSeen = 2
	next, _ := m.Update(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bravo", FoeHash: "bbb"})
	m = next.(Model)
	found := advancePlaybackUntil(&m, func(event battle.Event) bool {
		return event.Kind == battle.EventReplacement && event.Actor == m.battle.session.foeHash
	})
	if !found {
		t.Fatal("expected a replacement send-out")
	}
	m.syncBattleAnims()
	m.battle.battleIntro = false
	m.wipeHold = 0
	m.battle.battleAge = 0
	_, start := m.placeSprites(20)
	m.battle.battleAge = slideIn
	_, end := m.placeSprites(20)
	if !start.show || !end.show {
		t.Fatal("replacement sprite should be visible while it slides in")
	}
	if start.x <= end.x {
		t.Fatalf("foe replacement should slide in from the right: start %d end %d", start.x, end.x)
	}
}
