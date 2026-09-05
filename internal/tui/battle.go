package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/server"
	"termon.sh/internal/sprite"
)

const (
	minBattleWidth  = 100
	minBattleHeight = 32

	// Action holds are 100ms ticks. Narration advances automatically after
	// each side's group; Enter can reveal or advance it sooner.
	playSkipGrace   = 12
	holdGroupPause  = 7
	holdRevealPause = 5
	holdIntro       = 20
	holdResult      = 20
	holdWipe        = 10
	typeCPS         = 2
	slideIn         = 8
	slideLag        = 4
	faintHideAfter  = 12
	captureSeqLast  = 1
)

func actionHold(kind battle.EventKind) int {
	switch kind {
	case battle.EventTurnStarted:
		return 10
	case battle.EventMoveUsed:
		return 28
	case battle.EventMissed, battle.EventCriticalHit, battle.EventSuperEffective, battle.EventNotVeryEffective:
		return 20
	case battle.EventDamageDealt:
		return 22
	case battle.EventFainted:
		return 32
	case battle.EventForfeit, battle.EventBattleOver:
		return 18
	case battle.EventSendOut, battle.EventSwitched, battle.EventReplacement:
		return 18
	default:
		return 20
	}
}

type playGroup struct {
	start, end int
	pause      bool
}

// battleScreenModel owns all state whose lifetime is the battle screen,
// including playback, capture presentation, battle-log navigation, and
// deferred message ordering. The root delegates screen-local transitions and
// rendering through update and view, then applies explicit routing outcomes.
type battleScreenModel struct {
	battleScreenContext
	session               battleSession
	cursor                int
	logOpen               bool
	logTop                int
	logFollow             bool
	battleIntro           bool
	battleAge             int
	playing               bool
	playAt                int
	playHold              int
	playSeen              int
	playGrace             int
	playPause             bool
	playReveal            bool
	introHold             int
	resultHold            int
	playHoldTotal         int
	fightRoot             bool
	switchRoot            bool
	revealPending         bool
	bell                  bool
	lowHPRang             bool
	lastYouHP             int
	evCacheVer            uint64
	evCache               []battle.Event
	youAnim               sprite.Anim
	foeAnim               sprite.Anim
	youSlug               string
	foeSlug               string
	capture               server.CaptureStateMsg
	shownCapture          server.CaptureStateMsg
	captureOn             bool
	expeditionPhase       string
	pendingExpedition     expeditionFlowModel
	pendingProgression    progressionModel
	hasPendingProgression bool
	pendingBattle         battleSession
	hasPendingBattle      bool
	runConfirm            bool
	captureBeat           int
	captureHold           int
	outcome               battleOutcome
}

type battleSession struct {
	battle          *battle.Battle
	you             string
	foe             string
	foeHash         string
	expeditionPhase string
}

// battleScreenContext is borrowed from the root for one update or view. It
// contains global facts the battle screen may read, never screen-owned state.
type battleScreenContext struct {
	width, height int
	set           *content.Set
	tutorial      bool
	active        bool
	wipeHold      int
	wipeFinished  bool
	youInBattle   bool
	nextLesson    int
}

type battleOutcome struct {
	command       battleCommand
	activated     bool
	startWipe     bool
	advanceReveal bool
	progression   *progressionModel
	expedition    *expeditionFlowModel
	clearTutorial bool
}

type battleUpdateResult struct {
	model battleScreenModel
	battleOutcome
}

type battleInput interface {
	battleInput()
}

type battleTickInput struct{}

func (battleTickInput) battleInput() {}

type battleKeyInput struct{ msg tea.KeyMsg }

func (battleKeyInput) battleInput() {}

type battleStartInput struct{ session battleSession }

func (battleStartInput) battleInput() {}

type battleCaptureInput struct{ state server.CaptureStateMsg }

func (battleCaptureInput) battleInput() {}

type battleProgressionInput struct{ model progressionModel }

func (battleProgressionInput) battleInput() {}

type battleExpeditionInput struct {
	model              expeditionFlowModel
	waitForProgression bool
}

func (battleExpeditionInput) battleInput() {}

type battleOpenPendingExpeditionInput struct{}

func (battleOpenPendingExpeditionInput) battleInput() {}

type battleDeactivateCaptureInput struct{}

func (battleDeactivateCaptureInput) battleInput() {}

type battleCommandKind int

const (
	battleCommandNone battleCommandKind = iota
	battleCommandForfeit
	battleCommandSelect
	battleCommandReplace
	battleCommandStartLesson
	battleCommandEnterLobby
)

type battleCommand struct {
	kind      battleCommandKind
	action    battle.Action
	monsterID string
	lesson    int
}

func (m battleScreenModel) withContext(ctx battleScreenContext) battleScreenModel {
	m.battleScreenContext = ctx
	return m
}

func (m battleScreenModel) withoutContext() battleScreenModel {
	m.battleScreenContext = battleScreenContext{}
	return m
}

// update is the battle screen's state transition seam. The root translates
// server messages and delegates only messages that affect battle-local state.
func (m battleScreenModel) update(input battleInput, ctx battleScreenContext) battleUpdateResult {
	m = m.withContext(ctx)
	m.outcome = battleOutcome{}
	switch input := input.(type) {
	case battleTickInput:
		m.tick()
	case battleStartInput:
		m.startBattle(input.session)
	case battleCaptureInput:
		m.applyCaptureState(input.state)
	case battleProgressionInput:
		m.receiveProgression(input.model)
	case battleExpeditionInput:
		m.receiveExpedition(input)
	case battleOpenPendingExpeditionInput:
		m.openPendingExpedition()
	case battleDeactivateCaptureInput:
		m.captureOn = false
	case battleKeyInput:
		m, m.outcome.command = m.key(input.msg)
	}
	outcome := m.outcome
	m.outcome = battleOutcome{}
	return battleUpdateResult{model: m.withoutContext(), battleOutcome: outcome}
}

// view is the battle screen's rendering seam.
func (m battleScreenModel) view(ctx battleScreenContext) string {
	return m.withContext(ctx).renderBattle()
}

func (m *battleScreenModel) tick() {
	m.bell = false
	m.battleAge++
	if m.wipeFinished && m.battleIntro {
		m.introHold = holdIntro
	}
	if m.introHold > 0 && m.wipeHold == 0 {
		m.introHold--
	}
	if m.captureHold > 0 {
		m.captureHold--
	}
	if m.showingCaptureSeq() && m.captureHold == 0 {
		if m.captureBeat < captureSeqLast {
			m.captureBeat++
			m.captureHold = holdResult
		} else if m.active {
			m.openPendingProgression()
		}
	}
	if m.resultHold > 0 {
		m.resultHold--
	}
	if m.resultHold == 0 && !m.playing && !m.showingCaptureSeq() && m.active {
		if !m.openPendingProgression() {
			m.applyPendingBattle()
		}
	}
	m.advancePlayback()
	if m.revealPending && m.session.battle != nil && m.session.battle.State() == battle.StateRevealing {
		m.revealPending = false
		m.outcome.advanceReveal = true
	}
	m.ringLowHP()
}

func (m battleScreenModel) shouldDeferBattle(next battleSession) bool {
	if !m.active || m.session.battle == nil || next.battle == nil {
		return false
	}
	if next.battle == m.session.battle {
		return false
	}
	if m.hasPendingProgression || m.playing || m.battleIntro || m.wipeHold > 0 || m.resultHold > 0 {
		return true
	}
	return m.session.battle.State() == battle.StateOver
}

func (m *battleScreenModel) startBattle(next battleSession) {
	if m.shouldDeferBattle(next) {
		m.pendingBattle = next
		m.hasPendingBattle = true
		return
	}
	m.applyBattle(next)
}

func (m battleScreenModel) hasBattle() bool {
	return m.session.battle != nil
}

func (m battleScreenModel) matchesBattle(next *battle.Battle) bool {
	return m.session.battle != nil && m.session.battle == next
}

func (m battleScreenModel) hasDeferredBattle() bool {
	return m.hasPendingBattle
}

func (m battleScreenModel) holdsScreen() bool {
	if !m.active {
		return false
	}
	if m.playing || m.hasPendingProgression || m.hasPendingBattle || m.battleIntro || m.wipeHold > 0 || m.resultHold > 0 {
		return true
	}
	return m.hasBattle()
}

func (m battleScreenModel) opponentName() string {
	return m.session.foe
}

func (m battleScreenModel) shouldDeferRoute() bool {
	if !m.active || m.session.battle == nil {
		return false
	}
	if m.playing || m.battleIntro || m.wipeHold > 0 || m.showingCaptureSeq() {
		return true
	}
	return m.session.battle.State() == battle.StateOver
}

func (m *battleScreenModel) receiveProgression(next progressionModel) {
	if m.shouldDeferRoute() {
		m.pendingProgression = next
		m.hasPendingProgression = true
		return
	}
	m.outcome.progression = &next
}

func (m *battleScreenModel) receiveExpedition(input battleExpeditionInput) {
	if input.waitForProgression || m.shouldDeferRoute() {
		m.pendingExpedition = input.model
		return
	}
	switch input.model.msg.Phase {
	case "failed", "captured", "hunt_failed", "abandoned", "recovery":
		m.outcome.expedition = &input.model
	}
}

func (m *battleScreenModel) applyPendingBattle() bool {
	if !m.hasPendingBattle {
		return false
	}
	next := m.pendingBattle
	m.pendingBattle = battleSession{}
	m.hasPendingBattle = false
	m.applyBattle(next)
	return true
}

func (m *battleScreenModel) applyBattle(next battleSession) {
	m.hasPendingBattle = false
	m.pendingBattle = battleSession{}
	same := m.session.battle != nil && next.battle == m.session.battle
	if !same {
		m.playSeen = 0
		m.playing = false
		m.playAt = 0
		m.playHold = 0
		m.playHoldTotal = 0
		m.playGrace = 0
		m.playPause = false
		m.playReveal = false
		m.introHold = 0
		m.resultHold = 0
		m.runConfirm = false
		m.captureBeat = 0
		m.captureHold = 0
		m.captureOn = false
		m.capture = server.CaptureStateMsg{}
		m.shownCapture = server.CaptureStateMsg{}
		m.battleIntro = next.battle != nil && next.battle.Turn() == 0 &&
			next.battle.State() != battle.StateOver
		if next.battle != nil && next.battle.EventCount() <= 2 {
			m.outcome.startWipe = true
		}
		m.fightRoot = true
		m.switchRoot = false
		m.cursor = 0
		m.lowHPRang = false
		m.lastYouHP = 0
		if m.battleIntro && m.wipeHold == 0 && !m.outcome.startWipe {
			m.introHold = holdIntro
		}
	}
	m.session = next
	m.expeditionPhase = next.expeditionPhase
	if next.expeditionPhase != "" {
		m.captureOn = next.expeditionPhase == "target"
	}
	m.evCache = nil
	m.evCacheVer = 0
	m.syncBattleAnims()
	m.startPlayback()
	m.outcome.activated = true
}

func (m *battleScreenModel) ringLowHP() {
	if m.session.battle == nil || m.lowHPRang {
		return
	}
	you, _ := m.arenaFighters()
	if you.MaxHP < 1 {
		return
	}
	if m.lastYouHP > 0 && you.HP*4 <= you.MaxHP && m.lastYouHP*4 > you.MaxHP {
		m.bell = true
		m.lowHPRang = true
	}
	m.lastYouHP = you.HP
}

func playGroups(events []battle.Event) []playGroup {
	var out []playGroup
	for i := 0; i < len(events); {
		if events[i].Text == "" {
			i++
			continue
		}
		if events[i].Kind == battle.EventTurnStarted {
			out = append(out, playGroup{start: i, end: i + 1, pause: false})
			i++
			continue
		}
		start := i
		i++
		for i < len(events) {
			k := events[i].Kind
			if k == battle.EventMoveUsed || k == battle.EventForfeit ||
				k == battle.EventTurnStarted || k == battle.EventSendOut ||
				k == battle.EventSwitched || k == battle.EventReplacement {
				break
			}
			i++
		}
		out = append(out, playGroup{start: start, end: i, pause: true})
	}
	return out
}

func groupAt(events []battle.Event, idx int) (playGroup, bool) {
	for _, g := range playGroups(events) {
		if idx >= g.start && idx < g.end {
			return g, true
		}
	}
	return playGroup{}, false
}

func (m *battleScreenModel) startPlayback() {
	if m.session.battle == nil {
		return
	}
	events := m.battleEvents()
	if m.playing || len(events) <= m.playSeen {
		return
	}
	m.playing = true
	m.playPause = false
	m.playAt = m.playSeen
	for m.playAt < len(events) && events[m.playAt].Text == "" {
		m.playAt++
	}
	if m.playAt >= len(events) {
		m.finishPlayback()
		return
	}
	m.playGrace = playSkipGrace
	m.applyBeat(events[m.playAt])
}

func (m *battleScreenModel) applyCaptureState(msg server.CaptureStateMsg) {
	m.capture = msg
	m.captureOn = true
	if captureVisualProgressed(m.shownCapture, msg) && m.deferCaptureReveal() {
		m.maybeRevealCapture()
		return
	}
	m.revealCapture()
}

func captureVisualProgressed(from, to server.CaptureStateMsg) bool {
	if to.Gauge > from.Gauge {
		return true
	}
	done := make(map[string]bool, len(from.Objectives))
	for _, o := range from.Objectives {
		if o.Done {
			done[string(o.ID)] = true
		}
	}
	for _, o := range to.Objectives {
		if o.Done && !done[string(o.ID)] {
			return true
		}
	}
	return false
}

func (m battleScreenModel) deferCaptureReveal() bool {
	if m.playing || m.battleIntro || m.wipeHold > 0 {
		return true
	}
	if !m.active || m.session.battle == nil {
		return false
	}
	return m.session.battle.State() != battle.StateOver
}

func captureResultKind(kind battle.EventKind) bool {
	switch kind {
	case battle.EventDamageDealt, battle.EventMissed, battle.EventSwitched, battle.EventFainted:
		return true
	default:
		return false
	}
}

func (m *battleScreenModel) maybeRevealCapture() {
	e, ok := m.playEvent()
	if !ok || !captureResultKind(e.Kind) {
		return
	}
	m.revealCapture()
}

func (m *battleScreenModel) revealCapture() {
	m.shownCapture = m.capture
	if m.capture.Gauge >= 100 && !m.playing && !m.battleIntro && m.wipeHold == 0 {
		m.beginCaptureSeq()
	}
}

// battleEvents returns the cached event-log snapshot for the current battle,
// refetching only when the battle's version has changed. The double version
// check around the copy guarantees the snapshot is never a torn mix of old
// and new log state; Battle mutates its log under the same lock that bumps
// the version.
func (m *battleScreenModel) battleEvents() []battle.Event {
	b := m.session.battle
	if b == nil {
		return nil
	}
	for range 3 {
		v := b.EventsVersion()
		if m.evCache != nil && v == m.evCacheVer && v == b.EventsVersion() {
			return m.evCache
		}
		snap := b.Events()
		if b.EventsVersion() == v {
			m.evCacheVer = v
			m.evCache = snap
			return snap
		}
	}
	// Log kept mutating across retries: return an uncached fresh copy rather
	// than risk a stale snapshot paired with a current version.
	m.evCacheVer = 0
	m.evCache = nil
	return b.Events()
}

func (m battleScreenModel) battleSnap() battle.Snapshot {
	if m.session.battle == nil {
		return battle.Snapshot{}
	}
	return m.session.battle.Snapshot(m.session.you)
}

func (m *battleScreenModel) syncBattleAnims() {
	if m.session.battle == nil || m.set == nil {
		return
	}
	youSlug, foeSlug := m.fieldSpecies()
	if youSlug != m.youSlug {
		m.youAnim = compileSpecies(m.set, youSlug, true)
		m.youSlug = youSlug
	}
	if foeSlug != m.foeSlug {
		m.foeAnim = compileSpecies(m.set, foeSlug, false)
		m.foeSlug = foeSlug
	}
}

func (m *battleScreenModel) openPendingProgression() bool {
	if !m.hasPendingProgression {
		return false
	}
	msg := m.pendingProgression
	m.pendingProgression = progressionModel{}
	m.hasPendingProgression = false
	m.hasPendingBattle = false
	m.pendingBattle = battleSession{}
	m.outcome.progression = &msg
	return true
}

func (m *battleScreenModel) applyBeat(e battle.Event) {
	hold := actionHold(e.Kind)
	typed := (len([]rune(e.Text))+typeCPS-1)/typeCPS + 4
	m.playHold = max(hold, typed)
	m.playHoldTotal = m.playHold
	m.playReveal = false
	m.battleAge = 0
	m.syncBattleAnims()
	m.maybeRevealCapture()
}

func (m *battleScreenModel) finishPlayback() {
	if m.session.battle != nil {
		m.playSeen = len(m.battleEvents())
		if m.session.battle.State() == battle.StateOver {
			m.resultHold = holdResult
		}
		if m.session.battle.State() == battle.StateRevealing && !m.hasPendingProgression {
			m.revealPending = true
		}
	}
	m.playing = false
	m.playPause = false
	m.playReveal = false
	m.playHold = 0
	m.playAt = m.playSeen
	m.syncBattleAnims()
	m.fightRoot = true
	m.switchRoot = false
	m.runConfirm = false
	m.cursor = 0
	m.revealCapture()
	if m.captureLanded() {
		m.beginCaptureSeq()
		return
	}
	if m.session.battle == nil || m.session.battle.State() != battle.StateOver {
		m.openPendingProgression()
	}
}

func (m *battleScreenModel) advancePlayback() {
	if !m.playing || m.session.battle == nil {
		return
	}
	if m.playGrace > 0 {
		m.playGrace--
	}
	if m.playPause {
		m.playHold--
		if m.playHold <= 0 {
			m.continuePlayback()
		}
		return
	}
	m.playHold--
	if m.playHold > 0 {
		return
	}
	m.nextPlayEvent()
	if m.playPause {
		m.continuePlayback()
	}
}

func (m *battleScreenModel) nextPlayEvent() {
	if !m.playing || m.session.battle == nil {
		return
	}
	events := m.battleEvents()
	g, ok := groupAt(events, m.playAt)
	next := m.playAt + 1
	for next < len(events) && events[next].Text == "" {
		next++
	}
	if !ok || next >= g.end {
		if ok && g.pause {
			m.playAt = g.end - 1
			for m.playAt > g.start && events[m.playAt].Text == "" {
				m.playAt--
			}
			m.playPause = true
			m.playHold = holdGroupPause
			return
		}
		if next >= len(events) {
			m.finishPlayback()
			return
		}
		m.playAt = next
		m.applyBeat(events[m.playAt])
		return
	}
	m.playAt = next
	m.applyBeat(events[next])
}

func (m *battleScreenModel) acceleratePlayback() {
	if !m.playing || m.playGrace > 0 || m.session.battle == nil {
		return
	}
	if m.playPause {
		m.continuePlayback()
		return
	}
	e, ok := m.playEvent()
	if ok && !m.playReveal && typeOn(e.Text, m.battleAge) != e.Text {
		m.playReveal = true
		m.playHold = min(m.playHold, holdRevealPause)
		return
	}
	m.nextPlayEvent()
}

func (m *battleScreenModel) continuePlayback() {
	if !m.playing || !m.playPause || m.playGrace > 0 || m.session.battle == nil {
		return
	}
	events := m.battleEvents()
	g, ok := groupAt(events, m.playAt)
	next := m.playAt + 1
	if ok {
		next = g.end
	}
	for next < len(events) && events[next].Text == "" {
		next++
	}
	if next >= len(events) {
		m.finishPlayback()
		return
	}
	m.playPause = false
	m.playAt = next
	m.applyBeat(events[next])
}

func (m battleScreenModel) playEvent() (battle.Event, bool) {
	if !m.playing || m.session.battle == nil {
		return battle.Event{}, false
	}
	events := m.battleEvents()
	if m.playAt < 0 || m.playAt >= len(events) {
		return battle.Event{}, false
	}
	return events[m.playAt], true
}

func (m battleScreenModel) renderBattle() string {
	if m.width == 0 || m.height == 0 {
		return "sizing…"
	}
	if m.width < minBattleWidth || m.height < minBattleHeight {
		return fmt.Sprintf("terminal too small: need ≥%d×%d, have %dx%d",
			minBattleWidth, minBattleHeight, m.width, m.height)
	}
	if m.wipeHold > 0 {
		return renderWipe(m.width, m.height, m.wipeHold)
	}
	if m.session.battle == nil {
		return "battle…"
	}
	stripFoe := m.renderFoeStrip()
	msgH := 4
	captureBlock := ""
	captureH := 0
	if m.captureOn {
		captureBlock = renderCaptureBand(m.visibleCapture(), m.width)
		captureH = blockHeight(captureBlock)
	}
	arenaH := max(6, m.height-blockHeight(stripFoe)-msgH-captureH)
	out := stripFoe + "\n" + m.renderArena(arenaH)
	if captureBlock != "" {
		out += "\n" + captureBlock
	}
	out += "\n" + m.renderBattleMsg()
	if m.logOpen {
		out = m.overlayBattleLog(out)
	}
	if m.bell {
		out += "\a"
	}
	return out
}

func (m battleScreenModel) renderCapturedMonster(species string, arenaH int, ctx battleScreenContext) string {
	m = m.withContext(ctx)
	m.foeSlug = species
	m.foeAnim = compileSpecies(m.set, species, false)
	return m.renderArena(arenaH)
}

func renderWipe(width, height, wipeHold int) string {
	w, h := max(1, width), max(1, height)
	progress := float64(holdWipe-wipeHold+1) / float64(holdWipe)
	insetX := min(w/2, int(progress*float64(w)/2))
	insetY := min(h/2, int(progress*float64(h)/2))
	flip := wipeHold % 2
	var b strings.Builder
	for y := range h {
		for x := range w {
			edge := x < insetX || x >= w-insetX || y < insetY || y >= h-insetY
			checker := (x/2+y)%2 == flip
			if edge || checker {
				b.WriteRune('█')
			} else {
				b.WriteByte(' ')
			}
		}
		if y < h-1 {
			b.WriteByte('\n')
		}
	}
	return fillStyle.Render(b.String())
}

func typeOn(s string, ticks int) string {
	if s == "" {
		return s
	}
	n := min(len([]rune(s)), max(1, ticks+1)*typeCPS)
	return string([]rune(s)[:n])
}

func typeMark(done, typing bool, blink int) string {
	switch {
	case done:
		return blank(2) + promptStyle.Render("▼")
	case typing && (blink/4)%2 == 0:
		return promptStyle.Render("▌")
	case typing:
		return fillStyle.Render(" ")
	default:
		return ""
	}
}

func msgLines(text, mark string, inner int) []string {
	wrapW := inner
	if mark != "" {
		wrapW = max(1, inner-lipgloss.Width(mark))
	}
	wrapped := strings.Split(wrapWords(text, wrapW), "\n")
	lines := make([]string, 0, msgInnerH)
	for _, l := range wrapped {
		lines = append(lines, l)
		if len(lines) == msgInnerH {
			break
		}
	}
	for len(lines) < msgInnerH {
		lines = append(lines, "")
	}
	last := 0
	for i, line := range slices.Backward(lines) {
		if line != "" || i == 0 {
			last = i
			break
		}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = narrStyle.Render(l)
		if i == last {
			out[i] += mark
		}
	}
	return out
}

func (m battleScreenModel) youFoe() (battle.Fighter, battle.Fighter) {
	snap := m.battleSnap()
	you, _ := m.session.battle.Fighter(m.session.you)
	foe := battle.Fighter{Trainer: m.session.foeHash}
	for _, f := range snap.FoeRoster {
		if f.Active {
			foe.ID = f.ID
			foe.Name = f.Name
			foe.Species = f.Species
			foe.HP = f.HP
			foe.MaxHP = f.MaxHP
			if spec, ok := m.set.Species[f.Species]; ok {
				foe.Type = spec.Type
			}
			break
		}
	}
	return you, foe
}

func (m battleScreenModel) playheadEnd() int {
	events := m.battleEvents()
	if m.playing {
		return min(len(events), m.playAt+1)
	}
	return len(events)
}

// fieldedFighter names a fighter from the engine's projected plate: ID, HP,
// and MaxHP are the projection's, and the public identity fields come from
// the roster entry with that Monster ID.
func (m battleScreenModel) fieldedFighter(prev battle.Fighter, hp battle.FieldedHP, snap battle.Snapshot) battle.Fighter {
	f := prev
	f.ID, f.HP, f.MaxHP = hp.MonsterID, hp.HP, hp.MaxHP
	for _, mem := range snap.YourParty {
		if mem.ID != hp.MonsterID {
			continue
		}
		f.Name, f.Species = mem.Name, mem.Species
		if spec, ok := m.set.Species[mem.Species]; ok {
			f.Type = spec.Type
		}
		return f
	}
	for _, r := range snap.FoeRoster {
		if r.ID != hp.MonsterID {
			continue
		}
		f.Name, f.Species = r.Name, r.Species
		if spec, ok := m.set.Species[r.Species]; ok {
			f.Type = spec.Type
		}
		return f
	}
	return f
}

// arenaFighters is the pair on the field as of the playhead, so plates and
// sprites stay on the fading Monster until its replacement's send-out. One
// locked projection names each side's fielded Monster at that position with
// its own HP and MaxHP — the only HP a viewer was ever shown — and while
// playing, the damage beat under the playhead drains once from the exact
// pre-hit HP to the exact post-hit HP, with the transition derived from the
// beat's typed damage so no second read can mix ledgers.
func (m battleScreenModel) arenaFighters() (you, foe battle.Fighter) {
	you, foe = m.youFoe()
	if m.session.battle == nil {
		return you, foe
	}
	end := m.playheadEnd()
	snap := m.battleSnap()
	youHP, foeHP := m.session.battle.FieldedHPAfter(m.session.you, end)
	you = m.fieldedFighter(you, youHP, snap)
	foe = m.fieldedFighter(foe, foeHP, snap)
	if !m.playing || end == 0 {
		return you, foe
	}
	e := m.battleEvents()[end-1]
	if e.Kind != battle.EventDamageDealt || e.Damage <= 0 {
		return you, foe
	}
	var post int
	switch e.TargetID {
	case you.ID:
		post = you.HP
	case foe.ID:
		post = foe.HP
	default:
		return you, foe
	}
	drained := e.Damage // settled frames land on the full hit
	if m.playHoldTotal > 0 && !m.playPause {
		elapsed := min(m.playHoldTotal, max(0, m.playHoldTotal-m.playHold))
		drained = (e.Damage*elapsed + m.playHoldTotal/2) / m.playHoldTotal
	}
	hp := post + e.Damage - drained
	if e.TargetID == you.ID {
		you.HP = hp
	} else {
		foe.HP = hp
	}
	return you, foe
}

func (m battleScreenModel) fieldSpecies() (youSlug, foeSlug string) {
	you, foe := m.arenaFighters()
	return you.Species, foe.Species
}

type spritePlace struct {
	art  string
	x, y int
	show bool
}

func clamp01(t float64) float64 {
	return min(1, max(0, t))
}

func easeOut(t float64) float64 {
	t = clamp01(t)
	return t * (2 - t)
}

func (m battleScreenModel) introSlide() (youT, foeT float64) {
	if !m.battleIntro || m.wipeHold > 0 {
		return 1, 1
	}
	elapsed := holdIntro - m.introHold
	return easeOut(float64(elapsed-slideLag) / slideIn), easeOut(float64(elapsed) / slideIn)
}

func (m battleScreenModel) spriteSlide() (youT, foeT float64) {
	if m.battleIntro || m.wipeHold > 0 {
		return m.introSlide()
	}
	e, ok := m.playEvent()
	if !ok {
		return 1, 1
	}
	switch e.Kind {
	case battle.EventSwitched, battle.EventReplacement:
	default:
		return 1, 1
	}
	t := easeOut(float64(m.battleAge) / slideIn)
	switch e.Actor {
	case m.session.you:
		return t, 1
	case m.session.foeHash:
		return 1, t
	default:
		return 1, 1
	}
}

func (m battleScreenModel) shakeX(player bool) int {
	e, ok := m.playEvent()
	if !ok || m.playPause {
		return 0
	}
	switch e.Kind {
	case battle.EventDamageDealt, battle.EventCriticalHit, battle.EventSuperEffective, battle.EventNotVeryEffective:
	default:
		return 0
	}
	hurtPlayer := e.Actor != m.session.you
	if player != hurtPlayer {
		return 0
	}
	amp := 2
	if m.battleAge > 10 {
		amp = 1
	}
	if m.battleAge > 18 {
		return 0
	}
	if m.battleAge%2 == 1 {
		return -amp
	}
	return amp
}

func (m battleScreenModel) spriteGone(hash string) bool {
	if m.session.battle == nil {
		return false
	}
	events := m.battleEvents()
	end := m.playheadEnd()
	gone := false
	for i, e := range events[:end] {
		if e.Actor != hash {
			continue
		}
		switch e.Kind {
		case battle.EventFainted:
			if !m.playing || i < m.playAt {
				gone = true
				continue
			}
			if m.playPause || m.battleAge >= faintHideAfter {
				gone = true
			}
		case battle.EventSendOut, battle.EventSwitched, battle.EventReplacement:
			gone = false
		}
	}
	return gone
}

func (m battleScreenModel) faintSink(hash string) int {
	e, ok := m.playEvent()
	if !ok || e.Kind != battle.EventFainted || e.Actor != hash || m.playPause {
		return 0
	}
	return min(3, m.battleAge/4)
}

func (m battleScreenModel) placeSprites(arenaH int) (you, foe spritePlace) {
	youPose, foePose := m.battlePoses()
	you.art = m.youAnim.Joined[youPose]
	foe.art = m.foeAnim.Joined[foePose]
	youW, foeW := lipgloss.Width(you.art), lipgloss.Width(foe.art)
	youH := lipgloss.Height(you.art)
	youT, foeT := m.spriteSlide()
	youRest := 2
	foeRest := max(0, m.width-foeW-2)
	you.x = int(float64(-youW)+youT*float64(youRest+youW)+0.5) + m.shakeX(true)
	foe.x = int(float64(m.width)+foeT*float64(foeRest-m.width)+0.5) + m.shakeX(false)
	you.y = max(0, arenaH-youH) + m.faintSink(m.session.you)
	foe.y = m.faintSink(m.session.foeHash)
	you.show = you.art != "" && !m.spriteGone(m.session.you)
	foe.show = foe.art != "" && !m.spriteGone(m.session.foeHash)
	return you, foe
}

func (m battleScreenModel) renderArena(arenaH int) string {
	you, foe := m.arenaFighters()
	enemyPlate := foePlate(foe)
	playerPlate := youPlate(you)
	youSp, foeSp := m.placeSprites(arenaH)

	floor := make([]string, arenaH)
	for i := range floor {
		floor[i] = blank(m.width)
	}
	layers := []*lipgloss.Layer{
		lipgloss.NewLayer(strings.Join(floor, "\n")).X(0).Y(0),
		lipgloss.NewLayer(enemyPlate).X(0).Y(1),
	}
	if foeSp.show {
		layers = append(layers, lipgloss.NewLayer(foeSp.art).X(foeSp.x).Y(foeSp.y))
	}
	if youSp.show {
		layers = append(layers, lipgloss.NewLayer(youSp.art).X(youSp.x).Y(youSp.y))
	}
	layers = append(layers, lipgloss.NewLayer(playerPlate).
		X(max(0, m.width-lipgloss.Width(playerPlate))).
		Y(max(0, arenaH-lipgloss.Height(playerPlate))))

	canvas := lipgloss.NewCanvas(m.width, arenaH)
	canvas.Compose(lipgloss.NewCompositor(layers...))
	fillCanvas(canvas, screenBgRGB)
	lines := strings.Split(canvas.Render(), "\n")
	for len(lines) < arenaH {
		lines = append(lines, blank(m.width))
	}
	return strings.Join(lines[:arenaH], "\n")
}

func plateName(f battle.Fighter) string {
	name := strings.ToUpper(f.Name)
	if name == "" {
		name = strings.ToUpper(f.Species)
	}
	runes := []rune(name)
	if len(runes) > 10 {
		name = string(runes[:10])
	}
	return name
}

func foePlate(f battle.Fighter) string {
	row1 := plateName(f)
	row2 := "HP: " + hpMeter(f.HP, f.MaxHP, 14)
	return plateStyle.Render(row1 + "\n" + row2)
}

func youPlate(f battle.Fighter) string {
	row1 := plateName(f)
	row2 := fmt.Sprintf("HP: %s %d/%d", hpMeter(f.HP, f.MaxHP, 14), f.HP, f.MaxHP)
	return plateStyle.Render(row1 + "\n" + row2)
}

func hpMeter(hp, maxHP, width int) string {
	if maxHP < 1 {
		maxHP = 1
	}
	ratio := float64(hp) / float64(maxHP)
	fill := min(width, max(0, int(ratio*float64(width))))
	style := greenBar
	switch {
	case ratio < 0.25:
		style = redBar
	case ratio < 0.55:
		style = yellowBar
	}
	return style.Render(strings.Repeat("=", fill)) + dimStyle.Render(strings.Repeat("-", width-fill))
}

func (m battleScreenModel) battlePoses() (youPose, foePose string) {
	idle := sprite.PoseIdleA
	if (m.battleAge/6)%2 == 1 {
		idle = sprite.PoseIdleB
	}
	youPose, foePose = idle, idle
	e, ok := m.playEvent()
	if !ok || m.playPause {
		return youPose, foePose
	}
	youAct := e.Actor == m.session.you
	switch e.Kind {
	case battle.EventMoveUsed, battle.EventMissed:
		pose := sprite.PoseAtk1
		if (m.battleAge/3)%2 == 1 {
			pose = sprite.PoseAtk2
		}
		if youAct {
			youPose = pose
		} else {
			foePose = pose
		}
	case battle.EventDamageDealt, battle.EventCriticalHit, battle.EventSuperEffective, battle.EventNotVeryEffective:
		hurt := sprite.PoseHurt
		if (m.battleAge/3)%2 == 1 {
			hurt = sprite.PoseIdleA
		}
		if youAct {
			foePose = hurt
		} else {
			youPose = hurt
		}
	case battle.EventFainted:
		faints := []string{sprite.PoseFaint1, sprite.PoseFaint2, sprite.PoseFaint3}
		pose := faints[min(2, m.battleAge/4)]
		if youAct {
			youPose = pose
		} else {
			foePose = pose
		}
	}
	return youPose, foePose
}

const (
	msgInnerH  = 2
	cmdOuterW  = 14
	typeOuterW = 12
)

func chromeBox(outer int, body string) string {
	inner := chromeInner(outer)
	var lines []string
	for line := range strings.SplitSeq(strings.TrimRight(body, "\n"), "\n") {
		lines = append(lines, fillLine(line, inner))
	}
	for len(lines) < msgInnerH {
		lines = append(lines, fillLine("", inner))
	}
	return chromeStyle.Width(max(1, outer)).Height(msgInnerH).Render(strings.Join(lines, "\n"))
}

func chromeInner(outer int) int {
	return max(1, outer-4)
}

func (m battleScreenModel) narrBox(text, mark string) string {
	inner := chromeInner(m.width)
	room := inner
	if mark != "" {
		room = max(1, inner-lipgloss.Width(mark))
	}
	return m.rosterChrome(m.width, narrStyle.Render(fitLine(text, room))+mark)
}

func (m battleScreenModel) rosterChrome(outer int, line2 string) string {
	inner := chromeInner(outer)
	return chromeBox(outer, fitLine(m.youRosterLine(), inner)+"\n"+line2)
}

func (m battleScreenModel) rosterSplit(line2, right string, rightW int) string {
	leftW := max(1, m.width-rightW)
	return lipgloss.JoinHorizontal(lipgloss.Top, m.rosterChrome(leftW, fitLine(line2, chromeInner(leftW))), right)
}

func rosterMark(fainted bool) string {
	if fainted {
		return "×"
	}
	return "●"
}

func (m battleScreenModel) renderFoeStrip() string {
	snap := m.battleSnap()
	var parts []string
	for _, f := range snap.FoeRoster {
		parts = append(parts, rosterMark(f.Fainted)+" "+strings.ToUpper(f.Name))
	}
	if snap.FoeLocked {
		parts = append(parts, dimStyle.Render("LOCKED"))
	}
	return wrapRoster(parts, m.width)
}

func (m battleScreenModel) youRosterLine() string {
	snap := m.battleSnap()
	var parts []string
	for _, p := range snap.YourParty {
		parts = append(parts, fmt.Sprintf("%s %s %d/%d", rosterMark(p.Fainted), strings.ToUpper(p.Name), p.HP, p.MaxHP))
	}
	line := narrStyle.Render(strings.Join(parts, "  "))
	if snap.YouLocked {
		kind := strings.ToUpper(string(snap.YouLockKind))
		line += blank(2) + selStyle.Render("LOCKED "+kind)
	}
	return line
}

func wrapRoster(parts []string, width int) string {
	if width < 1 {
		width = 1
	}
	if len(parts) == 0 {
		return padCells("", width)
	}
	var lines []string
	var cur string
	for _, part := range parts {
		next := part
		if cur != "" {
			next = cur + "  " + part
		}
		if lipgloss.Width(next) <= width {
			cur = next
			continue
		}
		if cur != "" {
			lines = append(lines, fitLine(cur, width))
		}
		cur = part
	}
	if cur != "" {
		lines = append(lines, fitLine(cur, width))
	}
	return strings.Join(lines, "\n")
}

func blockHeight(s string) int {
	if s == "" {
		return 1
	}
	return max(1, lipgloss.Height(s))
}

func (m battleScreenModel) visibleCapture() server.CaptureStateMsg {
	if len(m.shownCapture.Objectives) > 0 || m.shownCapture.Gauge > 0 || m.shownCapture.Status != "" {
		return m.shownCapture
	}
	return m.capture
}

func (m battleScreenModel) captureLanded() bool {
	return m.captureOn && m.capture.Gauge >= 100
}

func (m battleScreenModel) showingCaptureSeq() bool {
	return m.captureLanded() && !m.playing && !m.battleIntro && m.wipeHold == 0
}

func (m *battleScreenModel) beginCaptureSeq() {
	m.fightRoot = false
	m.switchRoot = false
	m.runConfirm = false
	if m.captureHold == 0 && m.captureBeat == 0 {
		m.captureHold = holdResult
	}
}

func (m battleScreenModel) captureSeqLine() string {
	_, foe := m.youFoe()
	name := strings.ToUpper(foe.Name)
	if m.captureBeat < 1 {
		return "The Gauge is full. This match is over."
	}
	if name == "" {
		name = "the Wild Monster"
	}
	return fmt.Sprintf("We captured %s. They're yours now.", name)
}

func (m battleScreenModel) captureSeqKey(msg tea.KeyMsg) battleScreenModel {
	switch msg.String() {
	case "enter", " ":
		if m.captureHold > 0 {
			m.captureHold = 0
		}
		if m.captureBeat < captureSeqLast {
			m.captureBeat++
			m.captureHold = holdResult
			return m
		}
		m.openPendingProgression()
	}
	return m
}

func (m battleScreenModel) canSwitch() bool {
	return len(m.battleSnap().YourParty) > 1
}

func (m battleScreenModel) requiredLesson() bool {
	return m.tutorial
}

func (m battleScreenModel) commandLabels() []string {
	if m.canSwitch() {
		return []string{"FIGHT", "SWITCH", "RUN"}
	}
	return []string{"FIGHT", "RUN"}
}

func (m battleScreenModel) commandRow(cursor int) string {
	labels := m.commandLabels()
	if len(labels) == 0 {
		return ""
	}
	if cursor < 0 || cursor >= len(labels) {
		cursor = 0
	}
	return menuRow(cursor, labels...)
}

func commandPane(cursor int) string {
	return menuRow(cursor, "FIGHT", "SWITCH", "RUN")
}

func (m battleScreenModel) renderTypePane(you, foe battle.Fighter) string {
	typ := m.selectedMoveType(you)
	if typ == "" {
		typ = "-"
	}
	head := dimStyle.Render("TYPE/")
	if tag := m.selectedMoveMatchupTag(you, foe); tag != "" {
		ink := okStyle
		if tag == "½" {
			ink = warnStyle
		}
		head += " " + ink.Render(tag)
	}
	body := head + "\n" + typeInk(typ).Render(strings.ToUpper(typ))
	return chromeBox(typeOuterW, body)
}

func (m battleScreenModel) selectedMoveMatchupTag(you, foe battle.Fighter) string {
	if !m.captureOn || m.set == nil {
		return ""
	}
	moves := m.battleSnap().YourPartyActiveLoadout()
	if len(moves) == 0 {
		moves = you.Moves
	}
	if m.cursor < 0 || m.cursor >= len(moves) {
		return ""
	}
	mv, ok := m.set.Moves[moves[m.cursor]]
	if !ok {
		return ""
	}
	foeType := foe.Type
	if foeType == "" {
		if spec, ok := m.set.Species[foe.Species]; ok {
			foeType = spec.Type
		}
	}
	eff := m.set.Effectiveness(mv.Type, foeType)
	switch {
	case eff >= battle.SuperEffectiveAt:
		return "2×"
	case eff > 0 && eff < 1:
		return "½"
	default:
		return ""
	}
}

func (m battleScreenModel) promptCommandLine(prompt string, cursor int) string {
	inner := chromeInner(m.width)
	cmds := m.commandRow(cursor)
	gap := 2
	room := max(1, inner-lipgloss.Width(cmds)-gap)
	return padCells(narrStyle.Render(fitLine(prompt, room)), room) + blank(gap) + cmds
}

func (m battleScreenModel) sableOverLine() string {
	if m.session.battle != nil && m.session.battle.Reason() == battle.EndForfeit {
		return "We try this Lesson again. Fill the Gauge. Enter retries."
	}
	if m.session.battle != nil && m.session.battle.Winner() == m.session.you {
		return fmt.Sprintf("They fainted before the Gauge filled (%d/100). Enter retries this Lesson.", m.capture.Gauge)
	}
	return "Your partner fainted. We try this Lesson again. Enter retries."
}

func (m battleScreenModel) selectedMoveType(you battle.Fighter) string {
	moves := m.battleSnap().YourPartyActiveLoadout()
	if len(moves) == 0 {
		moves = you.Moves
	}
	if m.cursor < 0 || m.cursor >= len(moves) || m.set == nil {
		return ""
	}
	if mv, ok := m.set.Moves[moves[m.cursor]]; ok {
		if td, ok := m.set.Types[mv.Type]; ok && td.Name != "" {
			return td.Name
		}
		return mv.Type
	}
	return ""
}

func (m battleScreenModel) renderBattleMsg() string {
	you, foe := m.youFoe()
	snap := m.battleSnap()
	switch {
	case m.playing:
		text, mark := "", ""
		if e, ok := m.playEvent(); ok {
			text = e.Text
			if !m.playPause && !m.playReveal {
				text = typeOn(e.Text, m.battleAge)
				mark = typeMark(false, text != e.Text, m.battleAge)
			}
		}
		return m.narrBox(text, mark)
	case m.showingCaptureSeq():
		ready := m.captureHold <= 0
		return m.narrBox(m.captureSeqLine(), typeMark(ready, false, 0))
	case m.session.battle.State() == battle.StateOver:
		result := "you lost."
		if m.session.battle.Winner() == m.session.you {
			result = "you won!"
		}
		if m.session.battle.Reason() == battle.EndDisconnectTimeout {
			if m.session.battle.Winner() == m.session.you {
				result = "opponent disconnected. you won!"
			} else {
				result = "you disconnected."
			}
		}
		last := result
		if ev := m.battleEvents(); len(ev) > 0 && ev[len(ev)-1].Text != "" {
			last = ev[len(ev)-1].Text
		}
		ready := m.resultHold <= 0
		line := last
		if last != result {
			line = last + " " + result
		}
		if m.requiredLesson() && ready && !m.hasPendingProgression {
			line = m.sableOverLine()
		}
		return m.narrBox(line, typeMark(ready, false, 0))
	case m.battleIntro:
		foeLabel := strings.ToUpper(foe.Name)
		youLabel := strings.ToUpper(you.Name)
		send := "Foe sent out " + foeLabel + "!"
		goLine := "Go! " + youLabel + "!"
		if m.requiredLesson() {
			if m.canSwitch() {
				goLine = "SWITCH when the matchup is bad, then FIGHT."
			} else {
				goLine = "The bar is HP. Three different Moves fill the Gauge."
			}
		}
		if m.introHold <= 0 {
			goLine += "  ▼"
		}
		inner := chromeInner(m.width)
		line2 := narrStyle.Render(fitLine(send+"  "+goLine, inner))
		return m.rosterChrome(m.width, line2)
	case m.session.battle.Locked(m.session.you):
		return m.narrBox("Waiting for opponent…", "")
	case snap.ReplacementRequired:
		return m.replacePane(snap)
	case m.switchRoot:
		return m.switchPane(snap)
	case m.fightRoot:
		if m.expeditionPhase != "" {
			_, foe := m.youFoe()
			prefix := expeditionBattlePrefix(m.expeditionPhase, foe.Species)
			return m.rosterChrome(m.width, m.promptCommandLine(prefix, m.cursor))
		}
		if m.runConfirm {
			return m.rosterChrome(m.width, m.promptCommandLine("Leave this Lesson? Enter retries it. Esc stays.", m.cursor))
		}
		if m.requiredLesson() {
			prompt := "Pick FIGHT. Use three different Moves. Don't knock it out before the Gauge fills."
			if m.canSwitch() {
				prompt = "SWITCH when the matchup is bad, then FIGHT to fill the Gauge."
			}
			return m.rosterChrome(m.width, m.promptCommandLine(prompt, m.cursor))
		}
		return m.rosterChrome(m.width, m.promptCommandLine("What will "+plateName(you)+" do?", m.cursor))
	default:
		return m.rosterSplit(m.moveLine(you), m.renderTypePane(you, foe), typeOuterW)
	}
}

func (m battleScreenModel) switchPane(snap battle.Snapshot) string {
	reserves := snap.HealthyReserves()
	labels := make([]string, len(reserves))
	for i, r := range reserves {
		labels[i] = fmt.Sprintf("%d %s", i+1, strings.ToUpper(r.Name))
	}
	line2 := menuRow(m.cursor, labels...)
	if len(labels) == 0 {
		line2 = dimStyle.Render("No healthy reserves.")
	} else if m.requiredLesson() {
		cmds := menuRow(m.cursor, labels...)
		inner := chromeInner(m.width)
		gap := 2
		room := max(1, inner-lipgloss.Width(cmds)-gap)
		line2 = padCells(narrStyle.Render(fitLine("Pick a healthy reserve. That fills Safe switch.", room)), room) + blank(gap) + cmds
	}
	return m.rosterChrome(m.width, line2)
}

func (m battleScreenModel) replacePane(snap battle.Snapshot) string {
	reserves := snap.HealthyReserves()
	labels := make([]string, len(reserves))
	for i, r := range reserves {
		labels[i] = fmt.Sprintf("%d %s", i+1, strings.ToUpper(r.Name))
	}
	line2 := selStyle.Render("Choose a replacement.")
	if len(labels) > 0 {
		line2 += blank(2) + menuRow(m.cursor, labels...)
	}
	return m.rosterChrome(m.width, line2)
}

func (m battleScreenModel) moveLine(you battle.Fighter) string {
	snap := m.battleSnap()
	moves := snap.YourPartyActiveLoadout()
	if len(moves) == 0 {
		moves = you.Moves
	}
	labels := make([]string, 0, 4)
	for i := range min(4, len(moves)) {
		name := moves[i]
		if mv, ok := m.set.Moves[moves[i]]; ok {
			name = mv.Name
		}
		labels = append(labels, fmt.Sprintf("%d %s", i+1, strings.ToUpper(name)))
	}
	return menuRow(m.cursor, labels...)
}

func fitLine(s string, w int) string {
	if w < 1 {
		return ""
	}
	if lipgloss.Width(s) > w {
		return ansi.Truncate(s, w, "…")
	}
	return s
}

func (m battleScreenModel) overlayBattleLog(base string) string {
	return overlayCentered(base, m.battleLogDialog(), m.width, m.height)
}

func battleLogLayout(width, height int) (modalW, modalH, innerW, innerH int) {
	modalW = min(max(44, width*8/10), 78)
	modalH = min(max(12, height*7/10), max(7, height-2))
	return modalW, modalH, max(1, modalW-2), max(1, modalH-2)
}

func (m *battleScreenModel) battleLogLines(innerW int) []string {
	var events []battle.Event
	if m.session.battle != nil {
		events = m.battleEvents()
	}
	return formatBattleLog(events, m.session.you, innerW)
}

func (m *battleScreenModel) battleLogMaxTop() int {
	_, _, innerW, innerH := battleLogLayout(m.width, m.height)
	return max(0, len(m.battleLogLines(innerW))-innerH)
}

func (m battleScreenModel) battleLogPageSize() int {
	_, _, _, innerH := battleLogLayout(m.width, m.height)
	return max(1, innerH-1)
}

func (m *battleScreenModel) openBattleLog() {
	m.logOpen = true
	m.logTop = m.battleLogMaxTop()
	m.logFollow = true
}

func (m *battleScreenModel) setBattleLogTop(top int) {
	maxTop := m.battleLogMaxTop()
	m.logTop = min(max(top, 0), maxTop)
	m.logFollow = m.logTop == maxTop
}

func (m *battleScreenModel) scrollBattleLog(delta int) {
	top := m.logTop
	if m.logFollow {
		top = m.battleLogMaxTop()
	}
	m.setBattleLogTop(top + delta)
}

func (m battleScreenModel) battleLogDialog() string {
	modalW, modalH, innerW, innerH := battleLogLayout(m.width, m.height)
	you, foe := m.youFoe()
	mid := dimStyle.Render(fmt.Sprintf("%s %d  ·  %s %d", plateName(you), you.HP, plateName(foe), foe.HP))
	lines := m.battleLogLines(innerW)
	maxTop := max(0, len(lines)-innerH)
	top := min(max(m.logTop, 0), maxTop)
	if m.logFollow {
		top = maxTop
	}
	end := min(len(lines), top+innerH)
	body := strings.Join(lines[top:end], "\n")
	position := ""
	if maxTop > 0 {
		position = dimStyle.Render(fmt.Sprintf("%d–%d/%d", top+1, end, len(lines)))
	}
	footer := hintLine(keyHint("↑↓", "scroll"), keyHint("tab", "close"))
	return pageFrame(modalW, modalH, selStyle.Render("battle log"), mid, position, body, footer)
}

func formatBattleLog(events []battle.Event, you string, innerW int) []string {
	if len(events) == 0 {
		return []string{dimStyle.Render("No turns yet. Pick a move, then tab back.")}
	}
	var lines []string
	var beat []battle.Event
	flush := func() {
		if len(beat) == 0 {
			return
		}
		lines = append(lines, renderLogBeat(beat, you)...)
		beat = nil
	}
	for _, e := range events {
		switch e.Kind {
		case battle.EventTurnStarted:
			flush()
			if e.Turn > 0 {
				lines = append(lines, logTurnRule(e.Turn, innerW))
			}
		case battle.EventMoveUsed, battle.EventForfeit:
			flush()
			beat = append(beat, e)
		default:
			beat = append(beat, e)
		}
	}
	flush()
	return lines
}

func logTurnRule(turn, w int) string {
	label := fmt.Sprintf(" turn %d ", turn)
	if w <= lipgloss.Width(label) {
		return dimStyle.Render(strings.TrimSpace(label))
	}
	fill := max(0, w-lipgloss.Width(label))
	left := fill / 2
	return dimStyle.Render(strings.Repeat("─", left) + label + strings.Repeat("─", fill-left))
}

func renderLogBeat(evs []battle.Event, you string) []string {
	actor := evs[0].Actor
	who, ink := "foe", dimStyle
	if actor == you {
		who, ink = "you", selStyle
	}
	move, dmg, winner := "", 0, ""
	miss, crit, se, nve, faint, forfeit := false, false, false, false, false, false
	for _, e := range evs {
		switch e.Kind {
		case battle.EventMoveUsed:
			move = logMoveName(e.Text)
		case battle.EventMissed:
			miss = true
		case battle.EventCriticalHit:
			crit = true
		case battle.EventSuperEffective:
			se = true
		case battle.EventNotVeryEffective:
			nve = true
		case battle.EventDamageDealt:
			dmg = e.Damage
		case battle.EventFainted:
			faint = true
		case battle.EventForfeit:
			forfeit = true
		case battle.EventBattleOver:
			winner = e.Actor
		}
	}
	var lines []string
	switch {
	case forfeit:
		lines = append(lines, ink.Render(who+"  forfeited"))
	case miss:
		lines = append(lines, ink.Render(fmt.Sprintf("%-3s  %s", who, strings.ToUpper(move)))+dimStyle.Render("   miss"))
	case move != "":
		line := ink.Render(fmt.Sprintf("%-3s  %s", who, strings.ToUpper(move)))
		if dmg > 0 {
			line += narrStyle.Render(fmt.Sprintf("  %d", dmg))
		}
		var tags []string
		if se {
			tags = append(tags, okStyle.Render("2×"))
		}
		if nve {
			tags = append(tags, warnStyle.Render("½"))
		}
		if crit {
			tags = append(tags, promptStyle.Render("crit"))
		}
		if len(tags) > 0 {
			line += blank(2) + strings.Join(tags, blank(1))
		}
		lines = append(lines, line)
	}
	if faint {
		lines = append(lines, warnStyle.Render("     fainted"))
	}
	if winner != "" {
		if winner == you {
			lines = append(lines, okStyle.Render("     you won"))
		} else {
			lines = append(lines, warnStyle.Render("     you lost"))
		}
	}
	if len(lines) == 0 {
		return nil
	}
	return lines
}

func (m *battleScreenModel) openPendingExpedition() bool {
	if m.pendingExpedition.msg.Phase == "" {
		return false
	}
	msg := m.pendingExpedition
	m.pendingExpedition = expeditionFlowModel{}
	m.outcome.expedition = &msg
	return true
}

func (m battleScreenModel) key(msg tea.KeyMsg) (battleScreenModel, battleCommand) {
	key := msg.String()
	if key == "tab" {
		if m.logOpen {
			m.logOpen = false
		} else {
			m.openBattleLog()
		}
		return m, battleCommand{}
	}
	if m.logOpen {
		switch key {
		case "esc":
			m.logOpen = false
		case "up", "k":
			m.scrollBattleLog(-1)
		case "down", "j":
			m.scrollBattleLog(1)
		case "pgup", "ctrl+u":
			m.scrollBattleLog(-m.battleLogPageSize())
		case "pgdown", "ctrl+d":
			m.scrollBattleLog(m.battleLogPageSize())
		case "home", "g":
			m.setBattleLogTop(0)
		case "end", "G":
			m.setBattleLogTop(m.battleLogMaxTop())
		}
		return m, battleCommand{}
	}
	if m.wipeHold > 0 {
		return m, battleCommand{}
	}
	if m.battleIntro {
		if m.introHold > 0 || (msg.String() != "enter" && msg.String() != " ") {
			return m, battleCommand{}
		}
		m.battleIntro = false
		m.fightRoot = true
		m.switchRoot = false
		m.cursor = 0
		return m, battleCommand{}
	}
	if m.playing {
		switch msg.String() {
		case "enter", " ":
			m.acceleratePlayback()
		}
		return m, battleCommand{}
	}
	if m.showingCaptureSeq() {
		return m.captureSeqKey(msg), battleCommand{}
	}
	if m.session.battle != nil && m.session.battle.State() == battle.StateOver {
		if m.resultHold > 0 || (msg.String() != "enter" && msg.String() != " ") {
			return m, battleCommand{}
		}
		m.logOpen = false
		m.playing = false
		m.playPause = false
		if m.openPendingProgression() {
			m.session = battleSession{}
			m.evCache = nil
			m.evCacheVer = 0
			return m, battleCommand{}
		}
		if m.openPendingExpedition() {
			m.session = battleSession{}
			m.evCache = nil
			m.evCacheVer = 0
			return m, battleCommand{}
		}
		if m.applyPendingBattle() {
			return m, battleCommand{}
		}
		if m.youInBattle {
			return m, battleCommand{}
		}
		if m.requiredLesson() {
			if n := m.nextLesson; n != 0 {
				m.runConfirm = false
				return m, battleCommand{kind: battleCommandStartLesson, lesson: n}
			}
		}
		m.session = battleSession{}
		m.evCache = nil
		m.evCacheVer = 0
		m.outcome.clearTutorial = true
		return m, battleCommand{kind: battleCommandEnterLobby}
	}
	snap := m.battleSnap()
	if m.session.battle == nil {
		return m, battleCommand{}
	}
	if snap.ReplacementRequired {
		return m.replaceKey(msg, snap)
	}
	if m.session.battle.Locked(m.session.you) {
		return m, battleCommand{}
	}
	if m.switchRoot {
		return m.switchKey(msg, snap)
	}
	if m.fightRoot {
		if m.runConfirm {
			switch msg.String() {
			case "enter", " ", "y", "f":
				m.runConfirm = false
				return m, battleCommand{kind: battleCommandForfeit}
			case "esc", "n", "backspace":
				m.runConfirm = false
				return m, battleCommand{}
			}
			return m, battleCommand{}
		}
		labels := m.commandLabels()
		n := max(1, len(labels))
		runAt := n - 1
		switchAt := -1
		if m.canSwitch() {
			switchAt = 1
		}
		armRun := func() (battleScreenModel, battleCommand) {
			if m.requiredLesson() {
				m.runConfirm = true
				return m, battleCommand{}
			}
			return m, battleCommand{kind: battleCommandForfeit}
		}
		switch msg.String() {
		case "f":
			return armRun()
		case "s":
			if switchAt >= 0 {
				m.fightRoot = false
				m.switchRoot = true
				m.cursor = 0
			}
		case "2":
			if switchAt >= 0 {
				m.fightRoot = false
				m.switchRoot = true
				m.cursor = 0
			}
		case "up", "k", "down", "j", "left", "h", "right", "l":
			m.cursor = (m.cursor + 1) % n
		case "1":
			m.fightRoot = false
			m.cursor = 0
		case "enter", " ":
			switch m.cursor {
			case switchAt:
				m.fightRoot = false
				m.switchRoot = true
				m.cursor = 0
			case runAt:
				return armRun()
			default:
				m.fightRoot = false
				m.cursor = 0
			}
		}
		return m, battleCommand{}
	}
	moves := snap.YourPartyActiveLoadout()
	n := max(1, len(moves))
	switch msg.String() {
	case "esc", "backspace":
		m.fightRoot = true
		m.cursor = 0
	case "f":
		if m.requiredLesson() {
			m.fightRoot = true
			m.switchRoot = false
			m.runConfirm = true
			m.cursor = 0
			return m, battleCommand{}
		}
		return m, battleCommand{kind: battleCommandForfeit}
	case "up", "k":
		m.cursor = (m.cursor + 2) % n
	case "down", "j":
		m.cursor = (m.cursor + 2) % n
	case "left", "h":
		m.cursor = (m.cursor + n - 1) % n
	case "right", "l":
		m.cursor = (m.cursor + 1) % n
	case "1", "2", "3", "4":
		i := int(msg.String()[0] - '1')
		if i >= 0 && i < len(moves) {
			m.cursor = i
			return m, battleCommand{
				kind:   battleCommandSelect,
				action: battle.Action{Kind: battle.ActionMove, Move: moves[i]},
			}
		}
	case "enter", " ":
		if m.cursor >= 0 && m.cursor < len(moves) {
			return m, battleCommand{
				kind:   battleCommandSelect,
				action: battle.Action{Kind: battle.ActionMove, Move: moves[m.cursor]},
			}
		}
	}
	return m, battleCommand{}
}

func (m battleScreenModel) replaceKey(msg tea.KeyMsg, snap battle.Snapshot) (battleScreenModel, battleCommand) {
	reserves := snap.HealthyReserves()
	if len(reserves) == 0 {
		return m, battleCommand{}
	}
	n := len(reserves)
	switch msg.String() {
	case "up", "k":
		m.cursor = (m.cursor + n - 1) % n
	case "down", "j":
		m.cursor = (m.cursor + 1) % n
	case "1", "2", "3":
		i := int(msg.String()[0] - '1')
		if i >= 0 && i < n {
			id := reserves[i].ID
			return m, battleCommand{kind: battleCommandReplace, monsterID: id}
		}
	case "enter", " ":
		if m.cursor >= 0 && m.cursor < n {
			id := reserves[m.cursor].ID
			return m, battleCommand{kind: battleCommandReplace, monsterID: id}
		}
	}
	return m, battleCommand{}
}

func (m battleScreenModel) switchKey(msg tea.KeyMsg, snap battle.Snapshot) (battleScreenModel, battleCommand) {
	reserves := snap.HealthyReserves()
	if len(reserves) == 0 {
		m.switchRoot = false
		m.fightRoot = true
		return m, battleCommand{}
	}
	n := len(reserves)
	switch msg.String() {
	case "esc", "backspace":
		m.switchRoot = false
		m.fightRoot = true
		m.cursor = 0
	case "up", "k":
		m.cursor = (m.cursor + n - 1) % n
	case "down", "j":
		m.cursor = (m.cursor + 1) % n
	case "1", "2", "3":
		i := int(msg.String()[0] - '1')
		if i >= 0 && i < n {
			id := reserves[i].ID
			return m, battleCommand{
				kind:   battleCommandSelect,
				action: battle.Action{Kind: battle.ActionSwitch, SwitchTo: id},
			}
		}
	case "enter", " ":
		if m.cursor >= 0 && m.cursor < n {
			id := reserves[m.cursor].ID
			return m, battleCommand{
				kind:   battleCommandSelect,
				action: battle.Action{Kind: battle.ActionSwitch, SwitchTo: id},
			}
		}
	}
	return m, battleCommand{}
}

func logMoveName(text string) string {
	_, name, ok := strings.Cut(text, " used ")
	if !ok {
		return text
	}
	return strings.TrimSuffix(name, "!")
}

func compileSpecies(set *content.Set, slug string, faceRight bool) sprite.Anim {
	if set == nil {
		return sprite.Anim{}
	}
	art, ok := set.Arts[slug]
	if !ok {
		return sprite.Anim{}
	}
	typ := ""
	if sp, ok := set.Species[slug]; ok {
		typ = sp.Type
	}
	return sprite.CompileOn(art, typ, faceRight, screenBgHex)
}

func renderQueue(pos, waiting int) string {
	return strings.Join([]string{
		fmt.Sprintf("position %s of %s", selStyle.Render(strconv.Itoa(pos)), strconv.Itoa(waiting)),
		"",
		dimStyle.Render("waiting for another trainer"),
	}, "\n")
}

func renderDisplaced(w, h int) string {
	return place(w, h, warnStyle.Render("this seat was taken by a newer connection.")+"\n"+dimStyle.Render("q: quit"))
}
