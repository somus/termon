package server

import (
	"time"

	"termon.sh/internal/lobby"
)

const emoteHold = 4 * time.Second

func (h *Hub) roomForLocked(hash string) (*lobby.Room, int, bool) {
	id, ok := h.dojoOf[hash]
	if !ok {
		return nil, 0, false
	}
	room, ok := h.dojos[id]
	return room, id, ok
}

func (h *Hub) joinDojoLocked(p lobby.Presence) (*lobby.Room, int, error) {
	if room, id, ok := h.roomForLocked(p.Hash); ok {
		_, seated := room.Get(p.Hash)
		if seated || len(room.Snapshot()) < lobby.Capacity {
			if _, err := room.Join(p); err == nil {
				return room, id, nil
			}
		}
	}
	for id := 1; id <= h.nextDojo; id++ {
		room := h.dojos[id]
		if room == nil || len(room.Snapshot()) >= lobby.Capacity {
			continue
		}
		if _, err := room.Join(p); err == nil {
			h.dojoOf[p.Hash] = id
			return room, id, nil
		}
	}
	h.nextDojo++
	room := lobby.NewDojo()
	if _, err := room.Join(p); err != nil {
		return nil, 0, err
	}
	h.dojos[h.nextDojo] = room
	h.dojoOf[p.Hash] = h.nextDojo
	return room, h.nextDojo, nil
}

func (h *Hub) setPresenceLocked(hash string, update func(*lobby.Presence)) {
	if room, _, ok := h.roomForLocked(hash); ok {
		room.Set(hash, update)
	}
}

func (h *Hub) broadcastTrainerDojoLocked(out *outbox, hash string) {
	if _, id, ok := h.roomForLocked(hash); ok {
		h.broadcastDojoLocked(out, id)
	}
}

func (h *Hub) pruneDojoLocked(id int) {
	room := h.dojos[id]
	if id == 1 || room == nil || len(room.Snapshot()) != 0 {
		return
	}
	delete(h.dojos, id)
	delete(h.dirtyDojos, id)
	for hash, assigned := range h.dojoOf {
		if assigned == id {
			delete(h.dojoOf, hash)
		}
	}
}

func (h *Hub) forgetSeat(hash string, out *outbox) {
	h.queue.Cancel(hash)
	for to, ch := range h.challenges {
		if ch.from == hash || to == hash {
			delete(h.challenges, to)
			if to != hash {
				h.note(out, to, h.snapshotLocked(to))
			}
			if ch.from != hash && ch.from != to {
				h.note(out, ch.from, h.snapshotLocked(ch.from))
			}
		}
	}
	if room, id, ok := h.roomForLocked(hash); ok {
		room.Leave(hash)
		h.broadcastDojoLocked(out, id)
		h.pruneDojoLocked(id)
	}
}

// StartMatch starts a direct battle between two onboarded trainers.
func (h *Hub) StartMatch(a, b string) error {
	return h.startMatch(a, b)
}

// Move steps the Trainer in the Dojo.
func (h *Hub) Move(hash string, d lobby.Dir) error {
	h.mu.Lock()
	room, id, ok := h.roomForLocked(hash)
	if !ok {
		h.mu.Unlock()
		return playerFacing("lobby: unknown trainer")
	}
	err := room.Move(hash, d)
	if err == nil {
		h.dirtyDojos[id] = true
	}
	h.mu.Unlock()
	if err != nil {
		// Room movement outcomes (blocked, not now) are deliberate status
		// lines; their "lobby:" prefix also lets the TUI clear them on the
		// next successful move. They are fixed vocabulary without internal
		// detail, so passing them verbatim is safe.
		return playerFacing(err.Error())
	}
	return nil
}

// Emote shows a preset phrase above the Trainer for a few seconds.
func (h *Hub) Emote(hash, text string) {
	var out outbox
	h.mu.Lock()
	h.setPresenceLocked(hash, func(p *lobby.Presence) {
		p.Emote = text
		p.EmoteUntil = time.Now().Add(emoteHold)
	})
	h.broadcastTrainerDojoLocked(&out, hash)
	h.mu.Unlock()
	out.flush()
}

// Snapshot is the current Lobby view.
func (h *Hub) Snapshot(hash string) SnapshotMsg {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.snapshotLocked(hash)
}

func (h *Hub) snapshotLocked(hash string) SnapshotMsg {
	room, id, ok := h.roomForLocked(hash)
	if !ok {
		return SnapshotMsg{}
	}
	presences := room.Snapshot()
	return h.snapshotFromLocked(room, id, hash, presences)
}

// snapshotFromLocked assembles one recipient's view from an already-taken
// Room snapshot so a single broadcast pass shares its allocation cost.
// Caller must hold h.mu; presences must have come from room.Snapshot().
func (h *Hub) snapshotFromLocked(room *lobby.Room, id int, hash string, presences []lobby.Presence) SnapshotMsg {
	you, _ := room.Get(hash)
	others := make([]lobby.Presence, 0, len(presences))
	for _, p := range presences {
		if p.Hash != hash {
			others = append(others, p)
		}
	}
	msg := SnapshotMsg{You: you, Others: others, Dojo: id, Context: room.Context(hash)}
	if ch := h.challenges[hash]; ch != nil {
		from, _ := room.Get(ch.from)
		msg.Offer = &ChallengeOffer{FromHash: ch.from, FromHandle: from.Handle}
	}
	return msg
}

type packet struct {
	send func(any)
	msg  any
}

type outbox []packet

func (o outbox) flush() {
	for _, p := range o {
		if p.send != nil {
			p.send(p.msg)
		}
	}
}

func (h *Hub) note(out *outbox, hash string, msg any) {
	if c, ok := h.clients[hash]; ok && c.send != nil {
		*out = append(*out, packet{send: c.send, msg: msg})
	}
}

func (h *Hub) broadcastDojoLocked(out *outbox, id int) {
	delete(h.dirtyDojos, id)
	room := h.dojos[id]
	if room == nil {
		return
	}
	// One Room snapshot and one queue-membership pass per broadcast, then
	// per-recipient filtering of the shared slice: re-running both for every
	// seated trainer made a packed Dojo quadratic on each 100 ms tick.
	presences := room.Snapshot()
	var queued map[string]struct{}
	for _, p := range presences {
		if p.InBattle {
			continue
		}
		if p.InQueue {
			if queued == nil {
				queued = h.queue.Members()
			}
			if _, waiting := queued[p.Hash]; waiting {
				continue
			}
		}
		h.note(out, p.Hash, h.snapshotFromLocked(room, id, p.Hash, presences))
	}
}

func (h *Hub) broadcastTrainersDojosLocked(out *outbox, hashes ...string) {
	ids := make(map[int]struct{}, len(hashes))
	for _, hash := range hashes {
		if _, id, ok := h.roomForLocked(hash); ok {
			ids[id] = struct{}{}
		}
	}
	for id := range ids {
		h.broadcastDojoLocked(out, id)
	}
}
