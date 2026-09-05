package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

const (
	screenBgHex  = "#14121c"
	primaryHex   = "#6ee7f0"
	primaryLoHex = "#3d9eaa"
)

var (
	screenBg    = lipgloss.Color(screenBgHex)
	screenBgRGB = color.RGBA{R: 0x14, G: 0x12, B: 0x1c, A: 0xff}
	titleStyle  = ink(primaryHex).Bold(true)
	dimStyle    = ink("241")
	selStyle    = ink(primaryHex).Bold(true)
	okStyle     = ink("42")
	promptStyle = ink(primaryHex)
	greenBar    = ink("70")
	yellowBar   = ink("178")
	redBar      = ink("167")
	narrStyle   = ink("252")
	frameStyle  = lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(lipgloss.Color("252")).
			BorderBackground(screenBg).
			Background(screenBg).
			MarginBackground(screenBg)
	chromeStyle = frameStyle.Padding(0, 1)
	plateStyle  = chromeStyle
	warnStyle   = ink("167")
	fillStyle   = lipgloss.NewStyle().Background(screenBg).MarginBackground(screenBg)
	pageInk     = ink("241")
)

func ink(fg string) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(fg)).
		Background(screenBg).
		MarginBackground(screenBg)
}

func menuChoice(on bool, label string) string {
	if label == "" {
		return dimStyle.Render("  ")
	}
	if on {
		return selStyle.Render("▶ " + label)
	}
	return dimStyle.Render("  " + label)
}

func menuRow(cursor int, labels ...string) string {
	items := make([]string, len(labels))
	for i, label := range labels {
		items[i] = menuChoice(i == cursor, label)
	}
	return strings.Join(items, blank(2))
}

func menuCol(cursor int, labels ...string) string {
	items := make([]string, len(labels))
	for i, label := range labels {
		items[i] = menuChoice(i == cursor, label)
	}
	return strings.Join(items, "\n")
}

func menuGrid(cursor, cols, cellW int, labels []string) string {
	if cols < 1 {
		cols = 1
	}
	cells := make([]string, len(labels))
	for i, label := range labels {
		cells[i] = fitLine(menuChoice(i == cursor, label), cellW)
	}
	var rows []string
	for i := 0; i < len(cells); i += cols {
		end := min(len(cells), i+cols)
		rows = append(rows, strings.Join(cells[i:end], fillStyle.Render(" ")))
	}
	return strings.Join(rows, "\n")
}

func typeInk(typ string) lipgloss.Style {
	switch strings.ToLower(typ) {
	case "organic":
		return ink("70").Bold(true)
	case "thermal":
		return ink("208").Bold(true)
	case "coolant":
		return ink("81").Bold(true)
	case "current":
		return ink("226").Bold(true)
	case "virus":
		return ink("171").Bold(true)
	case "silicon":
		return ink("245").Bold(true)
	default:
		return promptStyle.Bold(true)
	}
}

func place(w, h int, s string) string {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, strings.TrimRight(s, "\n"),
		lipgloss.WithWhitespaceStyle(fillStyle))
}

func overlayCentered(base, layer string, w, h int) string {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	col := max(0, (w-lipgloss.Width(layer))/2)
	row := max(0, (h-lipgloss.Height(layer))/2)
	canvas := lipgloss.NewCanvas(w, h)
	canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(base).X(0).Y(0),
		lipgloss.NewLayer(layer).X(col).Y(row),
	))
	fillCanvas(canvas, screenBgRGB)
	return canvas.Render()
}

func modalCard(body string, maxW, maxH int) string {
	maxW = max(12, maxW)
	maxH = max(5, maxH)
	body = strings.TrimRight(body, "\n")
	wrapW := min(max(36, contentWidth(body)), maxW-8)
	inner := bodyStyle.Render(fitBlock(body, max(1, wrapW)))
	w := min(max(lipgloss.Width(inner)+2, 20), maxW)
	h := min(max(lipgloss.Height(inner)+2, 5), maxH)
	return pageFrame(w, h, "", "", "", inner, "")
}

func contentWidth(s string) int {
	w := 0
	for line := range strings.SplitSeq(s, "\n") {
		w = max(w, lipgloss.Width(line))
	}
	return w
}

func fillCanvas(c *lipgloss.Canvas, bg color.Color) {
	if c == nil {
		return
	}
	for y := range c.Height() {
		for x := range c.Width() {
			cell := c.CellAt(x, y)
			if cell == nil {
				c.SetCell(x, y, &uv.Cell{Content: " ", Width: 1, Style: uv.Style{Bg: bg}})
				continue
			}
			if cell.Style.Bg != nil {
				continue
			}
			if cell.Content == "" {
				cell.Content = " "
				cell.Width = 1
			}
			cell.Style.Bg = bg
			c.SetCell(x, y, cell)
		}
	}
}

func blank(w int) string {
	if w < 1 {
		return ""
	}
	return fillStyle.Render(strings.Repeat(" ", w))
}

func padCells(s string, w int) string {
	s = fitLine(s, w)
	if pad := w - lipgloss.Width(s); pad > 0 {
		s += blank(pad)
	}
	return s
}

func fillLine(s string, w int) string {
	if w < 1 {
		w = 1
	}
	return fillStyle.Width(w).Render(fitLine(s, w))
}

// pageFrame is the late.sh-style page chrome: rounded corners with titles
// spliced into the top edge and shortcuts into the bottom edge.
func pageFrame(w, h int, left, mid, right, body, footer string) string {
	if w < 3 {
		w = 3
	}
	if h < 3 {
		h = 3
	}
	innerW, innerH := w-2, h-2
	side := pageInk.Render("│")
	top := pageEdge(w, "╭", "╮", left, mid, right)
	bot := pageEdge(w, "╰", "╯", footer, "", "")
	lines := strings.Split(chromeBody(body, innerW, innerH), "\n")
	out := make([]string, 0, h)
	out = append(out, top)
	for _, line := range lines {
		out = append(out, side+fillStyle.Render(line)+side)
	}
	out = append(out, bot)
	return strings.Join(out, "\n")
}

func pageEdge(w int, open, end, left, mid, right string) string {
	inner := max(1, w-2)
	pad := func(s string) (string, int) {
		if s == "" {
			return "", 0
		}
		out := fillStyle.Render(" ") + s + fillStyle.Render(" ")
		return out, lipgloss.Width(out)
	}
	l, lw := pad(left)
	m, mw := pad(mid)
	r, rw := pad(right)
	extra := 0
	if l != "" {
		extra++
	}
	if r != "" {
		extra++
	}
	rest := inner - lw - mw - rw - extra
	if rest < 0 {
		m = ""
		rest = inner - lw - rw - extra
	}
	if rest < 0 {
		r = ""
		extra = 0
		if l != "" {
			extra = 1
		}
		rest = inner - lw - extra
	}
	if rest < 0 {
		l = ""
		rest = inner
	}
	leftFill, rightFill := rest, 0
	if m != "" {
		leftFill = rest / 2
		rightFill = rest - leftFill
	}
	var b strings.Builder
	b.WriteString(pageInk.Render(open))
	if l != "" {
		b.WriteString(pageInk.Render("─"))
		b.WriteString(l)
	}
	b.WriteString(pageInk.Render(strings.Repeat("─", max(0, leftFill))))
	b.WriteString(m)
	b.WriteString(pageInk.Render(strings.Repeat("─", max(0, rightFill))))
	if r != "" {
		b.WriteString(r)
		b.WriteString(pageInk.Render("─"))
	}
	b.WriteString(pageInk.Render(end))
	return padCells(b.String(), w)
}

func paint(s string, w, h int) string {
	return chromeBody(s, max(1, w), max(1, h))
}
