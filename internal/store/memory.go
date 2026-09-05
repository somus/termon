package store

import (
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"termon.sh/internal/content"
	"termon.sh/internal/game"
)

// MemoryStore is an in-memory Store backend for tests.
type MemoryStore struct {
	mu          sync.Mutex
	credentials map[string]string
	trainers    map[string]*memoryTrainer
	battles     map[string]storedBattle
	activities  map[string]storedActivity
	sessions    map[string]SessionRecord
	content     *content.Set
	failResults int
	failLoads   int
}

type memoryTrainer struct {
	id        string
	handle    string
	wins      int
	losses    int
	save      *game.Save
	version   int
	createdAt time.Time
}

type storedActivity struct {
	result ActivityResult
}

// NewMemoryStore creates an empty in-memory Store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		credentials: make(map[string]string),
		trainers:    make(map[string]*memoryTrainer),
		battles:     make(map[string]storedBattle),
		activities:  make(map[string]storedActivity),
		sessions:    make(map[string]SessionRecord),
	}
}

// UseContent injects content for progression mutations.
func (s *MemoryStore) UseContent(set *content.Set) {
	s.mu.Lock()
	s.content = set
	s.mu.Unlock()
}

// FailNextResults injects RecordBattleResult failures.
func (s *MemoryStore) FailNextResults(count int) {
	s.mu.Lock()
	s.failResults = count
	s.mu.Unlock()
}

// FailNextLoads injects LoadTrainer failures.
func (s *MemoryStore) FailNextLoads(count int) {
	s.mu.Lock()
	s.failLoads = count
	s.mu.Unlock()
}

// ResolveCredential returns the Trainer bound to credential.
func (s *MemoryStore) ResolveCredential(credential string) (*game.Trainer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.credentials[credential]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneMemoryTrainer(s.trainers[id]), nil
}

// CreateTrainer creates or resolves a Trainer for credential.
func (s *MemoryStore) CreateTrainer(credential string) (*game.Trainer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.credentials[credential]; ok {
		return cloneMemoryTrainer(s.trainers[id]), nil
	}
	trainer := &memoryTrainer{id: credential, createdAt: time.Now().UTC()}
	s.credentials[credential] = trainer.id
	s.trainers[trainer.id] = trainer
	return cloneMemoryTrainer(trainer), nil
}

// LoadTrainer returns a cloned Trainer by ID.
func (s *MemoryStore) LoadTrainer(id string) (*game.Trainer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failLoads > 0 {
		s.failLoads--
		return nil, errors.New("injected load failure")
	}
	row, ok := s.trainers[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneMemoryTrainer(row), nil
}

// CompleteOnboarding stores a Trainer's initial Save after starter minting.
func (s *MemoryStore) CompleteOnboarding(id string, save *game.Save) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.trainers[id]
	if !ok {
		return ErrNotFound
	}
	if row.save != nil {
		return ErrAlreadyOnboarded
	}
	if len(save.Collection) != 1 {
		return errors.New("store: onboarding requires one starter")
	}
	starter, err := mintStarterMonster(s.content, save.Collection[0].Species)
	if err != nil {
		return err
	}
	row.handle = save.Handle
	row.wins = save.Wins
	row.losses = save.Losses
	row.version = 1
	row.save = &game.Save{
		Handle: save.Handle, Wins: save.Wins, Losses: save.Losses,
		Collection: []game.Monster{starter},
		Party:      [3]string{starter.ID, "", ""},
	}
	return nil
}

// ResetTrainer clears onboarded state for id.
func (s *MemoryStore) ResetTrainer(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.trainers[id]
	if !ok {
		return ErrNotFound
	}
	row.handle = ""
	row.wins = 0
	row.losses = 0
	row.save = nil
	row.version = 0
	for key, act := range s.activities {
		if act.result.TrainerID == id {
			delete(s.activities, key)
		}
	}
	return nil
}

// RecordBattleResult atomically applies one Battle Result.
func (s *MemoryStore) RecordBattleResult(rec BattleRecord) (ResultRecords, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failResults > 0 {
		s.failResults--
		return ResultRecords{}, errors.New("injected result failure")
	}
	body, err := canonicalBattleBody(rec)
	if err != nil {
		return ResultRecords{}, err
	}
	if existing, ok := s.battles[rec.Result.ID]; ok {
		if existing.result != rec.Result || existing.body != body {
			return ResultRecords{}, ErrResultConflict
		}
		return s.memoryBattleRecords(rec.Result, false)
	}
	winner, winnerOK := s.trainers[rec.Result.Winner]
	loser, loserOK := s.trainers[rec.Result.Loser]
	if !winnerOK || !loserOK || winner.save == nil || loser.save == nil {
		return ResultRecords{}, ErrNotFound
	}
	winner.wins++
	loser.losses++
	winner.save.Wins = winner.wins
	loser.save.Losses = loser.losses
	if rec.ApplyRewards {
		prior := s.countPairResults24h(rec.Result.Winner, rec.Result.Loser, rec.Result.CompletedAt)
		decay := pvpDecayMultiplier(prior)
		wSave := cloneSave(winner.save)
		lSave := cloneSave(loser.save)
		if err := applyPvPRewards(s.content, wSave, rec.WinnerActive, rec.WinnerReserve, true, decay, rec.Result.ID); err != nil {
			return ResultRecords{}, err
		}
		if err := applyPvPRewards(s.content, lSave, rec.LoserActive, rec.LoserReserve, false, decay, rec.Result.ID); err != nil {
			return ResultRecords{}, err
		}
		if err := validateSave(s.content, wSave); err != nil {
			return ResultRecords{}, err
		}
		if err := validateSave(s.content, lSave); err != nil {
			return ResultRecords{}, err
		}
		winner.save = wSave
		loser.save = lSave
	}
	s.battles[rec.Result.ID] = storedBattle{result: rec.Result, body: body}
	return s.memoryBattleRecords(rec.Result, true)
}

func (s *MemoryStore) memoryBattleRecords(result BattleResult, applied bool) (ResultRecords, error) {
	return ResultRecords{
		Winner:  cloneSave(s.trainers[result.Winner].save),
		Loser:   cloneSave(s.trainers[result.Loser].save),
		Applied: applied,
	}, nil
}

func (s *MemoryStore) countPairResults24h(a, b string, at time.Time) int {
	windowStart := at.Add(-24 * time.Hour)
	count := 0
	for _, st := range s.battles {
		if st.result.CompletedAt.Before(windowStart) || !st.result.CompletedAt.Before(at) {
			continue
		}
		w, l := st.result.Winner, st.result.Loser
		if (w == a && l == b) || (w == b && l == a) {
			count++
		}
	}
	return count
}

// RecordActivityResult commits one solo activity and returns the updated Save.
func (s *MemoryStore) RecordActivityResult(rec ActivityRecord) (*game.Save, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, err := canonicalActivityPayload(rec)
	if err != nil {
		return nil, err
	}
	if existing, ok := s.activities[rec.NaturalKey]; ok {
		if existing.result.Payload != payload {
			return nil, ErrResultConflict
		}
		return cloneSave(s.trainers[rec.TrainerID].save), nil
	}
	row, ok := s.trainers[rec.TrainerID]
	if !ok || row.save == nil {
		return nil, ErrNotFound
	}
	seed, err := newOpaqueID()
	if err != nil {
		return nil, err
	}
	transition, err := planActivityTransition(s.content, row.save, rec, seed)
	if err != nil {
		return nil, err
	}
	s.activities[rec.NaturalKey] = storedActivity{result: transition.result}
	if transition.dailyMastery != nil {
		if _, ok := s.activities[transition.dailyMastery.NaturalKey]; !ok {
			s.activities[transition.dailyMastery.NaturalKey] = storedActivity{result: *transition.dailyMastery}
		}
	}
	row.save = transition.save
	return cloneSave(transition.save), nil
}

func (s *MemoryStore) mutateSave(trainerID string, fn func(*game.Save) error) (*game.Save, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.trainers[trainerID]
	if !ok || row.save == nil {
		return nil, ErrNotFound
	}
	save := cloneSave(row.save)
	if err := fn(save); err != nil {
		return nil, err
	}
	if err := validateSave(s.content, save); err != nil {
		return nil, err
	}
	row.save = save
	return cloneSave(save), nil
}

// SetParty replaces the Trainer's party slots.
func (s *MemoryStore) SetParty(trainerID string, party [3]string) (*game.Save, error) {
	return s.mutateSave(trainerID, func(save *game.Save) error {
		if err := validatePartyIDs(save, party); err != nil {
			return err
		}
		save.Party = party
		return nil
	})
}

// SetBattleLoadout updates one Monster's battle loadout, clearing matching
// progression notices when noticeIDs is non-empty.
func (s *MemoryStore) SetBattleLoadout(trainerID, monsterID string, moves, noticeIDs []string) (*game.Save, error) {
	return s.setLoadout(trainerID, monsterID, moves, noticeIDs)
}

func (s *MemoryStore) setLoadout(trainerID, monsterID string, moves, noticeIDs []string) (*game.Save, error) {
	return s.mutateSave(trainerID, func(save *game.Save) error {
		idx, ok := findMonster(save, monsterID)
		if !ok {
			return ErrUnknownMonster
		}
		if err := validateLoadout(save.Collection[idx].MoveLibrary, moves); err != nil {
			return err
		}
		save.Collection[idx].BattleLoadout = append([]string(nil), moves...)
		removeNotices(save, noticeIDs)
		return nil
	})
}

// AcknowledgeProgressionNotices removes the given progression notices.
func (s *MemoryStore) AcknowledgeProgressionNotices(trainerID string, noticeIDs []string) (*game.Save, error) {
	return s.mutateSave(trainerID, func(save *game.Save) error {
		removeNotices(save, noticeIDs)
		return nil
	})
}

// SetNickname updates one Monster's nickname.
func (s *MemoryStore) SetNickname(trainerID, monsterID, nickname string) (*game.Save, error) {
	validated, err := game.ValidateNickname(nickname)
	if err != nil {
		return nil, err
	}
	return s.mutateSave(trainerID, func(save *game.Save) error {
		idx, ok := findMonster(save, monsterID)
		if !ok {
			return ErrUnknownMonster
		}
		save.Collection[idx].Nickname = validated
		return nil
	})
}

// AcceptEvolution applies a pending evolution for monsterID.
func (s *MemoryStore) AcceptEvolution(trainerID, monsterID string) (*game.Save, error) {
	return s.mutateSave(trainerID, func(save *game.Save) error {
		return acceptEvolution(s.content, save, monsterID)
	})
}

// ActivityExists reports whether naturalKey is recorded for trainerID.
func (s *MemoryStore) ActivityExists(trainerID, naturalKey string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	act, ok := s.activities[naturalKey]
	if !ok || act.result.TrainerID != trainerID {
		return false, nil
	}
	return true, nil
}

// ActivityResult returns a stored activity row when naturalKey exists.
func (s *MemoryStore) ActivityResult(trainerID, naturalKey string) (*ActivityResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	act, ok := s.activities[naturalKey]
	if !ok || act.result.TrainerID != trainerID {
		return nil, nil
	}
	res := act.result
	return &res, nil
}

// StartSession records one authenticated in-memory session.
func (s *MemoryStore) StartSession(rec SessionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.trainers[rec.TrainerID]; !ok {
		return ErrNotFound
	}
	if _, exists := s.sessions[rec.ID]; !exists {
		s.sessions[rec.ID] = rec
	}
	return nil
}

// EndSession closes an open in-memory session.
func (s *MemoryStore) EndSession(sessionID string, endedAt time.Time, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.sessions[sessionID]
	if !ok {
		return nil
	}
	if rec.EndedAt.IsZero() {
		rec.EndedAt, rec.EndReason = endedAt, reason
		s.sessions[sessionID] = rec
	}
	return nil
}

// TrainerStats derives lifetime statistics from the in-memory records.
func (s *MemoryStore) TrainerStats(trainerID string) (TrainerStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.trainers[trainerID]
	if !ok {
		return TrainerStats{}, ErrNotFound
	}
	stats := TrainerStats{
		TrainerID: trainerID, CreatedAt: row.createdAt, Wins: row.wins, Losses: row.losses,
	}
	if row.save != nil {
		stats.CollectionSize = len(row.save.Collection)
	}
	currentWins, currentLosses := 0, 0
	results := make([]BattleResult, 0)
	for _, stored := range s.battles {
		if stored.result.Winner == trainerID || stored.result.Loser == trainerID {
			results = append(results, stored.result)
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].CompletedAt.Before(results[j].CompletedAt) })
	for _, result := range results {
		if result.Winner == trainerID {
			currentWins++
			currentLosses = 0
			stats.LongestStreak = max(stats.LongestStreak, currentWins)
			stats.CurrentStreak = currentWins
		} else {
			currentLosses++
			currentWins = 0
			stats.CurrentStreak = -currentLosses
		}
	}
	for _, stored := range s.activities {
		result := stored.result
		if result.TrainerID != trainerID {
			continue
		}
		var payload activityPayload
		_ = json.Unmarshal([]byte(result.Payload), &payload)
		if result.CapturedMonsterID != "" {
			stats.Captures++
		}
		if result.Kind == KindExpedition && (payload.Outcome == OutcomeCaptured || payload.Outcome == OutcomeHuntFailed) {
			stats.Expeditions++
		}
		if result.Kind == KindSparring && payload.Outcome == OutcomeCleared {
			stats.DojoClears++
		}
		if result.Kind == KindDailyMastery || payload.MasteryOnly {
			stats.MasteryMarks++
		}
	}
	for _, session := range s.sessions {
		if session.TrainerID != trainerID {
			continue
		}
		stats.Sessions++
		if session.EndedAt.IsZero() && !session.StartedAt.IsZero() {
			stats.PlayTime += time.Since(session.StartedAt)
		} else if session.EndedAt.After(session.StartedAt) {
			stats.PlayTime += session.EndedAt.Sub(session.StartedAt)
		}
	}
	return stats, nil
}

// WorldStats returns durable in-memory global totals.
func (s *MemoryStore) WorldStats() (WorldStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return WorldStats{
		RegisteredTrainers:  len(s.trainers),
		CompletedBattles:    len(s.battles),
		CompletedActivities: len(s.activities),
	}, nil
}

func cloneMemoryTrainer(row *memoryTrainer) *game.Trainer {
	if row == nil {
		return nil
	}
	tr := &game.Trainer{ID: row.id}
	if row.save != nil {
		tr.Save = cloneSave(row.save)
		tr.Save.Wins = row.wins
		tr.Save.Losses = row.losses
		tr.Save.Handle = row.handle
	}
	return tr
}
