package tui

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"termon.sh/internal/battle"
)

func TestHPPlatesPadFromBorder(t *testing.T) {
	you := youPlate(battle.Fighter{Name: "Aquabit", HP: 42, MaxHP: 42})
	if strings.Contains(you, "42/42┃") || strings.Contains(you, "42/42│") {
		t.Fatalf("player HP numbers sit on the border:\n%s", you)
	}
	foe := foePlate(battle.Fighter{Name: "Chippunk", HP: 30, MaxHP: 30})
	if strings.Contains(foe, "=┃") || strings.Contains(foe, "-┃") {
		t.Fatalf("foe HP bar sits on the border:\n%s", foe)
	}
}

func TestBattleViewRejectsTinyTerminal(t *testing.T) {
	m := battleModel(t, nil, 40, 20)
	v := m.renderBattle()
	if !strings.Contains(v, "too small") {
		t.Fatalf("expected min-size notice, got %q", v)
	}
}

func TestRosterSitsInActionChrome(t *testing.T) {
	m := battleModel(t, nil, 120, 40)
	m.battle.battleIntro = true
	msg := m.renderBattleMsg()
	view := ansi.Strip(msg)
	if lipgloss.Height(msg) != lipgloss.Height(chromeBox(120, "a\nb")) {
		t.Fatalf("action chrome height = %d, want 2 inner lines\n%s", lipgloss.Height(msg), view)
	}
	if !strings.Contains(view, "●") || !strings.Contains(view, "ROOTKIT") {
		t.Fatalf("roster missing from action chrome:\n%s", view)
	}
	if !strings.Contains(view, "Go! ROOTKIT!") {
		t.Fatalf("intro missing from action chrome:\n%s", view)
	}
}

func TestFightCommandsFitTwoLineChrome(t *testing.T) {
	m := battleModel(t, nil, 100, 32)
	m.battle.fightRoot = true
	m.battle.battleIntro = false
	msg := m.renderBattleMsg()
	view := ansi.Strip(msg)
	if lipgloss.Height(msg) != lipgloss.Height(chromeBox(100, "a\nb")) {
		t.Fatalf("fight chrome height = %d, cropped out of the 2-line box\n%s", lipgloss.Height(msg), view)
	}
	for _, want := range []string{"FIGHT", "RUN", "What will"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in fight chrome:\n%s", want, view)
		}
	}
	if strings.Contains(view, "SWITCH") {
		t.Fatalf("one-Monster fight should hide SWITCH:\n%s", view)
	}
}

func TestBattleViewShowsSpritesAndPlates(t *testing.T) {
	m := battleModel(t, nil, 120, 40)
	m.battle.battleIntro = true
	v := m.renderBattle()
	if !strings.Contains(v, "▀") && !strings.Contains(v, "▄") {
		t.Fatal("expected half-block sprite art")
	}
	if !strings.Contains(strings.ToUpper(v), "ROOTKIT") {
		t.Fatal("expected Rootkit plate")
	}
	if !strings.Contains(strings.ToUpper(v), "EMBERBYTE") {
		t.Fatal("expected Emberbyte plate")
	}
	if !strings.Contains(v, "HP:") {
		t.Fatal("expected HP plates")
	}
	if nums := regexp.MustCompile(`\d+/\s*\d+`).FindAllString(v, -1); len(nums) < 1 {
		t.Fatalf("expected HP numbers on player plate or strip, found %v", nums)
	}
	if strings.Contains(v, "THERMAL") || strings.Contains(v, "VIRUS") || strings.Contains(v, "ORGANIC") {
		t.Fatal("type should not leak onto HP plates")
	}
	next, _ := m.battleKey(press("enter"))
	m = next.(Model)
	v = m.renderBattle()
	if !strings.Contains(v, "FIGHT") || !strings.Contains(v, "RUN") {
		t.Fatal("expected FIGHT/RUN command menu")
	}
	if strings.Contains(v, "SWITCH") {
		t.Fatal("one-Monster fight should hide SWITCH")
	}
	if !strings.Contains(v, "What will") {
		t.Fatal("expected prompt window beside FIGHT/RUN")
	}
	next, _ = m.battleKey(press("enter"))
	m = next.(Model)
	v = m.renderBattle()
	if !strings.Contains(v, "ROOT ACCESS") {
		t.Fatal("expected move names in the fight menu")
	}
	if !strings.Contains(v, "TYPE/") {
		t.Fatal("expected TYPE pane for the selected move")
	}
	if strings.Contains(v, "100%") || strings.Contains(v, "PHY") {
		t.Fatal("fight menu should show type, not power/accuracy")
	}
}

func TestBattleCursorUsesApprovedGrid(t *testing.T) {
	m := battleModel(t, nil, 120, 40)
	m.battle.fightRoot = false
	next, _ := m.battleKey(press("right"))
	m = next.(Model)
	if m.battle.cursor != 1 {
		t.Fatalf("right: cursor %d, want 1", m.battle.cursor)
	}
	next, _ = m.battleKey(press("down"))
	m = next.(Model)
	if m.battle.cursor != 3 {
		t.Fatalf("down: cursor %d, want 3", m.battle.cursor)
	}
	next, _ = m.battleKey(press("left"))
	m = next.(Model)
	if m.battle.cursor != 2 {
		t.Fatalf("left: cursor %d, want 2", m.battle.cursor)
	}
}

func TestBattleChromeFitsWidth(t *testing.T) {
	m := battleModel(t, nil, 100, 32)
	m.battle.battleIntro = true
	m.battle.fightRoot = true
	for _, name := range []string{"intro", "fight", "moves"} {
		switch name {
		case "fight":
			m.battle.battleIntro = false
			m.battle.fightRoot = true
		case "moves":
			m.battle.battleIntro = false
			m.battle.fightRoot = false
		}
		v := m.renderBattle()
		for i, line := range strings.Split(v, "\n") {
			if w := lipgloss.Width(line); w > m.width {
				t.Fatalf("%s line %d width %d > %d", name, i, w, m.width)
			}
		}
	}
}

func TestActionBarAlignsWithPlayerPlate(t *testing.T) {
	m := battleModel(t, nil, 100, 32)
	m.battle.battleIntro = true
	plateW := lipgloss.Width(youPlate(battle.Fighter{Name: "Aquabit", HP: 42, MaxHP: 42}))
	msg, _, _ := strings.Cut(m.renderBattleMsg(), "\n")
	if w := lipgloss.Width(msg); w != m.width {
		t.Fatalf("action bar width %d, want %d", w, m.width)
	}
	arena := strings.Split(m.renderArena(m.height-4), "\n")
	var plateLine string
	for _, line := range arena {
		if strings.Contains(line, "ROOTKIT") || strings.Contains(line, "━") {
			plateLine = line
		}
	}
	if plateLine == "" {
		t.Fatal("expected player plate in the arena")
	}
	if w := lipgloss.Width(strings.TrimRight(plateLine, " ")); w != m.width {
		t.Fatalf("player plate right edge %d, want %d (plate %d)", lipgloss.Width(strings.TrimRight(plateLine, " ")), m.width, plateW)
	}
}

func TestPlateNameTruncatesByRunes(t *testing.T) {
	long := "パチリスパチリスパチリス" // 12 runes, 36 bytes
	got := plateName(battle.Fighter{Name: long})
	if !utf8.ValidString(got) {
		t.Fatalf("plateName produced invalid UTF-8: %q", got)
	}
	want := string([]rune(long)[:10])
	if got != want {
		t.Fatalf("plateName = %q, want first 10 runes %q", got, want)
	}
}
