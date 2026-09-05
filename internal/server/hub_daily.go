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

// DailyMenuInfo is today's Daily Challenge status for the Dojo menu.
type DailyMenuInfo struct {
	ID         string
	Objective  string
	Par        int
	ServerDay  string
	FirstClear bool
	Mastery    bool
	PolicyTier string
	PlayerLead []string
}

type dailyMode struct {
	soloMode
	serverDay   time.Time
	track       *dojo.DailyTracker
	mappedParty [3]string
}

func newDailyMode(fx dojo.DailyFixture, day time.Time, cfg dojo.PolicyConfig, sv *game.Save) *dailyMode {
	solo := soloMode{
		policyCfg:  cfg,
		saveBefore: cloneSaveXPView(sv),
	}
	return &dailyMode{
		soloMode:    solo,
		serverDay:   day,
		track:       dojo.NewDailyTracker(fx),
		mappedParty: sv.Party,
	}
}

func (mode *dailyMode) afterAction(h *Hub, m *match, hash string, action battle.Action, trainerMove string) bool {
	fromType, toType := "", ""
	switched := action.Kind == battle.ActionSwitch
	if switched {
		snap := m.bt.Snapshot(hash)
		for _, mem := range snap.YourParty {
			if mem.Active {
				if sp, ok := h.set.Species[mem.Species]; ok {
					fromType = sp.Type
				}
			}
			if mem.ID == action.SwitchTo {
				if sp, ok := h.set.Species[mem.Species]; ok {
					toType = sp.Type
				}
			}
		}
	}
	h.afterDailyTurn(m, mode, hash, trainerMove, switched, fromType, toType)
	return true
}

func (mode *dailyMode) finish(h *Hub, m *match) {
	h.finishSoloWithReward(m, &mode.soloMode, func() error {
		return h.persistDailyWin(m, mode, m.a)
	})
}

// StartDaily launches today's Daily Challenge with loaned teams.
func (h *Hub) StartDaily(hash string) error {
	h.mu.Lock()
	room, _, ok := h.roomForLocked(hash)
	expActive := h.expeditions[hash] != nil
	h.mu.Unlock()
	if !ok || !room.NearMaster(hash) {
		return playerFacing("stand next to Master Sable")
	}
	if expActive {
		return playerFacing("finish or abandon your expedition first")
	}
	sv, err := h.Load(hash)
	if err != nil || sv == nil {
		return missingParty(hash, err)
	}
	if err := game.RequireFullParty(sv); err != nil {
		return playerFacing("need a full party of three for the daily challenge")
	}
	h.mu.Lock()
	if h.matches[hash] != nil {
		h.mu.Unlock()
		return playerFacing("already in battle")
	}
	h.mu.Unlock()

	day := dojo.ServerDay(time.Now().UTC())
	fx := dojo.FixtureForDay(day)
	playerParty, oppParty, err := dojo.DailyParties(h.set, fx)
	if err != nil {
		return err
	}
	playerParty.Trainer = hash
	bt, err := battle.New(h.set, playerParty, oppParty, battle.Seeded(fx.Seed))
	if err != nil {
		return err
	}
	id, err := newMatchID()
	if err != nil {
		return err
	}
	cfg := dojo.TierConfig(fx.PolicyTier)
	cfg.NearBestBand = 0
	mode := newDailyMode(fx, day, cfg, sv)
	m := &match{
		id: id, bt: bt, a: hash, b: dojo.BotTrainer,
		handle: map[string]string{hash: sv.Handle, dojo.BotTrainer: "Daily " + fx.ID},
		mode:   mode,
	}
	var out outbox
	h.mu.Lock()
	if busy := h.busyLocked(hash, dojo.BotTrainer); busy != "" {
		h.mu.Unlock()
		return playerFacing("cannot start daily now")
	}
	h.matches[hash] = m
	h.setPresenceLocked(hash, func(p *lobby.Presence) { p.InBattle = true })
	h.broadcastTrainerDojoLocked(&out, hash)
	h.mu.Unlock()
	out.flush()
	h.pushBattle(m)
	return nil
}

func (h *Hub) dailyMenuInfo(hash string, day time.Time) DailyMenuInfo {
	fx := dojo.FixtureForDay(day)
	dayStr := dojo.ServerDayString(day)
	xpKey := hash + ":daily:" + dayStr
	masteryKey := hash + ":daily-mastery:" + dayStr
	// Fail closed: if the store errors, treat the activity as done rather than
	// promising XP or mastery the trainer may already have claimed.
	xpDone, err := h.saves.ActivityExists(hash, xpKey)
	if err != nil {
		xpDone = true
		h.logWarn("daily xp lookup failed", "trainer", hash, "err", err)
	}
	mastery, err := h.saves.ActivityExists(hash, masteryKey)
	if err != nil {
		mastery = true
		h.logWarn("daily mastery lookup failed", "trainer", hash, "err", err)
	}
	return DailyMenuInfo{
		ID: fx.ID, Objective: fx.Objective, Par: fx.Par, ServerDay: dayStr,
		FirstClear: !xpDone, Mastery: mastery, PolicyTier: fx.PolicyTier,
		PlayerLead: append([]string(nil), fx.PlayerLead...),
	}
}

func (h *Hub) afterDailyTurn(
	m *match,
	mode *dailyMode,
	hash string,
	trainerMove string,
	switched bool,
	fromType, toType string,
) {
	turn := m.bt.Turn()
	events := m.bt.Events()
	snap := m.bt.Snapshot(hash)
	mode.track.ObserveTurn(h.set, events, turn, hash, trainerMove, snap)
	if switched {
		foeT := ""
		for _, f := range snap.FoeRoster {
			if f.Active {
				if sp, ok := h.set.Species[f.Species]; ok {
					foeT = sp.Type
				}
			}
		}
		mode.track.RecordSafeSwitch(h.set, fromType, toType, foeT)
	}
}

func (h *Hub) persistDailyWin(m *match, mode *dailyMode, hash string) error {
	active, reserve := participationFromBattle(m.bt, hash)
	active, reserve = mapDailyParticipation(mode, active, reserve)
	turns := m.bt.Turn()
	snap := m.bt.Snapshot(hash)
	won := m.bt.Winner() == hash
	obj := mode.track.ObjectiveMet(h.set, won, turns, snap)
	par := mode.track.ParMet(won, turns)
	dayStr := dojo.ServerDayString(mode.serverDay)
	xpKey := hash + ":daily:" + dayStr
	masteryKey := hash + ":daily-mastery:" + dayStr
	xpExists, _ := h.saves.ActivityExists(hash, xpKey)
	masteryExists, _ := h.saves.ActivityExists(hash, masteryKey)

	if obj && !xpExists {
		before := cloneSaveXPView(mode.saveBefore)
		rec := store.ActivityRecord{
			Kind: store.KindDailyXP, NaturalKey: xpKey, TrainerID: hash,
			ActiveIDs: active, ReserveIDs: reserve, Outcome: store.OutcomeCleared,
			DailyParMet: par, CompletedAt: m.completedAt,
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
	if par && !masteryExists {
		rec := store.ActivityRecord{
			Kind: store.KindDailyMastery, NaturalKey: masteryKey, TrainerID: hash,
			ActiveIDs: active, ReserveIDs: reserve, Outcome: store.OutcomeMastery,
			MasteryOnly: true, CompletedAt: m.completedAt,
		}
		if _, err := h.recordActivityResult(rec); err != nil {
			return err
		}
	}
	return nil
}

func mapDailyParticipation(mode *dailyMode, active, reserve []string) ([]string, []string) {
	hasMapped := false
	for _, id := range mode.mappedParty {
		if id != "" {
			hasMapped = true
			break
		}
	}
	if !hasMapped {
		return active, reserve
	}
	loanToOwned := map[string]string{}
	for i, loanID := range []string{
		"daily:player-1", "daily:player-2", "daily:player-3",
	} {
		if mode.mappedParty[i] != "" {
			loanToOwned[loanID] = mode.mappedParty[i]
		}
	}
	mapIDs := func(ids []string) []string {
		var out []string
		for _, id := range ids {
			if mapped, ok := loanToOwned[id]; ok {
				out = append(out, mapped)
			}
		}
		return out
	}
	return mapIDs(active), mapIDs(reserve)
}

func (h *Hub) buildDojoMenu(hash string) (DojoMenuMsg, error) {
	day := dojo.ServerDay(time.Now().UTC())
	menu := DojoMenuMsg{ServerDay: dojo.ServerDayString(day), Daily: h.dailyMenuInfo(hash, day)}
	for _, tier := range []string{dojo.TierApprentice, dojo.TierRival, dojo.TierMaster} {
		key := fmt.Sprintf("%s:sparring:%s:%s", hash, tier, menu.ServerDay)
		done, _ := h.saves.ActivityExists(hash, key)
		switch tier {
		case dojo.TierApprentice:
			menu.SparringApprenticeClear = done
		case dojo.TierRival:
			menu.SparringRivalClear = done
		case dojo.TierMaster:
			menu.SparringMasterClear = done
		}
	}
	one, _ := h.saves.ActivityExists(hash, hash+":lesson:1")
	two, _ := h.saves.ActivityExists(hash, hash+":lesson:2")
	menu.Lesson1Done, menu.Lesson2Done = one, two
	preview, err := h.previewSparringInternal(hash, dojo.TierApprentice, day)
	if err == nil {
		menu.SparringPreview = preview.Slots
	}
	return menu, nil
}

func (h *Hub) previewSparringInternal(hash, tier string, day time.Time) (SparringPreviewMsg, error) {
	return h.sparringPreview(hash, tier, day, 0)
}
