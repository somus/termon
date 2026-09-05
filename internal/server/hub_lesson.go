package server

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"termon.sh/internal/battle"
	"termon.sh/internal/capture"
	"termon.sh/internal/dojo"
	"termon.sh/internal/game"
	"termon.sh/internal/lobby"
)

// DojoMenuMsg lists Capture Lessons when adjacent to Master Sable.
type DojoMenuMsg struct {
	Lesson1Done             bool
	Lesson2Done             bool
	Hint                    string
	ServerDay               string
	SparringApprenticeClear bool
	SparringRivalClear      bool
	SparringMasterClear     bool
	SparringPreview         []SparringPreviewSlot
	Daily                   DailyMenuInfo
}

// LessonIntentMsg may precede action selection in Lessons.
type LessonIntentMsg struct {
	Text string
}

// CaptureStateMsg updates gauge progress during Target Encounters.
type CaptureStateMsg struct {
	Gauge      int
	Objectives []CaptureObjectiveView
	Status     string
}

// CaptureObjectiveView is one visible Capture Objective on the Gauge band.
type CaptureObjectiveView struct {
	ID      capture.ObjectiveID
	Done    bool
	Award   int
	Focused bool
}

type lessonMode struct {
	soloMode
	lesson      int
	capture     *capture.Session
	wildSpecies string
	failures    int
}

func newLessonMode(lesson int, sv *game.Save, session *capture.Session, wildSpecies string) (*lessonMode, error) {
	if lesson != 1 && lesson != 2 {
		return nil, fmt.Errorf("server: invalid lesson mode %d", lesson)
	}
	if session == nil || len(session.Objectives) == 0 || wildSpecies == "" {
		return nil, errors.New("server: incomplete lesson mode")
	}
	solo := soloMode{saveBefore: cloneSaveXPView(sv)}
	return &lessonMode{
		soloMode:    solo,
		lesson:      lesson,
		capture:     session,
		wildSpecies: wildSpecies,
	}, nil
}

func (mode *lessonMode) afterAction(h *Hub, m *match, hash string, _ battle.Action, trainerMove string) bool {
	h.afterLessonTurn(m, mode, hash, trainerMove)
	return h.matchCurrent(hash, m)
}

func (mode *lessonMode) finish(h *Hub, m *match) {
	h.finishLessonMatch(m, mode)
}

func (h *Hub) finishLessonMatch(m *match, mode *lessonMode) {
	h.pushFinishedSoloBattle(m, &mode.soloMode)
	if wildFaintedFromSnap(m.bt.Snapshot(m.a)) && !mode.capture.Full() {
		h.retryLesson(m, mode, m.a)
		return
	}
	if m.bt.Winner() != m.a {
		h.retryLesson(m, mode, m.a)
		return
	}

	var out outbox
	h.mu.Lock()
	m.committing = false
	delete(h.matches, m.a)
	h.setPresenceLocked(m.a, func(p *lobby.Presence) { p.InBattle = false })
	h.broadcastTrainerDojoLocked(&out, m.a)
	h.mu.Unlock()
	out.flush()
}

// OpenDojoMenu returns lesson completion for the menu.
func (h *Hub) OpenDojoMenu(hash string) (DojoMenuMsg, error) {
	h.mu.Lock()
	room, _, ok := h.roomForLocked(hash)
	h.mu.Unlock()
	if !ok || !room.NearMaster(hash) {
		return DojoMenuMsg{}, playerFacing("stand next to Master Sable")
	}
	trainerID := hash
	return h.buildDojoMenu(trainerID)
}

// StartLesson launches a Capture Lesson solo match from Master Sable's menu.
func (h *Hub) StartLesson(hash string, lesson int) error {
	if lesson != 1 && lesson != 2 {
		return playerFacing("unknown lesson")
	}
	h.mu.Lock()
	room, _, ok := h.roomForLocked(hash)
	if !ok || !room.NearMaster(hash) {
		h.mu.Unlock()
		return playerFacing("stand next to Master Sable")
	}
	if h.matches[hash] != nil {
		h.mu.Unlock()
		return playerFacing("already in battle")
	}
	h.mu.Unlock()
	return h.launchLesson(hash, lesson)
}

// StartRequiredLesson seats the Trainer beside Master Sable and launches
// the next onboarding Capture Lesson. It does not require the Trainer to
// already be adjacent.
func (h *Hub) StartRequiredLesson(hash string, lesson int) error {
	if lesson != 1 && lesson != 2 {
		return playerFacing("unknown lesson")
	}
	h.mu.Lock()
	room, id, ok := h.roomForLocked(hash)
	if !ok {
		h.mu.Unlock()
		return playerFacing("cannot start lesson now")
	}
	if h.matches[hash] != nil {
		h.mu.Unlock()
		return playerFacing("already in battle")
	}
	if err := room.PlaceBesideMaster(hash); err != nil {
		h.mu.Unlock()
		return playerFacing("cannot start lesson now")
	}
	h.dirtyDojos[id] = true
	h.mu.Unlock()
	return h.launchLesson(hash, lesson)
}

func (h *Hub) launchLesson(hash string, lesson int) error {
	sv, err := h.Load(hash)
	if err != nil || sv == nil {
		return missingParty(hash, err)
	}
	starter, err := dojo.StarterFromSave(sv)
	if err != nil {
		return playerFacing("choose a starter first")
	}
	target, err := dojo.LessonTarget(starter, lesson)
	if err != nil {
		return err
	}
	return h.startLessonMatch(hash, lesson, sv, target)
}

func (h *Hub) startLessonMatch(hash string, lesson int, sv *game.Save, target string) error {
	members, fighters, err := lessonParty(sv, lesson)
	if err != nil {
		return err
	}
	wildSpec := h.set.Species[target]
	wildLevel := capture.WildLevel(fighters)
	captureHP := capture.TargetHP(h.set, fighters, wildSpec, wildLevel)
	wildMoves, err := dojo.WildLoadout(h.set, target)
	if err != nil {
		return err
	}
	wildMon := game.Monster{
		ID: dojo.BotTrainer + "-wild", Species: target, Level: wildLevel,
		BattleLoadout: wildMoves,
	}
	trainerParty := battle.Party{Trainer: hash, Members: members}
	wildParty := battle.Party{
		Trainer: dojo.BotTrainer, Members: []battle.PartyMember{{
			Monster: wildMon, MaxHP: captureHP,
		}},
		ClampOutgoingDamage: true,
	}
	seed, err := battle.RandomSeed()
	if err != nil {
		return fmt.Errorf("server: seed lesson: %w", err)
	}
	bt, err := battle.New(h.set, trainerParty, wildParty, battle.Seeded(seed))
	if err != nil {
		return err
	}
	objIDs := capture.AuthoredLessonObjectives(lesson)
	objs, err := capture.ObjectivesFromIDs(objIDs)
	if err != nil {
		return err
	}
	id, err := newMatchID()
	if err != nil {
		return err
	}
	mode, err := newLessonMode(lesson, sv, capture.NewSession(objs), target)
	if err != nil {
		return err
	}
	m := &match{
		id: id, bt: bt, a: hash, b: dojo.BotTrainer,
		handle: map[string]string{hash: sv.Handle, dojo.BotTrainer: "Wild " + wildSpec.Name},
		mode:   mode,
	}
	var out outbox
	h.mu.Lock()
	if busy := h.busyLocked(hash, dojo.BotTrainer); busy != "" {
		h.mu.Unlock()
		return playerFacing("cannot start lesson now")
	}
	h.matches[hash] = m
	h.setPresenceLocked(hash, func(p *lobby.Presence) { p.InBattle = true })
	h.broadcastTrainerDojoLocked(&out, hash)
	h.mu.Unlock()
	out.flush()
	h.pushBattle(m)
	h.pushCaptureState(m, mode.capture, "Gauge is full.", nil)
	h.pushLessonCoach(m, mode)
	return nil
}

func lessonParty(sv *game.Save, lesson int) ([]battle.PartyMember, []capture.PartyFighter, error) {
	var members []battle.PartyMember
	var fighters []capture.PartyFighter
	count := 1
	if lesson == 2 {
		count = 2
	}
	for i := 0; i < count; i++ {
		id := sv.Party[i]
		if id == "" {
			return nil, nil, fmt.Errorf("server: lesson %d needs party slot %d", lesson, i+1)
		}
		m, ok := game.MonsterByID(sv, id)
		if !ok {
			return nil, nil, fmt.Errorf("server: missing monster %q", id)
		}
		members = append(members, battle.PartyMember{Monster: m})
		fighters = append(fighters, capture.PartyFighter{
			Species: m.Species, Level: m.Level, Loadout: append([]string(nil), m.BattleLoadout...),
		})
	}
	return members, fighters, nil
}

func (h *Hub) pushCaptureState(
	m *match,
	session *capture.Session,
	fullStatus string,
	newly []capture.ObjectiveID,
) {
	if session == nil {
		return
	}
	status := "target only"
	if session.Full() {
		status = fullStatus
	}
	view := CaptureStateMsg{Gauge: session.Gauge, Status: status}
	if coach := captureCoach(newly); coach != "" {
		if session.Full() {
			view.Status = coach + " Gauge is full."
		} else {
			view.Status = coach
		}
	}
	for i, obj := range session.Objectives {
		view.Objectives = append(view.Objectives, CaptureObjectiveView{
			ID: obj.ID, Done: session.Completed[obj.ID], Award: obj.Award, Focused: i == 0,
		})
	}
	var out outbox
	h.mu.Lock()
	h.note(&out, m.a, view)
	h.mu.Unlock()
	out.flush()
}

func captureCoach(newly []capture.ObjectiveID) string {
	var parts []string
	for _, id := range newly {
		switch id {
		case capture.ReadTheMatchup:
			parts = append(parts, "Super-effective: that's a 2× Type.")
		case capture.ShowMoveVariety:
			parts = append(parts, "Three different Moves.")
		case capture.SafeSwitch:
			parts = append(parts, "You switched in safely.")
		case capture.HoldTheLine:
			parts = append(parts, "You held the line.")
		}
	}
	return strings.Join(parts, " ")
}

func (h *Hub) botLockSolo(m *match, mode *soloMode) error {
	st := m.bt.State()
	if st == battle.StateRevealing || st == battle.StateOver {
		return nil
	}
	if st == battle.StateAwaitingReplacement {
		if m.bt.Snapshot(dojo.BotTrainer).ReplacementRequired {
			return h.botReplaceSolo(m, mode)
		}
		return nil
	}
	if m.bt.Locked(dojo.BotTrainer) {
		return nil
	}
	view, ok := m.bt.PolicyViewFor(dojo.BotTrainer)
	if !ok {
		return errors.New("dojo: bot policy view")
	}
	cfg := mode.policyCfg
	if cfg.Tier == "" {
		cfg = dojo.TierConfig(dojo.TierApprentice)
	}
	act, exp, err := dojo.ChoosePolicyAction(h.set, view, cfg, m.btRand())
	if err != nil {
		return err
	}
	h.mu.Lock()
	mode.lastDecision = exp
	h.mu.Unlock()
	if err := m.bt.Select(dojo.BotTrainer, act); err != nil {
		return err
	}
	return nil
}

func (h *Hub) botReplaceSolo(m *match, mode *soloMode) error {
	view, ok := m.bt.PolicyViewFor(dojo.BotTrainer)
	if !ok {
		return errors.New("dojo: bot replace view")
	}
	cfg := mode.policyCfg
	if cfg.Tier == "" {
		cfg = dojo.TierConfig(dojo.TierApprentice)
	}
	id, exp, err := dojo.ChooseReplacement(h.set, view, cfg, m.btRand())
	if err != nil {
		return err
	}
	h.mu.Lock()
	mode.lastDecision = exp
	h.mu.Unlock()
	return m.bt.Replace(dojo.BotTrainer, id)
}

func (m *match) btRand() battle.Rand {
	// Policy tie-break material is drawn fresh from crypto/rand per decision,
	// matching the live Battle RNG convention: lesson and Sparring bots must
	// not replay a predictable script across trainers.
	seed, err := battle.RandomSeed()
	if err != nil {
		// crypto/rand failure is not survivable for fairness; degrade to the
		// wall clock rather than a predictable constant.
		seed = uint64(time.Now().UnixNano()) //nolint:gosec // fallback only
	}
	return battle.Seeded(seed)
}

func (h *Hub) afterLessonTurn(m *match, mode *lessonMode, hash string, trainerMove string) {
	turn := m.bt.Turn()
	events := m.bt.Events()
	snap := m.bt.Snapshot(hash)
	wildHP, wildMax := wildHPFromSnap(snap)
	in := capture.BuildTurnInput(events, turn, hash, dojo.BotTrainer, trainerMove, snap, wildHP, wildMax)
	newly := mode.capture.AfterTurn(in)
	h.pushCaptureState(m, mode.capture, "Gauge is full.", newly)
	wildFainted := wildFaintedFromSnap(snap)
	outcome := mode.capture.OutcomeAfterTurn(wildFainted)
	switch outcome {
	case "captured":
		h.finishLessonCaptured(m, mode, hash)
	case "hunt_failed":
		h.pushBattle(m)
		h.retryLesson(m, mode, hash)
	}
}

func wildHPFromSnap(snap battle.Snapshot) (hp, maxHP int) {
	for _, f := range snap.FoeRoster {
		if f.Active {
			return f.HP, f.MaxHP
		}
	}
	return 0, 0
}

func wildFaintedFromSnap(snap battle.Snapshot) bool {
	for _, f := range snap.FoeRoster {
		if f.Active {
			return f.Fainted
		}
	}
	return false
}

func (h *Hub) finishLessonCaptured(m *match, mode *lessonMode, hash string) {
	h.pushCaptureState(m, mode.capture, "Gauge is full.", nil)
	h.pushBattle(m)
	active, reserve := participationFromBattle(m.bt, hash)
	if err := h.persistLessonCapture(mode, hash, mode.wildSpecies, active, reserve); err != nil {
		h.noteLessonError(hash, err)
		h.retryLesson(m, mode, hash)
		return
	}
	h.releaseLessonMatch(hash)
}

func (h *Hub) retryLesson(m *match, mode *lessonMode, hash string) {
	mode.failures++
	hint := lessonHint(m, mode)
	status := lessonRetryStatus(m, mode)
	failCount := mode.failures
	h.releaseLessonMatch(hash)
	var out outbox
	h.mu.Lock()
	if status != "" {
		h.note(&out, hash, ErrorMsg{Text: status})
	}
	if hint != "" {
		h.note(&out, hash, LessonIntentMsg{Text: hint})
	}
	h.mu.Unlock()
	out.flush()
	sv, err := h.Load(hash)
	if err != nil || sv == nil {
		h.noteRetryFailed(hash, "could not reload your save", err)
		return
	}
	starter, err := dojo.StarterFromSave(sv)
	if err != nil {
		h.noteRetryFailed(hash, "could not read your party", err)
		return
	}
	target, err := dojo.LessonTarget(starter, mode.lesson)
	if err != nil {
		h.noteRetryFailed(hash, "could not resolve the Lesson target", err)
		return
	}
	if err := h.startLessonMatch(hash, mode.lesson, sv, target); err != nil {
		h.noteRetryFailed(hash, "could not restart the Lesson", err)
		return
	}
	h.mu.Lock()
	if nm := h.matches[hash]; nm != nil {
		if next, ok := nm.mode.(*lessonMode); ok {
			next.failures = failCount
		}
	}
	h.mu.Unlock()
}

func lessonRetryStatus(m *match, mode *lessonMode) string {
	if mode.capture.Full() {
		return ""
	}
	if m.bt != nil && m.bt.Winner() != m.a {
		if m.bt.Reason() == battle.EndForfeit {
			return "We try this Lesson again. Fill the Gauge."
		}
		return "Your partner fainted. We try this Lesson again."
	}
	return fmt.Sprintf("They fainted before the Gauge filled (%d/100). We try this Lesson again.", mode.capture.Gauge)
}

func (h *Hub) pushLessonCoach(m *match, mode *lessonMode) {
	text := "Fill the Gauge. Use three Moves. 2× on the TYPE pane is super-effective."
	if mode.lesson == 2 {
		text = "Switch to a better matchup, then fill the Gauge. 2× on the TYPE pane is super-effective."
	}
	var out outbox
	h.mu.Lock()
	h.note(&out, m.a, LessonIntentMsg{Text: text})
	h.mu.Unlock()
	out.flush()
}

func lessonHint(_ *match, mode *lessonMode) string {
	switch mode.failures {
	case 1:
		return "Check the Type matchup on the Gauge."
	case 2:
		if mode.lesson == 2 {
			return "Switch to a safer reserve, then fill the Gauge."
		}
		return "Use a weaker Move so the Gauge can fill before they faint."
	default:
		return ""
	}
}

func (h *Hub) releaseLessonMatch(hash string) {
	var out outbox
	h.mu.Lock()
	delete(h.matches, hash)
	h.setPresenceLocked(hash, func(p *lobby.Presence) { p.InBattle = false })
	h.note(&out, hash, h.snapshotLocked(hash))
	h.broadcastTrainerDojoLocked(&out, hash)
	h.mu.Unlock()
	out.flush()
}

// noteRetryFailed tells the trainer the Lesson retry did not happen and why,
// so a failed restart never strands them silently out of battle.
func (h *Hub) noteRetryFailed(hash, reason string, err error) {
	var out outbox
	h.mu.Lock()
	h.note(&out, hash, ErrorMsg{Text: "Lesson retry failed: " + reason + ". Start it again from the Dojo menu."})
	h.mu.Unlock()
	out.flush()
	h.logWarn("lesson retry failed", "trainer", hash, "reason", reason, "err", err)
}

func (h *Hub) noteLessonError(hash string, err error) {
	var out outbox
	h.mu.Lock()
	h.note(&out, hash, ErrorMsg{Text: "progress could not be saved"})
	h.mu.Unlock()
	out.flush()
	h.logWarn("lesson persist failed", "trainer", hash, "err", err)
}

func participationFromBattle(bt *battle.Battle, trainer string) (active, reserve []string) {
	snap := bt.Snapshot(trainer)
	entered := map[string]struct{}{}
	for _, ev := range bt.Events() {
		switch ev.Kind {
		case battle.EventMoveUsed, battle.EventSwitched, battle.EventReplacement:
			if ev.Actor == trainer && ev.MonsterID != "" {
				entered[ev.MonsterID] = struct{}{}
			}
		case battle.EventSendOut:
			if ev.Actor == trainer && ev.MonsterID != "" {
				entered[ev.MonsterID] = struct{}{}
			}
		}
	}
	for _, m := range snap.YourParty {
		if _, ok := entered[m.ID]; ok {
			active = append(active, m.ID)
		}
	}
	for _, m := range snap.YourParty {
		if _, ok := entered[m.ID]; !ok {
			reserve = append(reserve, m.ID)
		}
	}
	return active, reserve
}
