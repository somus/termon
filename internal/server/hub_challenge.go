package server

import (
	"time"

	"termon.sh/internal/game"
	"termon.sh/internal/lobby"
	"termon.sh/internal/telemetry"
)

const challengeWait = 30 * time.Second

// Challenge issues an adjacent Challenge.
func (h *Hub) Challenge(hash string) error {
	sv, err := h.Load(hash)
	if err != nil || sv == nil {
		return playerFacing("cannot challenge now")
	}
	if err := gameRequireFullParty(sv); err != nil {
		return err
	}
	if _, err := h.ensureQueueSets(hash, sv); err != nil {
		return err
	}
	var out outbox
	h.mu.Lock()
	room, _, ok := h.roomForLocked(hash)
	if !ok {
		h.mu.Unlock()
		return playerFacing("cannot challenge now")
	}
	p, ok := room.Get(hash)
	if !ok || p.InBattle || p.InQueue {
		h.mu.Unlock()
		return playerFacing("cannot challenge now")
	}
	adj := room.Adjacent(hash)
	var foe lobby.Presence
	found := false
	for _, n := range adj {
		if !n.InBattle && !n.InQueue {
			foe = n
			found = true
			break
		}
	}
	if !found {
		h.mu.Unlock()
		return playerFacing("stand next to a trainer and press C")
	}
	h.challenges[foe.Hash] = &challenge{
		from: hash, to: foe.Hash, deadline: time.Now().Add(challengeWait),
	}
	h.observeChallenge(ChallengeIssued)
	h.note(&out, foe.Hash, h.snapshotLocked(foe.Hash))
	h.note(&out, hash, ErrorMsg{Text: "challenge sent to " + foe.Handle})
	h.mu.Unlock()
	out.flush()
	h.recordEvent(telemetry.Event{Name: telemetry.EventChallengeIssued, TrainerID: hash})
	return nil
}

func gameRequireFullParty(sv *game.Save) error {
	if err := game.RequireFullParty(sv); err != nil {
		return playerFacing("need a full party of three with loadouts to battle")
	}
	return nil
}

// Respond accepts or declines an incoming Challenge.
func (h *Hub) Respond(hash string, accept bool) error {
	h.mu.Lock()
	ch := h.challenges[hash]
	if ch == nil {
		h.mu.Unlock()
		return playerFacing("no challenge")
	}
	delete(h.challenges, hash)
	from, to := ch.from, ch.to
	h.mu.Unlock()
	if !accept {
		var out outbox
		h.mu.Lock()
		h.observeChallenge(ChallengeDeclined)
		h.note(&out, from, ErrorMsg{Text: "challenge declined"})
		h.note(&out, to, h.snapshotLocked(to))
		h.mu.Unlock()
		out.flush()
		for _, trainerID := range []string{from, to} {
			h.recordEvent(telemetry.Event{
				Name: telemetry.EventChallengeEnded, TrainerID: trainerID,
				Properties: map[string]any{"outcome": "declined"},
			})
		}
		return nil
	}
	h.observeChallenge(ChallengeAccepted)
	for _, trainerID := range []string{from, to} {
		h.recordEvent(telemetry.Event{
			Name: telemetry.EventChallengeEnded, TrainerID: trainerID,
			Properties: map[string]any{"outcome": "accepted"},
		})
	}
	return h.startMatchFrom(from, to, "challenge")
}
