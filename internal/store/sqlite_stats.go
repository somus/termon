package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// StartSession records one authenticated SSH connection.
func (s *SQLiteStore) StartSession(rec SessionRecord) error {
	if rec.ID == "" || rec.TrainerID == "" || rec.StartedAt.IsZero() {
		return errors.New("store: invalid session record")
	}
	_, err := s.db.Exec(`INSERT INTO session_results(
		id, trainer_id, started_at, app_version, resume_target
	) VALUES (?, ?, ?, ?, ?) ON CONFLICT(id) DO NOTHING`,
		rec.ID, rec.TrainerID, rec.StartedAt.UTC().Format(time.RFC3339Nano),
		rec.AppVersion, rec.ResumeTarget)
	if err != nil {
		return fmt.Errorf("store: start session: %w", err)
	}
	return nil
}

// EndSession closes an open session. Repeated calls preserve the first end.
func (s *SQLiteStore) EndSession(sessionID string, endedAt time.Time, reason string) error {
	if sessionID == "" || endedAt.IsZero() || reason == "" {
		return errors.New("store: invalid session end")
	}
	_, err := s.db.Exec(`UPDATE session_results
		SET ended_at = ?, end_reason = ?
		WHERE id = ? AND ended_at IS NULL`,
		endedAt.UTC().Format(time.RFC3339Nano), reason, sessionID)
	if err != nil {
		return fmt.Errorf("store: end session: %w", err)
	}
	return nil
}

// TrainerStats derives authoritative lifetime statistics from durable records.
func (s *SQLiteStore) TrainerStats(trainerID string) (TrainerStats, error) {
	var stats TrainerStats
	stats.TrainerID = trainerID
	var created string
	if err := s.db.QueryRow(`SELECT created_at, wins, losses FROM trainers WHERE id = ?`, trainerID).
		Scan(&created, &stats.Wins, &stats.Losses); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TrainerStats{}, ErrNotFound
		}
		return TrainerStats{}, fmt.Errorf("store: load Trainer stats: %w", err)
	}
	stats.CreatedAt, _ = parseSQLiteTime(created)
	trainer, err := s.LoadTrainer(trainerID)
	if err != nil {
		return TrainerStats{}, err
	}
	if trainer.Save != nil {
		stats.CollectionSize = len(trainer.Save.Collection)
	}
	if err := s.db.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN captured_monster_id IS NOT NULL THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN kind = 'expedition' AND json_extract(payload, '$.outcome') IN ('captured', 'hunt_failed') THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN kind = 'sparring' AND json_extract(payload, '$.outcome') = 'cleared' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN kind = 'daily_mastery' OR json_extract(payload, '$.mastery_only') = 1 THEN 1 ELSE 0 END), 0)
		FROM activity_results WHERE trainer_id = ?`, trainerID).
		Scan(&stats.Captures, &stats.Expeditions, &stats.DojoClears, &stats.MasteryMarks); err != nil {
		return TrainerStats{}, fmt.Errorf("store: aggregate activity stats: %w", err)
	}
	if err := s.sessionStats(trainerID, &stats); err != nil {
		return TrainerStats{}, err
	}
	if err := s.streakStats(trainerID, &stats); err != nil {
		return TrainerStats{}, err
	}
	return stats, nil
}

func (s *SQLiteStore) sessionStats(trainerID string, stats *TrainerStats) error {
	rows, err := s.db.Query(`SELECT started_at, ended_at FROM session_results
		WHERE trainer_id = ? ORDER BY started_at`, trainerID)
	if err != nil {
		return fmt.Errorf("store: query session stats: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var started string
		var ended sql.NullString
		if err := rows.Scan(&started, &ended); err != nil {
			return fmt.Errorf("store: scan session stats: %w", err)
		}
		stats.Sessions++
		start, startErr := parseSQLiteTime(started)
		if !ended.Valid {
			if startErr == nil {
				stats.PlayTime += time.Since(start)
			}
			continue
		}
		end, endErr := parseSQLiteTime(ended.String)
		if startErr == nil && endErr == nil && end.After(start) {
			stats.PlayTime += end.Sub(start)
		}
	}
	return rows.Err()
}

func (s *SQLiteStore) streakStats(trainerID string, stats *TrainerStats) error {
	rows, err := s.db.Query(`SELECT winner_id FROM battle_results
		WHERE winner_id = ? OR loser_id = ? ORDER BY completed_at, id`, trainerID, trainerID)
	if err != nil {
		return fmt.Errorf("store: query streak stats: %w", err)
	}
	defer func() { _ = rows.Close() }()
	currentWins, currentLosses := 0, 0
	for rows.Next() {
		var winner string
		if err := rows.Scan(&winner); err != nil {
			return fmt.Errorf("store: scan streak stats: %w", err)
		}
		if winner == trainerID {
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
	return rows.Err()
}

// WorldStats returns durable global totals for a cached public display.
func (s *SQLiteStore) WorldStats() (WorldStats, error) {
	var stats WorldStats
	queries := []struct {
		query string
		dest  *int
	}{
		{"SELECT COUNT(*) FROM trainers", &stats.RegisteredTrainers},
		{"SELECT COUNT(*) FROM battle_results", &stats.CompletedBattles},
		{"SELECT COUNT(*) FROM activity_results", &stats.CompletedActivities},
	}
	for _, item := range queries {
		if err := s.db.QueryRow(item.query).Scan(item.dest); err != nil {
			return WorldStats{}, fmt.Errorf("store: load world stats: %w", err)
		}
	}
	return stats, nil
}

func parseSQLiteTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("store: invalid timestamp %q", value)
}
