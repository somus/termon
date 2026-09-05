package tui

import (
	"strings"
	"testing"

	"termon.sh/internal/lobby"
)

func TestQueuePath(t *testing.T) {
	t.Parallel()
	hub, set, store := testHub(t)
	p := joinOnboarded(t, hub, set, "queue-alpha")
	p.press("f")
	if p.m.screen == screenQueueEditor || p.m.screen == screenQueue {
		t.Fatal(p.dump("partial Party must not enter the Queue"))
	}

	p = joinReady(t, hub, set, store, "queue-ready")
	p.press("f")
	queueSetup := strings.ToLower(p.view())
	if !strings.Contains(queueSetup, "battle party") || strings.Contains(queueSetup, "eligible pool") || strings.Contains(queueSetup, "equip focused move") {
		t.Fatal(p.dump("pre-Queue screen should edit roster and order, not Moves"))
	}
	p.press("enter")
	if p.m.screen != screenQueue {
		t.Fatal(p.dump("expected Queue waiting screen"))
	}
	view := strings.ToLower(p.view())
	if !strings.Contains(view, "waiting") {
		t.Fatal(p.dump("Queue wait should name the wait"))
	}
	p.press("x")
	if p.m.screen != screenLobby {
		t.Fatal(p.dump("x should cancel the Queue"))
	}
	if p.m.snap.You.InQueue {
		t.Fatal("cancel should clear InQueue")
	}

	q := joinReady(t, hub, set, store, "queue-bravo")
	beforeP, beforeQ := battleLoadouts(p.save()), battleLoadouts(q.save())
	p.enterQueue()
	if p.m.screen != screenQueue {
		t.Fatal(p.dump("solo Queue should wait"))
	}
	q.enterQueue()
	playPair(p, q, func() bool { return pvpFinished(p, q) })
	if p.save().Wins+p.save().Losses == 0 || q.save().Wins+q.save().Losses == 0 {
		t.Fatal("Queue Battle should record a result for both Trainers")
	}
	if p.save().Wins+q.save().Wins != 1 || p.save().Losses+q.save().Losses != 1 {
		t.Fatalf("Queue record p=%d-%d q=%d-%d", p.save().Wins, p.save().Losses, q.save().Wins, q.save().Losses)
	}
	loadoutsUnchanged(t, beforeP, battleLoadouts(p.save()))
	loadoutsUnchanged(t, beforeQ, battleLoadouts(q.save()))
}

func TestAssignPartySlotSwapsExistingMembers(t *testing.T) {
	party := [3]string{"alpha", "bravo", "charlie"}
	got := assignPartySlot(party, 0, "charlie")
	want := [3]string{"charlie", "bravo", "alpha"}
	if got != want {
		t.Fatalf("assignPartySlot = %v, want %v", got, want)
	}
}

func TestChallengePath(t *testing.T) {
	t.Parallel()
	hub, set, store := testHub(t)
	p := joinReady(t, hub, set, store, "challenge-alpha")
	q := joinReady(t, hub, set, store, "challenge-bravo")

	p.walkAdjacentTo(lobby.NoticeBoardX, lobby.NoticeBoardY)
	q.walkAdjacentTo(lobby.MasterX, lobby.MasterY)
	p.flush()
	q.flush()
	p.press("c")
	if p.m.snap.You.InBattle || q.m.snap.You.InBattle {
		t.Fatal("Challenge without adjacency must not start a Battle")
	}
	q.flush()
	if q.m.snap.Offer != nil {
		t.Fatal(q.dump("distant Challenge should not offer"))
	}

	p.walkAdjacentToTrainer(q)
	p.press("c")
	q.flush()
	if q.m.snap.Offer == nil {
		t.Fatal(q.dump("expected incoming Challenge"))
	}
	q.press("n")
	p.flush()
	q.flush()
	if q.m.snap.Offer != nil {
		t.Fatal(q.dump("decline should clear the offer"))
	}
	if p.save().Wins+p.save().Losses+q.save().Wins+q.save().Losses != 0 {
		t.Fatal("declined Challenge must not record a Battle Result")
	}

	beforeP, beforeQ := battleLoadouts(p.save()), battleLoadouts(q.save())
	p.walkAdjacentToTrainer(q)
	p.press("c")
	q.flush()
	if q.m.snap.Offer == nil {
		t.Fatal(q.dump("expected Challenge after rematch"))
	}
	q.press("y")
	playPair(p, q, func() bool { return pvpFinished(p, q) })
	if p.save().Wins+q.save().Wins != 1 || p.save().Losses+q.save().Losses != 1 {
		t.Fatalf("Challenge record p=%d-%d q=%d-%d", p.save().Wins, p.save().Losses, q.save().Wins, q.save().Losses)
	}
	loadoutsUnchanged(t, beforeP, battleLoadouts(p.save()))
	loadoutsUnchanged(t, beforeQ, battleLoadouts(q.save()))
}
