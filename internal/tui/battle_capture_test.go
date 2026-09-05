package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"termon.sh/internal/battle"
	"termon.sh/internal/capture"
	"termon.sh/internal/onboard"
	"termon.sh/internal/server"
)

func lessonCaptureState() server.CaptureStateMsg {
	return server.CaptureStateMsg{
		Gauge:  0,
		Status: "target only",
		Objectives: []server.CaptureObjectiveView{
			{ID: capture.ShowMoveVariety, Award: 30, Focused: true},
			{ID: capture.ReadTheMatchup, Award: 35},
			{ID: capture.HoldTheLine, Award: 35},
		},
	}
}

func TestCaptureBandNamesObjectivesAndFitsWidth(t *testing.T) {
	state := lessonCaptureState()
	for _, w := range []int{80, 100, 120} {
		got := renderCaptureBand(state, w)
		view := ansi.Strip(got)
		if lipgloss.Height(got) != lipgloss.Height(chromeBox(w, "a\nb")) {
			t.Fatalf("width %d capture band height = %d, want the action chrome height %d\n%s",
				w, lipgloss.Height(got), lipgloss.Height(chromeBox(w, "a\nb")), view)
		}
		if !strings.Contains(view, "┏") && !strings.Contains(view, "┌") {
			t.Fatalf("width %d capture band is not a chrome box:\n%s", w, view)
		}
		for _, want := range []string{"Gauge 0/100", "target only"} {
			if !strings.Contains(view, want) {
				t.Fatalf("width %d missing %q\n%s", w, want, view)
			}
		}
		for i, line := range strings.Split(view, "\n") {
			if got := ansi.StringWidth(line); got > w {
				t.Fatalf("width %d line %d is %d cells:\n%s", w, i, got, line)
			}
			onMeter := strings.Contains(line, "Gauge")
			onObj := strings.Contains(line, "Use 3") || strings.Contains(line, "Read the") || strings.Contains(line, "Hold the")
			if onMeter && onObj {
				t.Fatalf("width %d put an objective on the gauge line:\n%s", w, view)
			}
		}
	}
	view := ansi.Strip(renderCaptureBand(state, 100))
	if strings.Contains(view, "show_move_variety") {
		t.Fatalf("still uses objective IDs:\n%s", view)
	}
	for _, want := range []string{"Use 3 different Moves", "Land a super-effective Move", "Hold the line"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q\n%s", want, view)
		}
	}
}

func TestCaptureBandWaitsForAttackResult(t *testing.T) {
	set := loadSet(t)
	bt := liveBattle(t, set)
	m := battleModel(t, bt, 120, 40)
	m.battle.battleIntro = false
	m.wipeHold = 0
	m.battle.playSeen = len(bt.Events())
	next, _ := m.Update(lessonCaptureState())
	m = next.(Model)
	view := ansi.Strip(m.renderBattle())
	if !strings.Contains(view, "Gauge 0/100") {
		t.Fatalf("expected an empty Gauge, got:\n%s", view)
	}

	you, _ := bt.Fighter("aaa")
	foe, _ := bt.Fighter("bbb")
	commitTurn(t, bt, you.Moves[0], foe.Moves[0])

	progressed := lessonCaptureState()
	progressed.Gauge = 30
	progressed.Status = "Three different Moves."
	progressed.Objectives[0].Done = true
	next, _ = m.Update(progressed)
	m = next.(Model)
	view = ansi.Strip(m.renderBattle())
	if !strings.Contains(view, "Gauge 0/100") {
		t.Fatalf("Gauge filled before the attack played:\n%s", view)
	}
	if strings.Contains(view, "[x]") {
		t.Fatalf("objective checked before the attack played:\n%s", view)
	}

	next, _ = m.Update(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bravo", FoeHash: "bbb"})
	m = next.(Model)
	m.wipeHold = 0
	if !m.battle.playing {
		t.Fatal("expected playback of the resolved turn")
	}
	if !advancePlaybackUntil(&m, func(event battle.Event) bool {
		return event.Kind == battle.EventMoveUsed
	}) {
		t.Fatal("never reached the Move")
	}
	view = ansi.Strip(m.renderBattle())
	if !strings.Contains(view, "Gauge 0/100") {
		t.Fatalf("Gauge filled on Move-used, before the result:\n%s", view)
	}
	if !advancePlaybackUntil(&m, func(event battle.Event) bool {
		return event.Kind == battle.EventDamageDealt
	}) {
		t.Fatal("never reached damage")
	}
	view = ansi.Strip(m.renderBattle())
	if !strings.Contains(view, "Gauge 30/100") {
		t.Fatalf("Gauge should fill when damage prints:\n%s", view)
	}
	if !strings.Contains(view, "[x]") {
		t.Fatalf("objective should check when damage prints:\n%s", view)
	}
}

func TestBattleCaptureLayoutFitsFrame(t *testing.T) {
	for _, size := range [][2]int{{100, 32}, {120, 40}} {
		m := battleModel(t, nil, size[0], size[1])
		m.battle.captureOn = true
		m.battle.capture = lessonCaptureState()
		m.battle.battleIntro = true
		view := ansi.Strip(strings.TrimSuffix(m.renderBattle(), "\a"))
		lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
		if len(lines) > size[1] {
			t.Fatalf("%dx%d battle is %d rows, overflows the frame\n%s", size[0], size[1], len(lines), view)
		}
		for i, line := range lines {
			if got := ansi.StringWidth(line); got > size[0] {
				t.Fatalf("%dx%d row %d is %d cells:\n%s", size[0], size[1], i, got, line)
			}
		}
		for _, want := range []string{"Use 3 different Moves", "Land a super-effective Move", "Hold the line", "Go! ROOTKIT!", "● ROOTKIT"} {
			if !strings.Contains(view, want) {
				t.Fatalf("%dx%d missing %q\n%s", size[0], size[1], want, view)
			}
		}
	}
}

func TestSableCoachFitsFooter(t *testing.T) {
	m := battleModel(t, nil, 100, 32)
	m.tutorial = true
	m.battle.captureOn = true
	m.battle.capture = lessonCaptureState()
	m.battle.fightRoot = true
	m.battle.battleIntro = false
	m.wipeHold = 0
	footer := ansi.Strip(m.battleFooter())
	if strings.Contains(footer, "…") {
		t.Fatalf("Sable's footer should not cut words:\n%s", footer)
	}
	framed := ansi.Strip(chromeFrame(100, 32, "termon", "Lesson 1", "handle", "body", footer))
	if strings.Contains(framed, "…") {
		t.Fatalf("framed footer should not cut words:\n%s", framed)
	}
}

func TestCaptureMovePaneShowsSuperEffective(t *testing.T) {
	set := loadSet(t)
	you, err := onboard.DefaultLoadout(set, "rootkit")
	if err != nil {
		t.Fatal(err)
	}
	foe, err := onboard.DefaultLoadout(set, "mistcache")
	if err != nil {
		t.Fatal(err)
	}
	you.ID, foe.ID = "aaa-lead", "bbb-lead"
	bt, err := battle.New(set,
		battle.Party{Trainer: "aaa", Members: []battle.PartyMember{{Monster: you}}},
		battle.Party{Trainer: "bbb", Members: []battle.PartyMember{{Monster: foe}}},
		battle.Seeded(1),
	)
	if err != nil {
		t.Fatal(err)
	}
	m := battleModel(t, bt, 120, 40)
	m.battle.captureOn = true
	m.battle.battleIntro = false
	m.battle.fightRoot = false
	m.wipeHold = 0
	view := ansi.Strip(m.renderBattleMsg())
	if !strings.Contains(view, "2×") {
		t.Fatalf("TYPE pane should mark a super-effective Move:\n%s", view)
	}
}

func TestGaugeFullNarratesCaptureAndBlocksFight(t *testing.T) {
	m := battleModel(t, nil, 120, 40)
	m.tutorial = true
	m.battle.captureOn = true
	m.battle.capture.Gauge = 100
	m.battle.capture.Status = "Gauge is full."
	m.battle.battleIntro = false
	m.battle.fightRoot = true
	m.battle.playing = false
	m.wipeHold = 0
	m.battle.resultHold = 0
	view := ansi.Strip(m.renderBattle())
	if strings.Contains(view, "FIGHT") {
		t.Fatalf("full Gauge should end the match, not offer FIGHT:\n%s", view)
	}
	if !strings.Contains(view, "Gauge is full") {
		t.Fatalf("Sable should say the Gauge filled:\n%s", view)
	}
	next, cmd := m.battleKey(press("1"))
	if cmd != nil {
		t.Fatal("full Gauge should ignore Move keys")
	}
	m = next.(Model)
	next, cmd = m.battleKey(press("enter"))
	if cmd != nil {
		t.Fatal("first Enter should stay on the capture sequence")
	}
	m = next.(Model)
	view = ansi.Strip(m.renderBattle())
	if !strings.Contains(strings.ToUpper(view), "CAPTURE") {
		t.Fatalf("Sable should name the capture:\n%s", view)
	}
	if strings.Contains(view, "FIGHT") {
		t.Fatalf("capture sequence should not reopen FIGHT:\n%s", view)
	}
}
