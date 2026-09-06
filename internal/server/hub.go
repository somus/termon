// Package server provides the Hub, which owns live session, Lobby, Queue, and
// match coordination and mediates durable Trainer progression through Store.
package server

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/game"
	"termon.sh/internal/lobby"
	"termon.sh/internal/onboard"
	"termon.sh/internal/queue"
	"termon.sh/internal/session"
	"termon.sh/internal/store"
	"termon.sh/internal/telemetry"
)

// SnapshotMsg is the Lobby view for one Trainer.
type SnapshotMsg struct {
	// Sequence orders captures within this Hub, not delivery or persistence.
	Sequence uint64
	You      lobby.Presence
	Others   []lobby.Presence
	Offer    *ChallengeOffer
	Dojo     int
	Context  string
}

// HubStats is a point-in-time count of process-owned coordination state.
type HubStats struct {
	Dojos          int
	Trainers       int
	ActiveSessions int
	Battles        int
	Queued         int
}

// ChallengeOffer is an incoming Challenge waiting on a response.
type ChallengeOffer struct {
	FromHash   string
	FromHandle string
}

// QueueMsg is the waiting-screen state.
type QueueMsg struct {
	Position int
	Waiting  int
}

// BattleMsg is a shared Battle plus which Trainer this view belongs to.
type BattleMsg struct {
	Battle          *battle.Battle
	You             string
	Foe             string
	FoeHash         string
	ExpeditionPhase string // prep1 | prep2 | target
	DecisionText    string
}

// SaveMsg replaces a connected Trainer's in-memory persistent state.
type SaveMsg struct {
	Save *game.Save
}

// DisplacedMsg means a newer connection took this seat.
type DisplacedMsg struct{}

// ReconnectingMsg is the opponent's disconnect-grace banner.
// An empty Handle clears it.
type ReconnectingMsg struct {
	Handle string
}

// ErrorMsg is a short status line for the TUI.
type ErrorMsg struct{ Text string }

type challenge struct {
	from, to string
	deadline time.Time
}

type client struct {
	send       func(any)
	kill       func()
	sessionID  string
	startedAt  time.Time
	endMu      sync.Mutex
	endReason  string
	finishOnce sync.Once
}

func (c *client) setEndReason(reason string) {
	c.endMu.Lock()
	c.endReason = reason
	c.endMu.Unlock()
}

func (c *client) getEndReason() string {
	c.endMu.Lock()
	defer c.endMu.Unlock()
	return c.endReason
}

func (c *client) Kill() {
	if c.send != nil {
		c.send(DisplacedMsg{})
	}
	if c.kill != nil {
		c.kill()
	}
}

// Hub is the process-owned game coordinator. Its state slices are grouped
// by concern in hub_dojo.go (Dojo seating and broadcasts), hub_challenge.go
// (pending Challenges), and hub_match.go (Battles and result persistence);
// every method there runs while holding h.mu.
type Hub struct {
	set           *content.Set
	saves         store.Store
	admission     Admission
	registrations *registrationLimiter

	mu          sync.Mutex
	sessions    *session.Registry
	clients     map[string]*client
	dojos       map[int]*lobby.Room
	dojoOf      map[string]int
	nextDojo    int
	queue       *queue.Queue
	challenges  map[string]*challenge // keyed by the recipient
	matches     map[string]*match
	expeditions map[string]*expeditionRun
	drops       map[string]time.Time // disconnected fighter → grace deadline
	dirtyDojos  map[int]bool
	// Allocated under mu so delayed outboxes cannot rewind a newer view.
	snapshotSequence uint64

	instruments Instruments
	logger      *slog.Logger
	telemetry   telemetry.Recorder
}

// Instrument attaches optional metrics and structured logging. Call before
// serving; both arguments may be nil to leave that signal off.
func (h *Hub) Instrument(instruments Instruments, logger *slog.Logger) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.instruments = instruments
	h.logger = logger
}

// RecordEvents attaches optional structured product telemetry.
func (h *Hub) RecordEvents(recorder telemetry.Recorder) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.telemetry = recorder
}

// NewHub constructs an empty Dojo.
func NewHub(set *content.Set, saves store.Store, admission Admission) *Hub {
	first := lobby.NewDojo()
	return &Hub{
		set:           set,
		saves:         saves,
		admission:     admission,
		registrations: newRegistrationLimiter(admission),
		sessions:      session.NewRegistry(),
		clients:       map[string]*client{},
		dojos:         map[int]*lobby.Room{1: first},
		dojoOf:        map[string]int{},
		nextDojo:      1,
		queue:         &queue.Queue{},
		challenges:    map[string]*challenge{},
		matches:       map[string]*match{},
		expeditions:   map[string]*expeditionRun{},
		drops:         map[string]time.Time{},
		dirtyDojos:    map[int]bool{},
	}
}

// Attach registers a live connection, displacing any older one for hash.
// The returned func forgets this connection if it is still the live one.
func (h *Hub) Attach(hash string, send func(any), kill func()) (detach func()) {
	return h.AttachSession(hash, "", send, kill)
}

// AttachSession registers a correlated SSH session and returns its detach hook.
func (h *Hub) AttachSession(hash, sessionID string, send func(any), kill func()) (detach func()) {
	startedAt := time.Now().UTC()
	c := &client{send: send, kill: kill, sessionID: sessionID, startedAt: startedAt, endReason: "client_disconnect"}
	old := h.sessions.Claim(hash, c)
	var out outbox
	h.mu.Lock()
	h.clients[hash] = c
	if _, dropped := h.drops[hash]; dropped {
		delete(h.drops, hash)
		if m := h.matches[hash]; m != nil {
			h.note(&out, m.other(hash), ReconnectingMsg{})
		}
	}
	h.mu.Unlock()
	out.flush()
	if old != nil {
		h.observeDisplacement(hash)
		if prior, ok := old.(*client); ok {
			prior.setEndReason("displaced")
		}
		old.Kill()
	}
	if sessionID != "" {
		h.recordEvent(telemetry.Event{
			Name: telemetry.EventSessionStarted, TrainerID: hash, SessionID: sessionID,
			Properties: map[string]any{"resume_target": h.resumeTarget(hash)},
		})
	}
	return func() {
		var out outbox
		h.mu.Lock()
		if h.clients[hash] != c {
			h.mu.Unlock()
			h.sessions.Drop(hash, c)
			h.finishSession(hash, c)
			return
		}
		delete(h.clients, hash)
		m := h.matches[hash]
		if m != nil && m.bt.State() != battle.StateOver {
			h.drops[hash] = time.Now().Add(battle.DisconnectGrace)
			h.note(&out, m.other(hash), ReconnectingMsg{Handle: m.handle[hash]})
		} else {
			h.forgetSeat(hash, &out)
		}
		h.mu.Unlock()
		h.sessions.Drop(hash, c)
		out.flush()
		h.finishSession(hash, c)
	}
}

func (h *Hub) finishSession(hash string, c *client) {
	c.finishOnce.Do(func() {
		if c.sessionID == "" {
			return
		}
		endedAt := time.Now().UTC()
		reason := c.getEndReason()
		if err := h.saves.EndSession(c.sessionID, endedAt, reason); err != nil {
			h.logWarn("session end persistence failed", "trainer_id", hash, "session_id", c.sessionID, "err", err)
		}
		h.recordEvent(telemetry.Event{
			Name: telemetry.EventSessionEnded, TrainerID: hash, SessionID: c.sessionID,
			Properties: map[string]any{
				"duration_seconds": endedAt.Sub(c.startedAt).Seconds(), "end_reason": reason,
			},
		})
	})
}

// StartSession persists session identity before it becomes visible to support tooling.
func (h *Hub) StartSession(trainerID, sessionID, appVersion string) error {
	return h.saves.StartSession(store.SessionRecord{
		ID: sessionID, TrainerID: trainerID, StartedAt: time.Now().UTC(),
		ResumeTarget: h.resumeTarget(trainerID), AppVersion: appVersion,
	})
}

func (h *Hub) resumeTarget(hash string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m := h.matches[hash]; m != nil && m.bt.State() != battle.StateOver {
		return "battle"
	}
	return "lobby"
}

func (h *Hub) recordEvent(event telemetry.Event) {
	if h.telemetry != nil {
		if event.SessionID == "" {
			event.SessionID = h.sessionID(event.TrainerID)
		}
		h.telemetry.Record(event)
	}
}

func (h *Hub) sessionID(trainerID string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c := h.clients[trainerID]; c != nil {
		return c.sessionID
	}
	return ""
}

// Authenticate resolves an SSH Credential or creates its stable Trainer.
// source is the client address the credential arrived from; it only gates
// creation of unknown credentials.
func (h *Hub) Authenticate(fingerprintHash, source string) (*game.Trainer, error) {
	trainer, err := h.saves.ResolveCredential(fingerprintHash)
	if errors.Is(err, store.ErrNotFound) {
		if !h.admission.OpenAccess {
			h.observeRegistration(RegDeniedDisabled)
			return nil, ErrRegistrationDisabled
		}
		if !h.registrations.allow(source, time.Now()) {
			h.observeRegistration(RegDeniedQuota)
			h.logWarn("registration denied", "reason", "quota", "source", source)
			return nil, ErrTooManyRegistrations
		}
		trainer, err = h.saves.CreateTrainer(fingerprintHash)
		if err != nil {
			h.observeRegistration(RegDeniedError)
			return nil, err
		}
		h.observeRegistration(RegCreated)
		h.recordEvent(telemetry.Event{
			ID:   telemetry.DeterministicID(telemetry.EventTrainerRegistered, trainer.ID),
			Name: telemetry.EventTrainerRegistered, TrainerID: trainer.ID,
			Properties: map[string]any{"registration_access": "open"},
		})
		return trainer, nil
	}
	return trainer, err
}

// Load returns the Save or nil when this Trainer still needs onboarding.
func (h *Hub) Load(id string) (*game.Save, error) {
	trainer, err := h.saves.LoadTrainer(id)
	if err != nil {
		return nil, err
	}
	return trainer.Save, nil
}

// CompleteOnboard persists a new Save and seats the Trainer in the Dojo.
// A durable Save makes onboarding successful; seating is best-effort. If
// seating cannot complete, the Save stays durable and the Trainer is
// seated by their next Resume or Lobby entry instead of failing the flow.
func (h *Hub) CompleteOnboard(id, handle, starter string) (*game.Save, error) {
	sv, err := onboard.NewSave(h.set, handle, starter)
	if err != nil {
		return nil, err
	}
	if err := h.saves.CompleteOnboarding(id, sv); err != nil {
		return nil, err
	}
	sv, err = h.Load(id)
	if err != nil {
		return nil, err
	}
	if err := h.EnterLobby(id); err != nil {
		if seatErr := h.EnterLobby(id); seatErr != nil {
			h.logWarn("onboarded but not seated", "trainer_id", id, "err", seatErr)
		}
	}
	h.recordEvent(telemetry.Event{
		ID:   telemetry.DeterministicID(telemetry.EventOnboardingCompleted, id),
		Name: telemetry.EventOnboardingCompleted, TrainerID: id,
		Properties: map[string]any{"starter_species": starter},
	})
	return sv, nil
}

// TrainerStats returns authoritative player-visible lifetime statistics.
func (h *Hub) TrainerStats(id string) (store.TrainerStats, error) {
	stats, err := h.saves.TrainerStats(id)
	if err == nil {
		h.recordEvent(telemetry.Event{Name: telemetry.EventStatsViewed, TrainerID: id})
	}
	return stats, err
}

// WorldStats returns durable global totals.
func (h *Hub) WorldStats() (store.WorldStats, error) {
	return h.saves.WorldStats()
}

// ResetTrainer deletes the Save and removes the Trainer from the Dojo so
// first-run can play again.
func (h *Hub) ResetTrainer(hash string) error {
	err := h.saves.ResetTrainer(hash)
	if err != nil {
		return err
	}
	var out outbox
	h.mu.Lock()
	// forgetSeat cancels any Queue entry, sweeps pending Challenges while
	// notifying their counterparties, and leaves and prunes the Dojo.
	h.forgetSeat(hash, &out)
	h.mu.Unlock()
	out.flush()
	return nil
}

// Resume returns the message that should land a reconnecting session:
// a live Battle, the Queue, or a Lobby Snapshot.
func (h *Hub) Resume(hash string) any {
	sv, err := h.Load(hash)
	if err != nil {
		// An unknown Trainer simply still needs onboarding; any other
		// failure is a store problem and must not masquerade as that.
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		h.logWarn("resume load failed", "err", err)
		return ErrorMsg{Text: UserMessage(err)}
	}
	if sv == nil {
		return nil
	}
	var out outbox
	h.mu.Lock()
	if _, dropped := h.drops[hash]; dropped {
		delete(h.drops, hash)
		if m := h.matches[hash]; m != nil {
			h.note(&out, m.other(hash), ReconnectingMsg{})
		}
	}
	if m := h.matches[hash]; m != nil {
		if m.pending || m.committing {
			h.mu.Unlock()
			out.flush()
			return ErrorMsg{Text: "result save failed; retrying"}
		}
		if m.bt.State() != battle.StateOver {
			foe := m.other(hash)
			msg := BattleMsg{Battle: m.bt, You: hash, Foe: m.handle[foe], FoeHash: foe}
			h.mu.Unlock()
			out.flush()
			return msg
		}
	}
	room, _, seated := h.roomForLocked(hash)
	if seated {
		p, ok := room.Get(hash)
		if !ok {
			seated = false
		}
		if seated && p.InQueue {
			pos, wait, _ := h.queue.Position(hash)
			h.mu.Unlock()
			out.flush()
			return QueueMsg{Position: pos, Waiting: wait}
		}
		if seated {
			snap := h.snapshotLocked(hash)
			h.mu.Unlock()
			out.flush()
			return snap
		}
	}
	h.mu.Unlock()
	out.flush()
	if err := h.EnterLobby(hash); err != nil {
		return ErrorMsg{Text: UserMessage(err)}
	}
	return h.Snapshot(hash)
}

// EnterLobby seats a returning Trainer (or re-seats after a Battle).
func (h *Hub) EnterLobby(hash string) error {
	sv, err := h.Load(hash)
	if errors.Is(err, store.ErrNotFound) || (err == nil && sv == nil) {
		return errors.New("server: no save to enter lobby")
	}
	if err != nil {
		return fmt.Errorf("server: enter lobby: %w", err)
	}
	species := ""
	if lead, err := game.PartyLead(sv); err == nil {
		species = lead.Species
	}
	var out outbox
	h.mu.Lock()
	room, _, err := h.joinDojoLocked(lobby.Presence{
		Hash: hash, Handle: sv.Handle, Species: species,
	})
	if err != nil {
		h.mu.Unlock()
		return err
	}
	room.Set(hash, func(p *lobby.Presence) {
		p.InBattle = false
		p.InQueue = false
		p.Handle = sv.Handle
		p.Species = species
	})
	h.note(&out, hash, h.snapshotLocked(hash))
	h.broadcastTrainerDojoLocked(&out, hash)
	h.mu.Unlock()
	out.flush()
	return nil
}

// Tick expires Challenges, emotes, and disconnect grace.
func (h *Hub) Tick(now time.Time) {
	var out outbox
	var expired []string
	var expiredChallenges []challenge
	var decisionExpired []decisionExpiry
	var revealExpired []*match
	h.mu.Lock()
	for to, ch := range h.challenges {
		if now.After(ch.deadline) {
			delete(h.challenges, to)
			expiredChallenges = append(expiredChallenges, *ch)
			h.observeChallenge(ChallengeExpired)
			h.note(&out, ch.from, ErrorMsg{Text: "challenge expired"})
			h.note(&out, to, h.snapshotLocked(to))
		}
	}
	for id, room := range h.dojos {
		for _, p := range room.Snapshot() {
			if p.Emote != "" && now.After(p.EmoteUntil) {
				room.Set(p.Hash, func(pr *lobby.Presence) {
					pr.Emote = ""
				})
				h.dirtyDojos[id] = true
			}
		}
	}
	for id := range h.dirtyDojos {
		h.broadcastDojoLocked(&out, id)
	}
	for hash, deadline := range h.drops {
		if now.After(deadline) {
			expired = append(expired, hash)
		}
	}
	for _, m := range h.matches {
		if m.bt == nil || m.pending || m.committing {
			continue
		}
		decisionExpired = append(decisionExpired, m.mode.tick(m, now)...)
		switch m.bt.State() {
		case battle.StateAwaitingActions, battle.StateAwaitingReplacement:
			m.revealDeadline = time.Time{}
		case battle.StateRevealing:
			if m.revealDeadline.IsZero() {
				m.revealDeadline = now.Add(revealWindow)
			} else if now.After(m.revealDeadline) {
				revealExpired = append(revealExpired, m)
			}
		default:
			m.revealDeadline = time.Time{}
		}
	}
	h.mu.Unlock()
	out.flush()
	for _, challenge := range expiredChallenges {
		for _, trainerID := range []string{challenge.from, challenge.to} {
			h.recordEvent(telemetry.Event{
				Name: telemetry.EventChallengeEnded, TrainerID: trainerID,
				Properties: map[string]any{"outcome": "expired"},
			})
		}
	}
	for _, dx := range decisionExpired {
		if err := dx.m.bt.ExpireDecision(dx.trainer); err == nil {
			h.finishMatch(dx.m)
		}
	}
	for _, m := range revealExpired {
		h.forceAdvanceReveal(m)
	}
	for _, hash := range expired {
		h.expireDrop(hash, now)
	}
	h.retryPendingResults(now)
}

// forceAdvanceReveal releases a Battle stuck in revealing past the server-
// owned window, so a client cannot stall it by withholding acknowledgement.
func (h *Hub) forceAdvanceReveal(m *match) {
	if err := m.bt.AdvanceReveal(); err != nil {
		return // state moved on between the sweep and here
	}
	if m.bt.State() == battle.StateAwaitingReplacement {
		if err := m.mode.lockBot(h, m); err != nil {
			h.logWarn("bot replacement after reveal timeout failed", "match", m.id, "err", err)
		}
	}
	h.pushBattle(m)
}

// Stats returns current multi-Dojo and matchmaking counts.
func (h *Hub) Stats() HubStats {
	h.mu.Lock()
	defer h.mu.Unlock()
	stats := HubStats{
		Dojos:          len(h.dojos),
		ActiveSessions: len(h.clients),
		Queued:         h.queue.Waiting(),
	}
	for _, room := range h.dojos {
		stats.Trainers += len(room.Snapshot())
	}
	matches := make(map[*match]struct{})
	for _, m := range h.matches {
		matches[m] = struct{}{}
	}
	stats.Battles = len(matches)
	return stats
}
