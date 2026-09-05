package server

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"termon.sh/internal/battle"
	"termon.sh/internal/capture"
	"termon.sh/internal/dojo"
	"termon.sh/internal/expedition"
	"termon.sh/internal/game"
	"termon.sh/internal/lobby"
	"termon.sh/internal/store"
	"termon.sh/internal/telemetry"
)

// ExpeditionFamilyCard is one Signal Board entry for the current server day.
type ExpeditionFamilyCard struct {
	Slug  string
	Name  string
	Type  string
	Theme string
	Index int
}

// SignalBoardMsg lists today's three Evolution Families.
type SignalBoardMsg struct {
	DayIndex  int
	ServerDay string
	Families  []ExpeditionFamilyCard
}

// ExpeditionMsg updates route chrome between encounters.
type ExpeditionMsg struct {
	Phase           string // recovery | captured | hunt_failed | abandoned | failed
	RunID           string
	Family          string
	FamilyName      string
	WildSpecies     string
	RecoveryNext    string // prep2 | target
	LastEncounter   string // prep1 | prep2 | target
	LastXPGained    int64
	CapturedSpecies string
}

type expeditionRun struct {
	runID        string
	trainer      string
	phase        string // prep1 | prep2 | target | recovery
	family       string
	prep1        string
	prep2        string
	seed         uint64
	serverDay    time.Time
	saveBefore   *game.Save
	recoveryNext string
}

type expeditionEncounter string

const (
	expeditionPrep1  expeditionEncounter = "prep1"
	expeditionPrep2  expeditionEncounter = "prep2"
	expeditionTarget expeditionEncounter = "target"
)

type expeditionMode struct {
	soloMode
	// phase stays fixed for this match while expeditionRun.phase can advance
	// to recovery after the encounter commits.
	phase       expeditionEncounter
	runID       string
	wildSpecies string
	capture     *capture.Session
}

func newExpeditionMode(
	phase string,
	runID string,
	wildSpecies string,
	sv *game.Save,
	objectives []capture.Objective,
) (*expeditionMode, error) {
	solo := soloMode{saveBefore: cloneSaveXPView(sv)}
	mode := &expeditionMode{
		soloMode:    solo,
		runID:       runID,
		wildSpecies: wildSpecies,
	}
	switch expeditionEncounter(phase) {
	case expeditionPrep1, expeditionPrep2:
		if len(objectives) != 0 {
			return nil, errors.New("server: prep expedition mode has capture objectives")
		}
		mode.phase = expeditionEncounter(phase)
	case expeditionTarget:
		if len(objectives) == 0 {
			return nil, errors.New("server: target expedition mode needs capture objectives")
		}
		mode.phase = expeditionTarget
		mode.capture = capture.NewSession(objectives)
	default:
		return nil, fmt.Errorf("server: invalid expedition phase %q", phase)
	}
	if runID == "" || wildSpecies == "" {
		return nil, errors.New("server: incomplete expedition mode")
	}
	return mode, nil
}

func (mode *expeditionMode) afterAction(h *Hub, m *match, hash string, _ battle.Action, trainerMove string) bool {
	if mode.phase == expeditionTarget {
		h.afterExpeditionTurn(m, mode, hash, trainerMove)
		return h.matchCurrent(hash, m)
	}
	return true
}

func (mode *expeditionMode) finish(h *Hub, m *match) {
	h.finishExpeditionMatch(m, mode)
}

func (h *Hub) finishExpeditionMatch(m *match, mode *expeditionMode) {
	h.mu.Lock()
	if m.bt == nil || h.matches[m.a] != m {
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()

	if m.bt.State() == battle.StateOver {
		if mode.phase == expeditionTarget {
			h.mu.Lock()
			still := h.matches[m.a] == m
			h.mu.Unlock()
			if still {
				if m.bt.Winner() != m.a {
					h.failExpedition(m, mode, m.a)
				} else if wildFaintedFromSnap(m.bt.Snapshot(m.a)) && !mode.capture.Full() {
					h.finishExpeditionHuntFailed(m, mode, m.a)
				}
			}
		} else if m.bt.Winner() == m.a {
			h.finishExpeditionPrepWin(m, mode, m.a)
		} else {
			h.failExpedition(m, mode, m.a)
		}
	}

	var out outbox
	h.mu.Lock()
	m.pending = false
	m.committing = false
	if h.matches[m.a] == m {
		h.note(&out, m.a, BattleMsg{Battle: m.bt, You: m.a, Foe: m.handle[m.b], FoeHash: m.b})
		delete(h.matches, m.a)
		h.setPresenceLocked(m.a, func(p *lobby.Presence) { p.InBattle = false })
	}
	h.mu.Unlock()
	out.flush()
}

func (mode *expeditionMode) pushBattle(h *Hub, m *match) {
	h.pushBattleToTrainer(m, &mode.soloMode, string(mode.phase))
}

// OpenSignalBoard returns today's Families when adjacent to the notice board.
func (h *Hub) OpenSignalBoard(hash string) (SignalBoardMsg, error) {
	h.mu.Lock()
	room, _, ok := h.roomForLocked(hash)
	h.mu.Unlock()
	if !ok || !room.NearNoticeBoard(hash) {
		return SignalBoardMsg{}, playerFacing("stand next to the Signal Board")
	}
	return h.signalBoardMsg(time.Now().UTC()), nil
}

func (h *Hub) signalBoardMsg(now time.Time) SignalBoardMsg {
	day := expedition.ServerDay(now)
	idx := expedition.DayIndex(day)
	slugs := expedition.FamiliesForDay(day)
	msg := SignalBoardMsg{
		DayIndex:  idx + 1,
		ServerDay: day.Format("2006-01-02"),
	}
	for i, slug := range slugs {
		sp := h.set.Species[slug]
		msg.Families = append(msg.Families, ExpeditionFamilyCard{
			Slug: slug, Name: sp.Name, Type: sp.Type,
			Theme: expedition.SupportTheme(slug), Index: i,
		})
	}
	return msg
}

// LaunchExpedition snapshots a target Family and starts Preparation Encounter 1.
func (h *Hub) LaunchExpedition(hash string, familyIndexOrSlug string) error {
	family, err := h.resolveBoardFamily(hash, familyIndexOrSlug)
	if err != nil {
		return err
	}
	h.mu.Lock()
	if h.expeditions[hash] != nil {
		h.mu.Unlock()
		return playerFacing("already on an expedition")
	}
	if h.matches[hash] != nil {
		h.mu.Unlock()
		return playerFacing("already in battle")
	}
	h.mu.Unlock()

	sv, err := h.Load(hash)
	if err != nil || sv == nil {
		return missingParty(hash, err)
	}
	members, fighters, err := expeditionParty(sv)
	if err != nil {
		return err
	}
	prep1, prep2, err := expedition.DrawPreps(expedition.SupportPool(family), family, expeditionSeed(family, hash))
	if err != nil {
		return fmt.Errorf("server: expedition preps: %w", err)
	}
	seed, err := battle.RandomSeed()
	if err != nil {
		return fmt.Errorf("server: seed expedition: %w", err)
	}
	runID, err := newMatchID()
	if err != nil {
		return err
	}
	run := &expeditionRun{
		runID: runID, trainer: hash, phase: "prep1", family: family,
		prep1: prep1, prep2: prep2, seed: seed,
		serverDay:  expedition.ServerDay(time.Now().UTC()),
		saveBefore: cloneSaveXPView(sv),
	}
	h.mu.Lock()
	// Recheck under the install lock: slow work above ran unlocked, so a
	// concurrent launch (or a new battle) may have claimed the trainer meanwhile.
	if h.expeditions[hash] != nil {
		h.mu.Unlock()
		return playerFacing("already on an expedition")
	}
	if h.matches[hash] != nil {
		h.mu.Unlock()
		return playerFacing("already in battle")
	}
	h.expeditions[hash] = run
	h.mu.Unlock()
	if err := h.startExpeditionEncounter(hash, run, "prep1", prep1, members, fighters, sv, nil); err != nil {
		h.clearExpedition(hash, run.runID)
		return err
	}
	h.recordEvent(telemetry.Event{
		ID:   telemetry.DeterministicID(telemetry.EventExpeditionStarted, runID),
		Name: telemetry.EventExpeditionStarted, TrainerID: hash,
		Properties: map[string]any{"target_family": family},
	})
	return nil
}

// ContinueExpedition resumes from recovery into the next encounter.
func (h *Hub) ContinueExpedition(hash string) error {
	h.mu.Lock()
	run := h.expeditions[hash]
	if run == nil || run.phase != "recovery" {
		h.mu.Unlock()
		return playerFacing("no expedition to continue")
	}
	next := run.recoveryNext
	if next != "prep2" && next != "target" {
		h.mu.Unlock()
		return playerFacing("expedition cannot continue")
	}
	// Claim the transition under the lock so concurrent Continues cannot both
	// pass the phase check and start two encounters.
	run.phase = next
	h.mu.Unlock()
	sv, err := h.Load(hash)
	if err != nil || sv == nil {
		return missingParty(hash, err)
	}
	members, fighters, err := expeditionParty(sv)
	if err != nil {
		return err
	}
	wild := run.prep2
	if next == "target" {
		wild = run.family
	}
	var objs []capture.Objective
	if next == "target" {
		objs, err = capture.Generate(h.set, fighters, run.family)
		if err != nil {
			return err
		}
	}
	run.phase = next
	if err := h.startExpeditionEncounter(hash, run, next, wild, members, fighters, sv, objs); err != nil {
		// Give back the claim so the trainer can retry the continuation.
		h.mu.Lock()
		run.phase = "recovery"
		h.mu.Unlock()
		return err
	}
	return nil
}

// AbandonExpedition ends the active route and keeps committed encounter XP.
func (h *Hub) AbandonExpedition(hash string) error {
	h.mu.Lock()
	run := h.expeditions[hash]
	if run == nil {
		h.mu.Unlock()
		return playerFacing("no active expedition")
	}
	delete(h.expeditions, hash)
	m := h.matches[hash]
	h.mu.Unlock()
	if m != nil {
		if _, ok := m.mode.(*expeditionMode); ok {
			_ = m.bt.Forfeit(hash)
			h.finishMatch(m)
		}
	}
	var out outbox
	h.mu.Lock()
	h.setPresenceLocked(hash, func(p *lobby.Presence) { p.InBattle = false })
	h.note(&out, hash, ExpeditionMsg{Phase: "abandoned", RunID: run.runID, Family: run.family})
	h.note(&out, hash, h.snapshotLocked(hash))
	h.mu.Unlock()
	out.flush()
	return nil
}

func (h *Hub) resolveBoardFamily(hash, familyIndexOrSlug string) (string, error) {
	now := time.Now().UTC()
	fams := expedition.FamiliesForDay(now)
	if idx, err := strconv.Atoi(familyIndexOrSlug); err == nil {
		if idx >= 1 && idx <= 3 {
			return fams[idx-1], nil
		}
		if idx >= 0 && idx < 3 {
			return fams[idx], nil
		}
	}
	slug := strings.ToLower(strings.TrimSpace(familyIndexOrSlug))
	for _, f := range fams {
		if f == slug {
			return f, nil
		}
	}
	_ = hash
	return "", playerFacing("that Family is not on today's board")
}

func expeditionSeed(family, hash string) uint64 {
	var s uint64
	for _, c := range family + ":" + hash {
		s = s*31 + uint64(c&0xff) //nolint:gosec // hash mixing only
	}
	return s
}

func expeditionParty(sv *game.Save) ([]battle.PartyMember, []capture.PartyFighter, error) {
	var members []battle.PartyMember
	var fighters []capture.PartyFighter
	occupied := 0
	for i := range 3 {
		id := sv.Party[i]
		if id == "" {
			continue
		}
		m, ok := game.MonsterByID(sv, id)
		if !ok {
			return nil, nil, fmt.Errorf("server: missing party monster %q", id)
		}
		if len(m.BattleLoadout) < 4 {
			return nil, nil, playerFacing("every party monster needs four moves loaded")
		}
		occupied++
		members = append(members, battle.PartyMember{Monster: m})
		fighters = append(fighters, capture.PartyFighter{
			Species: m.Species, Level: m.Level, Loadout: append([]string(nil), m.BattleLoadout...),
		})
	}
	if occupied == 0 {
		return nil, nil, playerFacing("bring at least one party monster")
	}
	return members, fighters, nil
}

func (h *Hub) startExpeditionEncounter(
	hash string,
	run *expeditionRun,
	phase, wildSpecies string,
	members []battle.PartyMember,
	fighters []capture.PartyFighter,
	sv *game.Save,
	objs []capture.Objective,
) error {
	mode, err := newExpeditionMode(phase, run.runID, wildSpecies, sv, objs)
	if err != nil {
		return err
	}
	target := mode.phase == expeditionTarget
	wildSpec := h.set.Species[wildSpecies]
	wildLevel := capture.WildLevel(fighters)
	wildMoves, err := dojo.WildLoadout(h.set, wildSpecies)
	if err != nil {
		return err
	}
	wildMon := game.Monster{
		ID: dojo.BotTrainer + "-wild", Species: wildSpecies, Level: wildLevel,
		BattleLoadout: wildMoves,
	}
	wildMember := battle.PartyMember{Monster: wildMon}
	if target {
		captureHP := capture.TargetHP(h.set, fighters, wildSpec, wildLevel)
		wildMember.MaxHP = captureHP
	}
	trainerParty := battle.Party{Trainer: hash, Members: members}
	wildParty := battle.Party{
		Trainer: dojo.BotTrainer, Members: []battle.PartyMember{wildMember},
	}
	if target {
		wildParty.ClampOutgoingDamage = true
	}
	bt, err := battle.New(h.set, trainerParty, wildParty, battle.Seeded(run.seed+encounterSeed(phase)))
	if err != nil {
		return err
	}
	id, err := newMatchID()
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
		return playerFacing("cannot start expedition now")
	}
	h.matches[hash] = m
	h.setPresenceLocked(hash, func(p *lobby.Presence) { p.InBattle = true })
	h.broadcastTrainerDojoLocked(&out, hash)
	h.mu.Unlock()
	out.flush()
	h.pushBattle(m)
	if mode.capture != nil {
		h.pushCaptureState(m, mode.capture, "Target captured immediately", nil)
	}
	return nil
}

func encounterSeed(phase string) uint64 {
	switch phase {
	case "prep1":
		return 1
	case "prep2":
		return 2
	case "target":
		return 3
	default:
		return 0
	}
}

func (h *Hub) afterExpeditionTurn(m *match, mode *expeditionMode, hash string, trainerMove string) {
	turn := m.bt.Turn()
	events := m.bt.Events()
	snap := m.bt.Snapshot(hash)
	wildHP, wildMax := wildHPFromSnap(snap)
	in := capture.BuildTurnInput(events, turn, hash, dojo.BotTrainer, trainerMove, snap, wildHP, wildMax)
	newly := mode.capture.AfterTurn(in)
	if len(newly) > 0 {
		h.pushCaptureState(m, mode.capture, "Target captured immediately", newly)
	}
	wildFainted := wildFaintedFromSnap(snap)
	outcome := mode.capture.OutcomeAfterTurn(wildFainted)
	switch outcome {
	case "captured":
		h.finishExpeditionCaptured(m, mode, hash)
	case "hunt_failed":
		h.finishExpeditionHuntFailed(m, mode, hash)
	}
}

func (h *Hub) finishExpeditionCaptured(m *match, mode *expeditionMode, hash string) {
	active, reserve := participationFromBattle(m.bt, hash)
	run := h.expeditionRunForMode(hash, mode)
	if run == nil {
		h.noteExpeditionError(hash, errors.New("missing expedition run"))
		return
	}
	before := cloneSaveXPView(mode.saveBefore)
	rec := store.ActivityRecord{
		Kind: store.KindExpedition, NaturalKey: mode.runID + ":target", TrainerID: hash,
		ActiveIDs: active, ReserveIDs: reserve, Outcome: store.OutcomeCaptured,
		Capture:     &store.CaptureSpec{Species: mode.wildSpecies, FillParty: false},
		CompletedAt: time.Now(),
	}
	after, err := h.recordActivityResult(rec)
	if err != nil {
		h.noteExpeditionError(hash, err)
		return
	}
	h.clearExpedition(hash, mode.runID)
	h.releaseExpeditionMatch(hash)
	sp := h.set.Species[mode.wildSpecies]
	var out outbox
	h.mu.Lock()
	h.note(&out, hash, SaveMsg{Save: after})
	h.note(&out, hash, progressionDiff(before, after, active, reserve, h.set))
	h.note(&out, hash, ExpeditionMsg{
		Phase: "captured", RunID: mode.runID, Family: run.family,
		FamilyName: sp.Name, CapturedSpecies: mode.wildSpecies, LastXPGained: 100,
	})
	h.mu.Unlock()
	out.flush()
}

func (h *Hub) finishExpeditionHuntFailed(m *match, mode *expeditionMode, hash string) {
	active, reserve := participationFromBattle(m.bt, hash)
	run := h.expeditionRunForMode(hash, mode)
	if run == nil {
		return
	}
	before := cloneSaveXPView(mode.saveBefore)
	rec := store.ActivityRecord{
		Kind: store.KindExpedition, NaturalKey: mode.runID + ":target", TrainerID: hash,
		ActiveIDs: active, ReserveIDs: reserve, Outcome: store.OutcomeHuntFailed,
		CompletedAt: time.Now(),
	}
	after, err := h.recordActivityResult(rec)
	if err != nil {
		h.noteExpeditionError(hash, err)
		return
	}
	h.clearExpedition(hash, mode.runID)
	h.releaseExpeditionMatch(hash)
	var out outbox
	h.mu.Lock()
	h.note(&out, hash, SaveMsg{Save: after})
	h.note(&out, hash, progressionDiff(before, after, active, reserve, h.set))
	h.note(&out, hash, ExpeditionMsg{
		Phase: "hunt_failed", RunID: mode.runID, Family: run.family,
		LastEncounter: "target", LastXPGained: 65,
	})
	h.mu.Unlock()
	out.flush()
}

func (h *Hub) finishExpeditionPrepWin(m *match, mode *expeditionMode, hash string) {
	run := h.expeditionRunForMode(hash, mode)
	if run == nil {
		return
	}
	phase := string(mode.phase) // prep1 | prep2; doubles as the activity Outcome.
	outcome := phase
	active, reserve := participationFromBattle(m.bt, hash)
	before := cloneSaveXPView(mode.saveBefore)
	rec := store.ActivityRecord{
		Kind: store.KindExpedition, NaturalKey: mode.runID + ":" + phase, TrainerID: hash,
		ActiveIDs: active, ReserveIDs: reserve, Outcome: outcome,
		CompletedAt: time.Now(),
	}
	after, err := h.recordActivityResult(rec)
	if err != nil {
		h.noteExpeditionError(hash, err)
		return
	}
	mode.saveBefore = after
	next := "prep2"
	if phase == "prep2" {
		next = "target"
	}
	run.phase = "recovery"
	run.recoveryNext = next
	run.saveBefore = cloneSaveXPView(after)
	h.releaseExpeditionMatch(hash)
	sp := h.set.Species[run.family]
	var out outbox
	h.mu.Lock()
	h.note(&out, hash, SaveMsg{Save: after})
	h.note(&out, hash, progressionDiff(before, after, active, reserve, h.set))
	h.note(&out, hash, ExpeditionMsg{
		Phase: "recovery", RunID: mode.runID, Family: run.family, FamilyName: sp.Name,
		RecoveryNext: next, LastEncounter: phase, LastXPGained: 40,
	})
	h.mu.Unlock()
	out.flush()
}

func (h *Hub) failExpedition(_ *match, mode *expeditionMode, hash string) {
	run := h.expeditionRunForMode(hash, mode)
	h.clearExpedition(hash, mode.runID)
	h.releaseExpeditionMatch(hash)
	if run == nil {
		return
	}
	var out outbox
	h.mu.Lock()
	h.note(&out, hash, ExpeditionMsg{Phase: "failed", RunID: mode.runID, Family: run.family})
	h.note(&out, hash, h.snapshotLocked(hash))
	h.mu.Unlock()
	out.flush()
}

func (h *Hub) expeditionRunLocked(hash string) *expeditionRun {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.expeditions[hash]
}

func (h *Hub) expeditionRunForMode(hash string, mode *expeditionMode) *expeditionRun {
	h.mu.Lock()
	defer h.mu.Unlock()
	run := h.expeditions[hash]
	if run == nil || run.runID != mode.runID {
		return nil
	}
	return run
}

func (h *Hub) clearExpedition(hash, runID string) {
	h.mu.Lock()
	if run := h.expeditions[hash]; run != nil && run.runID == runID {
		delete(h.expeditions, hash)
	}
	h.mu.Unlock()
}

func (h *Hub) releaseExpeditionMatch(hash string) {
	var out outbox
	h.mu.Lock()
	delete(h.matches, hash)
	h.setPresenceLocked(hash, func(p *lobby.Presence) { p.InBattle = false })
	h.note(&out, hash, h.snapshotLocked(hash))
	h.broadcastTrainerDojoLocked(&out, hash)
	h.mu.Unlock()
	out.flush()
}

func (h *Hub) noteExpeditionError(hash string, err error) {
	var out outbox
	h.mu.Lock()
	h.note(&out, hash, ErrorMsg{Text: "progress could not be saved"})
	h.mu.Unlock()
	out.flush()
	h.logWarn("expedition persist failed", "trainer", hash, "err", err)
}
