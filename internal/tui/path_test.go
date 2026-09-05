package tui

import (
	"strings"
	"testing"

	"termon.sh/internal/game"
	"termon.sh/internal/lobby"
)

func TestTutorialCaptureLessonFlow(t *testing.T) {
	t.Parallel()
	hub, set, _ := testHub(t)
	p := joinNew(t, hub, set, "tutorial-flow")
	p.finishOnboard()
	if p.m.screen != screenBattle {
		t.Fatal(p.dump("onboard should start Capture Lesson 1, not the Dojo floor"))
	}
	if !p.m.tutorial || !p.m.battle.captureOn {
		t.Fatal(p.dump("lesson 1 should keep the tutorial flag and Capture Gauge"))
	}
	if n := partySize(p.save()); n != 1 {
		t.Fatalf("party size %d, want the starter only", n)
	}

	p.playLessonUntilProgression(1, partySize(p.save()))
	if partySize(p.save()) < 2 {
		t.Fatal(p.dump("lesson 1 should capture into party slot 2 before the XP card"))
	}
	if p.m.screen != screenProgression {
		t.Fatal(p.dump("lesson 1 should overlay the Progression Summary on the Dojo"))
	}
	if p.m.wipeHold > 0 || p.m.battle.battleIntro {
		t.Fatal(p.dump("the XP card must not be replaced by a new send-out"))
	}
	view := strings.ToUpper(p.view())
	if !strings.Contains(view, "PROGRESSION") {
		t.Fatal(p.dump("lesson 1 should overlay the Progression Summary on the Dojo"))
	}

	p.press("enter")
	for range 20 {
		if p.m.screen == screenBattle {
			break
		}
		p.tick()
	}
	if p.m.screen != screenBattle {
		t.Fatal(p.dump("Enter on the first XP card should start Capture Lesson 2"))
	}
	if n := len(p.m.battleSnap().YourParty); n < 2 {
		t.Fatalf("lesson 2 party size %d, want two owned Monsters (not a lesson 1 restart)", n)
	}

	p.playLessonUntilProgression(2, partySize(p.save()))
	if p.m.screen != screenProgression {
		t.Fatal(p.dump("lesson 2 should show the Progression Summary"))
	}
	p.press("enter")
	if p.m.screen != screenLobby {
		t.Fatal(p.dump("after lesson 2 the XP card should return to the Dojo"))
	}
	if !game.FullParty(p.save()) {
		t.Fatalf("party after lessons = %+v", p.save().Party)
	}
	if p.m.tutorial {
		t.Fatal("tutorial should end once the Party is full")
	}
}

func TestLessonOneWorkbenchEscStartsLessonTwo(t *testing.T) {
	t.Parallel()
	hub, set, _ := testHub(t)
	p := joinNew(t, hub, set, "no-roam")
	p.finishOnboard()
	p.playLessonUntilProgression(1, partySize(p.save()))
	if p.m.screen != screenProgression {
		t.Fatal(p.dump("lesson 1 should overlay the Progression Summary"))
	}

	p.press("r")
	if p.m.screen != screenWorkbench {
		t.Fatal(p.dump("R on the XP card should open the Workbench"))
	}

	p.press("esc")
	for range 20 {
		if p.m.screen == screenBattle {
			break
		}
		p.tick()
	}
	if p.m.screen == screenLobby {
		t.Fatal(p.dump("esc from Workbench must not return to Dojo roam"))
	}
	if p.m.screen != screenBattle {
		t.Fatal(p.dump("esc from Workbench should start Capture Lesson 2"))
	}
	if n := len(p.m.battleSnap().YourParty); n < 2 {
		t.Fatalf("lesson 2 party size %d, want two owned Monsters", n)
	}
}

func TestTrainerPath(t *testing.T) {
	t.Parallel()
	hub, set, _ := testHub(t)
	p := joinNew(t, hub, set, "path-alpha")
	p.finishOnboard()
	if p.m.save == nil || len(p.m.save.Collection) != 1 || p.m.save.Party[0] == "" {
		t.Fatalf("starter save = %+v", p.m.save)
	}

	p.completeLesson(1)
	sv := p.save()
	if sv.Party[1] == "" {
		t.Fatal(p.dump("lesson 1 should fill party slot 2"))
	}

	p.completeLesson(2)
	sv = p.save()
	if !game.FullParty(sv) {
		t.Fatalf("party after lessons = %+v", sv.Party)
	}
	if p.m.screen != screenLobby {
		t.Fatal(p.dump("both lessons should return to the Dojo"))
	}

	p.openWorkbenchRoundtrip()

	q := joinNew(t, hub, set, "path-bravo")
	q.finishOnboard()
	q.completeLesson(1)
	q.completeLesson(2)
	if !game.FullParty(q.save()) {
		t.Fatal(q.dump("second trainer needs a Full Party"))
	}

	if p.m.snap.Offer != nil {
		p.press("n")
	}
	p.press("f")
	if p.m.screen != screenQueueEditor {
		t.Fatal(p.dump("expected Battle Party editor"))
	}
	p.press("enter")
	if q.m.snap.Offer != nil {
		q.press("n")
	}
	q.press("f")
	q.press("enter")
	playPair(p, q, func() bool {
		if p.m.screen != screenLobby || q.m.screen != screenLobby {
			return false
		}
		return p.save().Wins+p.save().Losses+q.save().Wins+q.save().Losses > 0
	})
	if p.save().Wins+p.save().Losses == 0 && q.save().Wins+q.save().Losses == 0 {
		t.Fatal("queue battle should record a win or loss")
	}

	before := len(p.save().Collection)
	p.walkAdjacentTo(lobby.NoticeBoardX, lobby.NoticeBoardY)
	p.press("enter")
	if p.m.screen != screenSignalBoard {
		t.Fatal(p.dump("expected Signal Board"))
	}
	p.press("enter")
	p.press("enter")
	p.playUntil(func() bool {
		if p.m.screen == screenLobby && !p.m.snap.You.InBattle {
			return len(p.save().Collection) > before || strings.Contains(strings.ToLower(p.view()), "hunt")
		}
		if p.m.screen == screenExpedition {
			msg := p.m.expeditionFlow.msg
			return msg.Phase == "captured" || msg.Phase == "hunt_failed" || msg.Phase == "failed"
		}
		return false
	})
	if p.m.screen == screenExpedition || p.m.screen == screenProgression {
		p.press("enter")
		if p.m.screen == screenProgression {
			p.press("enter")
		}
	}

	p.walkAdjacentTo(lobby.MasterX, lobby.MasterY)
	p.press("enter")
	p.press("enter")
	if p.m.screen != screenDojoMenu || p.m.dojo.view != dojoViewSparringTiers {
		t.Fatal(p.dump("expected Sparring tiers"))
	}
	p.press("enter")
	p.press("enter")
	p.playUntil(func() bool { return p.m.screen == screenLobby && !p.m.snap.You.InBattle })

	p.walkAdjacentTo(lobby.MasterX, lobby.MasterY)
	p.press("enter")
	p.press("down")
	p.press("enter")
	p.press("enter")
	p.playUntil(func() bool { return p.m.screen == screenLobby && !p.m.snap.You.InBattle })
}
