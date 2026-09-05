package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
	"termon.sh/internal/game"
	"termon.sh/internal/lobby"
	"termon.sh/internal/store"
	"termon.sh/internal/telemetry"
)

// match holds state shared by every server-owned Battle mode. PvP matches use
// distinct Trainers in a and b and are indexed under both; solo matches use a
// Trainer in a, a server-controlled participant in b, and are indexed only
// under a.
type match struct {
	id     string
	bt     *battle.Battle
	a, b   string
	handle map[string]string
	mode   matchMode
	// committing gates concurrent finish attempts. pending means a PvP result
	// failed to persist; it remains retryable unless exhausted and may coexist
	// with committing while a retry is in flight. exhausted is legal only with
	// pending after the retry limit; completedAt is fixed by the first finish
	// attempt and reused by retries.
	pending     bool
	committing  bool
	startedAt   time.Time
	completedAt time.Time
	entryPath   string
	exhausted   bool
	// revealDeadline is read or written while h.mu is held. The reveal window
	// applies to every Battle so a stalled client cannot hold one open.
	revealDeadline time.Time
	// nextRetryAt and retryAttempts serialize result-save retry backoff.
	// Both are only read or written while h.mu is held.
	nextRetryAt   time.Time
	retryAttempts int
}

// matchMode is the private seam for behavior that varies between the five
// kinds of server-owned Battle. A match has exactly one concrete mode, so
// state from different modes cannot be combined on match.
type matchMode interface {
	afterAction(h *Hub, m *match, hash string, action battle.Action, trainerMove string) bool
	lockBot(h *Hub, m *match) error
	finish(h *Hub, m *match)
	pushBattle(h *Hub, m *match)
	tick(m *match, now time.Time) []decisionExpiry
}

type pvpMode struct {
	saveBefore map[string]*game.Save
	// Decision Clock state is protected by h.mu. Only PvP owns a clock.
	clockBank      [2]time.Duration
	clockDeadline  [2]time.Time
	clockStarted   [2]bool
	clockWasLocked [2]bool
	clockKeyTurn   int
	clockKeyState  battle.State
}

type soloMode struct {
	policyCfg    dojo.PolicyConfig
	lastDecision dojo.DecisionExplanation
	saveBefore   *game.Save
}

func (*pvpMode) afterAction(*Hub, *match, string, battle.Action, string) bool { return true }

func (*pvpMode) lockBot(*Hub, *match) error { return nil }

func (*pvpMode) pushBattle(h *Hub, m *match) {
	h.pushBattleToBoth(m)
}

// decisionExpiry pairs a PvP match with the Trainer whose Decision Clock ran
// out, collected under the hub lock and resolved outside it.
type decisionExpiry struct {
	m       *match
	trainer string
}

// tick owns all PvP Decision Clock behavior. Hub calls it under h.mu and
// applies returned expiries outside the lock.
func (mode *pvpMode) tick(m *match, now time.Time) []decisionExpiry {
	st := m.bt.State()
	if st != battle.StateAwaitingActions && st != battle.StateAwaitingReplacement {
		mode.clockKeyState = ""
		return nil
	}

	mode.sweepDecisionClock(m, now)
	var expired []decisionExpiry
	for i, trainer := range [2]string{m.a, m.b} {
		if !mode.clockDeadline[i].IsZero() && !m.bt.Locked(trainer) && now.After(mode.clockDeadline[i]) {
			expired = append(expired, decisionExpiry{m: m, trainer: trainer})
		}
	}
	return expired
}

// sweepDecisionClock tops up and rearms the clock banks when the Battle enters
// a new awaiting phase, and freezes a side's bank while its action is locked.
func (mode *pvpMode) sweepDecisionClock(m *match, now time.Time) {
	st := m.bt.State()
	turn := m.bt.Turn()
	if mode.clockKeyState != st || mode.clockKeyTurn != turn {
		mode.clockKeyState, mode.clockKeyTurn = st, turn
		for i, trainer := range [2]string{m.a, m.b} {
			owes := st == battle.StateAwaitingActions ||
				(st == battle.StateAwaitingReplacement && m.bt.Snapshot(trainer).ReplacementRequired)
			if !owes {
				mode.clockDeadline[i] = time.Time{}
				continue
			}
			if mode.clockStarted[i] {
				mode.clockBank[i] = min(mode.clockBank[i]+clockPhaseGain, clockBankCap)
			} else {
				mode.clockBank[i] = clockBankStart
				mode.clockStarted[i] = true
			}
			mode.clockDeadline[i] = now.Add(mode.clockBank[i])
			mode.clockWasLocked[i] = m.bt.Locked(trainer)
		}
	}
	for i, trainer := range [2]string{m.a, m.b} {
		if mode.clockDeadline[i].IsZero() {
			continue
		}
		locked := m.bt.Locked(trainer)
		switch {
		case locked && !mode.clockWasLocked[i]:
			// Locking stops the clock: freeze what remains of the bank.
			mode.clockBank[i] = mode.clockDeadline[i].Sub(now)
			mode.clockWasLocked[i] = true
		case !locked:
			mode.clockWasLocked[i] = false
		}
	}
}

func (s *soloMode) lockBot(h *Hub, m *match) error {
	return h.botLockSolo(m, s)
}

func (s *soloMode) pushBattle(h *Hub, m *match) {
	h.pushBattleToTrainer(m, s, "")
}

func (*soloMode) tick(*match, time.Time) []decisionExpiry { return nil }

func (h *Hub) pushFinishedSoloBattle(m *match, mode *soloMode) {
	var out outbox
	h.mu.Lock()
	m.pending = false
	h.note(&out, m.a, BattleMsg{Battle: m.bt, You: m.a, Foe: m.handle[m.b], FoeHash: m.b})
	if mode.lastDecision.ReasonCode != "" {
		h.note(&out, m.a, DecisionExplanationMsg{
			Text:       mode.lastDecision.PrimaryReason,
			ReasonCode: mode.lastDecision.ReasonCode,
			Tier:       mode.lastDecision.Tier,
		})
	}
	h.mu.Unlock()
	out.flush()
}

func (h *Hub) finishSoloWithReward(m *match, mode *soloMode, persistWin func() error) {
	h.pushFinishedSoloBattle(m, mode)

	// The committing latch stays held across the persist so a concurrent
	// finishMatch cannot double-persist the mode's reward.
	if m.bt.Winner() == m.a {
		if err := persistWin(); err != nil {
			h.noteLessonError(m.a, err)
		}
	}

	var out outbox
	h.mu.Lock()
	m.pending = false
	m.committing = false
	delete(h.matches, m.a)
	h.setPresenceLocked(m.a, func(p *lobby.Presence) { p.InBattle = false })
	h.broadcastTrainerDojoLocked(&out, m.a)
	h.mu.Unlock()
	out.flush()
}

func (m *match) other(hash string) string {
	if m.a == hash {
		return m.b
	}
	return m.a
}

// SetQueueParty validates and persists the roster selected immediately before
// matchmaking. Move choices remain the persistent Workbench Battle Loadouts.
func (h *Hub) SetQueueParty(hash string, party [3]string) error {
	save, err := h.Load(hash)
	if err != nil || save == nil {
		return playerFacing("cannot prepare a Battle Party now")
	}
	candidate := *save
	candidate.Party = party
	if err := game.RequireFullParty(&candidate); err != nil {
		return playerFacing("choose three Monsters with Battle Loadouts first")
	}
	if _, err := h.ensureQueueSets(hash, &candidate); err != nil {
		return err
	}
	return h.SetParty(hash, party)
}

// FindBattle joins the FIFO Queue and pairs immediately when possible.
func (h *Hub) FindBattle(hash string) (position, waiting int, err error) {
	sv, err := h.Load(hash)
	if err != nil || sv == nil {
		return 0, 0, playerFacing("cannot queue now")
	}
	if err := game.RequireFullParty(sv); err != nil {
		return 0, 0, playerFacing("need a full party of three with loadouts to queue")
	}
	if _, err := h.ensureQueueSets(hash, sv); err != nil {
		return 0, 0, err
	}
	var out outbox
	h.mu.Lock()
	room, _, ok := h.roomForLocked(hash)
	if !ok {
		h.mu.Unlock()
		return 0, 0, playerFacing("cannot queue now")
	}
	p, ok := room.Get(hash)
	if !ok || p.InBattle {
		h.mu.Unlock()
		return 0, 0, playerFacing("cannot queue now")
	}
	pos, wait, err := h.queue.Join(hash, time.Now())
	if err != nil {
		h.mu.Unlock()
		return 0, 0, err
	}
	h.observeQueueJoin(QueueJoined)
	room.Set(hash, func(pr *lobby.Presence) { pr.InQueue = true })
	a, b, waitA, waitB, paired := h.queue.Pair(time.Now())
	h.broadcastTrainerDojoLocked(&out, hash)
	if !paired {
		h.note(&out, hash, QueueMsg{Position: pos, Waiting: wait})
	}
	h.mu.Unlock()
	out.flush()
	h.recordEvent(telemetry.Event{Name: telemetry.EventQueueJoined, TrainerID: hash})
	if paired {
		h.observeQueueJoin(QueuePaired)
		h.observeQueueWait(waitA)
		h.observeQueueWait(waitB)
		h.recordEvent(telemetry.Event{
			Name: telemetry.EventQueuePaired, TrainerID: a,
			Properties: map[string]any{"wait_seconds": waitA.Seconds()},
		})
		h.recordEvent(telemetry.Event{
			Name: telemetry.EventQueuePaired, TrainerID: b,
			Properties: map[string]any{"wait_seconds": waitB.Seconds()},
		})
		return 0, 0, h.startMatchFrom(a, b, "queue")
	}
	return pos, wait, nil
}

// CancelQueue leaves the waiting line.
func (h *Hub) CancelQueue(hash string) {
	var out outbox
	h.mu.Lock()
	cancelled := h.queue.Cancel(hash)
	h.setPresenceLocked(hash, func(p *lobby.Presence) { p.InQueue = false })
	h.note(&out, hash, h.snapshotLocked(hash))
	h.broadcastTrainerDojoLocked(&out, hash)
	h.mu.Unlock()
	out.flush()
	if cancelled {
		h.observeQueueJoin(QueueCancelled)
		h.recordEvent(telemetry.Event{Name: telemetry.EventQueueCancelled, TrainerID: hash})
	}
}

// SelectAction locks a Battle Action for this Trainer.
func (h *Hub) SelectAction(hash string, action battle.Action) error {
	h.mu.Lock()
	m := h.matches[hash]
	h.mu.Unlock()
	if m == nil {
		return playerFacing("not in battle")
	}
	trainerMove := ""
	if action.Kind == battle.ActionMove {
		trainerMove = action.Move
	}
	if err := m.bt.Select(hash, action); err != nil {
		return err
	}
	if err := m.mode.lockBot(h, m); err != nil {
		return err
	}
	if !m.mode.afterAction(h, m, hash, action, trainerMove) {
		return nil
	}
	if m.bt.State() == battle.StateOver {
		h.finishMatch(m)
	} else {
		h.pushBattle(m)
	}
	return nil
}

func (h *Hub) matchCurrent(hash string, m *match) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.matches[hash] == m
}

// Replace sends out a reserve Monster during awaiting_replacement.
func (h *Hub) Replace(hash, monsterID string) error {
	h.mu.Lock()
	m := h.matches[hash]
	h.mu.Unlock()
	if m == nil {
		return playerFacing("not in battle")
	}
	if err := m.bt.Replace(hash, monsterID); err != nil {
		return err
	}
	if err := m.mode.lockBot(h, m); err != nil {
		return err
	}
	if m.bt.State() == battle.StateOver {
		h.finishMatch(m)
	} else {
		h.pushBattle(m)
	}
	return nil
}

// AdvanceReveal ends the revealing playback window for this Trainer's Battle.
func (h *Hub) AdvanceReveal(hash string) error {
	h.mu.Lock()
	m := h.matches[hash]
	h.mu.Unlock()
	if m == nil {
		return playerFacing("not in battle")
	}
	if err := m.bt.AdvanceReveal(); err != nil {
		return err
	}
	if m.bt.State() == battle.StateAwaitingReplacement {
		if err := m.mode.lockBot(h, m); err != nil {
			return err
		}
	}
	h.pushBattle(m)
	return nil
}

// Forfeit surrenders the current Battle.
func (h *Hub) Forfeit(hash string) error {
	h.mu.Lock()
	m := h.matches[hash]
	h.mu.Unlock()
	if m == nil {
		return playerFacing("not in battle")
	}
	if err := m.bt.Forfeit(hash); err != nil {
		return err
	}
	h.finishMatch(m)
	return nil
}

// maxResultRetries caps result-save retries: past it the match enters a
// terminal exhausted state that stops retrying and releases the trainers.
// The outcome stays unrecorded (never partially written) and the Battle
// Result stays pending, so a later fix can still persist it by hand; see
// docs/design/durability.md for the at-least-once intent this bounds.
const maxResultRetries = 10

// Decision Clock parameters (docs/design/party-battles.md): each Trainer
// banks 60s, gains 10s at every awaiting phase up to the cap, and loses the
// Battle on exhaustion with the decision_timeout reason.
const (
	clockBankStart = 60 * time.Second
	clockPhaseGain = 10 * time.Second
	clockBankCap   = 60 * time.Second

	// revealWindow is the server-owned bound on the revealing playback phase.
	// It must comfortably exceed legitimate TUI narration (100ms ticks with
	// per-group holds) so normal playback never trips it, while still
	// releasing a Battle a client refuses to acknowledge.
	revealWindow = 12 * time.Second
)

// retryPendingResults re-attempts persisted-result saves for matches whose
// retry backoff has elapsed at now.
func (h *Hub) retryPendingResults(now time.Time) {
	h.mu.Lock()
	pending := make(map[string]*match)
	for _, m := range h.matches {
		if m.pending && !m.exhausted && (m.nextRetryAt.IsZero() || !now.Before(m.nextRetryAt)) {
			pending[m.id] = m
		}
	}
	h.mu.Unlock()
	for _, m := range pending {
		h.finishMatch(m)
	}
}

// expireDrop forfeits a trainer whose reconnect grace lapsed at now. It
// re-validates under the lock that the drop is still pending with the same
// or earlier deadline: a reattach (Attach deletes the drop) or a fresher
// drop cancels the timeout.
func (h *Hub) expireDrop(hash string, now time.Time) {
	h.mu.Lock()
	deadline, ok := h.drops[hash]
	if !ok || now.Before(deadline) {
		// The trainer reconnected (Attach removed the drop) or a fresher
		// drop replaced the expired snapshot: nothing to forfeit.
		h.mu.Unlock()
		return
	}
	delete(h.drops, hash)
	m := h.matches[hash]
	h.mu.Unlock()
	if m != nil && m.bt.State() != battle.StateOver {
		_ = m.bt.DisconnectTimeout(hash)
		h.finishMatch(m)
	}
	var out outbox
	h.mu.Lock()
	h.forgetSeat(hash, &out)
	h.mu.Unlock()
	out.flush()
}

// busyLocked reports which party, if any, cannot enter a new Battle right
// now: already seated in a live match, or flagged InBattle in their dojo.
func (h *Hub) busyLocked(a, b string) string {
	for _, hash := range [2]string{a, b} {
		if h.matches[hash] != nil {
			return hash
		}
		if room, _, ok := h.roomForLocked(hash); ok {
			if p, seated := room.Get(hash); seated && p.InBattle {
				return hash
			}
		}
	}
	return ""
}

// missingParty builds the error for a Trainer whose Save is missing or has
// no Party, wrapping the underlying store cause when there is one.
func missingParty(hash string, err error) error {
	if err != nil {
		return fmt.Errorf("server: missing party for %s: %w", hash, err)
	}
	return fmt.Errorf("server: missing party for %s", hash)
}

// abandonMatch unwinds a pair whose Battle failed to start: it clears any
// stale queue flag from their presence, tells both Trainers what happened,
// and refreshes their views so neither is stranded mid-flow. Trainers who
// meanwhile gained a legitimate seat elsewhere are left untouched.
func (h *Hub) abandonMatch(a, b string, cause error) {
	var out outbox
	h.mu.Lock()
	for _, hash := range [2]string{a, b} {
		if h.matches[hash] != nil {
			continue
		}
		h.setPresenceLocked(hash, func(p *lobby.Presence) { p.InQueue = false })
		h.note(&out, hash, ErrorMsg{Text: "the battle could not start; please try again"})
		h.note(&out, hash, h.snapshotLocked(hash))
		h.broadcastTrainerDojoLocked(&out, hash)
	}
	h.mu.Unlock()
	out.flush()
	h.logWarn("match start failed", "trainers", []string{a, b}, "err", cause)
}

func normalizedBattleParty(set *content.Set, trainer string, save *game.Save, sets [3][]string) (battle.Party, error) {
	p := battle.Party{Trainer: trainer}
	for i, id := range save.Party {
		m, ok := game.MonsterByID(save, id)
		if !ok {
			return battle.Party{}, fmt.Errorf("server: missing party monster %q", id)
		}
		nm, err := game.NormalizedMonster(set, m, sets[i])
		if err != nil {
			return battle.Party{}, err
		}
		st := game.QueueStats(set.Species[m.Species])
		stHeap := st
		p.Members = append(p.Members, battle.PartyMember{Monster: nm, Stats: allocStats(stHeap)})
	}
	return p, nil
}

func (h *Hub) ensureQueueSets(_ string, sv *game.Save) ([3][]string, error) {
	var out [3][]string
	for i, id := range sv.Party {
		m, ok := game.MonsterByID(sv, id)
		if !ok {
			return out, playerFacing("invalid party")
		}
		moves := append([]string(nil), m.BattleLoadout...)
		if err := game.ValidateQueueMoveSet(h.set, m, moves); err != nil {
			return out, playerFacing("adjust this party's Battle Loadouts in the Workbench first")
		}
		out[i] = moves
	}
	return out, nil
}

func (h *Hub) queueSetsFor(hash string, sv *game.Save) ([3][]string, error) {
	return h.ensureQueueSets(hash, sv)
}

func (h *Hub) startMatch(a, b string) error {
	return h.startMatchFrom(a, b, "direct")
}

func (h *Hub) startMatchFrom(a, b, entryPath string) error {
	svA, err := h.Load(a)
	if err != nil || svA == nil {
		err = missingParty(a, err)
		h.abandonMatch(a, b, err)
		return err
	}
	if err := game.RequireFullParty(svA); err != nil {
		err = playerFacing("need a full party of three with loadouts")
		h.abandonMatch(a, b, err)
		return err
	}
	setsA, err := h.queueSetsFor(a, svA)
	if err != nil {
		h.abandonMatch(a, b, err)
		return err
	}
	svB, err := h.Load(b)
	if err != nil || svB == nil {
		err = missingParty(b, err)
		h.abandonMatch(a, b, err)
		return err
	}
	if err := game.RequireFullParty(svB); err != nil {
		err = playerFacing("need a full party of three with loadouts")
		h.abandonMatch(a, b, err)
		return err
	}
	setsB, err := h.queueSetsFor(b, svB)
	if err != nil {
		h.abandonMatch(a, b, err)
		return err
	}
	seed, err := battle.RandomSeed()
	if err != nil {
		err = fmt.Errorf("server: seed Battle: %w", err)
		h.abandonMatch(a, b, err)
		return err
	}
	partyA, err := normalizedBattleParty(h.set, a, svA, setsA)
	if err != nil {
		h.abandonMatch(a, b, err)
		return err
	}
	partyB, err := normalizedBattleParty(h.set, b, svB, setsB)
	if err != nil {
		h.abandonMatch(a, b, err)
		return err
	}
	bt, err := battle.New(h.set, partyA, partyB, battle.Seeded(seed))
	if err != nil {
		h.abandonMatch(a, b, err)
		return err
	}
	id, err := newMatchID()
	if err != nil {
		h.abandonMatch(a, b, err)
		return err
	}
	m := &match{
		id: id, bt: bt, a: a, b: b, startedAt: time.Now().UTC(), entryPath: entryPath,
		handle: map[string]string{a: svA.Handle, b: svB.Handle},
		mode: &pvpMode{saveBefore: map[string]*game.Save{
			a: cloneSaveXPView(svA), b: cloneSaveXPView(svB),
		}},
	}
	var out outbox
	h.mu.Lock()
	if busy := h.busyLocked(a, b); busy != "" {
		h.note(&out, a, ErrorMsg{Text: "challenge can no longer start"})
		h.note(&out, b, ErrorMsg{Text: "challenge can no longer start"})
		// Pairing already popped the Queue, so drop any stale flag the
		// free trainer still carries instead of stranding them queued.
		h.setPresenceLocked(a, func(p *lobby.Presence) { p.InQueue = false })
		h.setPresenceLocked(b, func(p *lobby.Presence) { p.InQueue = false })
		h.broadcastTrainersDojosLocked(&out, a, b)
		h.mu.Unlock()
		out.flush()
		return fmt.Errorf("server: trainer %s is no longer available", busy)
	}
	h.matches[a] = m
	h.matches[b] = m
	h.queue.Cancel(a)
	h.queue.Cancel(b)
	delete(h.challenges, a)
	delete(h.challenges, b)
	h.setPresenceLocked(a, func(p *lobby.Presence) { p.InBattle = true; p.InQueue = false })
	h.setPresenceLocked(b, func(p *lobby.Presence) { p.InBattle = true; p.InQueue = false })
	h.broadcastTrainersDojosLocked(&out, a, b)
	h.mu.Unlock()
	out.flush()
	h.pushBattle(m)
	for _, trainerID := range []string{a, b} {
		h.recordEvent(telemetry.Event{
			ID:   telemetry.DeterministicID(telemetry.EventBattleStarted, id, trainerID),
			Name: telemetry.EventBattleStarted, TrainerID: trainerID, BattleID: id,
			Properties: map[string]any{"entry_path": entryPath},
		})
	}
	return nil
}

func (h *Hub) finishMatch(m *match) {
	h.mu.Lock()
	if m.bt == nil {
		h.mu.Unlock()
		return
	}
	if cur := h.matches[m.a]; cur != m {
		h.mu.Unlock()
		return
	}
	if m.committing {
		h.mu.Unlock()
		return
	}
	m.committing = true
	if m.completedAt.IsZero() {
		m.completedAt = time.Now()
	}
	h.mu.Unlock()
	m.mode.finish(h, m)
}

func (mode *pvpMode) finish(h *Hub, m *match) {
	h.finishPvPMatch(m, mode)
}

func summarizeBattle(m *match, mode *pvpMode) store.BattleStats {
	startedAt := m.startedAt
	if startedAt.IsZero() || startedAt.After(m.completedAt) {
		startedAt = m.completedAt
	}
	stats := store.BattleStats{
		Version: 1, StartedAt: startedAt, DurationMS: m.completedAt.Sub(startedAt).Milliseconds(),
		Turns: m.bt.Turn(), EntryPath: m.entryPath,
		TrainerStats: []store.BattleTrainerStats{{TrainerID: m.a}, {TrainerID: m.b}},
	}
	byTrainer := map[string]*store.BattleTrainerStats{
		m.a: &stats.TrainerStats[0], m.b: &stats.TrainerStats[1],
	}
	for trainerID, before := range mode.saveBefore {
		current := byTrainer[trainerID]
		if current == nil || before == nil {
			continue
		}
		for _, monsterID := range before.Party {
			if monsterID == "" {
				continue
			}
			monster, ok := game.MonsterByID(before, monsterID)
			if ok {
				current.Monsters = append(current.Monsters, store.BattleMonster{ID: monster.ID, Species: monster.Species})
			}
		}
	}
	winner := m.bt.Winner()
	for trainerID, current := range byTrainer {
		current.Result = "lost"
		if trainerID == winner {
			current.Result = "won"
		}
	}
	lastMove := make(map[string]string, len(byTrainer))
	for _, event := range m.bt.Events() {
		current := byTrainer[event.Actor]
		if current == nil {
			continue
		}
		switch event.Kind {
		case battle.EventMoveUsed:
			current.Moves++
			lastMove[event.Actor] = event.MoveSlug
			moveStats(current, event.MoveSlug).Uses++
		case battle.EventMissed:
			current.Misses++
			moveStats(current, lastMove[event.Actor]).Misses++
		case battle.EventCriticalHit:
			current.CriticalHits++
			moveStats(current, lastMove[event.Actor]).CriticalHits++
		case battle.EventSwitched, battle.EventReplacement:
			current.Switches++
		case battle.EventDamageDealt:
			current.DamageDealt += event.Damage
			moveStats(current, lastMove[event.Actor]).DamageDealt += event.Damage
		case battle.EventFainted:
			current.Faints++
		}
	}
	return stats
}

func moveStats(trainer *store.BattleTrainerStats, slug string) *store.BattleMoveStats {
	for i := range trainer.MoveStats {
		if trainer.MoveStats[i].Slug == slug {
			return &trainer.MoveStats[i]
		}
	}
	trainer.MoveStats = append(trainer.MoveStats, store.BattleMoveStats{Slug: slug})
	return &trainer.MoveStats[len(trainer.MoveStats)-1]
}

func (h *Hub) finishPvPMatch(m *match, mode *pvpMode) {
	refreshed := make(map[string]*game.Save, 2)
	winner := m.bt.Winner()
	loser := m.other(winner)
	reason := string(m.bt.Reason())
	applyRewards := reason == string(battle.EndKO)
	winActive, winReserve := participationFromBattle(m.bt, winner)
	loseActive, loseReserve := participationFromBattle(m.bt, loser)
	beforeWinner := cloneSaveXPView(mode.saveBefore[winner])
	beforeLoser := cloneSaveXPView(mode.saveBefore[loser])
	records, err := h.saves.RecordBattleResult(store.BattleRecord{
		Result: store.BattleResult{
			ID: m.id, Winner: winner, Loser: loser,
			Reason: reason, CompletedAt: m.completedAt,
		},
		WinnerActive: winActive, WinnerReserve: winReserve,
		LoserActive: loseActive, LoserReserve: loseReserve,
		ApplyRewards: applyRewards,
		Stats:        summarizeBattle(m, mode),
	})
	if err != nil {
		var out outbox
		h.mu.Lock()
		m.pending = true
		m.committing = false
		m.retryAttempts++
		backoff := (1 << min(m.retryAttempts, 5)) * 500 * time.Millisecond
		m.nextRetryAt = time.Now().Add(backoff)
		if m.retryAttempts >= maxResultRetries {
			m.exhausted = true
			h.logWarn("result save failed; giving up",
				"match", m.id, "attempt", m.retryAttempts, "terminal", true,
				"trainers", []string{m.a, m.b}, "err", err)
			h.note(&out, m.a, ErrorMsg{Text: "progress could not be saved; ask the operator"})
			h.note(&out, m.b, ErrorMsg{Text: "progress could not be saved; ask the operator"})
			if cur := h.matches[m.a]; cur == m {
				delete(h.matches, m.a)
			}
			if cur := h.matches[m.b]; cur == m {
				delete(h.matches, m.b)
			}
			delete(h.drops, m.a)
			delete(h.drops, m.b)
			h.setPresenceLocked(m.a, func(p *lobby.Presence) { p.InBattle = false })
			h.setPresenceLocked(m.b, func(p *lobby.Presence) { p.InBattle = false })
			h.broadcastTrainersDojosLocked(&out, m.a, m.b)
			h.mu.Unlock()
			out.flush()
			return
		}
		h.logWarn("result save failed; will retry",
			"match", m.id, "attempt", m.retryAttempts,
			"trainers", []string{m.a, m.b}, "err", err)
		if m.retryAttempts == 1 {
			h.note(&out, m.a, ErrorMsg{Text: "result save failed; retrying"})
			h.note(&out, m.b, ErrorMsg{Text: "result save failed; retrying"})
		}
		h.mu.Unlock()
		out.flush()
		return
	}
	refreshed[winner] = records.Winner
	refreshed[loser] = records.Loser
	if records.Applied {
		for _, trainerID := range []string{winner, loser} {
			result := "lost"
			if trainerID == winner {
				result = "won"
			}
			h.recordEvent(telemetry.Event{
				ID:   telemetry.DeterministicID(telemetry.EventBattleEnded, m.id, trainerID),
				Name: telemetry.EventBattleEnded, TrainerID: trainerID, BattleID: m.id,
				Properties: map[string]any{
					"result": result, "reason": reason, "turn_count": m.bt.Turn(),
					"duration_seconds": m.completedAt.Sub(m.startedAt).Seconds(), "entry_path": m.entryPath,
				},
			})
		}
	}
	var out outbox
	h.mu.Lock()
	m.pending = false
	m.committing = false
	m.retryAttempts = 0
	m.nextRetryAt = time.Time{}
	h.note(&out, m.a, BattleMsg{Battle: m.bt, You: m.a, Foe: m.handle[m.b], FoeHash: m.b})
	h.note(&out, m.b, BattleMsg{Battle: m.bt, You: m.b, Foe: m.handle[m.a], FoeHash: m.a})
	for hash, sv := range refreshed {
		h.note(&out, hash, SaveMsg{Save: sv})
		if applyRewards {
			active, reserve := winActive, winReserve
			before := beforeWinner
			if hash == loser {
				active, reserve = loseActive, loseReserve
				before = beforeLoser
			}
			h.note(&out, hash, progressionDiff(before, sv, active, reserve, h.set))
		}
	}
	if cur := h.matches[m.a]; cur == m {
		delete(h.matches, m.a)
	}
	if cur := h.matches[m.b]; cur == m {
		delete(h.matches, m.b)
	}
	delete(h.drops, m.a)
	delete(h.drops, m.b)
	h.setPresenceLocked(m.a, func(p *lobby.Presence) { p.InBattle = false })
	h.setPresenceLocked(m.b, func(p *lobby.Presence) { p.InBattle = false })
	h.mu.Unlock()
	out.flush()
}

func allocStats(st [5]int) *[5]int {
	cp := st
	return &cp
}

func newMatchID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("server: generate Battle ID: %w", err)
	}
	return hex.EncodeToString(id[:]), nil
}

func (h *Hub) pushBattle(m *match) {
	m.mode.pushBattle(h, m)
}

func (h *Hub) pushBattleToBoth(m *match) {
	var out outbox
	h.mu.Lock()
	h.note(&out, m.a, BattleMsg{Battle: m.bt, You: m.a, Foe: m.handle[m.b], FoeHash: m.b})
	h.note(&out, m.b, BattleMsg{Battle: m.bt, You: m.b, Foe: m.handle[m.a], FoeHash: m.a})
	h.mu.Unlock()
	out.flush()
}

func (h *Hub) pushBattleToTrainer(m *match, mode *soloMode, expeditionPhase string) {
	var out outbox
	h.mu.Lock()
	decision := mode.lastDecision
	msg := BattleMsg{
		Battle: m.bt, You: m.a, Foe: m.handle[m.b], FoeHash: m.b,
		ExpeditionPhase: expeditionPhase,
	}
	if decision.ReasonCode != "" {
		msg.DecisionText = decision.PrimaryReason
	}
	h.note(&out, m.a, msg)
	h.mu.Unlock()
	out.flush()
}
