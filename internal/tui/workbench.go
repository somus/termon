package tui

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"termon.sh/internal/content"
	"termon.sh/internal/game"
	"termon.sh/internal/server"
	"termon.sh/internal/sprite"
	"termon.sh/internal/store"
)

const (
	wbPaneCollection = 26
	wbPaneParty      = 24
)

const (
	wbFilterAll = iota
	wbFilterParty
	wbFilterBench
	wbFilterAttention
)

const (
	wbSortRecent = iota
	wbSortLevel
	wbSortSpecies
)

const (
	wbTabCollection = iota
	wbTabSpecies
	wbTabTrainerStats
)

const (
	// Workbench layers are mutually exclusive routes above the browse layer.
	// Each confirmation layer follows one editor and returns to that editor on
	// cancel. searchOpen is entered only from browse and preempts layer input.
	wbLayerBrowse = iota
	wbLayerActions
	wbLayerPartySlot
	wbLayerPartyConfirm
	wbLayerLoadout
	wbLayerLoadoutConfirm
	wbLayerNickname
	wbLayerMoveNotice
	wbLayerEvolution
	wbLayerEvolutionConfirm
	wbLayerSpeciesFilter
)

// workbenchModel owns transient Workbench state; Model.save remains the
// authoritative Collection snapshot. selectedID identifies the Monster that
// Collection actions target and survives selection sync while that Monster
// exists; cursor is the current tab's list position.
type workbenchModel struct {
	tab          int
	filter       int
	sort         int
	search       string
	searchOpen   bool
	selectedID   string
	cursor       int
	layer        int
	subCursor    int
	partySlot    int
	loadoutSlot  int
	pickMove     string
	nickInput    string
	speciesSlug  string
	confirmText  string
	ackNoticeIDs []string
	focusMove    string
	trainerStats *store.TrainerStats
	worldStats   *store.WorldStats
	statsError   string
}

func newWorkbench(m Model) workbenchModel {
	w := workbenchModel{selectedID: firstVisibleMonsterID(m, workbenchModel{})}
	w.cursor = indexOfMonster(m, w.selectedID, w)
	return w
}

func (m *Model) openWorkbench() {
	m.screen = screenWorkbench
	m.wb = newWorkbench(*m)
}

func (w *workbenchModel) syncSelection(m *Model) {
	if _, ok := game.MonsterByID(m.save, w.selectedID); ok {
		w.cursor = indexOfMonster(*m, w.selectedID, *w)
		return
	}
	w.selectedID = firstVisibleMonsterID(*m, *w)
	w.cursor = indexOfMonster(*m, w.selectedID, *w)
}

func (m Model) workbenchKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	key := msg.String()
	if m.wb.searchOpen {
		return m.workbenchSearchKey(key)
	}
	switch m.wb.layer {
	case wbLayerPartyConfirm, wbLayerLoadoutConfirm, wbLayerEvolutionConfirm:
		return m.workbenchConfirmKey(key)
	case wbLayerNickname:
		return m.workbenchNicknameKey(key)
	case wbLayerPartySlot:
		return m.workbenchPartySlotKey(key)
	case wbLayerLoadout:
		return m.workbenchLoadoutKey(key)
	case wbLayerActions:
		return m.workbenchActionsKey(key)
	case wbLayerMoveNotice:
		return m.workbenchMoveNoticeKey(key)
	case wbLayerEvolution:
		return m.workbenchEvolutionKey(key)
	case wbLayerSpeciesFilter:
		return m.workbenchSpeciesFilterKey(key)
	default:
		return m.workbenchBrowseKey(key)
	}
}

func (m Model) workbenchBrowseKey(key string) (Model, tea.Cmd) {
	if m.wb.tab == wbTabTrainerStats && key != "tab" && key != "esc" {
		return m, nil
	}
	switch key {
	case "esc":
		if n := nextRequiredLesson(m.save); n != 0 {
			return m.startNextRequiredLesson()
		}
		m.screen = screenLobby
		return m, nil
	case "tab":
		m.wb.tab = (m.wb.tab + 1) % 3
		m.wb.cursor = 0
		if m.wb.tab == wbTabCollection {
			m.wb.syncSelection(&m)
		}
		if m.wb.tab == wbTabTrainerStats {
			return m, m.loadTrainerStatsCmd()
		}
		return m, nil
	case "/":
		m.wb.searchOpen = true
		return m, nil
	case "f":
		m.wb.filter = (m.wb.filter + 1) % 4
		m.wb.cursor = 0
		m.wb.syncSelection(&m)
		return m, nil
	case "s":
		if m.wb.tab == wbTabCollection {
			m.wb.sort = (m.wb.sort + 1) % 3
			m.wb.cursor = 0
			m.wb.syncSelection(&m)
		}
		return m, nil
	case "c":
		m.wb.search = ""
		m.wb.filter = wbFilterAll
		m.wb.speciesSlug = ""
		m.wb.cursor = 0
		m.wb.syncSelection(&m)
		return m, nil
	case "up", "k":
		m = m.withWBCursor(-1)
		return m, nil
	case "down", "j":
		m = m.withWBCursor(1)
		return m, nil
	case "enter", " ":
		if m.wb.tab == wbTabSpecies {
			families := evolutionFamilies(m.set)
			if len(families) == 0 {
				return m, nil
			}
			m.wb.speciesSlug = families[m.wb.cursor].chain[0]
			m.wb.layer = wbLayerSpeciesFilter
			m.wb.subCursor = 0
			return m, nil
		}
		if len(visibleMonsters(m, m.wb)) == 0 {
			return m, nil
		}
		m.wb.layer = wbLayerActions
		m.wb.subCursor = 0
		return m, nil
	case "p":
		if m.wb.tab != wbTabCollection || m.wb.selectedID == "" {
			return m, nil
		}
		m.wb.layer = wbLayerPartySlot
		m.wb.subCursor = 0
		return m, nil
	}
	return m, nil
}

func (m Model) workbenchActionsKey(key string) (Model, tea.Cmd) {
	actions := m.wbActionList()
	switch key {
	case "esc", "backspace":
		m.wb.layer = wbLayerBrowse
		return m, nil
	case "up", "k":
		if len(actions) > 0 {
			m.wb.subCursor = (m.wb.subCursor + len(actions) - 1) % len(actions)
		}
		return m, nil
	case "down", "j":
		if len(actions) > 0 {
			m.wb.subCursor = (m.wb.subCursor + 1) % len(actions)
		}
		return m, nil
	case "enter", " ":
		if len(actions) == 0 {
			return m, nil
		}
		switch actions[m.wb.subCursor] {
		case "Party":
			m.wb.layer = wbLayerPartySlot
			m.wb.subCursor = 0
		case "Moves":
			m.wb.layer = wbLayerLoadout
			m.wb.subCursor = 0
			m.wb.loadoutSlot = 0
			m.wb.pickMove = ""
		case "Nickname":
			m.wb.layer = wbLayerNickname
			if mon, ok := game.MonsterByID(m.save, m.wb.selectedID); ok {
				m.wb.nickInput = mon.Nickname
			}
		case "Review move unlock":
			m.wb.openMoveNotice(m)
		case "Review evolution":
			m.wb.layer = wbLayerEvolution
			m.wb.subCursor = 0
		case "Remove from Party":
			return m, m.hubSaveCmd(func() error {
				return m.hub.SetParty(m.hash, m.partyWithout(m.wb.selectedID))
			})
		}
		return m, nil
	}
	return m, nil
}

func (m Model) workbenchPartySlotKey(key string) (Model, tea.Cmd) {
	switch key {
	case "esc", "backspace":
		m.wb.layer = wbLayerBrowse
		return m, nil
	case "1", "2", "3":
		slot := int(key[0] - '1')
		return m.wbAssignPartySlot(m, slot)
	case "up", "k":
		m.wb.subCursor = (m.wb.subCursor + 2) % 3
		return m, nil
	case "down", "j":
		m.wb.subCursor = (m.wb.subCursor + 1) % 3
		return m, nil
	case "enter", " ":
		return m.wbAssignPartySlot(m, m.wb.subCursor)
	}
	return m, nil
}

func (m Model) wbAssignPartySlot(model Model, slot int) (Model, tea.Cmd) {
	if model.wb.selectedID == "" {
		return model, nil
	}
	currentSlot := partySlotOf(model.save, model.wb.selectedID)
	occupied := model.save.Party[slot]
	if occupied == model.wb.selectedID {
		return model, nil
	}
	if occupied != "" && currentSlot >= 0 && currentSlot != slot {
		next := model.save.Party
		next[slot], next[currentSlot] = next[currentSlot], next[slot]
		model.wb.layer = wbLayerBrowse
		return model, model.hubSaveCmd(func() error { return model.hub.SetParty(model.hash, next) })
	}
	if occupied != "" && currentSlot < 0 {
		model.wb.partySlot = slot
		if rep, ok := game.MonsterByID(model.save, occupied); ok {
			model.wb.confirmText = fmt.Sprintf("Replace %s in slot %d?", monsterDisplayName(model.set, rep), slot+1)
		}
		model.wb.layer = wbLayerPartyConfirm
		return model, nil
	}
	next := model.save.Party
	if currentSlot >= 0 {
		next[currentSlot] = ""
	}
	next[slot] = model.wb.selectedID
	model.wb.layer = wbLayerBrowse
	return model, model.hubSaveCmd(func() error { return model.hub.SetParty(model.hash, next) })
}

func (m Model) workbenchConfirmKey(key string) (Model, tea.Cmd) {
	switch key {
	case "esc", "n", "backspace":
		m.wb.layer = previousConfirmLayer(m.wb.layer)
		return m, nil
	case "y", "enter", " ":
		switch m.wb.layer {
		case wbLayerPartyConfirm:
			next := m.save.Party
			if cur := partySlotOf(m.save, m.wb.selectedID); cur >= 0 {
				next[cur] = ""
			}
			next[m.wb.partySlot] = m.wb.selectedID
			m.wb.layer = wbLayerBrowse
			return m, m.hubSaveCmd(func() error { return m.hub.SetParty(m.hash, next) })
		case wbLayerLoadoutConfirm:
			loadout := paddedLoadout(currentLoadout(m.save, m.wb.selectedID))
			loadout[m.wb.loadoutSlot] = m.wb.pickMove
			m.wb.layer = wbLayerLoadout
			noticeIDs := append([]string(nil), m.wb.ackNoticeIDs...)
			m.wb.ackNoticeIDs = nil
			return m, m.hubSaveCmd(func() error {
				return m.hub.SetBattleLoadout(m.hash, m.wb.selectedID, compactMoves(loadout), noticeIDs)
			})
		case wbLayerEvolutionConfirm:
			m.wb.layer = wbLayerBrowse
			return m, m.hubSaveCmd(func() error { return m.hub.AcceptEvolution(m.hash, m.wb.selectedID) })
		}
	}
	return m, nil
}

func previousConfirmLayer(layer int) int {
	switch layer {
	case wbLayerPartyConfirm:
		return wbLayerPartySlot
	case wbLayerLoadoutConfirm:
		return wbLayerLoadout
	case wbLayerEvolutionConfirm:
		return wbLayerEvolution
	default:
		return wbLayerBrowse
	}
}

func (m Model) workbenchLoadoutKey(key string) (Model, tea.Cmd) {
	lib := moveLibrary(m.save, m.wb.selectedID)
	loadout := paddedLoadout(currentLoadout(m.save, m.wb.selectedID))
	switch key {
	case "esc", "backspace":
		m.wb.layer = wbLayerActions
		m.wb.pickMove = ""
		return m, nil
	case "up", "k":
		if m.wb.subCursor < 4 {
			m.wb.subCursor = (m.wb.subCursor + 3) % 4
		} else {
			m.wb.subCursor = (m.wb.subCursor+len(lib)-1-4)%max(1, len(lib)) + 4
		}
		return m, nil
	case "down", "j":
		if m.wb.subCursor < 4 {
			m.wb.subCursor = (m.wb.subCursor + 1) % 4
		} else {
			m.wb.subCursor = (m.wb.subCursor-4+1)%max(1, len(lib)) + 4
		}
		return m, nil
	case "left", "h":
		if m.wb.subCursor >= 4 {
			m.wb.subCursor = m.wb.loadoutSlot
		}
		return m, nil
	case "right", "l":
		if m.wb.subCursor < 4 && len(lib) > 0 {
			m.wb.subCursor = 4
		}
		return m, nil
	case "1", "2", "3", "4":
		slot := int(key[0] - '1')
		m.wb.loadoutSlot = slot
		m.wb.subCursor = slot
		if m.wb.pickMove != "" {
			return m.wbApplyLoadoutMove(m)
		}
		return m, nil
	case "enter", " ":
		if m.wb.subCursor < 4 {
			m.wb.loadoutSlot = m.wb.subCursor
			if m.wb.pickMove != "" {
				return m.wbApplyLoadoutMove(m)
			}
			return m, nil
		}
		if len(lib) == 0 {
			return m, nil
		}
		move := lib[m.wb.subCursor-4]
		if moveEquipped(loadout, move, -1) {
			m.status = "move already equipped"
			m.statusHold = holdStatus
			return m, nil
		}
		m.wb.pickMove = move
		if m.wb.loadoutSlot < 4 && loadout[m.wb.loadoutSlot] == "" {
			return m.wbApplyLoadoutMove(m)
		}
		return m, nil
	case "d":
		if m.wb.subCursor >= 4 || loadout[m.wb.subCursor] == "" {
			return m, nil
		}
		remaining := 0
		for _, mv := range loadout {
			if mv != "" {
				remaining++
			}
		}
		if remaining <= 1 {
			m.status = "loadout needs at least one move"
			m.statusHold = holdStatus
			return m, nil
		}
		loadout[m.wb.subCursor] = ""
		return m, m.hubSaveCmd(func() error {
			return m.hub.SetBattleLoadout(m.hash, m.wb.selectedID, compactMoves(loadout), nil)
		})
	}
	return m, nil
}

func (m Model) wbApplyLoadoutMove(model Model) (Model, tea.Cmd) {
	loadout := paddedLoadout(currentLoadout(model.save, model.wb.selectedID))
	old := loadout[model.wb.loadoutSlot]
	if old != "" && old != model.wb.pickMove {
		model.wb.confirmText = fmt.Sprintf("%s -> %s", moveName(model.set, old), moveName(model.set, model.wb.pickMove))
		model.wb.layer = wbLayerLoadoutConfirm
		return model, nil
	}
	loadout[model.wb.loadoutSlot] = model.wb.pickMove
	model.wb.pickMove = ""
	noticeIDs := append([]string(nil), model.wb.ackNoticeIDs...)
	model.wb.ackNoticeIDs = nil
	cmd := model.hubSaveCmd(func() error {
		return model.hub.SetBattleLoadout(model.hash, model.wb.selectedID, compactMoves(loadout), noticeIDs)
	})
	return model, cmd
}

func (m Model) workbenchNicknameKey(key string) (Model, tea.Cmd) {
	switch key {
	case "esc":
		m.wb.layer = wbLayerActions
		return m, nil
	case "enter":
		nick, err := game.ValidateNickname(m.wb.nickInput)
		if err != nil {
			m.status = "invalid nickname"
			m.statusHold = holdStatus
			return m, nil
		}
		m.wb.layer = wbLayerBrowse
		return m, m.hubSaveCmd(func() error { return m.hub.SetNickname(m.hash, m.wb.selectedID, nick) })
	case "backspace":
		if r := []rune(m.wb.nickInput); len(r) > 0 {
			m.wb.nickInput = string(r[:len(r)-1])
		}
		return m, nil
	default:
		r := []rune(key)
		if len(r) == 1 && len([]rune(m.wb.nickInput)) < 16 {
			m.wb.nickInput += key
		}
	}
	return m, nil
}

func (m Model) workbenchMoveNoticeKey(key string) (Model, tea.Cmd) {
	notice := m.wbMoveUnlockNotice()
	switch key {
	case "esc", "backspace":
		m.wb.layer = wbLayerBrowse
		return m, nil
	case "up", "k", "down", "j":
		m.wb.subCursor = 1 - m.wb.subCursor
		return m, nil
	case "enter", " ":
		if m.wb.subCursor == 0 {
			m.wb.layer = wbLayerLoadout
			m.wb.subCursor = 4
			if len(notice.Moves) > 0 {
				m.wb.focusMove = notice.Moves[0]
				m.wb.pickMove = notice.Moves[0]
				lib := moveLibrary(m.save, m.wb.selectedID)
				for i, mv := range lib {
					if mv == notice.Moves[0] {
						m.wb.subCursor = i + 4
						break
					}
				}
			}
			return m, nil
		}
		return m, m.hubSaveCmd(func() error {
			return m.hub.AcknowledgeProgressionNotices(m.hash, []string{notice.ID})
		})
	}
	return m, nil
}

func (m Model) workbenchEvolutionKey(key string) (Model, tea.Cmd) {
	switch key {
	case "esc", "backspace":
		m.wb.layer = wbLayerBrowse
		return m, nil
	case "up", "k", "down", "j":
		m.wb.subCursor = 1 - m.wb.subCursor
		return m, nil
	case "enter", " ":
		if m.wb.subCursor == 1 {
			m.wb.layer = wbLayerBrowse
			return m, nil
		}
		m.wb.layer = wbLayerEvolutionConfirm
		m.wb.confirmText = "Evolve? This cannot be undone."
		return m, nil
	}
	return m, nil
}

func (m Model) workbenchSpeciesFilterKey(key string) (Model, tea.Cmd) {
	switch key {
	case "esc", "backspace":
		m.wb.layer = wbLayerBrowse
		return m, nil
	case "enter", " ":
		m.wb.tab = wbTabCollection
		m.wb.cursor = 0
		m.wb.layer = wbLayerBrowse
		m.wb.syncSelection(&m)
		return m, nil
	}
	return m, nil
}

func (m Model) workbenchSearchKey(key string) (Model, tea.Cmd) {
	switch key {
	case "esc":
		m.wb.searchOpen = false
		return m, nil
	case "enter":
		m.wb.searchOpen = false
		m.wb.cursor = 0
		m.wb.syncSelection(&m)
		return m, nil
	case "backspace":
		if r := []rune(m.wb.search); len(r) > 0 {
			m.wb.search = string(r[:len(r)-1])
		}
		m.wb.cursor = 0
		m.wb.syncSelection(&m)
		return m, nil
	default:
		r := []rune(key)
		if len(r) == 1 {
			m.wb.search += key
			m.wb.cursor = 0
			m.wb.syncSelection(&m)
		}
	}
	return m, nil
}

func (w *workbenchModel) openMoveNotice(m Model) {
	w.layer = wbLayerMoveNotice
	w.subCursor = 0
	if n := m.wbMoveUnlockNoticeFor(w.selectedID); n != nil {
		w.ackNoticeIDs = []string{n.ID}
		if len(n.Moves) > 0 {
			w.focusMove = n.Moves[0]
		}
	}
}

func (m Model) wbMoveUnlockNotice() game.ProgressionNotice {
	if n := m.wbMoveUnlockNoticeFor(m.wb.selectedID); n != nil {
		return *n
	}
	return game.ProgressionNotice{}
}

func (m Model) wbMoveUnlockNoticeFor(id string) *game.ProgressionNotice {
	if m.save == nil {
		return nil
	}
	for i := range m.save.Notices {
		n := &m.save.Notices[i]
		if n.MonsterID == id && n.Kind == "move_unlock" {
			return n
		}
	}
	return nil
}

func (m Model) withWBCursor(delta int) Model {
	if m.wb.tab == wbTabSpecies {
		n := len(evolutionFamilies(m.set))
		if n == 0 {
			return m
		}
		m.wb.cursor = (m.wb.cursor + delta + n) % n
		return m
	}
	ids := visibleMonsters(m, m.wb)
	if len(ids) == 0 {
		return m
	}
	m.wb.cursor = (m.wb.cursor + delta + len(ids)) % len(ids)
	m.wb.selectedID = ids[m.wb.cursor]
	return m
}

func (m Model) wbActionList() []string {
	var out []string
	if m.wb.selectedID == "" {
		return out
	}
	if partySlotOf(m.save, m.wb.selectedID) >= 0 {
		out = append(out, "Remove from Party")
	} else {
		out = append(out, "Party")
	}
	out = append(out, "Moves", "Nickname")
	if m.wbMoveUnlockNoticeFor(m.wb.selectedID) != nil {
		out = append(out, "Review move unlock")
	}
	if mon, ok := game.MonsterByID(m.save, m.wb.selectedID); ok && mon.EvolutionPending {
		out = append(out, "Review evolution")
	}
	return out
}

func (m Model) partyWithout(id string) [3]string {
	next := m.save.Party
	for i := range next {
		if next[i] == id {
			next[i] = ""
		}
	}
	return next
}

func (m Model) hubSaveCmd(fn func() error) tea.Cmd {
	return func() tea.Msg {
		if err := fn(); err != nil {
			return m.hub.ErrorMessage(m.hash, "workbench_change", err)
		}
		sv, err := m.hub.Load(m.hash)
		if err != nil {
			return m.hub.ErrorMessage(m.hash, "workbench_reload", err)
		}
		return server.SaveMsg{Save: sv}
	}
}

type trainerStatsMsg struct {
	trainer store.TrainerStats
	world   store.WorldStats
	err     error
}

func (m Model) loadTrainerStatsCmd() tea.Cmd {
	return func() tea.Msg {
		trainer, err := m.hub.TrainerStats(m.hash)
		if err != nil {
			return trainerStatsMsg{err: err}
		}
		world, err := m.hub.WorldStats()
		return trainerStatsMsg{trainer: trainer, world: world, err: err}
	}
}

func (m Model) renderWorkbench() string {
	if m.save == nil || m.set == nil {
		return "workbench…"
	}
	if m.wb.tab == wbTabTrainerStats {
		return m.renderWBTrainerStats()
	}
	centerW := max(20, m.width-wbPaneCollection-wbPaneParty-2)
	left := m.renderWBCollection(wbPaneCollection, m.height)
	center := m.renderWBCenter(centerW, m.height)
	right := m.renderWBParty(wbPaneParty, m.height)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, blank(1), center, blank(1), right)
}

func (m Model) renderWBTrainerStats() string {
	body := dimStyle.Render("Loading Trainer statistics…")
	if m.wb.statsError != "" {
		body = warnStyle.Render("Statistics unavailable") + "\n" + dimStyle.Render(m.wb.statsError)
	} else if stats := m.wb.trainerStats; stats != nil {
		matches := stats.Wins + stats.Losses
		winRate := 0.0
		if matches > 0 {
			winRate = float64(stats.Wins) * 100 / float64(matches)
		}
		streak := strconv.Itoa(stats.CurrentStreak)
		if stats.CurrentStreak > 0 {
			streak = fmt.Sprintf("W%d", stats.CurrentStreak)
		} else if stats.CurrentStreak < 0 {
			streak = fmt.Sprintf("L%d", -stats.CurrentStreak)
		}
		lines := []string{
			selStyle.Render("SUPPORT ID  ") + groupedSupportID(stats.TrainerID),
			"Trainer since  " + stats.CreatedAt.UTC().Format("2006-01-02"),
			"",
			fmt.Sprintf("Battles        %d", matches),
			fmt.Sprintf("Record         %dW / %dL  (%.1f%%)", stats.Wins, stats.Losses, winRate),
			"Current streak " + streak,
			fmt.Sprintf("Best streak    W%d", stats.LongestStreak),
			"",
			fmt.Sprintf("Collection     %d Monsters", stats.CollectionSize),
			fmt.Sprintf("Captures       %d", stats.Captures),
			fmt.Sprintf("Expeditions    %d", stats.Expeditions),
			fmt.Sprintf("Dojo clears    %d", stats.DojoClears),
			fmt.Sprintf("Mastery Marks  %d", stats.MasteryMarks),
			"",
			fmt.Sprintf("SSH sessions   %d", stats.Sessions),
			"Play time      " + compactDuration(stats.PlayTime),
		}
		if world := m.wb.worldStats; world != nil {
			lines = append(lines, "", titleStyle.Render("WORLD"),
				fmt.Sprintf("Registered Trainers  %d", world.RegisteredTrainers),
				fmt.Sprintf("Completed Battles    %d", world.CompletedBattles),
			)
		}
		body = strings.Join(lines, "\n")
	}
	footer := dimStyle.Render("tab: next · esc: Dojo")
	content := titleStyle.Render("Trainer Stats") + "\n\n" + body
	return chromeBox(max(20, m.width), place(max(1, m.width-4), max(1, m.height-4), content)+"\n"+footer)
}

func groupedSupportID(id string) string {
	id = strings.ToUpper(strings.ReplaceAll(id, "-", ""))
	parts := make([]string, 0, (len(id)+3)/4)
	for len(id) > 4 {
		parts = append(parts, id[:4])
		id = id[4:]
	}
	if id != "" {
		parts = append(parts, id)
	}
	return strings.Join(parts, "-")
}

func compactDuration(duration time.Duration) string {
	if duration < time.Minute {
		return "<1m"
	}
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	if hours == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

func terminalTooSmall(w, h int) string {
	return fmt.Sprintf("terminal too small: need ≥%d×%d, have %dx%d", minBattleWidth, minBattleHeight, w, h)
}

func (m Model) renderWBCollection(w, h int) string {
	title := "Collection"
	if m.wb.tab == wbTabSpecies {
		title = "Species Index"
	}
	head := titleStyle.Render(title)
	body := m.renderWBCollectionBody(w, h-2)
	return chromeBox(w, head+"\n"+body)
}

func (m Model) renderWBCollectionBody(w, h int) string {
	if m.wb.tab == wbTabSpecies {
		return m.renderWBSpeciesList(w, h)
	}
	ids := visibleMonsters(m, m.wb)
	if len(ids) == 0 {
		msg := "No owned Monsters match"
		if m.wb.search != "" || m.wb.filter != wbFilterAll || m.wb.speciesSlug != "" {
			return dimStyle.Render(msg) + "\n" + dimStyle.Render("c: clear filter")
		}
		return dimStyle.Render(msg)
	}
	var lines []string
	for i, id := range ids {
		mon, _ := game.MonsterByID(m.save, id)
		line := collectionRow(m, mon, i == m.wb.cursor)
		lines = append(lines, fitLine(line, max(1, w-4)))
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func collectionRow(m Model, mon game.Monster, sel bool) string {
	name := monsterDisplayName(m.set, mon)
	mark := ""
	if monsterNeedsAttention(m.save, mon) {
		mark = warnStyle.Render("*")
	}
	slot := partySlotOf(m.save, mon.ID)
	slotLabel := ""
	if slot >= 0 {
		slotLabel = dimStyle.Render(fmt.Sprintf(" P%d", slot+1))
	}
	row := fmt.Sprintf("L%d %s%s%s", mon.Level, name, slotLabel, mark)
	if sel {
		return menuChoice(true, row)
	}
	return menuChoice(false, row)
}

func (m Model) renderWBSpeciesList(_ int, h int) string {
	families := evolutionFamilies(m.set)
	var lines []string
	for i, fam := range families {
		sp, ok := m.set.Species[fam.chain[0]]
		if !ok {
			continue
		}
		stage := fmt.Sprintf("stage %d/%d", fam.stageIndex(fam.chain[0])+1, len(fam.chain))
		count := ownedSpeciesCount(m.save, fam.chain[0])
		row := fmt.Sprintf("%s · %s · owned %d", speciesDisplayName(sp), stage, count)
		if i == m.wb.cursor {
			lines = append(lines, menuChoice(true, row))
		} else {
			lines = append(lines, menuChoice(false, row))
		}
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderWBCenter(w, h int) string {
	head := "Selected Monster"
	body := m.renderWBCenterBody(w, h-2)
	return chromeBox(w, titleStyle.Render(head)+"\n"+body)
}

func (m Model) renderWBCenterBody(w, h int) string {
	switch m.wb.layer {
	case wbLayerLoadout, wbLayerLoadoutConfirm:
		if m.wb.layer == wbLayerLoadoutConfirm {
			return confirmBody(m.wb.confirmText, w, h)
		}
		return m.renderWBLoadoutEditor(w, h)
	case wbLayerPartyConfirm:
		return confirmBody(m.wb.confirmText, w, h)
	case wbLayerEvolutionConfirm:
		return confirmBody(m.wb.confirmText, w, h)
	case wbLayerNickname:
		return promptStyle.Render("Nickname: "+m.wb.nickInput) + typeMark(false, true, 0)
	case wbLayerMoveNotice:
		return m.renderWBMoveNotice(w, h)
	case wbLayerEvolution:
		return m.renderWBEvolution(w, h)
	case wbLayerPartySlot:
		return m.renderWBPartyChooser(w, h)
	case wbLayerActions:
		return m.renderWBActions(w, h)
	case wbLayerSpeciesFilter:
		return narrStyle.Render("Show owned "+speciesDisplayName(m.set.Species[m.wb.speciesSlug])+"?") + "\n" + menuCol(0, "Show owned")
	default:
		return m.renderWBMonsterDossier(w, h)
	}
}

func confirmBody(text string, w, h int) string {
	body := warnStyle.Render(text) + "\n\n" + menuRow(0, "YES", "NO")
	return place(w, h, body)
}

func (m Model) renderWBMonsterDossier(_ int, h int) string {
	mon, ok := game.MonsterByID(m.save, m.wb.selectedID)
	if !ok {
		return dimStyle.Render("Select a Monster")
	}
	sp, _ := m.set.Species[mon.Species]
	name := monsterDisplayName(m.set, mon)
	typ := sp.Type
	if td, ok := m.set.Types[typ]; ok && td.Name != "" {
		typ = td.Name
	}
	xpNext := game.XPForLevel(mon.Level + 1)
	if mon.Level >= 50 {
		xpNext = game.XPForLevel(50)
	}
	lines := []string{
		selStyle.Render(strings.ToUpper(name)),
		typeInk(typ).Render(strings.ToUpper(typ)) + dimStyle.Render(" · "+strings.ToUpper(sp.Name)),
		fmt.Sprintf("Level %d  XP %d/%d", mon.Level, mon.XP, xpNext),
	}
	lines = append(lines, m.renderWBStats(sp, mon.Level)...)
	if art, ok := m.set.Arts[mon.Species]; ok {
		anim := sprite.CompileOn(art, sp.Type, true, screenBgHex)
		pose := sprite.PoseIdleA
		spriteLines := anim.Frames[pose]
		if len(spriteLines) > 0 && h > len(lines)+len(spriteLines) {
			lines = append(lines, strings.Join(spriteLines, "\n"))
		}
	}
	loadout := strings.Join(loadoutNames(m.set, mon.BattleLoadout), ", ")
	if loadout == "" {
		loadout = "empty"
	}
	lines = append(lines, dimStyle.Render("Loadout: "+loadout))
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderWBStats(sp content.Species, level int) []string {
	return []string{
		fmt.Sprintf("HP %d  ATK %d  DEF %d", game.NaturalStat(sp.BaseStats.HP, level),
			game.NaturalStat(sp.BaseStats.Attack, level), game.NaturalStat(sp.BaseStats.Defense, level)),
		fmt.Sprintf("SPA %d  SPD %d", game.NaturalStat(sp.BaseStats.SpAttack, level),
			game.NaturalStat(sp.BaseStats.Speed, level)),
	}
}

func (m Model) renderWBActions(w, h int) string {
	actions := m.wbActionList()
	body := menuCol(m.wb.subCursor, actions...)
	return place(w, h, body)
}

func (m Model) renderWBPartyChooser(w, h int) string {
	labels := []string{
		"1 Opening lead (slot 1)",
		"2 Slot 2",
		"3 Slot 3",
	}
	return place(w, h, menuCol(m.wb.subCursor, labels...))
}

func (m Model) renderWBLoadoutEditor(w, h int) string {
	lib := moveLibrary(m.save, m.wb.selectedID)
	loadout := paddedLoadout(currentLoadout(m.save, m.wb.selectedID))
	leftW := max(16, w/3)
	rightW := max(1, w-leftW-1)
	var left []string
	for i := range 4 {
		label := strconv.Itoa(i+1) + " "
		mv := loadout[i]
		if mv == "" {
			label += "-"
		} else {
			label += moveName(m.set, mv)
		}
		left = append(left, menuChoice(m.wb.subCursor == i, label))
	}
	var right []string
	for i, mv := range lib {
		row := moveLibraryRow(m.set, mv, loadout, m.wb.subCursor == i+4, mv == m.wb.focusMove)
		right = append(right, row)
	}
	leftBody := strings.Join(left, "\n")
	rightBody := strings.Join(right, "\n")
	if lipgloss.Height(leftBody) > h {
		leftLines := strings.Split(leftBody, "\n")
		leftBody = strings.Join(leftLines[:h], "\n")
	}
	if lipgloss.Height(rightBody) > h {
		rightLines := strings.Split(rightBody, "\n")
		rightBody = strings.Join(rightLines[:h], "\n")
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		chromeBox(leftW, leftBody),
		blank(1),
		chromeBox(rightW, rightBody),
	)
}

func moveLibraryRow(set *content.Set, slug string, loadout [4]string, sel, focus bool) string {
	mv, ok := set.Moves[slug]
	if !ok {
		return menuChoice(sel, slug)
	}
	mark := ""
	if moveEquipped(loadout, slug, -1) {
		mark = " *"
	}
	if focus {
		mark += " <"
	}
	row := fmt.Sprintf("%s %s %s pwr%.0f acc%.0f%s", mv.Name, mv.Type, mv.Category, mv.Power, mv.Accuracy, mark)
	return menuChoice(sel, row)
}

func (m Model) renderWBMoveNotice(w, h int) string {
	n := m.wbMoveUnlockNotice()
	body := narrStyle.Render("Move added to Move Library. Battle Loadout unchanged.") + "\n"
	if len(n.Moves) > 0 {
		body += dimStyle.Render("New: "+moveName(m.set, n.Moves[0])) + "\n"
	}
	body += menuCol(m.wb.subCursor, "Edit loadout", "Keep loadout")
	return place(w, h, body)
}

func (m Model) renderWBEvolution(w, h int) string {
	mon, ok := game.MonsterByID(m.save, m.wb.selectedID)
	if !ok {
		return ""
	}
	sp := m.set.Species[mon.Species]
	if sp.EvolvesTo == nil {
		return dimStyle.Render("No evolution available")
	}
	next := m.set.Species[sp.EvolvesTo.Species]
	var lines []string
	lines = append(lines, "Now: "+speciesDisplayName(sp))
	lines = append(lines, "Next: "+speciesDisplayName(next))
	lines = append(lines, fmt.Sprintf("HP %d -> %d", game.NaturalStat(sp.BaseStats.HP, mon.Level), game.NaturalStat(next.BaseStats.HP, mon.Level)))
	if m.wb.layer == wbLayerEvolutionConfirm {
		lines = append(lines, "", warnStyle.Render(m.wb.confirmText))
		lines = append(lines, menuRow(0, "YES", "NO"))
	} else {
		lines = append(lines, "", menuCol(m.wb.subCursor, "Evolve", "Defer"))
	}
	return place(w, h, strings.Join(lines, "\n"))
}

func (m Model) renderWBParty(w, _ int) string {
	head := titleStyle.Render("Party")
	var lines []string
	for i := range 3 {
		label := strconv.Itoa(i + 1)
		if i == 0 {
			label += " LEAD"
		}
		id := m.save.Party[i]
		if id == "" {
			lines = append(lines, dimStyle.Render(label+": -"))
			continue
		}
		mon, ok := game.MonsterByID(m.save, id)
		if !ok {
			lines = append(lines, dimStyle.Render(label+": ?"))
			continue
		}
		name := monsterDisplayName(m.set, mon)
		typ := m.set.Species[mon.Species].Type
		load := strings.Join(loadoutNames(m.set, mon.BattleLoadout), "/")
		lines = append(lines, fitLine(fmt.Sprintf("%s %s L%d %s", label, name, mon.Level, typ), wbPaneParty-4))
		if load != "" {
			lines = append(lines, dimStyle.Render(fitLine(load, wbPaneParty-4)))
		}
	}
	body := strings.Join(lines, "\n")
	return chromeBox(w, head+"\n"+body)
}

func (m Model) workbenchFooter() string {
	if m.wb.searchOpen {
		return hintLine(keyHint("enter", "done"), keyHint("esc", "cancel")) + "  " + promptStyle.Render("/"+m.wb.search)
	}
	switch m.wb.layer {
	case wbLayerLoadout, wbLayerLoadoutConfirm:
		return hintLine(keyHint("arrows", "move"), keyHint("enter", "assign"), keyHint("d", "clear slot"), keyHint("esc", "back"))
	case wbLayerPartySlot, wbLayerPartyConfirm:
		return hintLine(keyHint("1-3", "slot"), keyHint("enter", "choose"), keyHint("esc", "back"))
	case wbLayerNickname:
		return hintLine(keyHint("enter", "save"), keyHint("esc", "back"))
	case wbLayerActions:
		return hintLine(keyHint("enter", "choose"), keyHint("esc", "back"))
	case wbLayerMoveNotice:
		return hintLine(keyHint("enter", "choose"), keyHint("esc", "back"))
	case wbLayerEvolution, wbLayerEvolutionConfirm:
		return hintLine(keyHint("enter", "choose"), keyHint("esc", "back"))
	default:
		filterNames := []string{"ALL", "PARTY", "BENCH", "ATTENTION"}
		sortNames := []string{"RECENT", "LEVEL", "SPECIES"}
		esc := "dojo"
		if nextRequiredLesson(m.save) != 0 {
			esc = "lesson"
		}
		return hintLine(
			keyHint("/", "search"),
			keyHint("f", filterNames[m.wb.filter]),
			keyHint("s", sortNames[m.wb.sort]),
			keyHint("tab", "tab"),
			keyHint("p", "party"),
			keyHint("esc", esc),
		)
	}
}

// --- collection helpers ---

func visibleMonsters(m Model, w workbenchModel) []string {
	if m.save == nil {
		return nil
	}
	var ids []string
	for _, mon := range m.save.Collection {
		if w.speciesSlug != "" && mon.Species != w.speciesSlug {
			continue
		}
		if w.search != "" && !matchesSearch(m.set, mon, w.search) {
			continue
		}
		switch w.filter {
		case wbFilterParty:
			if partySlotOf(m.save, mon.ID) < 0 {
				continue
			}
		case wbFilterBench:
			if partySlotOf(m.save, mon.ID) >= 0 {
				continue
			}
		case wbFilterAttention:
			if !monsterNeedsAttention(m.save, mon) {
				continue
			}
		}
		ids = append(ids, mon.ID)
	}
	sortMonsterIDs(m, w, ids)
	return ids
}

func sortMonsterIDs(m Model, w workbenchModel, ids []string) {
	order := map[string]int{}
	for i, mon := range m.save.Collection {
		order[mon.ID] = i
	}
	switch w.sort {
	case wbSortLevel:
		slices.SortFunc(ids, func(a, b string) int {
			ma, _ := game.MonsterByID(m.save, a)
			mb, _ := game.MonsterByID(m.save, b)
			if ma.Level != mb.Level {
				return cmp.Compare(mb.Level, ma.Level)
			}
			return cmp.Compare(order[b], order[a])
		})
	case wbSortSpecies:
		slices.SortFunc(ids, func(a, b string) int {
			ma, _ := game.MonsterByID(m.save, a)
			mb, _ := game.MonsterByID(m.save, b)
			an := strings.ToLower(speciesDisplayName(m.set.Species[ma.Species]))
			bn := strings.ToLower(speciesDisplayName(m.set.Species[mb.Species]))
			if an != bn {
				return strings.Compare(an, bn)
			}
			return cmp.Compare(order[b], order[a])
		})
	default:
		slices.SortFunc(ids, func(a, b string) int { return cmp.Compare(order[b], order[a]) })
	}
}

func firstVisibleMonsterID(m Model, w workbenchModel) string {
	ids := visibleMonsters(m, w)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func indexOfMonster(m Model, id string, w workbenchModel) int {
	ids := visibleMonsters(m, w)
	for i, got := range ids {
		if got == id {
			return i
		}
	}
	return 0
}

func matchesSearch(set *content.Set, mon game.Monster, q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return true
	}
	if strings.Contains(strings.ToLower(mon.Nickname), q) {
		return true
	}
	if sp, ok := set.Species[mon.Species]; ok {
		return strings.Contains(strings.ToLower(sp.Name), q) || strings.Contains(strings.ToLower(mon.Species), q)
	}
	return strings.Contains(strings.ToLower(mon.Species), q)
}

func partySlotOf(save *game.Save, id string) int {
	if save == nil {
		return -1
	}
	for i, slot := range save.Party {
		if slot == id {
			return i
		}
	}
	return -1
}

func monsterNeedsAttention(save *game.Save, mon game.Monster) bool {
	if mon.EvolutionPending {
		return true
	}
	for _, n := range save.Notices {
		if n.MonsterID == mon.ID && (n.Kind == "move_unlock" || n.Kind == "capture_review") {
			return true
		}
	}
	return false
}

func monsterDisplayName(set *content.Set, mon game.Monster) string {
	if mon.Nickname != "" {
		return mon.Nickname
	}
	if sp, ok := set.Species[mon.Species]; ok && sp.Name != "" {
		return sp.Name
	}
	return mon.Species
}

func speciesDisplayName(sp content.Species) string {
	if sp.Name != "" {
		return sp.Name
	}
	return sp.Slug
}

func moveName(set *content.Set, slug string) string {
	if mv, ok := set.Moves[slug]; ok && mv.Name != "" {
		return mv.Name
	}
	return slug
}

func loadoutNames(set *content.Set, moves []string) []string {
	out := make([]string, 0, len(moves))
	for _, mv := range moves {
		if mv != "" {
			out = append(out, moveName(set, mv))
		}
	}
	return out
}

func currentLoadout(save *game.Save, id string) []string {
	mon, ok := game.MonsterByID(save, id)
	if !ok {
		return nil
	}
	return append([]string(nil), mon.BattleLoadout...)
}

func moveLibrary(save *game.Save, id string) []string {
	mon, ok := game.MonsterByID(save, id)
	if !ok {
		return nil
	}
	return append([]string(nil), mon.MoveLibrary...)
}

func paddedLoadout(loadout []string) [4]string {
	var out [4]string
	for i := 0; i < 4 && i < len(loadout); i++ {
		out[i] = loadout[i]
	}
	return out
}

func compactMoves(loadout [4]string) []string {
	var out []string
	for _, mv := range loadout {
		if mv != "" {
			out = append(out, mv)
		}
	}
	return out
}

func moveEquipped(loadout [4]string, move string, skip int) bool {
	for i, mv := range loadout {
		if i == skip {
			continue
		}
		if mv == move {
			return true
		}
	}
	return false
}

func ownedSpeciesCount(save *game.Save, species string) int {
	n := 0
	for _, mon := range save.Collection {
		if mon.Species == species {
			n++
		}
	}
	return n
}

type evolutionFamily struct {
	chain []string
}

func (f evolutionFamily) stageIndex(slug string) int {
	return slices.Index(f.chain, slug)
}

func evolutionFamilies(set *content.Set) []evolutionFamily {
	if set == nil {
		return nil
	}
	successor := map[string]bool{}
	for _, sp := range set.Species {
		if sp.EvolvesTo != nil {
			successor[sp.EvolvesTo.Species] = true
		}
	}
	var bases []string
	for slug := range set.Species {
		if !successor[slug] {
			bases = append(bases, slug)
		}
	}
	slices.Sort(bases)
	out := make([]evolutionFamily, 0, len(bases))
	for _, base := range bases {
		var chain []string
		cur := base
		for {
			chain = append(chain, cur)
			sp, ok := set.Species[cur]
			if !ok || sp.EvolvesTo == nil {
				break
			}
			cur = sp.EvolvesTo.Species
		}
		out = append(out, evolutionFamily{chain: chain})
	}
	return out
}
