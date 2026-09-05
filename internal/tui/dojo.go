package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"termon.sh/internal/capture"
	"termon.sh/internal/dojo"
	"termon.sh/internal/game"
	"termon.sh/internal/server"
)

type dojoView int

const (
	dojoViewMain dojoView = iota
	dojoViewSparringTiers
	dojoViewSparringPreview
	dojoViewDaily
)

type dojoMenuModel struct {
	menu    server.DojoMenuMsg
	view    dojoView
	cursor  int
	tier    string
	preview server.SparringPreviewMsg
}

type queueEditorModel struct {
	party  [3]string
	pool   []string
	cursor int
}

type progressionModel struct {
	msg    server.ProgressionMsg
	cursor int
}

func (m *Model) openDojoMenu(msg server.DojoMenuMsg) {
	m.dojo = dojoMenuModel{menu: msg, view: dojoViewMain}
	m.screen = screenDojoMenu
}

func (m *Model) openQueueEditor() error {
	if m.save == nil || m.set == nil {
		return errors.New("no save")
	}
	ed := queueEditorModel{party: m.save.Party}
	for _, monster := range m.save.Collection {
		ed.pool = append(ed.pool, monster.ID)
	}
	if len(ed.pool) == 0 {
		return errors.New("empty Collection")
	}
	m.queueEd = ed
	m.screen = screenQueueEditor
	return nil
}

func (m Model) dojoMenuKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()
	if m.dojo.view == dojoViewSparringPreview && key == "r" && !m.dojo.preview.FirstClear {
		tier := m.dojo.tier
		remix := m.dojo.preview.Remix + 1
		return m, func() tea.Msg {
			preview, err := m.hub.PreviewSparring(m.hash, tier, remix)
			if err != nil {
				return server.ErrorMsg{Text: server.UserMessage(err)}
			}
			return sparringPreviewMsg{preview: preview, tier: tier}
		}
	}
	switch key {
	case "esc", "q":
		switch m.dojo.view {
		case dojoViewMain:
			m.screen = screenLobby
			return m, nil
		default:
			m.dojo.view = dojoViewMain
			m.dojo.cursor = 0
			return m, nil
		}
	case "up", "k":
		m.dojo.cursor = (m.dojo.cursor - 1 + dojoItemCount(m.dojo)) % dojoItemCount(m.dojo)
	case "down", "j":
		m.dojo.cursor = (m.dojo.cursor + 1) % dojoItemCount(m.dojo)
	case "enter", " ":
		return m.dojoEnter()
	}
	return m, nil
}

func dojoItemCount(d dojoMenuModel) int {
	switch d.view {
	case dojoViewMain:
		return len(dojoMainItems(d.menu))
	case dojoViewSparringTiers:
		return 3
	case dojoViewSparringPreview, dojoViewDaily:
		return 1
	default:
		return 1
	}
}

type sparringPreviewMsg struct {
	preview server.SparringPreviewMsg
	tier    string
}

func (m Model) dojoEnter() (Model, tea.Cmd) {
	switch m.dojo.view {
	case dojoViewMain:
		items := dojoMainItems(m.dojo.menu)
		if m.dojo.cursor < 0 || m.dojo.cursor >= len(items) {
			return m, nil
		}
		switch items[m.dojo.cursor].action {
		case dojoActionLesson1:
			return m, m.hubCmd(func() error { return m.hub.StartLesson(m.hash, 1) })
		case dojoActionLesson2:
			return m, m.hubCmd(func() error { return m.hub.StartLesson(m.hash, 2) })
		case dojoActionSparring:
			m.dojo.view = dojoViewSparringTiers
			m.dojo.cursor = 0
			return m, nil
		case dojoActionDaily:
			m.dojo.view = dojoViewDaily
			m.dojo.cursor = 0
			return m, nil
		}
	case dojoViewSparringTiers:
		tiers := []string{dojo.TierApprentice, dojo.TierRival, dojo.TierMaster}
		tier := tiers[m.dojo.cursor]
		return m, func() tea.Msg {
			preview, err := m.hub.PreviewSparring(m.hash, tier, 0)
			if err != nil {
				return server.ErrorMsg{Text: server.UserMessage(err)}
			}
			return sparringPreviewMsg{preview: preview, tier: tier}
		}
	case dojoViewSparringPreview:
		tier := m.dojo.tier
		remix := m.dojo.preview.Remix
		return m, m.hubCmd(func() error { return m.hub.StartSparring(m.hash, tier, remix) })
	case dojoViewDaily:
		return m, m.hubCmd(func() error { return m.hub.StartDaily(m.hash) })
	}
	return m, nil
}

func (m Model) queueEditorKey(key string) (Model, tea.Cmd) {
	switch key {
	case "esc":
		m.screen = screenLobby
		return m, nil
	case "p":
		m.openWorkbench()
		return m, nil
	case "enter", " ":
		party := m.queueEd.party
		return m, m.hubCmd(func() error {
			if err := m.hub.SetQueueParty(m.hash, party); err != nil {
				return err
			}
			_, _, err := m.hub.FindBattle(m.hash)
			return err
		})
	case "up", "k":
		m.queueEd.cursor = max(0, m.queueEd.cursor-1)
	case "down", "j":
		m.queueEd.cursor = min(len(m.queueEd.pool)-1, m.queueEd.cursor+1)
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '3' &&
			m.queueEd.cursor >= 0 && m.queueEd.cursor < len(m.queueEd.pool) {
			m.queueEd.party = assignPartySlot(m.queueEd.party, int(key[0]-'1'), m.queueEd.pool[m.queueEd.cursor])
		}
	}
	return m, nil
}

func assignPartySlot(party [3]string, target int, monsterID string) [3]string {
	if target < 0 || target >= len(party) || monsterID == "" || party[target] == monsterID {
		return party
	}
	for i, id := range party {
		if id == monsterID {
			party[i], party[target] = party[target], party[i]
			return party
		}
	}
	party[target] = monsterID
	return party
}

func (m Model) progressionKey(key string) (Model, tea.Cmd) {
	switch key {
	case "enter", " ", "esc":
		if m.openPendingExpedition() {
			return m, nil
		}
		if m.progression.cursor == 0 || len(m.progression.msg.Entries) == 0 {
			if n := nextRequiredLesson(m.save); n != 0 {
				return m.startNextRequiredLesson()
			}
			m.tutorial = false
			m.screen = screenLobby
			return m, nil
		}
		m.openWorkbench()
		return m, nil
	case "r":
		m.openWorkbench()
		return m, nil
	}
	return m, nil
}

func nextRequiredLesson(save *game.Save) int {
	if save == nil || game.FullParty(save) {
		return 0
	}
	if save.Party[1] == "" {
		return 1
	}
	return 2
}

func (m Model) startNextRequiredLesson() (Model, tea.Cmd) {
	n := nextRequiredLesson(m.save)
	if n == 0 {
		m.tutorial = false
		return m, nil
	}
	m.tutorial = true
	return m, m.hubCmd(func() error { return m.hub.StartRequiredLesson(m.hash, n) })
}

func renderDojoMenu(menu server.DojoMenuMsg, d dojoMenuModel) string {
	switch d.view {
	case dojoViewSparringTiers:
		return renderDojoSparringTiers(menu, d)
	case dojoViewSparringPreview:
		return renderDojoSparringPreview(d.preview)
	case dojoViewDaily:
		return renderDojoDaily(menu)
	default:
		return renderDojoMain(menu, d.cursor)
	}
}

func renderDojoMain(menu server.DojoMenuMsg, cursor int) string {
	lines := []string{
		titleStyle.Render("DOJO MASTER"),
		dimStyle.Render("Server Day " + menu.ServerDay),
		"",
	}
	for i, item := range dojoMainItems(menu) {
		lines = append(lines, dojoRow(item.label, item.done, cursor == i))
	}
	lines = append(lines, "", dimStyle.Render("↑↓ select · Enter · Esc back"))
	if menu.Hint != "" {
		lines = append(lines, "", promptStyle.Render(menu.Hint))
	}
	return strings.Join(lines, "\n")
}

type dojoMainAction int

const (
	dojoActionLesson1 dojoMainAction = iota
	dojoActionLesson2
	dojoActionSparring
	dojoActionDaily
)

type dojoMainItem struct {
	label  string
	done   bool
	action dojoMainAction
}

func dojoMainItems(menu server.DojoMenuMsg) []dojoMainItem {
	var items []dojoMainItem
	if !menu.Lesson1Done || !menu.Lesson2Done {
		items = append(items,
			dojoMainItem{label: "Lesson 1: Capture basics", done: menu.Lesson1Done, action: dojoActionLesson1},
			dojoMainItem{label: "Lesson 2: Switch and capture", done: menu.Lesson2Done, action: dojoActionLesson2},
		)
	}
	return append(items,
		dojoMainItem{label: "Sparring: tiered practice", action: dojoActionSparring},
		dojoMainItem{label: "Daily Challenge: " + menu.Daily.ID, done: menu.Daily.Mastery, action: dojoActionDaily},
	)
}

func renderDojoSparringTiers(menu server.DojoMenuMsg, d dojoMenuModel) string {
	clears := []bool{menu.SparringApprenticeClear, menu.SparringRivalClear, menu.SparringMasterClear}
	xps := []int64{65, 90, 130}
	labels := []string{"Apprentice", "Rival", "Master"}
	var lines []string
	lines = append(lines, titleStyle.Render("SPARRING"))
	for i, label := range labels {
		tag := fmt.Sprintf("first clear +%d XP", xps[i])
		if clears[i] {
			tag = "cleared today"
		}
		lines = append(lines, dojoRow(label+": "+tag, clears[i], d.cursor == i))
	}
	lines = append(lines, "", dimStyle.Render("Enter preview · Esc back"))
	return strings.Join(lines, "\n")
}

func renderDojoSparringPreview(p server.SparringPreviewMsg) string {
	var lines []string
	lines = append(lines, titleStyle.Render("SPARRING PREVIEW"))
	roster := "daily roster"
	if p.Remix > 0 {
		roster = fmt.Sprintf("practice remix %d", p.Remix)
	}
	lines = append(lines, fmt.Sprintf("Tier: %s  Day: %s  %s", p.Tier, p.ServerDay, roster))
	if p.FirstClear {
		lines = append(lines, fmt.Sprintf("First clear: +%d XP", p.XP))
	} else {
		lines = append(lines, dimStyle.Render("Already cleared today. Remixes are practice only"))
	}
	lines = append(lines, "", p.PolicySummary, dimStyle.Render("Daily roster is shared across tiers"))
	for _, s := range p.Slots {
		lines = append(lines, fmt.Sprintf("  Slot %d %s L%d %s (%s)", s.Slot, s.Species, s.Level, s.Type, s.Role))
	}
	footer := "Enter confirm · Esc back"
	if !p.FirstClear {
		footer = "R remix · Enter confirm · Esc back"
	}
	lines = append(lines, "", dimStyle.Render(footer))
	return strings.Join(lines, "\n")
}

func renderDojoDaily(menu server.DojoMenuMsg) string {
	d := menu.Daily
	var lines []string
	lines = append(lines, titleStyle.Render("DAILY CHALLENGE"))
	lines = append(lines, fmt.Sprintf("%s · objective %s · par %d turns", d.ID, d.Objective, d.Par))
	if d.FirstClear {
		lines = append(lines, "First clear: +180 XP")
	} else {
		lines = append(lines, dimStyle.Render("Objective XP already earned today"))
	}
	if d.Mastery {
		lines = append(lines, dimStyle.Render("Mastery Mark earned"))
	} else {
		lines = append(lines, "Par mastery available")
	}
	lines = append(lines, "", dimStyle.Render("Enter start · Esc back"))
	return strings.Join(lines, "\n")
}

func dojoRow(label string, done, focused bool) string {
	mark := "[ ]"
	if done {
		mark = "[x]"
	}
	line := mark + " " + label
	if focused {
		return selectedStyle.Render(line)
	}
	return line
}

func renderQueueEditor(ed queueEditorModel, save *game.Save) string {
	var party, roster []string
	for i := range ed.party {
		party = append(party, fmt.Sprintf("%d. %s", i+1, partyMonsterName(save, ed.party[i])))
	}
	for i, id := range ed.pool {
		monster, ok := game.MonsterByID(save, id)
		if !ok {
			continue
		}
		readiness := "ready"
		if len(monster.BattleLoadout) == 0 {
			readiness = "needs loadout"
		}
		line := fmt.Sprintf("%-18s Lv%-2d  %s", partyMonsterName(save, id), monster.Level, readiness)
		if i == ed.cursor {
			line = selectedStyle.Render(line)
		}
		roster = append(roster, line)
	}
	return strings.Join([]string{
		titleStyle.Render("FIND BATTLE"),
		dimStyle.Render("Choose your three-Monster roster and opening order."), "",
		"Battle Party", strings.Join(party, "\n"), "",
		"Collection", strings.Join(roster, "\n"), "",
		dimStyle.Render("↑/↓ focus · 1-3 assign slot · P edit loadouts · Enter queue · Esc cancel"),
	}, "\n")
}

func partyMonsterName(save *game.Save, monsterID string) string {
	if save == nil || monsterID == "" {
		return "empty"
	}
	monster, ok := game.MonsterByID(save, monsterID)
	if !ok {
		return "?"
	}
	if monster.Nickname != "" {
		return monster.Nickname
	}
	return monster.Species
}

func renderProgressionSummary(msg server.ProgressionMsg, nextLesson int) string {
	var lines []string
	lines = append(lines, titleStyle.Render("PROGRESSION SUMMARY"))
	for _, e := range msg.Entries {
		share := ""
		if e.Share != "" {
			share = " (" + e.Share + " share)"
		}
		lines = append(lines, fmt.Sprintf("Slot %d %s  +%d XP  Lv%d%s", e.Slot, e.Name, e.XPGained, e.Level, share))
		for _, mv := range e.Unlocked {
			lines = append(lines, dimStyle.Render("  unlocked "+mv))
		}
		if e.EvolutionPending {
			lines = append(lines, accentStyle.Render("  evolution ready"))
		}
	}
	if len(msg.Entries) == 0 {
		lines = append(lines, dimStyle.Render("No party progression this time."))
	}
	footer := "Enter return · R review in Workbench"
	if nextLesson != 0 {
		footer = fmt.Sprintf("Enter starts Lesson %d · R review in Workbench", nextLesson)
	}
	lines = append(lines, "", dimStyle.Render(footer))
	return strings.Join(lines, "\n")
}

func renderCaptureBand(state server.CaptureStateMsg, width int) string {
	if width < 1 {
		width = 1
	}
	inner := chromeInner(width)
	line1 := renderCaptureGaugeLine(state, inner)
	line2 := fitLine(strings.Join(captureObjectiveRows(state), blank(2)), inner)
	return chromeBox(width, line1+"\n"+line2)
}

func renderCaptureGaugeLine(state server.CaptureStateMsg, width int) string {
	label := dimStyle.Render(fmt.Sprintf("Gauge %d/100", state.Gauge))
	status := dimStyle.Render(state.Status)
	for barW := min(28, max(8, width/5)); barW >= 8; barW-- {
		line := label + blank(2) + hpMeter(state.Gauge, 100, barW) + blank(2) + status
		if lipgloss.Width(line) <= width {
			return line
		}
	}
	return label + blank(2) + status
}

func captureObjectiveRows(state server.CaptureStateMsg) []string {
	rows := make([]string, 0, len(state.Objectives))
	for _, o := range state.Objectives {
		mark := "[ ]"
		if o.Done {
			mark = "[x]"
		}
		name := string(o.ID)
		if cat, ok := capture.ObjectiveByID(o.ID); ok && cat.DisplayName != "" {
			name = cat.DisplayName
		}
		line := fmt.Sprintf("%s +%d  %s", mark, o.Award, name)
		if o.Done {
			line = dimStyle.Render(line)
		} else {
			line = narrStyle.Render(line)
		}
		rows = append(rows, line)
	}
	return rows
}

func fitBlock(s string, width int) string {
	var out []string
	for l := range strings.SplitSeq(s, "\n") {
		out = append(out, fitLine(l, width))
	}
	return strings.Join(out, "\n")
}

var (
	bodyStyle     = lipgloss.NewStyle().Padding(1, 2)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e4ad48"))
	accentStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#70a957"))
)
