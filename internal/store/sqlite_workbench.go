package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"termon.sh/internal/game"
)

func (s *SQLiteStore) mutateSave(trainerID string, fn func(*game.Save) error) (*game.Save, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("store: begin save mutation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	save, err := s.loadSaveFrom(tx, trainerID)
	if err != nil {
		return nil, err
	}
	if err := fn(save); err != nil {
		return nil, err
	}
	if err := validateSave(s.content, save); err != nil {
		return nil, err
	}
	if err := persistSave(tx, trainerID, save); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: commit save mutation: %w", err)
	}
	return cloneSave(save), nil
}

// SetParty validates and persists Party slot order.
func (s *SQLiteStore) SetParty(trainerID string, party [3]string) (*game.Save, error) {
	return s.mutateSave(trainerID, func(save *game.Save) error {
		if err := validatePartyIDs(save, party); err != nil {
			return err
		}
		save.Party = party
		return nil
	})
}

// SetBattleLoadout sets one Monster's Battle Loadout, removing notice IDs
// atomically when noticeIDs is non-empty.
func (s *SQLiteStore) SetBattleLoadout(trainerID, monsterID string, moves, noticeIDs []string) (*game.Save, error) {
	return s.setLoadout(trainerID, monsterID, moves, noticeIDs)
}

func (s *SQLiteStore) setLoadout(trainerID, monsterID string, moves, noticeIDs []string) (*game.Save, error) {
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

// AcknowledgeProgressionNotices deletes open notices by ID.
func (s *SQLiteStore) AcknowledgeProgressionNotices(trainerID string, noticeIDs []string) (*game.Save, error) {
	return s.mutateSave(trainerID, func(save *game.Save) error {
		removeNotices(save, noticeIDs)
		return nil
	})
}

// SetNickname validates and persists a Monster nickname.
func (s *SQLiteStore) SetNickname(trainerID, monsterID, nickname string) (*game.Save, error) {
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

// AcceptEvolution applies a pending Evolution for one Monster.
func (s *SQLiteStore) AcceptEvolution(trainerID, monsterID string) (*game.Save, error) {
	return s.mutateSave(trainerID, func(save *game.Save) error {
		return acceptEvolution(s.content, save, monsterID)
	})
}

// RecordActivityResult commits one solo activity outcome and rewards.
func (s *SQLiteStore) RecordActivityResult(rec ActivityRecord) (*game.Save, error) {
	payload, err := canonicalActivityPayload(rec)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("store: begin activity result: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if existing, err := loadActivityByKey(tx, rec.NaturalKey); err != nil {
		return nil, err
	} else if existing != nil {
		if existing.Payload != payload {
			return nil, ErrResultConflict
		}
		save, err := s.loadSaveFrom(tx, rec.TrainerID)
		if err != nil {
			return nil, err
		}
		return cloneSave(save), nil
	}

	save, err := s.loadSaveFrom(tx, rec.TrainerID)
	if err != nil {
		return nil, err
	}
	seed, err := newOpaqueID()
	if err != nil {
		return nil, err
	}
	transition, err := planActivityTransition(s.content, save, rec, seed)
	if err != nil {
		return nil, err
	}
	if err := insertActivityResult(tx, transition.result); err != nil {
		return nil, err
	}

	if transition.dailyMastery != nil {
		if existing, err := loadActivityByKey(tx, transition.dailyMastery.NaturalKey); err != nil {
			return nil, err
		} else if existing == nil {
			if err := insertActivityResult(tx, *transition.dailyMastery); err != nil {
				return nil, err
			}
		}
	}

	if err := persistSave(tx, rec.TrainerID, transition.save); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: commit activity result: %w", err)
	}
	return cloneSave(transition.save), nil
}

func insertActivityResult(tx *sql.Tx, result ActivityResult) error {
	if _, err := tx.Exec(
		`INSERT INTO activity_results(id, natural_key, trainer_id, kind, completed_at, payload, captured_monster_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		result.ID, result.NaturalKey, result.TrainerID, result.Kind,
		result.CompletedAt.UTC().Format(time.RFC3339Nano), result.Payload,
		nullString(result.CapturedMonsterID),
	); err != nil {
		return fmt.Errorf("store: insert activity result: %w", err)
	}
	return nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func loadActivityByKey(tx *sql.Tx, key string) (*ActivityResult, error) {
	var (
		id, naturalKey, trainerID, kind, completedAt, payload string
		captured                                              sql.NullString
	)
	err := tx.QueryRow(
		`SELECT id, natural_key, trainer_id, kind, completed_at, payload, captured_monster_id
		FROM activity_results WHERE natural_key = ?`, key,
	).Scan(&id, &naturalKey, &trainerID, &kind, &completedAt, &payload, &captured)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: load activity result: %w", err)
	}
	at, err := time.Parse(time.RFC3339Nano, completedAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse activity completed_at: %w", err)
	}
	res := &ActivityResult{
		ID: id, NaturalKey: naturalKey, TrainerID: trainerID, Kind: ActivityKind(kind),
		CompletedAt: at, Payload: payload,
	}
	if captured.Valid {
		res.CapturedMonsterID = captured.String
	}
	return res, nil
}

// ActivityExists reports whether naturalKey was committed for trainerID.
func (s *SQLiteStore) ActivityExists(trainerID, naturalKey string) (bool, error) {
	var id string
	err := s.db.QueryRow(
		`SELECT id FROM activity_results WHERE natural_key = ? AND trainer_id = ?`,
		naturalKey, trainerID,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: activity exists: %w", err)
	}
	return true, nil
}

// ActivityResult returns one stored activity row for trainerID and naturalKey.
func (s *SQLiteStore) ActivityResult(trainerID, naturalKey string) (*ActivityResult, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := loadActivityByKey(tx, naturalKey)
	if err != nil {
		return nil, err
	}
	if res == nil || res.TrainerID != trainerID {
		return nil, nil
	}
	_ = tx.Rollback()
	return res, nil
}
