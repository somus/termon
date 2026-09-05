package tui

import (
	"fmt"
	"math/rand/v2"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"termon.sh/internal/content"
	"termon.sh/internal/lobby"
	"termon.sh/internal/onboard"
	"termon.sh/internal/sprite"
)

type onboardStage int

const (
	stageWelcome onboardStage = iota
	stageTalk
	stageHandle
	stageHandleInput
	stageHandleOK
	stageStarter
	stageConfirm
	stageJoined
	stageLesson
)

type onboardModel struct {
	stage   onboardStage
	page    int
	handle  string
	input   string
	starter int
	cursor  int
	age     int
	lineAge int
	anims   [3]sprite.Anim
}

var (
	adjectives = []string{"polite", "brave", "quiet", "swift", "cosmic", "grumpy", "lucky", "clever"}
	nouns      = []string{"otter", "gecko", "heron", "mole", "ferret", "panda", "raven", "tapir"}
	talkPages  = []string{
		"I am Master Sable. This Dojo is my hall.",
		"TERMON are creatures you raise and battle over SSH.",
		"You are a Trainer. Pick a partner, learn their Moves, fight others.",
		"First, who are you?",
	}
	lessonPages = []string{
		"A Trainer fights with three partners. You have one.",
		"Two Capture Lessons fill your Party. Other Trainers wait until then. Press p later for Party and Moves.",
		"Fill the Capture Gauge to 100. Use three different Moves. 2× on the TYPE pane is super-effective.",
		"A KO with a short Gauge fails - you retry. Let's begin.",
	}
	nameChoices = []string{"KEEP", "REROLL", "TYPE"}
	titleTag    = "the terminal is the arena"
)

func randomHandle() string {
	return fmt.Sprintf("%s-%s-%d", adjectives[rand.IntN(len(adjectives))], nouns[rand.IntN(len(nouns))], 10+rand.IntN(90)) //nolint:gosec // cosmetic suggestion, not an identity secret
}

func newOnboard(set *content.Set) onboardModel {
	o := onboardModel{handle: randomHandle()}
	if set == nil {
		return o
	}
	for i, slug := range onboard.StarterSlugs {
		art, ok := set.Arts[slug]
		if !ok {
			continue
		}
		typ := ""
		if sp, ok := set.Species[slug]; ok {
			typ = sp.Type
		}
		o.anims[i] = sprite.CompileOn(art, typ, true, screenBgHex)
	}
	return o
}

func (o onboardModel) advance() onboardModel {
	o.lineAge = 0
	o.cursor = 0
	return o
}

func (o onboardModel) lineReady(text string) bool {
	return typeOn(text, o.lineAge) == text
}

func (o onboardModel) update(msg tea.KeyMsg) (onboardModel, bool) {
	key := msg.String()
	switch o.stage {
	case stageWelcome:
		o.stage = stageTalk
		return o.advance(), false
	case stageTalk:
		if key != "enter" && key != " " {
			return o, false
		}
		if !o.lineReady(talkPages[o.page]) {
			return o, false
		}
		if o.page+1 < len(talkPages) {
			o.page++
			return o.advance(), false
		}
		o.stage = stageHandle
		return o.advance(), false
	case stageHandle:
		switch key {
		case "r":
			o.handle = randomHandle()
		case "e":
			o.input = ""
			o.stage = stageHandleInput
			return o.advance(), false
		case "left", "h":
			o.cursor = (o.cursor + len(nameChoices) - 1) % len(nameChoices)
		case "right", "l":
			o.cursor = (o.cursor + 1) % len(nameChoices)
		case "enter", " ":
			switch o.cursor {
			case 1:
				o.handle = randomHandle()
			case 2:
				o.input = ""
				o.stage = stageHandleInput
				return o.advance(), false
			default:
				o.stage = stageHandleOK
				return o.advance(), false
			}
		}
	case stageHandleInput:
		switch key {
		case "enter":
			if onboard.ValidHandle(o.input) {
				o.handle = strings.TrimSpace(o.input)
				o.stage = stageHandleOK
				return o.advance(), false
			}
		case "backspace":
			if r := []rune(o.input); len(r) > 0 {
				o.input = string(r[:len(r)-1])
			}
		case "esc":
			o.stage = stageHandle
			return o.advance(), false
		default:
			r := []rune(key)
			if len(r) == 1 && len([]rune(o.input)) < 16 {
				o.input += key
			}
		}
	case stageHandleOK:
		if (key == "enter" || key == " ") && o.lineReady(o.handleOKText()) {
			o.stage = stageStarter
			return o.advance(), false
		}
	case stageStarter:
		switch key {
		case "left", "h":
			o.starter = (o.starter + 2) % 3
		case "right", "l":
			o.starter = (o.starter + 1) % 3
		case "1", "2", "3":
			o.starter = int(key[0] - '1')
		case "enter", " ":
			o.stage = stageConfirm
			return o.advance(), false
		}
	case stageConfirm:
		switch key {
		case "left", "h", "up", "k", "right", "l", "down", "j":
			o.cursor = 1 - o.cursor
		case "n", "esc", "backspace":
			o.stage = stageStarter
			return o.advance(), false
		case "enter", " ":
			if o.cursor == 1 {
				o.stage = stageStarter
				return o.advance(), false
			}
			o.stage = stageJoined
			return o.advance(), false
		}
	case stageJoined:
		if (key == "enter" || key == " ") && o.lineReady(o.joinedText(nil)) {
			o.stage = stageLesson
			o.page = 0
			return o.advance(), false
		}
	case stageLesson:
		if key != "enter" && key != " " {
			return o, false
		}
		if !o.lineReady(lessonPages[o.page]) {
			return o, false
		}
		if o.page+1 < len(lessonPages) {
			o.page++
			return o.advance(), false
		}
		return o, true
	}
	return o, false
}

func (o onboardModel) talkText() string {
	switch o.stage {
	case stageLesson:
		if o.page >= 0 && o.page < len(lessonPages) {
			return lessonPages[o.page]
		}
	default:
		if o.page >= 0 && o.page < len(talkPages) {
			return talkPages[o.page]
		}
	}
	return ""
}

func (o onboardModel) handleOKText() string {
	return "Right! So you are " + strings.ToUpper(o.handle) + "!"
}

func (o onboardModel) joinedText(set *content.Set) string {
	name, _, _ := o.starterInfo(set)
	return strings.ToUpper(name) + " joined " + strings.ToUpper(o.handle) + "!"
}

func (o onboardModel) confirmText(set *content.Set) string {
	name, _, _ := o.starterInfo(set)
	return "So, you want " + strings.ToUpper(name) + "?"
}

func (o onboardModel) starterInfo(set *content.Set) (name, typ, flavor string) {
	slug := onboard.StarterSlugs[o.starter]
	name = slug
	if set == nil {
		return name, typ, flavor
	}
	if sp, ok := set.Species[slug]; ok {
		name, typ, flavor = sp.Name, sp.Type, sp.Flavor
		if td, ok := set.Types[sp.Type]; ok && td.Name != "" {
			typ = td.Name
		}
	}
	return name, typ, flavor
}

func (o onboardModel) view(w, h int, set *content.Set) string {
	if w < 1 {
		w = 80
	}
	if h < 1 {
		h = 24
	}
	if o.stage == stageWelcome {
		return o.titleView(w, h)
	}
	arenaH := max(6, h-4)
	return o.arena(w, arenaH) + "\n" + o.box(w, set)
}

func (o onboardModel) titleView(w, h int) string {
	mark := fancyBanner()
	markW := 0
	for line := range strings.SplitSeq(mark, "\n") {
		if lw := lipgloss.Width(line); lw > markW {
			markW = lw
		}
	}
	typed := typeOn(titleTag, o.lineAge)
	sub := titleSubtitle(markW, typed, typeMark(false, !o.lineReady(titleTag), o.age))
	press := "press any key"
	if o.lineReady(titleTag) && (o.age/5)%2 == 0 {
		press += "  ▼"
	}
	innerW, innerH := max(1, w-2), max(1, h-2)
	body := mark + "\n\n" + sub + "\n\n" + promptStyle.Render(press)
	return pageFrame(w, h, "", "", "", place(innerW, innerH, body), keyHint("q", "quit"))
}

func titleSubtitle(w int, text, mark string) string {
	return lipgloss.PlaceHorizontal(
		w,
		lipgloss.Center,
		narrStyle.Render(text)+mark,
		lipgloss.WithWhitespaceStyle(fillStyle),
	)
}

func fancyBanner() string {
	raw := strings.Split(strings.Trim(banner, "\n"), "\n")
	hi := ink(primaryHex).Bold(true)
	lo := ink(primaryLoHex)
	out := make([]string, len(raw))
	for i, line := range raw {
		if i%2 == 0 {
			out[i] = hi.Render(line)
		} else {
			out[i] = lo.Render(line)
		}
	}
	return strings.Join(out, "\n")
}

func (o onboardModel) arena(w, h int) string {
	switch o.stage {
	case stageHandle:
		body := titleStyle.Render("What is your name?") + "\n\n" + selStyle.Render(o.handle)
		return place(w, h, body)
	case stageHandleInput:
		return place(w, h, titleStyle.Render("What is your name?"))
	case stageTalk, stageHandleOK, stageLesson:
		return renderDojoBackdrop(w, h, lobby.MasterX, lobby.MasterY)
	case stageStarter, stageConfirm, stageJoined:
		return place(w, h, o.starterSprite())
	default:
		return place(w, h, "")
	}
}

func (o onboardModel) starterSprite() string {
	pose := sprite.PoseIdleA
	if (o.age/6)%2 == 1 {
		pose = sprite.PoseIdleB
	}
	lines := o.anims[o.starter].Frames[pose]
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func (o onboardModel) box(w int, set *content.Set) string {
	switch o.stage {
	case stageTalk, stageLesson:
		text := o.talkText()
		typed := typeOn(text, o.lineAge)
		ready := o.lineReady(text)
		return dialogBox(w, typed, typeMark(ready, !ready, o.age))
	case stageHandle:
		return chromeBox(w, menuRow(o.cursor, nameChoices...)+"\n")
	case stageHandleInput:
		return chromeBox(w, promptStyle.Render("> "+o.input)+typeMark(false, true, o.age)+"\n")
	case stageHandleOK:
		text := o.handleOKText()
		typed := typeOn(text, o.lineAge)
		ready := o.lineReady(text)
		return dialogBox(w, typed, typeMark(ready, !ready, o.age))
	case stageStarter:
		name, typ, flavor := o.starterInfo(set)
		inner := chromeInner(w)
		head := selStyle.Render(strings.ToUpper(name)) + narrStyle.Render(", the ") +
			typeInk(typ).Render(strings.ToUpper(typ)) + narrStyle.Render(" species.")
		blurb := dimStyle.Render(fitLine(flavor, inner))
		return chromeBox(w, head+"\n"+blurb)
	case stageConfirm:
		left := dialogBox(max(1, w-cmdOuterW), o.confirmText(set), "")
		return lipgloss.JoinHorizontal(lipgloss.Top, left, yesNoPane(o.cursor))
	case stageJoined:
		text := o.joinedText(set)
		typed := typeOn(text, o.lineAge)
		ready := o.lineReady(text)
		return dialogBox(w, typed, typeMark(ready, !ready, o.age))
	default:
		return dialogBox(w, "", "")
	}
}

func dialogBox(width int, text, mark string) string {
	return chromeBox(width, strings.Join(msgLines(text, mark, chromeInner(width)), "\n"))
}

func yesNoPane(cursor int) string {
	return chromeBox(cmdOuterW, menuCol(cursor, "YES", "NO"))
}

func wrapWords(s string, width int) string {
	if width < 1 || s == "" {
		return s
	}
	words := strings.Fields(s)
	var lines []string
	var cur string
	for _, word := range words {
		next := word
		if cur != "" {
			next = cur + " " + word
		}
		if lipgloss.Width(next) <= width {
			cur = next
			continue
		}
		if cur != "" {
			lines = append(lines, cur)
		}
		cur = word
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return strings.Join(lines, "\n")
}

const banner = `
 ████████╗███████╗██████╗ ███╗   ███╗ ██████╗ ███╗   ██╗
 ╚══██╔══╝██╔════╝██╔══██╗████╗ ████║██╔═══██╗████╗  ██║
    ██║   █████╗  ██████╔╝██╔████╔██║██║   ██║██╔██╗ ██║
    ██║   ██╔══╝  ██╔══██╗██║╚██╔╝██║██║   ██║██║╚██╗██║
    ██║   ███████╗██║  ██║██║ ╚═╝ ██║╚██████╔╝██║ ╚████║
    ╚═╝   ╚══════╝╚═╝  ╚═╝╚═╝     ╚═╝ ╚═════╝ ╚═╝  ╚═══╝`
