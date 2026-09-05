package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"termon.sh/internal/content"
)

func loadOnboardSet(t *testing.T) *content.Set {
	t.Helper()
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestKeyHintPaintsGap(t *testing.T) {
	got := keyHint("enter", "continue")
	if !strings.Contains(got, "enter") || !strings.Contains(got, "continue") {
		t.Fatal("expected the key and label")
	}
	if !strings.Contains(got, "48;2;20;18;28") {
		t.Fatal("space between key and label should carry the screen background")
	}
}

func TestViewPaintsBackground(t *testing.T) {
	m := New("aaa", nil, nil, nil)
	m.width, m.height = 80, 24
	v := m.View()
	if v.BackgroundColor != screenBgRGB {
		t.Fatalf("background %v, want %v", v.BackgroundColor, screenBgRGB)
	}
	lines := strings.Split(strings.TrimRight(v.Content, "\n"), "\n")
	if len(lines) != 24 {
		t.Fatalf("view height %d, want 24", len(lines))
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w != 80 {
			t.Fatalf("line %d width %d, want 80", i, w)
		}
	}
	if !strings.Contains(v.Content, "48;2;20;18;28") {
		t.Fatal("view cells should carry the screen background")
	}
}

func TestTitleSubtitlePaintsCenteredPadding(t *testing.T) {
	got := titleSubtitle(40, titleTag, "")
	if lipgloss.Width(got) != 40 {
		t.Fatalf("subtitle width %d, want 40", lipgloss.Width(got))
	}
	if count := strings.Count(got, "48;2;20;18;28"); count < 3 {
		t.Fatalf("subtitle background spans = %d, want text and both padding spans", count)
	}
}

func TestTitleFrameHasPaintedOuterMargin(t *testing.T) {
	m := New("aaa", nil, nil, nil)
	m.width, m.height = 80, 24
	v := m.View().Content
	plain := strings.Split(ansi.Strip(v), "\n")
	if len(plain) != 24 {
		t.Fatalf("title height %d, want 24", len(plain))
	}
	if strings.Contains(plain[0], "╭") || !strings.Contains(plain[1], "╭") {
		t.Fatal("title frame should have a one-cell outer margin")
	}
	first, _, _ := strings.Cut(v, "\n")
	if !strings.Contains(first, "48;2;20;18;28") {
		t.Fatal("outer margin should carry the screen background")
	}
}

func TestMenuChoiceUsesPrimary(t *testing.T) {
	on := menuChoice(true, "FIGHT")
	off := menuChoice(false, "RUN")
	if !strings.Contains(on, "▶") || !strings.Contains(on, "38;2;110;231;240") {
		t.Fatal("selected choice should use the ice caret")
	}
	if strings.Contains(off, "▶") || strings.Contains(off, "38;2;110;231;240") {
		t.Fatal("idle choice should stay dim")
	}
	grid := menuGrid(2, 2, 16, []string{"1 PING", "2 COLD", "3 PIPE", "4 DEAD"})
	if !strings.Contains(grid, "3 PIPE") {
		t.Fatal("grid should keep the numbered labels")
	}
}

func TestChromeBodyPaintsPadding(t *testing.T) {
	out := chromeBody("", 12, 3)
	if !strings.Contains(out, "48;2;20;18;28") {
		t.Fatal("empty chrome rows should carry the screen background")
	}
}

func TestPageEdgeFitsWidth(t *testing.T) {
	line := pageEdge(40, "╭", "╮", titleStyle.Render("termon"), dimStyle.Render("Dojo"), selStyle.Render("oksomu"))
	if lipgloss.Width(line) != 40 {
		t.Fatalf("top edge width %d, want 40", lipgloss.Width(line))
	}
	bot := pageEdge(40, "╰", "╯", keyHint("q", "leave"), "", "")
	if lipgloss.Width(bot) != 40 {
		t.Fatalf("bottom edge width %d, want 40", lipgloss.Width(bot))
	}
}

func TestOnboardTalkBeforeName(t *testing.T) {
	o := newOnboard(loadOnboardSet(t))
	v := o.view(80, 24, nil)
	if !strings.Contains(v, "press any key") && !strings.Contains(typeOn("press any key", 0), "pr") {
		t.Fatal("title should invite a keypress")
	}
	next, done := o.update(press("enter"))
	if done || next.stage != stageTalk {
		t.Fatalf("title should open the talk, stage=%d done=%v", next.stage, done)
	}
	if !strings.Contains(next.view(80, 24, nil), "▌") {
		t.Fatal("talk should show a typing caret")
	}
	next.lineAge = 99
	v = next.view(80, 24, nil)
	if !strings.Contains(v, "I am Master Sable") {
		t.Fatal("Sable should introduce himself after the title")
	}
	assertSableOnDojoFloor(t, v)
	if strings.Contains(v, "▌") {
		t.Fatal("finished talk should drop the caret")
	}
	if strings.Contains(v, "identity check") {
		t.Fatal("name form should wait until the talk finishes")
	}
}

func TestTypingCaretFollowsText(t *testing.T) {
	mark := typeMark(false, true, 0)
	lines := msgLines("I am", mark, 40)
	got := ansi.Strip(lines[0])
	if !strings.Contains(got, "I am▌") {
		t.Fatalf("caret should sit after the typed text, got %q", got)
	}
}

func TestOnboardNameMenu(t *testing.T) {
	o := newOnboard(nil)
	o.stage = stageHandle
	o.lineAge = 99
	v := o.view(80, 24, nil)
	if !strings.Contains(v, "What is your name?") {
		t.Fatal("expected name prompt")
	}
	for _, item := range []string{"KEEP", "REROLL", "TYPE"} {
		if !strings.Contains(v, item) {
			t.Fatalf("missing %s", item)
		}
	}
	if strings.Contains(v, "▶KEEP") {
		t.Fatal("cursor should be ▶ KEEP, not flush against the label")
	}
	before := o.handle
	next, _ := o.update(press("r"))
	if next.handle == before {
		t.Fatal("r should reroll the handle")
	}
	next, _ = next.update(press("e"))
	if next.stage != stageHandleInput {
		t.Fatal("e should open type-in")
	}
	next.input = "red-trainer"
	next, _ = next.update(press("enter"))
	if next.stage != stageHandleOK || next.handle != "red-trainer" {
		t.Fatalf("typed handle should confirm, stage=%d handle=%q", next.stage, next.handle)
	}
	next.lineAge = 99
	v = next.view(80, 24, nil)
	if !strings.Contains(v, "RED-TRAINER") {
		t.Fatal("expected the spoken name confirm")
	}
	assertSableOnDojoFloor(t, v)
}

func TestStarterPickerUsesSpriteArt(t *testing.T) {
	set := loadOnboardSet(t)
	o := newOnboard(set)
	o.stage = stageStarter
	v := o.view(80, 40, set)
	if strings.Contains(v, "[___]") || strings.Contains(v, "/|###|") {
		t.Fatal("starter picker still uses prototype ASCII art")
	}
	if !strings.Contains(v, "▀") && !strings.Contains(v, "▄") {
		t.Fatal("expected half-block sprite art for starters")
	}
	if !strings.Contains(strings.ToUpper(v), "ROOTKIT") {
		t.Fatal("expected Rootkit from the content pack")
	}
	if !strings.Contains(strings.ToUpper(v), "ORGANIC") {
		t.Fatal("expected species type from the content pack")
	}
	if strings.Contains(strings.ToUpper(v), "EMBERBYTE") || strings.Contains(strings.ToUpper(v), "AQUABIT") {
		t.Fatal("picker should show one partner at a time")
	}
	if strings.Contains(v, "ATK") || strings.Contains(v, "SPA") {
		t.Fatal("starter pick should not show stat bars")
	}
	next, _ := o.update(press("right"))
	v = next.view(80, 40, set)
	if !strings.Contains(strings.ToUpper(v), "EMBERBYTE") {
		t.Fatal("right should cycle to Emberbyte")
	}
}

func TestOnboardConfirmThenJoin(t *testing.T) {
	set := loadOnboardSet(t)
	o := newOnboard(set)
	o.stage = stageStarter
	next, done := o.update(press("enter"))
	if done || next.stage != stageConfirm {
		t.Fatal("enter on a partner should ask to confirm")
	}
	v := next.view(80, 40, set)
	if !strings.Contains(v, "So, you want ROOTKIT?") {
		t.Fatal("expected yes/no confirm")
	}
	if !strings.Contains(v, "YES") || !strings.Contains(v, "NO") {
		t.Fatal("expected YES/NO")
	}
	next.cursor = 1
	next, done = next.update(press("enter"))
	if done || next.stage != stageStarter {
		t.Fatal("NO should return to the picker")
	}
	next, _ = next.update(press("enter"))
	next, done = next.update(press("enter"))
	if done || next.stage != stageJoined {
		t.Fatal("YES should assign the partner")
	}
	next.lineAge = 99
	if !strings.Contains(next.view(80, 40, set), "ROOTKIT joined") {
		t.Fatal("expected join line")
	}
	next, done = next.update(press("enter"))
	if done || next.stage != stageLesson {
		t.Fatal("join should open the how-to-fight lesson")
	}
}

func TestOnboardTalkAnswersWhatWhyHow(t *testing.T) {
	o := newOnboard(nil)
	o.stage = stageTalk
	o.lineAge = 99
	var heard []string
	for i := range talkPages {
		o.page = i
		o.lineAge = 99
		heard = append(heard, o.view(80, 24, nil))
	}
	all := strings.Join(heard, "\n")
	if !strings.Contains(all, "I am Master Sable") {
		t.Fatal("Sable should introduce himself on the first talk page")
	}
	assertSableOnDojoFloor(t, all)
	if !strings.Contains(all, "creatures you raise") {
		t.Fatal("first-run should say what a TERMON is")
	}
	if !strings.Contains(all, "Trainer") || !strings.Contains(all, "fight") {
		t.Fatal("first-run should say why you are here")
	}
	o.stage = stageLesson
	var lesson []string
	for i := range lessonPages {
		o.page = i
		o.lineAge = 99
		lesson = append(lesson, o.view(80, 24, nil))
	}
	how := strings.Join(lesson, "\n")
	assertSableOnDojoFloor(t, how)
	if !strings.Contains(how, "three partners") || !strings.Contains(how, "Capture Lessons") {
		t.Fatal("briefing should explain the two captures for a Full Party")
	}
	if !strings.Contains(how, "Other Trainers wait") {
		t.Fatal("briefing should say PvP waits on a Full Party")
	}
	if !strings.Contains(how, "Party and Moves") {
		t.Fatal("lesson should mention party management")
	}
	if !strings.Contains(how, "Capture Gauge") || !strings.Contains(how, "three different Moves") {
		t.Fatal("briefing should teach filling the Gauge with three Moves")
	}
	if strings.Contains(how, "practice") {
		t.Fatal("lesson should not promise a practice fight")
	}
	if strings.Contains(how, "…") || strings.Contains(how, "Gau...") {
		t.Fatalf("briefing should not cut words:\n%s", how)
	}
	o.page = len(lessonPages) - 1
	o.lineAge = 99
	_, done := o.update(press("enter"))
	if !done {
		t.Fatal("last lesson page should finish onboarding")
	}
}

func assertSableOnDojoFloor(t *testing.T, v string) {
	t.Helper()
	plain := ansi.Strip(v)
	if !strings.Contains(plain, "MASTER") {
		t.Fatal("expected Master Sable on the Dojo floor")
	}
	if !strings.Contains(plain, "╰─┬─╯") {
		t.Fatal("expected the practice gong beside Sable, not a blank arena")
	}
}
