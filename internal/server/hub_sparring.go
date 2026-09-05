package server

import (
	"fmt"
	"time"

	"termon.sh/internal/battle"
	"termon.sh/internal/dojo"
	"termon.sh/internal/game"
	"termon.sh/internal/lobby"
	"termon.sh/internal/store"
)

// SparringPreviewSlot is one opposing Monster in the roster preview.
type SparringPreviewSlot struct {
	Slot    int
	Role    string
	Species string
	Level   int
	Type    string
}

// SparringPreviewMsg is shown before confirming Sparring.
type SparringPreviewMsg struct {
	Tier          string
	XP            int64
	FirstClear    bool
	Slots         []SparringPreviewSlot
	PolicySummary string
	ServerDay     string
	Remix         int
}

// DecisionExplanationMsg surfaces the latest Dojo bot choice after resolve.
type DecisionExplanationMsg struct {
	Text       string
	ReasonCode string
	Tier       string
}

type sparringMode struct {
	soloMode
	tier      string
	serverDay time.Time
}

func newSparringMode(tier string, day time.Time, sv *game.Save) (*sparringMode, error) {
	if tier != dojo.TierApprentice && tier != dojo.TierRival && tier != dojo.TierMaster {
		return nil, fmt.Errorf("server: invalid sparring tier %q", tier)
	}
	solo := soloMode{
		policyCfg:  dojo.TierConfig(tier),
		saveBefore: cloneSaveXPView(sv),
	}
	return &sparringMode{
		soloMode:  solo,
		tier:      tier,
		serverDay: day,
	}, nil
}

func (*sparringMode) afterAction(*Hub, *match, string, battle.Action, string) bool { return true }

func (mode *sparringMode) finish(h *Hub, m *match) {
	h.finishSoloWithReward(m, &mode.soloMode, func() error {
		return h.persistSparringWin(m, mode, m.a)
	})
}

// PreviewSparring builds the daily roster preview for tier without starting a
// Battle. Remix selects another balanced roster only after today's first clear.
func (h *Hub) PreviewSparring(hash, tier string, remix int) (SparringPreviewMsg, error) {
	if tier != dojo.TierApprentice && tier != dojo.TierRival && tier != dojo.TierMaster {
		return SparringPreviewMsg{}, playerFacing("unknown sparring tier")
	}
	h.mu.Lock()
	room, _, ok := h.roomForLocked(hash)
	h.mu.Unlock()
	if !ok || !room.NearMaster(hash) {
		return SparringPreviewMsg{}, playerFacing("stand next to Master Sable")
	}
	return h.sparringPreview(hash, tier, dojo.ServerDay(time.Now().UTC()), remix)
}

func (h *Hub) sparringPreview(hash, tier string, day time.Time, remix int) (SparringPreviewMsg, error) {
	sv, err := h.Load(hash)
	if err != nil || sv == nil {
		return SparringPreviewMsg{}, missingParty(hash, err)
	}
	if err := game.RequireFullParty(sv); err != nil {
		return SparringPreviewMsg{}, playerFacing("need a full party of three for sparring")
	}
	day = dojo.ServerDay(day)
	dayString := dojo.ServerDayString(day)
	key := fmt.Sprintf("%s:sparring:%s:%s", hash, tier, dayString)
	cleared, _ := h.saves.ActivityExists(hash, key)
	if !cleared || remix < 0 {
		remix = 0
	}
	party, err := naturalPartyMonsters(sv)
	if err != nil {
		return SparringPreviewMsg{}, err
	}
	cycle := day.Unix()/86400 + int64(remix)
	roster, err := dojo.BuildSparringRoster(h.set, party, cycle)
	if err != nil {
		return SparringPreviewMsg{}, err
	}
	var slots []SparringPreviewSlot
	for _, s := range roster {
		slots = append(slots, SparringPreviewSlot{
			Slot: s.Slot, Role: s.Role, Species: s.Species, Level: s.Level, Type: s.Type,
		})
	}
	xp := int64(65)
	switch tier {
	case dojo.TierRival:
		xp = 90
	case dojo.TierMaster:
		xp = 130
	}
	return SparringPreviewMsg{
		Tier: tier, XP: xp, FirstClear: !cleared, Slots: slots,
		PolicySummary: sparringPolicySummary(tier),
		ServerDay:     dayString,
		Remix:         remix,
	}, nil
}

func sparringPolicySummary(tier string) string {
	switch tier {
	case dojo.TierRival:
		return "Rival: one-turn scoring, 15% near-best band"
	case dojo.TierMaster:
		return "Master: bounded two-turn expectimax, 5% near-best band"
	default:
		return "Apprentice: weighted Type effectiveness, cautious Switch"
	}
}

// StartSparring launches a Sparring attempt after preview confirm.
func (h *Hub) StartSparring(hash, tier string, remix int) error {
	h.mu.Lock()
	expActive := h.expeditions[hash] != nil
	h.mu.Unlock()
	if expActive {
		return playerFacing("finish or abandon your expedition first")
	}
	preview, err := h.PreviewSparring(hash, tier, remix)
	if err != nil {
		return err
	}
	h.mu.Lock()
	if h.matches[hash] != nil {
		h.mu.Unlock()
		return playerFacing("already in battle")
	}
	h.mu.Unlock()

	sv, err := h.Load(hash)
	if err != nil || sv == nil {
		return missingParty(hash, err)
	}
	partyMon, err := naturalPartyMonsters(sv)
	if err != nil {
		return err
	}
	day, err := time.Parse("2006-01-02", preview.ServerDay)
	if err != nil {
		return fmt.Errorf("server: parse sparring day: %w", err)
	}
	cycle := day.Unix()/86400 + int64(preview.Remix)
	roster, err := dojo.BuildSparringRoster(h.set, partyMon, cycle)
	if err != nil {
		return err
	}
	trainerMembers := make([]battle.PartyMember, 3)
	for i, id := range sv.Party {
		m, ok := game.MonsterByID(sv, id)
		if !ok {
			return playerFacing("invalid party")
		}
		trainerMembers[i] = battle.PartyMember{Monster: m}
	}
	var dojoMembers []battle.PartyMember
	for i, slot := range roster {
		dojoMembers = append(dojoMembers, battle.PartyMember{Monster: game.Monster{
			ID:      fmt.Sprintf("%s-%d", dojo.BotTrainer, i+1),
			Species: slot.Species, Level: slot.Level, BattleLoadout: append([]string(nil), slot.Loadout...),
		}})
	}
	seed, err := battle.RandomSeed()
	if err != nil {
		return fmt.Errorf("server: seed sparring: %w", err)
	}
	bt, err := battle.New(h.set,
		battle.Party{Trainer: hash, Members: trainerMembers},
		battle.Party{Trainer: dojo.BotTrainer, Members: dojoMembers},
		battle.Seeded(seed),
	)
	if err != nil {
		return err
	}
	id, err := newMatchID()
	if err != nil {
		return err
	}
	mode, err := newSparringMode(tier, day, sv)
	if err != nil {
		return err
	}
	m := &match{
		id: id, bt: bt, a: hash, b: dojo.BotTrainer,
		handle: map[string]string{hash: sv.Handle, dojo.BotTrainer: "Dojo " + tierTitle(tier)},
		mode:   mode,
	}
	var out outbox
	h.mu.Lock()
	if busy := h.busyLocked(hash, dojo.BotTrainer); busy != "" {
		h.mu.Unlock()
		return playerFacing("cannot start sparring now")
	}
	h.matches[hash] = m
	h.setPresenceLocked(hash, func(p *lobby.Presence) { p.InBattle = true })
	h.broadcastTrainerDojoLocked(&out, hash)
	h.mu.Unlock()
	out.flush()
	h.pushBattle(m)
	return nil
}

func tierTitle(tier string) string {
	switch tier {
	case dojo.TierRival:
		return "Rival"
	case dojo.TierMaster:
		return "Master"
	default:
		return "Apprentice"
	}
}

func naturalPartyMonsters(sv *game.Save) ([]game.Monster, error) {
	out := make([]game.Monster, 3)
	for i, id := range sv.Party {
		m, ok := game.MonsterByID(sv, id)
		if !ok {
			return nil, fmt.Errorf("server: missing party slot %d", i+1)
		}
		out[i] = m
	}
	return out, nil
}

func (h *Hub) persistSparringWin(m *match, mode *sparringMode, hash string) error {
	active, reserve := participationFromBattle(m.bt, hash)
	day := dojo.ServerDayString(mode.serverDay)
	key := fmt.Sprintf("%s:sparring:%s:%s", hash, mode.tier, day)
	exists, _ := h.saves.ActivityExists(hash, key)
	if exists {
		return nil
	}
	before := cloneSaveXPView(mode.saveBefore)
	rec := store.ActivityRecord{
		Kind: store.KindSparring, NaturalKey: key, TrainerID: hash,
		ActiveIDs: active, ReserveIDs: reserve, Outcome: store.OutcomeCleared,
		SparringTier: mode.tier, CompletedAt: m.completedAt,
	}
	after, err := h.recordActivityResult(rec)
	if err != nil {
		return err
	}
	var out outbox
	h.mu.Lock()
	h.note(&out, hash, SaveMsg{Save: after})
	h.note(&out, hash, progressionDiff(before, after, active, reserve, h.set))
	h.mu.Unlock()
	out.flush()
	return nil
}
