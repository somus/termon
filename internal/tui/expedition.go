package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"termon.sh/internal/server"
	"termon.sh/internal/sprite"
)

const (
	expCardW   = 30
	expCardGap = 2
)

type signalBoardModel struct {
	board server.SignalBoardMsg
	card  int
	armed bool
}

type expeditionFlowModel struct {
	msg server.ExpeditionMsg
}

func (m *Model) openSignalBoard(msg server.SignalBoardMsg) {
	m.signalBoard = signalBoardModel{board: msg, card: 0}
	m.screen = screenSignalBoard
}

func (m *Model) openExpeditionFlow(msg server.ExpeditionMsg) {
	m.expeditionFlow = expeditionFlowModel{msg: msg}
	m.screen = screenExpedition
	result := m.battle.update(battleDeactivateCaptureInput{}, m.battleContext())
	m.battle = result.model
}

func (m Model) signalBoardKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc", "q":
		m.screen = screenLobby
		m.signalBoard.armed = false
		return m, nil
	case "left", "h":
		m.signalBoard.card = (m.signalBoard.card + 2) % 3
	case "right", "l":
		m.signalBoard.card = (m.signalBoard.card + 1) % 3
	case "1", "2", "3":
		m.signalBoard.card = int(key[0] - '1')
	case "enter", " ":
		if !m.signalBoard.armed {
			m.signalBoard.armed = true
			return m, nil
		}
		family := m.signalBoard.board.Families[m.signalBoard.card].Slug
		return m, m.hubCmd(func() error { return m.hub.LaunchExpedition(m.hash, family) })
	case "x":
		return m, m.hubCmd(func() error { return m.hub.AbandonExpedition(m.hash) })
	default:
		if m.signalBoard.armed && key == "b" {
			m.signalBoard.armed = false
		}
	}
	return m, nil
}

func (m Model) expeditionFlowKey(key string) (Model, tea.Cmd) {
	msg := m.expeditionFlow.msg
	switch msg.Phase {
	case "recovery":
		switch key {
		case "esc", "x":
			return m, m.hubCmd(func() error { return m.hub.AbandonExpedition(m.hash) })
		case "enter", " ":
			return m, m.hubCmd(func() error { return m.hub.ContinueExpedition(m.hash) })
		}
	case "captured":
		switch key {
		case "enter", " ", "esc":
			m.screen = screenLobby
			return m, nil
		case "p":
			m.openWorkbench()
			return m, nil
		}
	case "hunt_failed", "abandoned", "failed":
		if key == "enter" || key == " " || key == "esc" {
			m.screen = screenLobby
			return m, nil
		}
	}
	return m, nil
}

func (m Model) renderSignalBoard() string {
	if m.width < minBattleWidth || m.height < minBattleHeight {
		return fmt.Sprintf("terminal too small: need ≥%d×%d, have %dx%d",
			minBattleWidth, minBattleHeight, m.width, m.height)
	}
	chrome := m.signalBoardChrome()
	chromeH := lipgloss.Height(chrome)
	arenaH := max(6, m.height-chromeH)
	body := centerBlock(m.width, arenaH, m.signalBoardCards(m.width))
	return body + "\n" + chrome
}

func (m Model) renderExpeditionFlow() string {
	if m.width < minBattleWidth || m.height < minBattleHeight {
		return fmt.Sprintf("terminal too small: need ≥%d×%d, have %dx%d",
			minBattleWidth, minBattleHeight, m.width, m.height)
	}
	chrome := m.expeditionFlowChrome()
	chromeH := lipgloss.Height(chrome)
	arenaH := max(6, m.height-chromeH)
	var arena string
	if m.expeditionFlow.msg.Phase == "recovery" || m.expeditionFlow.msg.Phase == "abandoned" {
		arena = centerBlock(m.width, arenaH, m.signalBoardCards(m.width))
	} else if m.expeditionFlow.msg.Phase == "captured" && m.set != nil {
		arena = m.battle.renderCapturedMonster(
			m.expeditionFlow.msg.CapturedSpecies,
			arenaH,
			m.battleContext(),
		)
	} else {
		arena = centerBlock(m.width, arenaH, dimStyle.Render("Route ended."))
	}
	return arena + "\n" + chrome
}

func (m Model) signalBoardChrome() string {
	inner := chromeInner(m.width)
	line := func(s string) string { return narrStyle.Render(fitLine(s, inner)) }
	board := m.signalBoard.board
	if m.signalBoard.armed {
		f := board.Families[m.signalBoard.card]
		return chromeBox(m.width, line(fmt.Sprintf("Launch %s. Party ready. Prep has no capture.", strings.ToUpper(f.Name)))+"\n"+
			expMenuRow(0, "START", "BACK", "ABANDON"))
	}
	names := make([]string, 3)
	for i, f := range board.Families {
		names[i] = strings.ToUpper(f.Name)
	}
	head := fmt.Sprintf("Today's Families · day %d of 8 · every Family once per cycle", board.DayIndex)
	return chromeBox(m.width, line(head)+"\n"+expMenuRow(m.signalBoard.card, names[0], names[1], names[2]))
}

func (m Model) expeditionFlowChrome() string {
	inner := chromeInner(m.width)
	line := func(s string) string { return narrStyle.Render(fitLine(s, inner)) }
	msg := m.expeditionFlow.msg
	switch msg.Phase {
	case "recovery":
		next := strings.ToUpper(msg.RecoveryNext)
		if next == "PREP2" {
			next = "PREP 2"
		}
		return chromeBox(m.width, line(fmt.Sprintf("%s committed. Party healed. XP +%d kept.", strings.ToUpper(msg.LastEncounter), msg.LastXPGained))+"\n"+
			expMenuRow(0, next, "ABANDON"))
	case "captured":
		name := msg.CapturedSpecies
		if msg.FamilyName != "" {
			name = msg.FamilyName
		}
		return chromeBox(m.width, line(fmt.Sprintf("CAPTURED · Collection +1 · %s Lv.1 · XP +%d", strings.ToUpper(name), msg.LastXPGained))+"\n"+
			expMenuRow(0, "DOJO", "WORKBENCH"))
	case "hunt_failed":
		return chromeBox(m.width, line(fmt.Sprintf("Hunt failed. No capture. XP +%d kept.", msg.LastXPGained))+"\n"+
			expMenuRow(0, "DOJO"))
	case "abandoned":
		return chromeBox(m.width, line("Abandoned. Lost the target. Kept completed XP. Next run starts at prep 1.")+"\n"+
			expMenuRow(0, "DOJO"))
	default:
		return chromeBox(m.width, line("Expedition ended.")+"\n"+expMenuRow(0, "DOJO"))
	}
}

func (m Model) familyCard(f server.ExpeditionFamilyCard, focused bool) string {
	border := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(lipgloss.Color("252")).
		BorderBackground(screenBg).
		Background(screenBg).
		MarginBackground(screenBg).
		Width(expCardW).
		Padding(0, 0)
	if focused {
		border = border.BorderForeground(lipgloss.Color(primaryHex))
	}
	mark := "  "
	title := typeInk(f.Type).Render(strings.ToUpper(f.Name))
	if focused {
		mark = selStyle.Render("▶ ")
		title = selStyle.Render(strings.ToUpper(f.Name))
	}
	innerW := expCardW - 2
	art := place(innerW, expThumbH, m.familyThumb(f.Slug, innerW, expThumbH))
	inner := mark + title + "\n" +
		art + "\n" +
		dimStyle.Render(f.Type+" family") + "\n" +
		dimStyle.Render("support: "+f.Theme)
	return border.Render(inner)
}

func (m Model) familyThumb(slug string, maxW, maxH int) string {
	if m.set == nil {
		return dimStyle.Render("(no art)")
	}
	art, ok := m.set.Arts[slug]
	if !ok || maxW < 4 {
		return dimStyle.Render("(no art)")
	}
	grid := fitExpCardGrid(art.Grid, maxW, maxH)
	return strings.Join(sprite.RenderOn(grid, art.RunePalette(), screenBgHex), "\n")
}

const expThumbH = 16

func fitExpCardGrid(grid []string, maxW, maxH int) []string {
	grid = trimExpTransparent(grid)
	h := len(grid)
	factor := 1
	if maxH > 0 {
		maxPx := maxH * 2
		if h > maxPx {
			factor = (h + maxPx - 1) / maxPx
		}
	}
	if factor > 1 {
		grid = sprite.Downsample(grid, factor)
	}
	if maxW > 0 && expGridWidth(grid) > maxW {
		grid = clipExpColumns(grid, maxW)
	}
	return grid
}

func expGridWidth(grid []string) int {
	w := 0
	for _, row := range grid {
		if n := len([]rune(row)); n > w {
			w = n
		}
	}
	return w
}

func trimExpTransparent(grid []string) []string {
	grid = sprite.Trim(grid)
	w := expGridWidth(grid)
	if w == 0 {
		return grid
	}
	rows := make([][]rune, len(grid))
	for i, row := range grid {
		rr := []rune(row)
		for len(rr) < w {
			rr = append(rr, '.')
		}
		rows[i] = rr
	}
	emptyCol := func(x int) bool {
		for _, row := range rows {
			if row[x] != '.' && row[x] != ' ' {
				return false
			}
		}
		return true
	}
	left, right := 0, w-1
	for left <= right && emptyCol(left) {
		left++
	}
	for right >= left && emptyCol(right) {
		right--
	}
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = string(row[left : right+1])
	}
	return sprite.Trim(out)
}

func clipExpColumns(grid []string, maxW int) []string {
	w := expGridWidth(grid)
	if maxW < 1 || w <= maxW {
		return grid
	}
	start := (w - maxW) / 2
	out := make([]string, len(grid))
	for i, row := range grid {
		rr := []rune(row)
		for len(rr) < w {
			rr = append(rr, '.')
		}
		out[i] = string(rr[start : start+maxW])
	}
	return out
}

func (m Model) signalBoardCards(_ int) string {
	board := m.signalBoard.board
	if len(board.Families) == 0 {
		return ""
	}
	gap := expCardGap
	n := len(board.Families)
	cards := make([]string, n)
	for i, f := range board.Families {
		cards[i] = m.familyCard(f, i == m.signalBoard.card)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cards[0], blank(gap), cards[1], blank(gap), cards[2])
}

func centerBlock(w, h int, s string) string {
	src := strings.Split(strings.TrimRight(s, "\n"), "\n")
	blockW := 0
	for _, line := range src {
		blockW = max(blockW, lipgloss.Width(line))
	}
	left := max(0, (w-blockW)/2)
	top := max(0, (h-len(src))/2)
	out := make([]string, h)
	for i := range out {
		out[i] = blank(w)
	}
	for i, line := range src {
		row := top + i
		if row < 0 || row >= h {
			continue
		}
		out[row] = padCells(blank(left)+line, w)
	}
	return strings.Join(out, "\n")
}

func expMenuRow(focus int, labels ...string) string {
	parts := make([]string, len(labels))
	for i, l := range labels {
		if i == focus {
			parts[i] = selStyle.Render(l)
		} else {
			parts[i] = dimStyle.Render(l)
		}
	}
	return strings.Join(parts, dimStyle.Render("  "))
}

func expeditionBattlePrefix(phase, wildName string) string {
	switch phase {
	case "prep1":
		return fmt.Sprintf("PREP 1/3 · Wild %s · No capture.", strings.ToUpper(wildName))
	case "prep2":
		return fmt.Sprintf("PREP 2/3 · Wild %s · No capture.", strings.ToUpper(wildName))
	case "target":
		return fmt.Sprintf("TARGET · Wild %s · Capture Gauge live.", strings.ToUpper(wildName))
	default:
		return ""
	}
}
