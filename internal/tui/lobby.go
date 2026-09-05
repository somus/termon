package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"termon.sh/internal/battle"
	"termon.sh/internal/capture"
	"termon.sh/internal/game"
	"termon.sh/internal/lobby"
	"termon.sh/internal/server"
)

const (
	lobbyTileW     = 9
	lobbyTileH     = 4
	lobbyWalkTicks = 4
)

var (
	dojoTatamiA = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9b8a61")).
			Background(lipgloss.Color("#29261d")).
			MarginBackground(screenBg)
	dojoTatamiB = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7d7254")).
			Background(lipgloss.Color("#353024")).
			MarginBackground(screenBg)
	dojoCourtInk = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#d2bb7b")).
			Background(lipgloss.Color("#3b3425")).
			MarginBackground(screenBg)
	dojoCourtMark = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#e4ad48")).
			Background(lipgloss.Color("#3b3425")).
			MarginBackground(screenBg).
			Bold(true)
	dojoCourtBorder = ink("#b65345").Bold(true)
	dojoWallInk     = ink("#76503a")
	dojoWallEdge    = ink("#b67a43").Bold(true)
	dojoWoodInk     = ink("#a66d3f")
	dojoBannerInk   = ink("#cf5142").Bold(true)
	dojoScrollInk   = ink("#d8c69c")
	dojoPlantInk    = ink("#70a957").Bold(true)
	dojoWaterInk    = ink("#72b6c8")
	dojoClothInk    = ink("#c5b68f")
)

type walkAnimation struct {
	remaining int
	started   int
	period    int
}

type emoteOverlay struct {
	art  string
	x, y int
}

func renderLobby(snap server.SnapshotMsg, width, height int) string {
	return renderLobbyAnimated(snap, width, height, nil, 0, 0, 0, false)
}

func renderLobbyAnimated(
	snap server.SnapshotMsg,
	width, height int,
	walking map[string]walkAnimation,
	age, cameraX, cameraY int,
	cameraOn bool,
) string {
	mapHeight := max(1, height-1)
	viewCols := max(1, width/lobbyTileW)
	viewRows := max(1, mapHeight/lobbyTileH)
	worldX, worldY := cameraX, cameraY
	if !cameraOn {
		worldX = cameraOrigin(snap.You.X, viewCols, lobby.Width)
		worldY = cameraOrigin(snap.You.Y, viewRows, lobby.Height)
	}
	mapWidth := viewCols * lobbyTileW
	renderedHeight := viewRows * lobbyTileH
	offsetX := max(0, (width-mapWidth)/2)
	offsetY := max(0, (mapHeight-renderedHeight)/2)

	layout := lobby.SharedLayout()
	layers := []*lipgloss.Layer{
		lipgloss.NewLayer(cachedDojoFloor(layout, worldX, worldY, viewCols, viewRows)).
			X(offsetX).
			Y(offsetY),
	}
	emotes := []emoteOverlay{}

	trainers := append([]lobby.Presence(nil), snap.Others...)
	trainers = append(trainers, snap.You)
	sort.Slice(trainers, func(i, j int) bool {
		if trainers[i].Y != trainers[j].Y {
			return trainers[i].Y < trainers[j].Y
		}
		if trainers[i].X != trainers[j].X {
			return trainers[i].X < trainers[j].X
		}
		return trainers[i].Handle < trainers[j].Handle
	})
	for _, trainer := range trainers {
		if trainer.X < worldX || trainer.X >= worldX+viewCols ||
			trainer.Y < worldY || trainer.Y >= worldY+viewRows {
			continue
		}
		x := offsetX + (trainer.X-worldX)*lobbyTileW
		y := offsetY + (trainer.Y-worldY)*lobbyTileH
		animation, moving := walking[trainer.Hash]
		frame := 0
		if moving {
			frame = (age - animation.started) / animation.period
		}
		layers = append(layers, lipgloss.NewLayer(renderTrainer(
			trainer,
			trainer.Hash == snap.You.Hash,
			moving,
			frame,
			layout.SurfaceAt(trainer.X, trainer.Y),
		)).X(x).Y(y))
	}
	for _, trainer := range trainers {
		if trainer.Emote == "" || trainer.X < worldX || trainer.X >= worldX+viewCols ||
			trainer.Y < worldY || trainer.Y >= worldY+viewRows {
			continue
		}
		bubble := renderEmoteBubble(trainer.Emote)
		bubbleWidth := lipgloss.Width(bubble)
		x := offsetX + (trainer.X-worldX)*lobbyTileW + (lobbyTileW-bubbleWidth)/2
		x = min(max(0, x), max(0, width-bubbleWidth))
		y := offsetY + (trainer.Y-worldY)*lobbyTileH - 3
		if y < 0 {
			y = offsetY + (trainer.Y-worldY+1)*lobbyTileH
		}
		emotes = append(emotes, emoteOverlay{art: bubble, x: x, y: y})
	}

	canvas := lipgloss.NewCanvas(max(1, width), mapHeight)
	canvas.Compose(lipgloss.NewCompositor(layers...))
	for _, emote := range emotes {
		overlayEmote(canvas, emote.art, emote.x, emote.y)
	}
	fillCanvas(canvas, screenBgRGB)
	context := ""
	if snap.Context != "" {
		context = promptStyle.Render("* ") + dimStyle.Render(fitLine(snap.Context, max(1, width-2)))
	}
	return canvas.Render() + "\n" + padCells(context, width)
}

// renderDojoBackdrop paints the Dojo floor centered on a tile, with no trainers
// or context line. Used while Master Sable speaks during onboarding.
func renderDojoBackdrop(width, height, focusX, focusY int) string {
	width = max(1, width)
	height = max(1, height)
	viewCols := max(1, width/lobbyTileW)
	viewRows := max(1, height/lobbyTileH)
	worldX := cameraOrigin(focusX, viewCols, lobby.Width)
	worldY := cameraOrigin(focusY, viewRows, lobby.Height)
	mapWidth := viewCols * lobbyTileW
	renderedHeight := viewRows * lobbyTileH
	offsetX := max(0, (width-mapWidth)/2)
	offsetY := max(0, (height-renderedHeight)/2)
	canvas := lipgloss.NewCanvas(width, height)
	canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(cachedDojoFloor(lobby.SharedLayout(), worldX, worldY, viewCols, viewRows)).
			X(offsetX).
			Y(offsetY),
	))
	fillCanvas(canvas, screenBgRGB)
	return canvas.Render()
}

// dojoFloorKey identifies one composed static floor layer.
type dojoFloorKey struct{ worldX, worldY, cols, rows int }

// The floor is pure static geometry, so composed layers are cached per
// viewport. Sessions may report different terminal sizes, so the cache holds
// several keys; it clears wholesale when that bound is exceeded. Rendering
// runs on the single Bubble Tea goroutine, but tests may touch this
// concurrently, hence the mutex.
var (
	dojoFloorMu    sync.Mutex
	dojoFloorCache = map[dojoFloorKey]string{}
)

const dojoFloorCacheLimit = 64

func cachedDojoFloor(layout *lobby.DojoLayout, worldX, worldY, cols, rows int) string {
	k := dojoFloorKey{worldX, worldY, cols, rows}
	dojoFloorMu.Lock()
	defer dojoFloorMu.Unlock()
	if s, ok := dojoFloorCache[k]; ok {
		return s
	}
	s := renderDojoFloor(layout, worldX, worldY, cols, rows)
	if len(dojoFloorCache) >= dojoFloorCacheLimit {
		dojoFloorCache = map[dojoFloorKey]string{}
	}
	dojoFloorCache[k] = s
	return s
}

func renderDojoFloor(room *lobby.DojoLayout, worldX, worldY, cols, rows int) string {
	lines := make([]string, 0, rows*lobbyTileH)
	for tileY := range rows {
		row := make([]string, lobbyTileH)
		for tileX := range cols {
			art := renderDojoTile(room, worldX+tileX, worldY+tileY)
			artLines := strings.Split(art, "\n")
			for i := range lobbyTileH {
				row[i] += artLines[i]
			}
		}
		lines = append(lines, row...)
	}
	return strings.Join(lines, "\n")
}

func renderDojoTile(room *lobby.DojoLayout, x, y int) string {
	if obj, ok := room.ObjectAt(x, y); ok {
		block := lobbyBlock
		switch room.SurfaceAt(x, y) {
		case lobby.SurfaceTatami:
			block = renderTatamiBlock
		case lobby.SurfaceCourt, lobby.SurfaceCourtStart, lobby.SurfaceCourtCrest:
			block = renderCourtBackedBlock
		}
		switch obj.Kind {
		case lobby.ObjectPillar:
			return block(dojoWoodInk, "╥═╥", "║▓║", "║█║", "╨═╨")
		case lobby.ObjectMaster:
			return block(yellowBar, "MASTER", "◉", "╱╋╲", "╱ ╲")
		case lobby.ObjectGong:
			return block(yellowBar, "╭───╮", "│ ◉ │", "╰─┬─╯", " ╱ ╲")
		case lobby.ObjectDummy:
			return block(dojoWoodInk, " ◉", "─╂─", " ║", "╱ ╲")
		case lobby.ObjectSign:
			return block(dojoCourtBorder, "", obj.Label, "╶───────╴", "─────────")
		case lobby.ObjectScroll:
			return block(dojoScrollInk, "", "≋≋≋", "╶─────╴", "─────────")
		case lobby.ObjectFloorboard:
			return block(dojoTatamiB, "", "╴ ╶", "───────", "─────────")
		case lobby.ObjectSleeper:
			return block(dojoPlantInk, "z", "⌣", "╱ ╲", "─────────")
		case lobby.ObjectBanner:
			return block(dojoBannerInk, "╭─╥─╮", "│╲◆╱│", "│ ║ │", "╰─╨─╯")
		case lobby.ObjectWallScroll:
			return block(dojoScrollInk, "╭─┬─╮", "│≋≋≋│", "│≋≋≋│", "╰─┴─╯")
		case lobby.ObjectLantern:
			return block(yellowBar, " ╥", "╭◇╮", "│█│", "╰┬╯")
		case lobby.ObjectCrest:
			return block(yellowBar, "╭─────╮", "│ ╲◆╱ │", "│ ╱◇╲ │", "╰─────╯")
		case lobby.ObjectTrophyCase:
			return block(yellowBar, "╭─────╮", "│♜ ◆ ♜│", "│ ▔▔▔ │", "╰─────╯")
		case lobby.ObjectBadgeDisplay:
			return block(dojoBannerInk, "╭─────╮", "│◆ ◇ ◆│", "│ ◇ ◆ │", "╰─────╯")
		case lobby.ObjectPlant:
			return block(dojoPlantInk, "♣♣♣", "╲♣│♣╱", " ╲│╱", " ╰─╯")
		case lobby.ObjectBench:
			return block(dojoBannerInk, "", "╭═════╮", "╰┬┬┬┬╯", " ╵   ╵")
		case lobby.ObjectCubbies:
			return block(dojoWoodInk, "╭─────╮", "│□ □ □│", "│□ □ □│", "╰─────╯")
		case lobby.ObjectGearRack:
			return block(dojoWoodInk, "╭─────╮", "│╱╱╱╱╱│", "│╲╲╲╲╲│", "╰─┴─┴─╯")
		case lobby.ObjectPracticePads:
			return block(dojoBannerInk, "╭─────╮", "│ ▣ ▣ │", "│ ▣ ▣ │", "╰─────╯")
		case lobby.ObjectWaterUrn:
			return block(dojoWaterInk, " ╭─╮", "╭╯≈╰╮", "│ ≋ │", "╰───╯")
		case lobby.ObjectRecordTerminal:
			return block(promptStyle, "╭─────╮", "│01>_ │", "╰─┬─┬─╯", "  ╰─╯")
		case lobby.ObjectNoticeBoard:
			return block(dojoScrollInk, "╭─────╮", "│• ─ •│", "│ ── •│", "╰─┬─┬─╯")
		case lobby.ObjectFirstAid:
			return block(redBar, "╭─────╮", "│  +  │", "│ +++ │", "╰─────╯")
		case lobby.ObjectTowelStation:
			return block(dojoClothInk, "╭─────╮", "│≋≋ ≋≋│", "│≋≋ ≋≋│", "╰─────╯")
		}
	}
	switch room.SurfaceAt(x, y) {
	case lobby.SurfaceCourt:
		return renderCourtTile(dojoCourtInk)
	case lobby.SurfaceCourtBorder:
		return renderCourtBorder(x, y)
	case lobby.SurfaceCourtStart:
		return lobbyBlock(dojoCourtMark, "", "╽", "╿", "")
	case lobby.SurfaceCourtCrest:
		return lobbyBlock(dojoCourtMark, "╲   ╱", "◇", "╱ ◆ ╲", "")
	case lobby.SurfaceNorthWall:
		return lobbyBlock(dojoWallInk, "┏━━━━━━━┓", "┃▓▓▓▓▓▓▓┃", "┃╱╲╱╲╱╲╱┃", "┗━━━━━━━┛")
	case lobby.SurfaceWall:
		return renderDojoWall(x, y)
	}
	if room.Blocked(x, y) {
		return lobbyBlock(pageInk, "█", "█", "█", "█")
	}
	return renderTatamiTile()
}

func renderTatamiTile() string {
	return renderTatamiBlock(dojoTatamiA)
}

func renderCourtTile(style lipgloss.Style) string {
	return lobbyBlock(style)
}

func renderTatamiBlock(style lipgloss.Style, rows ...string) string {
	lines := lobbyBlockLines(rows)
	for row, line := range lines {
		left, right := dojoTatamiA.GetBackground(), dojoTatamiB.GetBackground()
		if row >= lobbyTileH/2 {
			left, right = right, left
		}
		lines[row] = style.Background(left).Render(ansi.Cut(line, 0, lobbyTileW/2)) +
			style.Background(right).Render(ansi.Cut(line, lobbyTileW/2, lobbyTileW))
	}
	return strings.Join(lines, "\n")
}

func renderCourtBackedBlock(style lipgloss.Style, rows ...string) string {
	background := dojoCourtInk.GetBackground()
	return lobbyBlock(style.Background(background).MarginBackground(background), rows...)
}

func renderCourtBorder(x, y int) string {
	background := dojoCourtInk.GetBackground()
	style := dojoCourtBorder.Background(background).MarginBackground(background)
	if y == lobby.CourtMinY {
		line := "━━━━━━━━━"
		if x == lobby.CourtMinX {
			return lobbyBlock(style, "┏━━━━━━━━", "┃        ", "┃        ", "┃        ")
		}
		if x == lobby.CourtMaxX {
			return lobbyBlock(style, "━━━━━━━━┓", "        ┃", "        ┃", "        ┃")
		}
		return lobbyBlock(style, line)
	}
	if y == lobby.CourtMaxY {
		line := "━━━━━━━━━"
		if x == lobby.CourtMinX {
			return lobbyBlock(style, "┃        ", "┃        ", "┃        ", "┗━━━━━━━━")
		}
		if x == lobby.CourtMaxX {
			return lobbyBlock(style, "        ┃", "        ┃", "        ┃", "━━━━━━━━┛")
		}
		return lobbyBlock(style, "", "", "", line)
	}
	if x == lobby.CourtMinX {
		return lobbyBlock(style, "┃        ", "┃        ", "┃        ", "┃        ")
	}
	return lobbyBlock(style, "        ┃", "        ┃", "        ┃", "        ┃")
}

func renderDojoWall(x, y int) string {
	if y == 0 {
		return lobbyBlock(dojoWallEdge, "█████████", "▓━━━━━━━▓", "▓╲╱╲╱╲╱▓", "┗━━━━━━━┛")
	}
	if y == lobby.Height-1 {
		return lobbyBlock(dojoWallEdge, "━━━━━━━━━", "", "", "")
	}
	if x == 0 {
		return lobbyBlock(dojoWallEdge, "        ┃", "        ┃", "        ┃", "        ┃")
	}
	return lobbyBlock(dojoWallEdge, "┃", "┃", "┃", "┃")
}

func renderTrainer(
	p lobby.Presence,
	local, walking bool,
	frame int,
	surface lobby.SurfaceKind,
) string {
	head := "◉"
	if local {
		head = "◎"
	} else if p.InBattle {
		head = "×"
	} else if p.InQueue {
		head = "◇"
	}
	legs := "╱ ╲"
	if walking {
		if frame%2 == 0 {
			legs = "╱ ┘"
		} else {
			legs = "└ ╲"
		}
	}
	style := narrStyle
	if local {
		style = selStyle
	} else if p.InBattle {
		style = dimStyle
	}
	block := lobbyBlock
	switch surface {
	case lobby.SurfaceTatami:
		block = renderTatamiBlock
	case lobby.SurfaceCourt, lobby.SurfaceCourtBorder, lobby.SurfaceCourtStart, lobby.SurfaceCourtCrest:
		block = renderCourtBackedBlock
	}
	return block(style, head, "╱╋╲", legs, "["+fitLine(p.Handle, lobbyTileW-2)+"]")
}

func (m *Model) markLobbyMovement(next server.SnapshotMsg) {
	previous := make(map[string]lobby.Presence, len(m.snap.Others)+1)
	if m.snap.You.Hash != "" {
		previous[m.snap.You.Hash] = m.snap.You
	}
	for _, trainer := range m.snap.Others {
		previous[trainer.Hash] = trainer
	}
	if m.lobbyWalk == nil {
		m.lobbyWalk = make(map[string]walkAnimation)
	}
	if m.lobbyLastMove == nil {
		m.lobbyLastMove = make(map[string]int)
	}
	trainers := append([]lobby.Presence(nil), next.Others...)
	trainers = append(trainers, next.You)
	for _, trainer := range trainers {
		before, ok := previous[trainer.Hash]
		if ok && (before.X != trainer.X || before.Y != trainer.Y) {
			period := 2
			if last, seen := m.lobbyLastMove[trainer.Hash]; seen {
				period = min(4, max(1, (m.lobbyAge-last)/2))
			}
			m.lobbyWalk[trainer.Hash] = walkAnimation{
				remaining: lobbyWalkTicks,
				started:   m.lobbyAge,
				period:    period,
			}
			m.lobbyLastMove[trainer.Hash] = m.lobbyAge
		}
	}
}

func renderEmoteBubble(text string) string {
	text = fitLine(text, 11)
	innerWidth := lipgloss.Width(text) + 2
	tail := innerWidth / 2
	top := "╭" + strings.Repeat("─", innerWidth) + "╮"
	middle := "│ " + text + " │"
	bottom := "╰" + strings.Repeat("─", tail) + "┬" +
		strings.Repeat("─", innerWidth-tail-1) + "╯"
	style := promptStyle.UnsetBackground().UnsetMarginBackground()
	return style.Render(strings.Join([]string{top, middle, bottom}, "\n"))
}

func overlayEmote(canvas *lipgloss.Canvas, art string, x, y int) {
	if canvas == nil || art == "" {
		return
	}
	overlay := lipgloss.NewCanvas(lipgloss.Width(art), lipgloss.Height(art))
	overlay.Compose(lipgloss.NewLayer(art))
	for row := range overlay.Height() {
		for column := range overlay.Width() {
			source := overlay.CellAt(column, row)
			under := canvas.CellAt(x+column, y+row)
			if source == nil || under == nil {
				continue
			}
			cell := source.Clone()
			cell.Style.Bg = under.Style.Bg
			canvas.SetCell(x+column, y+row, cell)
		}
	}
}

func lobbyBlock(style lipgloss.Style, rows ...string) string {
	return style.Render(strings.Join(lobbyBlockLines(rows), "\n"))
}

func lobbyBlockLines(rows []string) []string {
	lines := make([]string, lobbyTileH)
	for i := range lobbyTileH {
		line := ""
		if i < len(rows) {
			line = fitLine(rows[i], lobbyTileW)
		}
		pad := max(0, lobbyTileW-lipgloss.Width(line))
		left := pad / 2
		lines[i] = strings.Repeat(" ", left) + line + strings.Repeat(" ", pad-left)
	}
	return lines
}

func cameraOrigin(center, visible, total int) int {
	if visible >= total {
		return 0
	}
	return min(max(0, center-visible/2), total-visible)
}

func (m *Model) syncLobbyCamera(trainer lobby.Presence, force bool) {
	if trainer.Hash == "" || m.width == 0 || m.height == 0 {
		return
	}
	viewCols := max(1, max(1, m.width-2)/lobbyTileW)
	viewRows := max(1, max(1, m.height-3)/lobbyTileH)
	outside := trainer.X < m.lobbyCameraX || trainer.X >= m.lobbyCameraX+viewCols ||
		trainer.Y < m.lobbyCameraY || trainer.Y >= m.lobbyCameraY+viewRows
	if force || !m.lobbyCameraOn || outside {
		m.lobbyCameraX = cameraOrigin(trainer.X, viewCols, lobby.Width)
		m.lobbyCameraY = cameraOrigin(trainer.Y, viewRows, lobby.Height)
		m.lobbyCameraOn = true
	}
}

func dojoFooter(snap server.SnapshotMsg, emotePick bool, status string) string {
	if status != "" {
		return warnStyle.Render(status)
	}
	if snap.Offer != nil {
		return promptStyle.Render(snap.Offer.FromHandle+" challenged you!") + dimStyle.Render("  ") +
			keyHint("y", "accept") + dimStyle.Render(" · ") + keyHint("n", "decline")
	}
	if emotePick {
		return dimStyle.Render("emote  ") + keyHint("1", "gl hf") + dimStyle.Render("  ") +
			keyHint("2", "gg") + dimStyle.Render("  ") + keyHint("3", "well fought") +
			dimStyle.Render("  ") + keyHint("4", "rematch?") + dimStyle.Render("  ") +
			keyHint("5", "hello!") + dimStyle.Render("  ") + keyHint("esc", "back")
	}
	return strings.Join([]string{
		keyHint("arrows/hjkl", "walk"),
		keyHint("c", "challenge"),
		keyHint("f", "find"),
		keyHint("p", "party"),
		keyHint("e", "emote"),
		keyHint("n", "reset"),
		keyHint("q", "leave"),
	}, dimStyle.Render(" · "))
}

func keyHint(key, label string) string {
	return promptStyle.Render(key) + blank(1) + dimStyle.Render(label)
}

func hintLine(parts ...string) string {
	return strings.Join(parts, dimStyle.Render(" · "))
}

func chromeBrand() string {
	return titleStyle.Render("termon")
}

func (m Model) chromeID() string {
	handle, species := "", ""
	wins, losses := 0, 0
	if m.save != nil {
		handle, wins, losses = m.save.Handle, m.save.Wins, m.save.Losses
		if lead, err := game.PartyLead(m.save); err == nil {
			species = strings.ToUpper(lead.Species)
		}
	}
	if m.screen == screenOnboard {
		if m.onboard.handle != "" {
			handle = m.onboard.handle
		}
		if m.onboard.stage >= stageStarter {
			name, _, _ := m.onboard.starterInfo(m.set)
			species = strings.ToUpper(name)
		}
	}
	if m.snap.You.Handle != "" {
		handle = m.snap.You.Handle
	}
	if m.snap.You.Species != "" {
		species = strings.ToUpper(m.snap.You.Species)
	}
	if handle == "" {
		return ""
	}
	out := selStyle.Render(handle)
	if species != "" {
		out += dimStyle.Render(" · " + species)
	}
	if m.save != nil {
		out += dimStyle.Render("  ") + okStyle.Render(strconv.Itoa(wins)) +
			dimStyle.Render("–") + dimStyle.Render(strconv.Itoa(losses))
	}
	return out
}

func (m Model) chromeMid() string {
	switch m.screen {
	case screenOnboard:
		switch m.onboard.stage {
		case stageTalk:
			return dimStyle.Render("First run")
		case stageHandle, stageHandleInput, stageHandleOK:
			return dimStyle.Render("Name")
		case stageStarter, stageConfirm, stageJoined:
			return dimStyle.Render("Choose a partner")
		case stageLesson:
			return dimStyle.Render("How to fight")
		default:
			return dimStyle.Render("First run")
		}
	case screenQueue:
		return dimStyle.Render(fmt.Sprintf("Find battle · %d waiting", m.queue.Waiting))
	case screenBattle:
		battleScreen := m.battle.withContext(m.battleContext())
		if m.reconnecting != "" {
			return warnStyle.Render("Opponent lost connection. Reconnecting…")
		}
		if m.tutorial {
			if battleScreen.canSwitch() {
				return dimStyle.Render("Lesson 2")
			}
			return dimStyle.Render("Lesson 1")
		}
		if foe := battleScreen.opponentName(); foe != "" {
			return dimStyle.Render("vs " + foe)
		}
		return dimStyle.Render("Battle")
	case screenDisplaced:
		return dimStyle.Render("Seat taken")
	case screenWorkbench:
		return dimStyle.Render("Workbench")
	case screenSignalBoard:
		return dimStyle.Render("Signal Board")
	case screenExpedition:
		return dimStyle.Render("Expedition")
	default:
		if m.snap.Dojo > 0 {
			return dimStyle.Render(fmt.Sprintf("Dojo %d · %d inside", m.snap.Dojo, 1+len(m.snap.Others)))
		}
		return dimStyle.Render(fmt.Sprintf("Dojo · %d inside", 1+len(m.snap.Others)))
	}
}

func (m Model) chromeFooter() string {
	if m.modal != modalNone {
		return hintLine(keyHint("esc", "close"), keyHint("S", "stats"), keyHint("?", "help"))
	}
	if m.status != "" && m.screen != screenBattle {
		return warnStyle.Render(m.status)
	}
	switch m.screen {
	case screenOnboard:
		return onboardFooter(m.onboard)
	case screenQueue:
		return hintLine(keyHint("x", "cancel"), keyHint("q", "leave"))
	case screenBattle:
		return m.battle.withContext(m.battleContext()).footer()
	case screenDisplaced:
		return keyHint("q", "quit")
	case screenWorkbench:
		return m.workbenchFooter()
	case screenSignalBoard:
		if m.signalBoard.armed {
			return hintLine(keyHint("enter", "start"), keyHint("esc", "back"), keyHint("x", "abandon"))
		}
		return hintLine(keyHint("←/→", "family"), keyHint("1-3", "pick"), keyHint("enter", "arm"), keyHint("esc", "dojo"))
	case screenExpedition:
		switch m.expeditionFlow.msg.Phase {
		case "recovery":
			return hintLine(keyHint("enter", "continue"), keyHint("x", "abandon"))
		case "captured":
			return hintLine(keyHint("enter", "dojo"), keyHint("p", "workbench"))
		default:
			return hintLine(keyHint("enter", "dojo"))
		}
	default:
		if nextRequiredLesson(m.save) != 0 && m.snap.Offer == nil && !m.emotePick {
			return hintLine(keyHint("p", "party"), keyHint("S", "stats"), keyHint("?", "help"), keyHint("q", "leave"))
		}
		return hintLine(dojoFooter(m.snap, m.emotePick, ""), keyHint("S", "stats"), keyHint("?", "help"))
	}
}

func onboardFooter(o onboardModel) string {
	switch o.stage {
	case stageTalk, stageHandleOK, stageJoined, stageLesson:
		return keyHint("enter", "continue")
	case stageHandle:
		return hintLine(keyHint("←/→", "choose"), keyHint("enter", "keep"), keyHint("r", "reroll"), keyHint("e", "type"))
	case stageHandleInput:
		return hintLine(keyHint("enter", "done"), keyHint("esc", "back"))
	case stageStarter:
		return hintLine(keyHint("←/→", "cycle"), keyHint("1-3", "jump"), keyHint("enter", "choose"))
	case stageConfirm:
		return hintLine(keyHint("enter", "yes"), keyHint("n", "no"))
	default:
		return keyHint("enter", "continue")
	}
}

func (m battleScreenModel) footer() string {
	if m.logOpen {
		return keyHint("tab", "close log")
	}
	if m.session.battle != nil && m.session.battle.State() == battle.StateOver {
		if m.resultHold <= 0 {
			label := "dojo"
			if m.requiredLesson() && !m.hasPendingProgression {
				label = "retry"
			}
			return hintLine(keyHint("enter", label), keyHint("tab", "log"))
		}
		return keyHint("tab", "log")
	}
	if m.playing {
		return hintLine(keyHint("enter", "faster"), keyHint("tab", "log"), keyHint("f", "forfeit"))
	}
	if m.showingCaptureSeq() {
		return hintLine(keyHint("enter", "continue"), keyHint("tab", "log"))
	}
	if m.battleIntro {
		if m.introHold <= 0 {
			return hintLine(keyHint("enter", "continue"), keyHint("tab", "log"))
		}
		return keyHint("tab", "log")
	}
	if coach := m.sableCoachLine(); coach != "" {
		return narrStyle.Render(coach)
	}
	if m.fightRoot {
		if m.canSwitch() {
			return hintLine(keyHint("enter", "fight"), keyHint("2", "switch"), keyHint("f", "run"), keyHint("tab", "log"))
		}
		return hintLine(keyHint("enter", "fight"), keyHint("f", "run"), keyHint("tab", "log"))
	}
	return hintLine(keyHint("arrows/hjkl", "aim"), keyHint("enter", "lock"), keyHint("esc", "back"), keyHint("tab", "log"))
}

func (m battleScreenModel) sableCoachLine() string {
	if !m.requiredLesson() {
		return ""
	}
	open := m.openCaptureNames()
	still := ""
	if len(open) == 1 {
		still = " Still open: " + open[0] + "."
	} else if len(open) > 1 {
		still = fmt.Sprintf(" %d objectives still open.", len(open))
	}
	switch {
	case m.switchRoot:
		return "Sable: switch into a healthy reserve." + still
	case m.fightRoot && m.runConfirm:
		return "Sable: Enter leaves and retries this Lesson. Esc stays."
	case m.fightRoot && m.canSwitch():
		return "Sable: SWITCH if the matchup is bad, then FIGHT." + still
	case m.fightRoot:
		return "Sable: pick FIGHT, then three different Moves." + still
	case m.canSwitch():
		return "Sable: pick a Move. 2× on the TYPE pane is super-effective." + still
	default:
		return "Sable: pick a different Move. 2× on the TYPE pane is super-effective." + still
	}
}

func (m battleScreenModel) openCaptureNames() []string {
	var names []string
	for _, o := range m.capture.Objectives {
		if o.Done {
			continue
		}
		name := string(o.ID)
		if cat, ok := capture.ObjectiveByID(o.ID); ok && cat.DisplayName != "" {
			name = cat.DisplayName
		}
		names = append(names, name)
	}
	return names
}

func (m Model) wrapChrome(body string) string {
	return chromeFrame(max(1, m.width), max(1, m.height), chromeBrand(), m.chromeMid(), m.chromeID(), body, m.chromeFooter())
}

func chromeFrame(w, h int, left, mid, right, body, footer string) string {
	return pageFrame(w, h, left, mid, right, body, fitLine(footer, max(1, w-8)))
}

func chromeBody(body string, w, h int) string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, line := range lines {
		lines[i] = padCells(line, w)
	}
	return strings.Join(lines, "\n")
}

var emotes = []string{"gl hf", "gg", "well fought", "rematch?", "hello!"}
