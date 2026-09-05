package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"termon.sh/internal/server"
)

func TestOnboardLessonStartsCaptureLesson(t *testing.T) {
	h, set, _ := testHub(t)
	if _, err := h.Authenticate("aaa", "10.0.0.1"); err != nil {
		t.Fatal(err)
	}
	var inbox []any
	h.Attach("aaa", func(msg any) { inbox = append(inbox, msg) }, func() {})
	m := New("aaa", nil, set, h)
	m.width, m.height = 120, 40
	m.onboard.stage = stageLesson
	m.onboard.page = len(lessonPages) - 1
	m.onboard.lineAge = 99
	m.onboard.handle = "alpha"
	m.onboard.starter = 0
	next, cmd := m.Update(press("enter"))
	if cmd == nil {
		t.Fatal("last lesson page should persist the trainer")
	}
	out := cmd()
	msg, ok := out.(onboardedMsg)
	if !ok || msg.save == nil {
		t.Fatalf("expected onboardedMsg, got %T", out)
	}
	next, cmd = next.(Model).Update(msg)
	if cmd == nil {
		t.Fatal("Master Sable's briefing should start Capture Lesson 1")
	}
	if started := cmd(); started != nil {
		if errMsg, ok := started.(server.ErrorMsg); ok {
			t.Fatalf("required lesson: %s", errMsg.Text)
		}
	}
	m = next.(Model)
	if !m.tutorial {
		t.Fatal("required lessons should keep tutorial coaching")
	}
	sawBattle := false
	for _, item := range inbox {
		if _, ok := item.(server.BattleMsg); ok {
			sawBattle = true
		}
	}
	if !sawBattle {
		t.Fatal("required lesson should start a Capture Lesson battle")
	}
}

func TestResumePartialPartyStartsRequiredLesson(t *testing.T) {
	h, set, _ := testHub(t)
	if _, err := h.Authenticate("aaa", "10.0.0.1"); err != nil {
		t.Fatal(err)
	}
	sv, err := h.CompleteOnboard("aaa", "alpha", "rootkit")
	if err != nil {
		t.Fatal(err)
	}
	var inbox []any
	h.Attach("aaa", func(msg any) { inbox = append(inbox, msg) }, func() {})
	m := New("aaa", sv, set, h)
	m.width, m.height = 120, 40
	snap := h.Resume("aaa")
	next, cmd := m.Update(snap)
	if cmd == nil {
		t.Fatal("resume with a partial Party should start the next required Lesson")
	}
	if started := cmd(); started != nil {
		if errMsg, ok := started.(server.ErrorMsg); ok {
			t.Fatalf("required lesson: %s", errMsg.Text)
		}
	}
	m = next.(Model)
	if !m.tutorial {
		t.Fatal("required Lessons should keep Sable coaching")
	}
}

func TestTutorialCoachesFightMenu(t *testing.T) {
	m := battleModel(t, nil, 120, 40)
	m.tutorial = true
	m.battle.battleIntro = false
	m.battle.fightRoot = true
	m.wipeHold = 0
	v := m.renderBattle()
	if !strings.Contains(v, "Pick FIGHT") {
		t.Fatal("tutorial should coach FIGHT")
	}
	if !strings.Contains(v, "three different Moves") {
		t.Fatal("Sable should teach filling the Gauge with three Moves")
	}
	if strings.Contains(v, "RUN skips") {
		t.Fatal("tutorial should not tell them RUN skips the lesson")
	}
	if strings.Contains(v, "SWITCH") {
		t.Fatal("Lesson 1 should hide SWITCH")
	}
	footer := ansi.Strip(m.battleFooter())
	if !strings.Contains(footer, "Sable") || !strings.Contains(footer, "FIGHT") {
		t.Fatalf("fight footer should keep Sable's instructions:\n%s", footer)
	}
	m.battle.fightRoot = false
	footer = ansi.Strip(m.battleFooter())
	if !strings.Contains(footer, "different Move") {
		t.Fatalf("move grid should keep Sable's instructions:\n%s", footer)
	}
}

func TestRequiredLessonRunConfirmsBeforeForfeit(t *testing.T) {
	m := battleModel(t, nil, 120, 40)
	m.tutorial = true
	m.battle.fightRoot = true
	m.battle.battleIntro = false
	next, cmd := m.battleKey(press("f"))
	if cmd != nil {
		t.Fatal("first RUN should confirm, not forfeit")
	}
	m = next.(Model)
	if !m.battle.runConfirm {
		t.Fatal("RUN should arm confirm")
	}
	if !strings.Contains(m.renderBattle(), "Leave this Lesson") {
		t.Fatal("Sable should ask before leaving")
	}
	next, cmd = m.battleKey(press("esc"))
	if cmd != nil {
		t.Fatal("esc should cancel RUN")
	}
	if next.(Model).battle.runConfirm {
		t.Fatal("esc should clear confirm")
	}
}
