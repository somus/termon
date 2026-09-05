// Package tui owns the per-session Bubble Tea state machine for onboarding,
// Lobby, Workbench, Dojo, Expedition, Queue, and Battle flows.
package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"termon.sh/internal/content"
	"termon.sh/internal/game"
	"termon.sh/internal/lobby"
	"termon.sh/internal/onboard"
	"termon.sh/internal/server"
)

type tickMsg struct{}

type onboardedMsg struct {
	save *game.Save
}

type resetMsg struct{}

type modalKind int

const (
	modalNone modalKind = iota
	modalStats
	modalHelp
)

type movedMsg struct {
	snapshot server.SnapshotMsg
}

type screen int

const (
	screenOnboard screen = iota
	screenLobby
	screenDojoMenu
	screenQueueEditor
	screenProgression
	screenQueue
	screenBattle
	screenWorkbench
	screenSignalBoard
	screenExpedition
	screenDisplaced

	holdStatus = 30
)

// Model owns root routing for one SSH session. screen selects exactly one key
// and view route; child models own state within that route, while Model
// arbitrates server messages that may preempt or defer the active route.
type Model struct {
	width, height  int
	hash           string
	set            *content.Set
	hub            *server.Hub
	save           *game.Save
	screen         screen
	onboard        onboardModel
	snap           server.SnapshotMsg
	queue          server.QueueMsg
	battle         battleScreenModel
	emotePick      bool
	status         string
	statusHold     int
	wipeHold       int
	resetArm       int
	tutorial       bool
	reconnecting   string
	lobbyWalk      map[string]walkAnimation
	lobbyLastMove  map[string]int
	lobbyAge       int
	lobbyCameraX   int
	lobbyCameraY   int
	lobbyCameraOn  bool
	wb             workbenchModel
	dojo           dojoMenuModel
	queueEd        queueEditorModel
	progression    progressionModel
	signalBoard    signalBoardModel
	expeditionFlow expeditionFlowModel
	modal          modalKind

	// Memoized frame. Update marks frameDirty whenever visible state may
	// have changed and rebuilds the painted frame; View reuses it verbatim
	// otherwise. frameBuilds counts rebuilds for test observation.
	frame       string
	frameDirty  bool
	frameW      int
	frameH      int
	frameBuilds int

	outputBusy     func() bool
	frameSkipped   func()
	renderDeferred bool
}

// New starts onboarding when save is nil, otherwise the Dojo.
func New(hash string, save *game.Save, set *content.Set, hub *server.Hub) Model {
	m := Model{hash: hash, save: save, set: set, hub: hub, onboard: newOnboard(set)}
	if save != nil {
		m.screen = screenLobby
	}
	return m
}

// WithOutputPressure suppresses cosmetic painting while ordered SSH output is
// pending. Callbacks must be fast and non-blocking. State and commands still
// advance on every tick; direct input and Hub messages always rebuild normally.
func (m Model) WithOutputPressure(busy func() bool, skipped func()) Model {
	m.outputBusy, m.frameSkipped = busy, skipped
	return m
}

func clockCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m Model) battleContext() battleScreenContext {
	return battleScreenContext{
		width:       m.width,
		height:      m.height,
		set:         m.set,
		tutorial:    m.tutorial,
		active:      m.screen == screenBattle,
		wipeHold:    m.wipeHold,
		youInBattle: m.snap.You.InBattle,
		nextLesson:  nextRequiredLesson(m.save),
	}
}

// Init seats returning Trainers in the Dojo and starts sprite ticks.
func (m Model) Init() tea.Cmd {
	if m.save == nil {
		return clockCmd()
	}
	return tea.Batch(clockCmd(), func() tea.Msg {
		return m.hub.Resume(m.hash)
	})
}

// Update handles keys and hub messages. It rebuilds the memoized frame when
// the message (or an animation this tick advanced) can change what View draws.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.handleUpdate(msg)
	updated, ok := model.(Model)
	if !ok {
		return model, cmd
	}
	if _, tick := msg.(tickMsg); tick && updated.outputBusy != nil && updated.outputBusy() && updated.frame != "" {
		updated.renderDeferred = updated.renderDeferred || updated.frameDirty
		updated.frameDirty = false
		if updated.frameSkipped != nil {
			updated.frameSkipped()
		}
		return updated, cmd
	}
	updated.frameDirty = updated.frameDirty || updated.renderDeferred
	updated.renderDeferred = false
	if updated.frameDirty || updated.frame == "" ||
		updated.frameW != updated.width || updated.frameH != updated.height {
		updated.buildFrame()
	}
	return updated, cmd
}

// handleUpdate is the message switch; it only marks frameDirty and mutates
// state. Frame construction happens in Update so the cache survives in the
// returned model value.
func (m Model) handleUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.frameDirty = true
	if updated, cmd, handled := m.handleInfoModalInput(msg); handled {
		return updated, cmd
	}
	switch msg := msg.(type) {
	case tickMsg:
		touched := m.tickTouchesFrame()
		m.onboard.age++
		m.onboard.lineAge++
		m.lobbyAge++
		for hash, animation := range m.lobbyWalk {
			animation.remaining--
			if animation.remaining <= 0 {
				delete(m.lobbyWalk, hash)
			} else {
				m.lobbyWalk[hash] = animation
			}
		}
		if m.statusHold > 0 {
			m.statusHold--
			if m.statusHold == 0 {
				m.status = ""
			}
		}
		if m.resetArm > 0 {
			m.resetArm--
		}
		wipeFinished := false
		if m.wipeHold > 0 {
			m.wipeHold--
			wipeFinished = m.wipeHold == 0
		}
		ctx := m.battleContext()
		ctx.wipeFinished = wipeFinished
		cmds := m.applyBattleUpdate(m.battle.update(battleTickInput{}, ctx))
		m.frameDirty = touched
		return m, tea.Batch(append(cmds, clockCmd())...)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.syncLobbyCamera(m.snap.You, true)
	case server.SnapshotMsg:
		m.markLobbyMovement(msg)
		m.syncLobbyCamera(msg.You, false)
		m.snap = msg
		if m.screen == screenWorkbench || m.holdsLobbyOverlay() {
			return m, nil
		}
		if msg.You.InQueue || m.screen == screenOnboard || m.screen == screenDisplaced {
			return m, nil
		}
		if m.holdsBattleScreen() {
			return m, nil
		}
		battleScreen := m.battle.withContext(m.battleContext())
		if m.screen == screenBattle && battleScreen.hasBattle() {
			return m, nil
		}
		if m.screen != screenBattle || !msg.You.InBattle {
			m.screen = screenLobby
		}
		if n := nextRequiredLesson(m.save); n != 0 && !msg.You.InBattle &&
			m.screen == screenLobby && !battleScreen.hasBattle() && !battleScreen.hasDeferredBattle() {
			m.tutorial = true
			return m, m.hubCmd(func() error { return m.hub.StartRequiredLesson(m.hash, n) })
		}
	case movedMsg:
		if strings.HasPrefix(m.status, "lobby:") {
			m.status = ""
			m.statusHold = 0
		}
		return m.handleUpdate(msg.snapshot)
	case server.QueueMsg:
		m.queue = msg
		m.screen = screenQueue
	case server.ReconnectingMsg:
		m.reconnecting = msg.Handle
	case server.BattleMsg:
		if (m.screen == screenProgression || m.screen == screenExpedition) &&
			m.battle.matchesBattle(msg.Battle) {
			return m, nil
		}
		session := battleSessionFrom(msg)
		return m, tea.Batch(m.applyBattleUpdate(m.battle.update(
			battleStartInput{session: session},
			m.battleContext(),
		))...)
	case server.SaveMsg:
		m.save = msg.Save
		if m.screen == screenWorkbench {
			m.wb.syncSelection(&m)
		}
	case trainerStatsMsg:
		if msg.err != nil {
			m.wb.statsError = server.UserMessage(msg.err)
		} else {
			m.wb.trainerStats = &msg.trainer
			m.wb.worldStats = &msg.world
			m.wb.statsError = ""
		}
	case server.CaptureStateMsg:
		return m, tea.Batch(m.applyBattleUpdate(m.battle.update(battleCaptureInput{state: msg}, m.battleContext()))...)
	case server.ProgressionMsg:
		return m, tea.Batch(m.applyBattleUpdate(m.battle.update(
			battleProgressionInput{model: progressionModel{msg: msg}},
			m.battleContext(),
		))...)
	case sparringPreviewMsg:
		m.dojo.preview = msg.preview
		m.dojo.tier = msg.tier
		m.dojo.view = dojoViewSparringPreview
		m.dojo.cursor = 0
	case server.DecisionExplanationMsg:
		m.status = msg.Text
		m.statusHold = holdStatus
	case server.DojoMenuMsg:
		m.openDojoMenu(msg)
	case server.SignalBoardMsg:
		m.openSignalBoard(msg)
	case server.ExpeditionMsg:
		return m, tea.Batch(m.applyBattleUpdate(m.battle.update(battleExpeditionInput{
			model:              expeditionFlowModel{msg: msg},
			waitForProgression: msg.Phase == "captured" && m.screen == screenProgression,
		}, m.battleContext()))...)
	case server.LessonIntentMsg:
		m.status = msg.Text
		m.statusHold = holdStatus
	case server.DisplacedMsg:
		// Show the notice; quitting is the q handler's job below.
		m.screen = screenDisplaced
	case server.ErrorMsg:
		m.status = msg.Text
		m.statusHold = holdStatus
		if m.save != nil && m.screen == screenOnboard {
			if n := nextRequiredLesson(m.save); n != 0 {
				m.tutorial = true
				return m, m.hubCmd(func() error { return m.hub.StartRequiredLesson(m.hash, n) })
			}
			m.tutorial = false
			m.screen = screenLobby
			m.wipeHold = holdWipe
		}
	case onboardedMsg:
		m.save = msg.save
		m.tutorial = true
		m.snap = m.hub.Snapshot(m.hash)
		if n := nextRequiredLesson(m.save); n != 0 {
			return m, m.hubCmd(func() error { return m.hub.StartRequiredLesson(m.hash, n) })
		}
		m.tutorial = false
		m.screen = screenLobby
		m.wipeHold = holdWipe
		return m, nil
	case resetMsg:
		m.save = nil
		m.onboard = newOnboard(m.set)
		m.screen = screenOnboard
		m.status = ""
		m.statusHold = 0
		m.tutorial = false
	case tea.KeyMsg:
		if _, ok := msg.(tea.KeyReleaseMsg); ok {
			return m, nil
		}
		return m.key(msg)
	}
	return m, nil
}

func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.screen {
	case screenOnboard:
		if msg.String() == "q" && m.onboard.stage == stageWelcome {
			return m, tea.Quit
		}
		next, done := m.onboard.update(msg)
		m.onboard = next
		if done {
			handle := m.onboard.handle
			starter := onboard.StarterSlugs[m.onboard.starter]
			return m, func() tea.Msg {
				sv, err := m.hub.CompleteOnboard(m.hash, handle, starter)
				if err != nil {
					return m.hub.ErrorMessage(m.hash, "complete_onboarding", err)
				}
				return onboardedMsg{save: sv}
			}
		}
	case screenDojoMenu:
		return m.dojoMenuKey(msg)
	case screenSignalBoard:
		return m.signalBoardKey(msg)
	case screenExpedition:
		next, cmd := m.expeditionFlowKey(msg.String())
		return next, cmd
	case screenQueueEditor:
		return m.queueEditorKey(msg.String())
	case screenProgression:
		return m.progressionKey(msg.String())
	case screenLobby:
		return m.lobbyKey(msg)
	case screenWorkbench:
		return m.workbenchKey(msg)
	case screenQueue:
		if msg.String() == "x" || msg.String() == "q" {
			return m, func() tea.Msg {
				m.hub.CancelQueue(m.hash)
				return m.hub.Snapshot(m.hash)
			}
		}
	case screenBattle:
		return m.battleKey(msg)
	case screenDisplaced:
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) lobbyKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() != "n" {
		m.resetArm = 0
	}
	if m.snap.Offer != nil {
		switch msg.String() {
		case "y":
			return m, m.hubCmd(func() error { return m.hub.Respond(m.hash, true) })
		case "n":
			return m, m.hubCmd(func() error { return m.hub.Respond(m.hash, false) })
		}
		return m, nil
	}
	if m.emotePick {
		switch msg.String() {
		case "esc", "e":
			m.emotePick = false
		case "1", "2", "3", "4", "5":
			text := emotes[msg.String()[0]-'1']
			m.emotePick = false
			return m, func() tea.Msg {
				m.hub.Emote(m.hash, text)
				return m.hub.Snapshot(m.hash)
			}
		}
		return m, nil
	}
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "n":
		if m.resetArm == 0 {
			m.resetArm = holdStatus
			m.status = "press n again to erase your save"
			m.statusHold = holdStatus
			return m, nil
		}
		return m, func() tea.Msg {
			if err := m.hub.ResetTrainer(m.hash); err != nil {
				return m.hub.ErrorMessage(m.hash, "reset_trainer", err)
			}
			return resetMsg{}
		}
	}
	if n := nextRequiredLesson(m.save); n != 0 {
		if msg.String() == "p" {
			m.openWorkbench()
			return m, nil
		}
		return m.startNextRequiredLesson()
	}
	switch msg.String() {
	case "e":
		m.emotePick = true
	case "c":
		return m, m.hubCmd(func() error { return m.hub.Challenge(m.hash) })
	case "f":
		if err := m.openQueueEditor(); err != nil {
			return m, m.hubCmd(func() error { return err })
		}
		return m, nil
	case "enter":
		if m.snap.Context != "" && strings.Contains(m.snap.Context, "Master Sable") {
			return m, func() tea.Msg {
				menu, err := m.hub.OpenDojoMenu(m.hash)
				if err != nil {
					return m.hub.ErrorMessage(m.hash, "open_dojo_menu", err)
				}
				return menu
			}
		}
		if m.snap.Context != "" && strings.Contains(m.snap.Context, "Signal Board") {
			return m, func() tea.Msg {
				board, err := m.hub.OpenSignalBoard(m.hash)
				if err != nil {
					return m.hub.ErrorMessage(m.hash, "open_signal_board", err)
				}
				return board
			}
		}
	case "p":
		m.openWorkbench()
		return m, nil
	case "up", "k", "w":
		return m, m.moveCmd(lobby.North)
	case "down", "j", "s":
		return m, m.moveCmd(lobby.South)
	case "left", "h", "a":
		return m, m.moveCmd(lobby.West)
	case "right", "l", "d":
		return m, m.moveCmd(lobby.East)
	}
	return m, nil
}

func (m Model) moveCmd(d lobby.Dir) tea.Cmd {
	return func() tea.Msg {
		if err := m.hub.Move(m.hash, d); err != nil {
			return m.hub.ErrorMessage(m.hash, "move", err)
		}
		return movedMsg{snapshot: m.hub.Snapshot(m.hash)}
	}
}

func (m Model) hubCmd(fn func() error) tea.Cmd {
	return func() tea.Msg {
		if err := fn(); err != nil {
			return m.hub.ErrorMessage(m.hash, "command", err)
		}
		return m.hub.Snapshot(m.hash)
	}
}

func (m Model) battleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cmds := m.applyBattleUpdate(m.battle.update(battleKeyInput{msg: msg}, m.battleContext()))
	return m, tea.Batch(cmds...)
}

func battleSessionFrom(msg server.BattleMsg) battleSession {
	return battleSession{
		battle:          msg.Battle,
		you:             msg.You,
		foe:             msg.Foe,
		foeHash:         msg.FoeHash,
		expeditionPhase: msg.ExpeditionPhase,
	}
}

func (m *Model) applyBattleUpdate(result battleUpdateResult) []tea.Cmd {
	m.battle = result.model
	if result.startWipe {
		m.wipeHold = holdWipe
	}
	if result.activated {
		m.reconnecting = ""
		m.screen = screenBattle
	}
	if result.progression != nil {
		m.progression = *result.progression
		m.screen = screenProgression
		m.frame = ""
		m.frameDirty = true
	}
	if result.expedition != nil {
		m.openExpeditionFlow(result.expedition.msg)
	}
	if result.clearTutorial {
		m.tutorial = false
	}

	var cmds []tea.Cmd
	if result.advanceReveal {
		cmds = append(cmds, m.hubCmd(func() error { return m.hub.AdvanceReveal(m.hash) }))
	}
	switch result.command.kind {
	case battleCommandNone:
	case battleCommandForfeit:
		cmds = append(cmds, m.hubCmd(func() error { return m.hub.Forfeit(m.hash) }))
	case battleCommandSelect:
		action := result.command.action
		cmds = append(cmds, m.hubCmd(func() error { return m.hub.SelectAction(m.hash, action) }))
	case battleCommandReplace:
		monsterID := result.command.monsterID
		cmds = append(cmds, m.hubCmd(func() error { return m.hub.Replace(m.hash, monsterID) }))
	case battleCommandStartLesson:
		lesson := result.command.lesson
		m.tutorial = true
		cmds = append(cmds, m.hubCmd(func() error { return m.hub.StartRequiredLesson(m.hash, lesson) }))
	case battleCommandEnterLobby:
		cmds = append(cmds, func() tea.Msg {
			if err := m.hub.EnterLobby(m.hash); err != nil {
				return server.ErrorMsg{Text: server.UserMessage(err)}
			}
			return m.hub.Snapshot(m.hash)
		})
	}
	return cmds
}

// tickTouchesFrame reports whether this tick changes something the next View
// would draw, judged from pre-tick state. Anything with a running countdown or
// active animation is treated as dirty (cheap) rather than predicting the
// exact tick the pixels move. Audited against every mutation in the tickMsg
// branch:
//
//   - status: footer renders it until it clears, including the clearing tick.
//   - screenOnboard: typewriter text, blinking cursors/marks, and idle sprite
//     poses all derive from onboard.age/lineAge indefinitely.
//   - screenBattle: battleAge drives idle pose alternation ((age/6)%2), playback
//     typing/shake/faint animation, and hold-driven wipes; never static.
//   - screenLobby and lobby overlays: the wipe transition overlays the floor;
//     walkers animate legs while lobbyWalk entries exist. Plain lobbyAge is
//     only movement bookkeeping.
//   - displaced: static between messages; emote expiry arrives via server
//     snapshots, which are non-tick messages and always mark dirty.
//   - bell/lowHP only affect battle rendering; resetArm and lastYouHP are
//     never rendered.
func (m Model) tickTouchesFrame() bool {
	if m.status != "" {
		return true
	}
	switch m.screen {
	case screenOnboard, screenBattle:
		return true
	case screenLobby, screenDojoMenu, screenQueueEditor, screenProgression, screenQueue:
		return m.wipeHold > 0 || len(m.lobbyWalk) > 0
	case screenWorkbench:
		return false
	default:
		return false
	}
}

func (m Model) holdsLobbyOverlay() bool {
	switch m.screen {
	case screenDojoMenu, screenQueueEditor, screenProgression:
		return true
	default:
		return false
	}
}

func (m Model) holdsBattleScreen() bool {
	if m.screen == screenSignalBoard || m.screen == screenExpedition || m.screen == screenProgression {
		return true
	}
	return m.battle.withContext(m.battleContext()).holdsScreen()
}

func (m *Model) openPendingExpedition() bool {
	result := m.battle.update(battleOpenPendingExpeditionInput{}, m.battleContext())
	routed := result.expedition != nil
	m.applyBattleUpdate(result)
	return routed
}

// View paints the active screen, reusing the memoized frame until Update
// marked state dirty or the terminal size changed.
func (m Model) View() tea.View {
	content := m.frame
	if m.frameDirty || content == "" || m.frameW != m.width || m.frameH != m.height {
		content = m.buildFrame()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.BackgroundColor = screenBgRGB
	return v
}

// buildFrame renders the current screen and stores it as the memoized frame.
// Called from Update (persisting through the returned model) and from View as
// a fallback for models that were never updated.
func (m *Model) buildFrame() string {
	var content string
	switch {
	case m.screen == screenOnboard && m.onboard.stage == stageWelcome:
		frameW := max(3, m.width-2)
		frameH := max(3, m.height-2)
		content = place(m.width, m.height, m.onboard.view(frameW, frameH, m.set))
	case m.wipeHold > 0 && (m.screen == screenBattle || m.screen == screenLobby):
		content = renderWipe(m.width, m.height, m.wipeHold)
	default:
		outerW, outerH := m.width, m.height
		inner := *m
		inner.width = max(1, m.width-2)
		inner.height = max(1, m.height-2)
		if m.screen == screenWorkbench && (outerW < minBattleWidth || outerH < minBattleHeight) {
			body := place(inner.width, inner.height, terminalTooSmall(outerW, outerH))
			content = m.wrapChrome(body)
		} else {
			content = m.wrapChrome(inner.innerView())
		}
	}
	content = paint(content, m.width, m.height)
	m.frame = content
	m.frameW, m.frameH = m.width, m.height
	m.frameBuilds++
	m.frameDirty = false
	return content
}

func (m Model) innerView() string {
	if m.modal != modalNone {
		kind := m.modal
		m.modal = modalNone
		base := m.innerView()
		return overlayCentered(base, modalCard(m.renderInfoModal(kind), m.width, m.height), m.width, m.height)
	}
	switch m.screen {
	case screenOnboard:
		return m.onboard.view(m.width, m.height, m.set)
	case screenDojoMenu:
		return m.overlayOnPrevious(renderDojoMenu(m.dojo.menu, m.dojo))
	case screenQueueEditor:
		return m.overlayOnPrevious(renderQueueEditor(m.queueEd, m.save))
	case screenProgression:
		return m.overlayOnPrevious(renderProgressionSummary(m.progression.msg, nextRequiredLesson(m.save)))
	case screenQueue:
		return m.overlayOnPrevious(renderQueue(m.queue.Position, m.queue.Waiting))
	case screenBattle:
		return m.battle.view(m.battleContext())
	case screenDisplaced:
		return renderDisplaced(m.width, m.height)
	case screenWorkbench:
		return m.renderWorkbench()
	case screenSignalBoard:
		return m.renderSignalBoard()
	case screenExpedition:
		return m.renderExpeditionFlow()
	default:
		return m.previousScreen()
	}
}

func (m Model) overlayOnPrevious(body string) string {
	return overlayCentered(m.previousScreen(), modalCard(body, m.width, m.height), m.width, m.height)
}

func (m Model) previousScreen() string {
	return renderLobbyAnimated(
		m.snap,
		m.width,
		m.height,
		m.lobbyWalk,
		m.lobbyAge,
		m.lobbyCameraX,
		m.lobbyCameraY,
		m.lobbyCameraOn,
	)
}
