package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
	"termon.sh/internal/game"
)

const currentSaveVersion = 1

type savePayload struct {
	Collection []game.Monster           `json:"collection"`
	Party      [3]string                `json:"party"`
	Notices    []game.ProgressionNotice `json:"notices"`
}

type storedBattle struct {
	result BattleResult
	body   string
}

type battleResultBody struct {
	WinnerActive  []string    `json:"winner_active,omitempty"`
	WinnerReserve []string    `json:"winner_reserve,omitempty"`
	LoserActive   []string    `json:"loser_active,omitempty"`
	LoserReserve  []string    `json:"loser_reserve,omitempty"`
	ApplyRewards  bool        `json:"apply_rewards"`
	Stats         BattleStats `json:"stats"`
}

type activityPayload struct {
	Kind           string   `json:"kind"`
	Outcome        string   `json:"outcome"`
	ActiveIDs      []string `json:"active_ids"`
	ReserveIDs     []string `json:"reserve_ids"`
	CaptureSpecies string   `json:"capture_species,omitempty"`
	FillParty      bool     `json:"fill_party,omitempty"`
	MasteryOnly    bool     `json:"mastery_only,omitempty"`
}

func newOpaqueID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("store: generate id: %w", err)
	}
	return hex.EncodeToString(id[:]), nil
}

func encodePayload(p savePayload) ([]byte, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("store: encode save: %w", err)
	}
	return b, nil
}

type legacyPartyMonster struct {
	Species  string   `json:"species"`
	Nickname string   `json:"nickname"`
	Moves    []string `json:"moves"`
}

type legacySavePayload struct {
	Party []legacyPartyMonster `json:"party"`
}

func decodePayload(raw []byte) (savePayload, bool, error) {
	var current savePayload
	err := json.Unmarshal(raw, &current)
	if err == nil {
		return current, false, nil
	}
	converted, err := convertLegacyPartyPayload(raw)
	if err != nil {
		return savePayload{}, false, err
	}
	return converted, true, nil
}

func convertLegacyPartyPayload(raw []byte) (savePayload, error) {
	var legacy legacySavePayload
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return savePayload{}, err
	}
	if len(legacy.Party) == 0 {
		return savePayload{}, errors.New("legacy party is empty")
	}
	collection := make([]game.Monster, 0, len(legacy.Party))
	var party [3]string
	for i, old := range legacy.Party {
		if old.Species == "" {
			return savePayload{}, errors.New("legacy party member missing species")
		}
		id, err := newOpaqueID()
		if err != nil {
			return savePayload{}, err
		}
		moves := append([]string(nil), old.Moves...)
		collection = append(collection, game.Monster{
			ID:            id,
			Species:       old.Species,
			Nickname:      old.Nickname,
			XP:            0,
			Level:         1,
			MoveLibrary:   moves,
			BattleLoadout: append([]string(nil), moves...),
		})
		if i < 3 {
			party[i] = id
		}
	}
	return savePayload{Collection: collection, Party: party}, nil
}

func payloadFromSave(save *game.Save) savePayload {
	return savePayload{
		Collection: cloneMonsters(save.Collection),
		Party:      save.Party,
		Notices:    cloneNotices(save.Notices),
	}
}

func saveFromPayload(handle string, wins, losses int, p savePayload) *game.Save {
	return &game.Save{
		Handle:     handle,
		Wins:       wins,
		Losses:     losses,
		Collection: cloneMonsters(p.Collection),
		Party:      p.Party,
		Notices:    cloneNotices(p.Notices),
	}
}

func cloneMonsters(in []game.Monster) []game.Monster {
	out := make([]game.Monster, len(in))
	for i, m := range in {
		out[i] = cloneMonster(m)
	}
	return out
}

func cloneMonster(m game.Monster) game.Monster {
	cloned := m
	cloned.MoveLibrary = append([]string(nil), m.MoveLibrary...)
	cloned.BattleLoadout = append([]string(nil), m.BattleLoadout...)
	return cloned
}

func cloneNotices(in []game.ProgressionNotice) []game.ProgressionNotice {
	out := make([]game.ProgressionNotice, len(in))
	for i, n := range in {
		out[i] = n
		if len(n.Moves) > 0 {
			out[i].Moves = append([]string(nil), n.Moves...)
		}
	}
	return out
}

func cloneSave(save *game.Save) *game.Save {
	if save == nil {
		return nil
	}
	return saveFromPayload(save.Handle, save.Wins, save.Losses, payloadFromSave(save))
}

func canonicalBattleBody(rec BattleRecord) (string, error) {
	body := battleResultBody{
		WinnerActive:  append([]string(nil), rec.WinnerActive...),
		WinnerReserve: append([]string(nil), rec.WinnerReserve...),
		LoserActive:   append([]string(nil), rec.LoserActive...),
		LoserReserve:  append([]string(nil), rec.LoserReserve...),
		ApplyRewards:  rec.ApplyRewards,
		Stats:         rec.Stats,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("store: encode battle body: %w", err)
	}
	return string(b), nil
}

func canonicalActivityPayload(rec ActivityRecord) (string, error) {
	p := activityPayload{
		Kind:        string(rec.Kind),
		Outcome:     rec.Outcome,
		ActiveIDs:   append([]string(nil), rec.ActiveIDs...),
		ReserveIDs:  append([]string(nil), rec.ReserveIDs...),
		MasteryOnly: rec.MasteryOnly,
	}
	if rec.Capture != nil {
		p.CaptureSpecies = rec.Capture.Species
		p.FillParty = rec.Capture.FillParty
	}
	b, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("store: encode activity payload: %w", err)
	}
	return string(b), nil
}

func firstFourMovepool(spec content.Species) ([]string, error) {
	if len(spec.Movepool) < 4 {
		return nil, fmt.Errorf("store: species %s movepool too short", spec.Slug)
	}
	moves := make([]string, 4)
	for i := range 4 {
		moves[i] = spec.Movepool[i].Move
	}
	return moves, nil
}

func sanitizeSaveMoves(set *content.Set, save *game.Save) bool {
	if set == nil || save == nil {
		return false
	}
	changed := false
	for i := range save.Collection {
		if sanitizeMonsterMoves(set, &save.Collection[i]) {
			changed = true
		}
	}
	return changed
}

func sanitizeMonsterMoves(set *content.Set, m *game.Monster) bool {
	sp, ok := set.Species[m.Species]
	if !ok {
		return false
	}
	legal := make(map[string]struct{}, len(sp.Movepool))
	for _, entry := range sp.Movepool {
		if _, ok := set.Moves[entry.Move]; ok {
			legal[entry.Move] = struct{}{}
		}
	}
	lib, libChanged := keepKnownMoves(m.MoveLibrary, legal)
	load, loadChanged := keepKnownMoves(m.BattleLoadout, legal)
	if len(lib) == 0 {
		minted, err := firstFourMovepool(sp)
		if err != nil {
			return false
		}
		lib = minted
		libChanged = true
	}
	if added := unlocksForLevel(set, m.Species, m.Level, lib); len(added) > 0 {
		lib = append(lib, added...)
		libChanged = true
	}
	if len(load) == 0 || (loadChanged && len(load) < 4) {
		load = fillLoadout(load, lib)
		loadChanged = true
	}
	if !libChanged && !loadChanged {
		return false
	}
	m.MoveLibrary = lib
	m.BattleLoadout = load
	return true
}

func fillLoadout(load, library []string) []string {
	out := append([]string(nil), load...)
	have := make(map[string]struct{}, 4)
	for _, slug := range out {
		have[slug] = struct{}{}
	}
	for _, slug := range library {
		if len(out) == 4 {
			break
		}
		if _, ok := have[slug]; ok {
			continue
		}
		have[slug] = struct{}{}
		out = append(out, slug)
	}
	return out
}

func keepKnownMoves(slugs []string, legal map[string]struct{}) ([]string, bool) {
	out := make([]string, 0, len(slugs))
	seen := make(map[string]struct{}, len(slugs))
	dropped := false
	for _, slug := range slugs {
		if _, ok := legal[slug]; !ok {
			dropped = true
			continue
		}
		if _, dup := seen[slug]; dup {
			dropped = true
			continue
		}
		seen[slug] = struct{}{}
		out = append(out, slug)
	}
	return out, dropped
}

func mintStarterMonster(set *content.Set, species string) (game.Monster, error) {
	id, err := newOpaqueID()
	if err != nil {
		return game.Monster{}, err
	}
	return monsterForSpecies(set, species, id)
}

func monsterForSpecies(set *content.Set, species, id string) (game.Monster, error) {
	if set == nil {
		return game.Monster{}, errors.New("store: nil content")
	}
	sp, ok := set.Species[species]
	if !ok {
		return game.Monster{}, fmt.Errorf("store: unknown species %q", species)
	}
	moves, err := firstFourMovepool(sp)
	if err != nil {
		return game.Monster{}, err
	}
	return game.Monster{
		ID:               id,
		Species:          species,
		Level:            1,
		MoveLibrary:      append([]string(nil), moves...),
		BattleLoadout:    append([]string(nil), moves...),
		EvolutionPending: evolutionPending(set, species, 1),
	}, nil
}

func findMonster(save *game.Save, id string) (int, bool) {
	for i, m := range save.Collection {
		if m.ID == id {
			return i, true
		}
	}
	return 0, false
}

func validateSave(set *content.Set, save *game.Save) error {
	seen := make(map[string]struct{}, len(save.Collection))
	for _, m := range save.Collection {
		if m.ID == "" {
			return fmt.Errorf("%w: empty monster id", ErrCorruptSave)
		}
		if _, dup := seen[m.ID]; dup {
			return fmt.Errorf("%w: duplicate monster id", ErrCorruptSave)
		}
		seen[m.ID] = struct{}{}
		if m.XP < 0 || m.XP > game.XPForLevel(50) {
			return fmt.Errorf("%w: xp out of range", ErrCorruptSave)
		}
		if m.Level != game.LevelForXP(m.XP) {
			return fmt.Errorf("%w: level mismatch", ErrCorruptSave)
		}
		if err := validateLoadout(m.MoveLibrary, m.BattleLoadout); err != nil {
			return fmt.Errorf("%w: %w", ErrCorruptSave, err)
		}
	}
	partySeen := make(map[string]struct{})
	for _, id := range save.Party {
		if id == "" {
			continue
		}
		if _, dup := partySeen[id]; dup {
			return fmt.Errorf("%w: duplicate party id", ErrCorruptSave)
		}
		partySeen[id] = struct{}{}
		if _, ok := seen[id]; !ok {
			return fmt.Errorf("%w: unknown party monster", ErrCorruptSave)
		}
	}
	for i := range save.Collection {
		save.Collection[i].EvolutionPending = evolutionPending(set, save.Collection[i].Species, save.Collection[i].Level)
	}
	return nil
}

func validateLoadout(library, loadout []string) error {
	if len(loadout) < 1 || len(loadout) > 4 {
		return ErrInvalidLoadout
	}
	libSet := make(map[string]struct{}, len(library))
	for _, slug := range library {
		libSet[slug] = struct{}{}
	}
	seen := make(map[string]struct{}, len(loadout))
	for _, slug := range loadout {
		if slug == "" {
			return ErrInvalidLoadout
		}
		if _, dup := seen[slug]; dup {
			return ErrInvalidLoadout
		}
		seen[slug] = struct{}{}
		if _, ok := libSet[slug]; !ok {
			return ErrInvalidLoadout
		}
	}
	return nil
}

func validatePartyIDs(save *game.Save, party [3]string) error {
	seen := make(map[string]struct{})
	for _, id := range party {
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			return ErrInvalidParty
		}
		seen[id] = struct{}{}
		if _, ok := findMonster(save, id); !ok {
			return ErrInvalidParty
		}
	}
	return nil
}

func evolutionPending(set *content.Set, species string, level int) bool {
	if set == nil {
		return false
	}
	sp, ok := set.Species[species]
	if !ok || sp.EvolvesTo == nil {
		return false
	}
	return level >= sp.EvolvesTo.Level
}

func unlocksForLevel(set *content.Set, species string, newLevel int, library []string) []string {
	if set == nil {
		return nil
	}
	sp, ok := set.Species[species]
	if !ok {
		return nil
	}
	known := make(map[string]struct{}, len(library))
	for _, slug := range library {
		known[slug] = struct{}{}
	}
	var added []string
	for _, entry := range sp.Movepool {
		if entry.Level <= newLevel {
			if _, ok := known[entry.Move]; !ok {
				added = append(added, entry.Move)
				known[entry.Move] = struct{}{}
			}
		}
	}
	return added
}

func applyXPToMonster(
	set *content.Set,
	m *game.Monster,
	add int64,
	sourceKey string,
	notices *[]game.ProgressionNotice,
	newID func() (string, error),
) error {
	if add <= 0 {
		return nil
	}
	oldLevel := m.Level
	m.XP = game.ClampXP(m.XP + add)
	m.Level = game.LevelForXP(m.XP)
	if m.Level == oldLevel {
		m.EvolutionPending = evolutionPending(set, m.Species, m.Level)
		return nil
	}
	added := unlocksForLevel(set, m.Species, m.Level, m.MoveLibrary)
	if len(added) > 0 {
		m.MoveLibrary = append(m.MoveLibrary, added...)
		id, err := newID()
		if err != nil {
			return err
		}
		*notices = append(*notices, game.ProgressionNotice{
			ID: id, Kind: "move_unlock", MonsterID: m.ID, SourceKey: sourceKey,
			Moves: append([]string(nil), added...),
		})
	}
	m.EvolutionPending = evolutionPending(set, m.Species, m.Level)
	return nil
}

func pvpDecayMultiplier(priorCount int) float64 {
	switch {
	case priorCount <= 1:
		return 1.0
	case priorCount <= 3:
		return 0.75
	default:
		return 0.50
	}
}

func applyPvPRewards(set *content.Set, save *game.Save, active, reserve []string, isWinner bool, decay float64, sourceKey string) error {
	base := max(int64(float64(pvpBaseXP)*decay), int64(0))
	winnerBonus := int64(0)
	if isWinner {
		winnerBonus = int64(float64(pvpWinnerBonus) * decay)
	}
	reserveShare := base * pvpReserveSharePct / 100
	// Deduped in first-seen order: notices and XP application must stay
	// deterministic for the same battle input, so no map iteration.
	seen := make(map[string]struct{}, len(active)+len(reserve))
	for _, id := range active {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		share := base
		if isWinner {
			share += winnerBonus
		}
		if err := rewardMonster(set, save, id, share, sourceKey, newOpaqueID); err != nil {
			return err
		}
	}
	for _, id := range reserve {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		if err := rewardMonster(set, save, id, reserveShare, sourceKey, newOpaqueID); err != nil {
			return err
		}
	}
	return nil
}

// Reward pricing in XP (docs/design/xp-progression.md reward table).
// Centralized so playtesting tunes one place; promoting these to content/
// data awaits a progression section in the content schema.
const (
	pvpBaseXP          = 130
	pvpWinnerBonus     = 25
	pvpReserveSharePct = 40

	expeditionPrepXP   = 40
	expeditionTargetXP = 65
	captureBonusXP     = 35

	lessonXP = 90

	dailyXP = 180
)

// Sparring tier payouts, keyed by the dojo tier names.
const (
	sparringApprenticeXP = 65
	sparringRivalXP      = 90
	sparringMasterXP     = 130
)

// activityXP prices one completed activity: base is the share every eligible
// participant earns from, activeBonus is paid only to Monsters that entered and
// resolved a turn (xp-progression.md reward table). The reserve share is 40% of
// base alone, so active-only bonuses never leak to the reserve.
func activityXP(rec ActivityRecord) (base, activeBonus int64) {
	if rec.MasteryOnly || rec.Kind == KindDailyMastery {
		return 0, 0
	}
	switch rec.Kind {
	case KindExpedition:
		switch rec.Outcome {
		case OutcomePrep1, OutcomePrep2:
			return expeditionPrepXP, 0
		case OutcomeTarget, OutcomeHuntFailed, OutcomeCaptured:
			if rec.Outcome == OutcomeCaptured {
				return expeditionTargetXP, captureBonusXP
			}
			return expeditionTargetXP, 0
		default:
			return 0, 0
		}
	case KindLesson:
		return lessonXP, 0
	case KindSparring:
		switch strings.ToLower(rec.SparringTier) {
		case dojo.TierApprentice:
			return sparringApprenticeXP, 0
		case dojo.TierRival:
			return sparringRivalXP, 0
		case dojo.TierMaster:
			return sparringMasterXP, 0
		default:
			return sparringApprenticeXP, 0
		}
	case KindDailyXP:
		return dailyXP, 0
	default:
		return 0, 0
	}
}

func applyActivityRewards(
	set *content.Set,
	save *game.Save,
	rec ActivityRecord,
	sourceKey string,
	newID func() (string, error),
) error {
	base, activeBonus := activityXP(rec)
	activeShare := base + activeBonus
	activeSet := make(map[string]struct{}, len(rec.ActiveIDs))
	for _, id := range rec.ActiveIDs {
		activeSet[id] = struct{}{}
	}
	reserveShare := base * 40 / 100
	for _, id := range rec.ActiveIDs {
		if err := rewardMonster(set, save, id, activeShare, sourceKey, newID); err != nil {
			return err
		}
	}
	for _, id := range rec.ReserveIDs {
		if _, active := activeSet[id]; active {
			continue
		}
		if err := rewardMonster(set, save, id, reserveShare, sourceKey, newID); err != nil {
			return err
		}
	}
	return nil
}

func rewardMonster(
	set *content.Set,
	save *game.Save,
	monsterID string,
	xp int64,
	sourceKey string,
	newID func() (string, error),
) error {
	idx, ok := findMonster(save, monsterID)
	if !ok {
		return ErrUnknownMonster
	}
	return applyXPToMonster(set, &save.Collection[idx], xp, sourceKey, &save.Notices, newID)
}

func firstVacantPartySlot(party [3]string) int {
	for i, id := range party {
		if id == "" {
			return i
		}
	}
	return -1
}

func removeNotices(save *game.Save, ids []string) {
	if len(ids) == 0 {
		return
	}
	remove := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		remove[id] = struct{}{}
	}
	kept := save.Notices[:0]
	for _, n := range save.Notices {
		if _, drop := remove[n.ID]; !drop {
			kept = append(kept, n)
		}
	}
	save.Notices = kept
}

func acceptEvolution(set *content.Set, save *game.Save, monsterID string) error {
	idx, ok := findMonster(save, monsterID)
	if !ok {
		return ErrUnknownMonster
	}
	m := &save.Collection[idx]
	if !m.EvolutionPending {
		return ErrEvolutionNotPending
	}
	sp, ok := set.Species[m.Species]
	if !ok || sp.EvolvesTo == nil {
		return ErrEvolutionNotPending
	}
	successor := sp.EvolvesTo.Species
	added := unlocksForLevel(set, successor, m.Level, m.MoveLibrary)
	if len(added) > 0 {
		m.MoveLibrary = append(m.MoveLibrary, added...)
		id, err := newOpaqueID()
		if err != nil {
			return err
		}
		save.Notices = append(save.Notices, game.ProgressionNotice{
			ID: id, Kind: "move_unlock", MonsterID: m.ID, SourceKey: "evolution:" + m.ID,
			Moves: append([]string(nil), added...),
		})
	}
	m.Species = successor
	m.EvolutionPending = evolutionPending(set, m.Species, m.Level)
	return nil
}
