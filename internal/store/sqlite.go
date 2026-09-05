package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // Register the pure-Go SQLite database driver.

	"termon.sh/internal/content"
	"termon.sh/internal/game"
	"termon.sh/internal/latency"
)

const currentSchemaVersion = 4

// ErrSchemaTooNew means the database was written by a newer termond binary.
var ErrSchemaTooNew = errors.New("store: database schema is newer than this binary")

var migrations = []string{
	`CREATE TABLE trainers (
		id TEXT PRIMARY KEY,
		handle TEXT,
		wins INTEGER NOT NULL DEFAULT 0,
		losses INTEGER NOT NULL DEFAULT 0,
		save_version INTEGER,
		save_payload BLOB,
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE ssh_credentials (
		fingerprint_hash TEXT PRIMARY KEY,
		trainer_id TEXT NOT NULL REFERENCES trainers(id) ON DELETE CASCADE
	);`,
	`CREATE TABLE battle_results (
		id TEXT PRIMARY KEY,
		winner_id TEXT NOT NULL REFERENCES trainers(id),
		loser_id TEXT NOT NULL REFERENCES trainers(id),
		reason TEXT NOT NULL,
		completed_at TEXT NOT NULL,
		CHECK (winner_id <> loser_id)
	);`,
	`CREATE TABLE activity_results (
		id TEXT PRIMARY KEY,
		natural_key TEXT NOT NULL UNIQUE,
		trainer_id TEXT NOT NULL REFERENCES trainers(id),
		kind TEXT NOT NULL,
		completed_at TEXT NOT NULL,
		payload TEXT NOT NULL,
		captured_monster_id TEXT
	);
	ALTER TABLE battle_results ADD COLUMN result_body TEXT NOT NULL DEFAULT '{}';`,
	`CREATE TABLE session_results (
		id TEXT PRIMARY KEY,
		trainer_id TEXT NOT NULL REFERENCES trainers(id),
		started_at TEXT NOT NULL,
		ended_at TEXT,
		end_reason TEXT,
		app_version TEXT NOT NULL,
		resume_target TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX session_results_trainer_started
		ON session_results(trainer_id, started_at DESC);
	CREATE INDEX battle_results_winner_completed
		ON battle_results(winner_id, completed_at DESC);
	CREATE INDEX battle_results_loser_completed
		ON battle_results(loser_id, completed_at DESC);
	CREATE INDEX activity_results_trainer_kind
		ON activity_results(trainer_id, kind, completed_at DESC);`,
}

// SQLiteStore persists Trainers and Battle Results in one SQLite database.
type SQLiteStore struct {
	db          *sql.DB
	path        string
	journalMode string
	synchronous string
	content     *content.Set

	// wantSynchronous is the configured mode; readProfile cross-checks it.
	wantSynchronous string

	metricsMu sync.Mutex
	writes    writeSamples
}

// UseContent injects the read-only content pack for progression mutations.
func (s *SQLiteStore) UseContent(set *content.Set) {
	s.content = set
}

// SQLiteSynchronous controls when WAL commits reach durable storage.
type SQLiteSynchronous string

// Durability modes for the SQLite WAL, from strongest to fastest.
const (
	// SQLiteSynchronousFull preserves acknowledged commits across host failures.
	SQLiteSynchronousFull SQLiteSynchronous = "FULL"
	// SQLiteSynchronousNormal omits per-commit WAL synchronization.
	SQLiteSynchronousNormal SQLiteSynchronous = "NORMAL"
)

// SQLiteOptions controls SQLite behavior that changes its durability profile.
type SQLiteOptions struct {
	Synchronous SQLiteSynchronous
}

type writeSamples struct {
	wait   []time.Duration
	sql    []time.Duration
	commit []time.Duration
}

// maxWriteSamples bounds each latency sample slice so a long-lived process's
// memory stays flat; oldest samples are dropped as new ones arrive.
const maxWriteSamples = 1024

func appendSample(samples []time.Duration, d time.Duration) []time.Duration {
	samples = append(samples, d)
	if len(samples) > maxWriteSamples {
		return slices.Clone(samples[len(samples)-maxWriteSamples:])
	}
	return samples
}

// LatencyPercentiles summarizes one measured write phase.
type LatencyPercentiles struct {
	P50 time.Duration
	P95 time.Duration
	P99 time.Duration
}

// SQLiteMetrics describes Battle Result writes and database pool contention.
type SQLiteMetrics struct {
	JournalMode          string
	Synchronous          string
	WriteTransactions    int
	WriteWait            LatencyPercentiles
	WriteSQL             LatencyPercentiles
	WriteCommit          LatencyPercentiles
	DatabaseWaitCount    int64
	DatabaseWaitDuration time.Duration
	WALBytes             int64
}

// NewSQLiteStore opens or creates a migrated WAL-mode database at path.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	return OpenSQLiteStore(path, SQLiteOptions{Synchronous: SQLiteSynchronousNormal})
}

// OpenSQLiteStore opens SQLite with an explicit durability profile.
//
// Connection-scoped PRAGMAs (foreign_keys, busy_timeout, synchronous) are
// passed via DSN parameters so EVERY connection database/sql creates gets
// them — not just the first one. journal_mode=WAL persists in the database
// file but is repeated harmlessly. readProfile then cross-checks the
// effective settings at boot. The synchronous mode is accepted
// case-insensitively and normalized to the canonical constant.
func OpenSQLiteStore(path string, options SQLiteOptions) (*SQLiteStore, error) {
	if options.Synchronous == "" {
		options.Synchronous = SQLiteSynchronousFull
	}
	switch strings.ToUpper(strings.TrimSpace(string(options.Synchronous))) {
	case string(SQLiteSynchronousFull):
		options.Synchronous = SQLiteSynchronousFull
	case string(SQLiteSynchronousNormal):
		options.Synchronous = SQLiteSynchronousNormal
	default:
		return nil, fmt.Errorf("store: unsupported SQLite synchronous mode %q", options.Synchronous)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("store: create database directory: %w", err)
	}
	// Resolve to an absolute path: relative paths in a file: URI are
	// ambiguous across drivers (modernc's parser rejects them).
	if absPath, err := filepath.Abs(path); err == nil {
		path = absPath
	}
	dsn := url.URL{
		Scheme: "file",
		Path:   path,
		RawQuery: url.Values{
			"_pragma": []string{
				"foreign_keys(1)",
				"busy_timeout(5000)",
				"journal_mode(WAL)",
				"synchronous(" + string(options.Synchronous) + ")",
			},
		}.Encode(),
	}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, fmt.Errorf("store: open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &SQLiteStore{db: db, path: path, wantSynchronous: strings.ToLower(string(options.Synchronous))}
	if err := s.readProfile(); err != nil {
		_ = db.Close()
		return nil, err
	}
	var foreignKeys int
	if err := s.db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: read SQLite foreign_keys: %w", err)
	}
	if s.journalMode != "wal" || s.synchronous != s.wantSynchronous || foreignKeys != 1 {
		_ = db.Close()
		return nil, fmt.Errorf("store: sqlite profile mismatch: journal=%s synchronous=%s foreign_keys=%d, want wal/%s/1",
			s.journalMode, s.synchronous, foreignKeys, s.wantSynchronous)
	}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := s.Readiness(context.Background(), true); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// SQLiteReadiness describes the database compatibility boundary checked before
// SSH starts and reported by the readiness endpoint.
type SQLiteReadiness struct {
	SchemaVersion          int    `json:"schema_version"`
	SupportedSchemaVersion int    `json:"supported_schema_version"`
	MaxSaveVersion         int    `json:"max_save_version"`
	SupportedSaveVersion   int    `json:"supported_save_version"`
	JournalMode            string `json:"journal_mode"`
	Synchronous            string `json:"synchronous"`
	Integrity              string `json:"integrity"`
}

// Readiness verifies that SQLite is reachable and that its schema and Saves
// are compatible with this binary. deep also runs SQLite's quick integrity
// check; startup and restore verification use it, while frequent probes do not.
func (s *SQLiteStore) Readiness(ctx context.Context, deep bool) (SQLiteReadiness, error) {
	health := SQLiteReadiness{
		SupportedSchemaVersion: currentSchemaVersion,
		SupportedSaveVersion:   currentSaveVersion,
		JournalMode:            s.journalMode,
		Synchronous:            s.synchronous,
		Integrity:              "not_run",
	}
	if err := s.db.PingContext(ctx); err != nil {
		return health, fmt.Errorf("store: ping sqlite: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&health.SchemaVersion); err != nil {
		return health, fmt.Errorf("store: read schema version for readiness: %w", err)
	}
	if health.SchemaVersion > currentSchemaVersion {
		return health, fmt.Errorf("%w: database=%d supported=%d", ErrSchemaTooNew, health.SchemaVersion, currentSchemaVersion)
	}
	var incompatibleSaves int
	if err := s.db.QueryRowContext(ctx, `SELECT
		COALESCE(MAX(save_version), 0),
		COALESCE(SUM(CASE WHEN save_version IS NOT NULL AND save_version <> ? THEN 1 ELSE 0 END), 0)
		FROM trainers`, currentSaveVersion).Scan(&health.MaxSaveVersion, &incompatibleSaves); err != nil {
		return health, fmt.Errorf("store: read Save version for readiness: %w", err)
	}
	if health.MaxSaveVersion > currentSaveVersion {
		return health, fmt.Errorf("%w: database=%d supported=%d", ErrSaveTooNew, health.MaxSaveVersion, currentSaveVersion)
	}
	if incompatibleSaves > 0 {
		return health, fmt.Errorf("store: %d Saves have an unsupported version; supported=%d", incompatibleSaves, currentSaveVersion)
	}
	if deep {
		if err := s.db.QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&health.Integrity); err != nil {
			return health, fmt.Errorf("store: run SQLite quick_check: %w", err)
		}
		if health.Integrity != "ok" {
			return health, fmt.Errorf("store: SQLite quick_check: %s", health.Integrity)
		}
	}
	return health, nil
}

func (s *SQLiteStore) readProfile() error {
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&s.journalMode); err != nil {
		return fmt.Errorf("store: read SQLite journal mode: %w", err)
	}
	var synchronous int
	if err := s.db.QueryRow("PRAGMA synchronous").Scan(&synchronous); err != nil {
		return fmt.Errorf("store: read SQLite synchronous mode: %w", err)
	}
	switch synchronous {
	case 1:
		s.synchronous = "normal"
	case 2:
		s.synchronous = "full"
	default:
		return fmt.Errorf("store: unexpected SQLite synchronous mode %d", synchronous)
	}
	return nil
}

func (s *SQLiteStore) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("store: create migrations table: %w", err)
	}
	var version int
	if err := s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version); err != nil {
		return fmt.Errorf("store: read schema version: %w", err)
	}
	if version > currentSchemaVersion {
		return fmt.Errorf("%w: database=%d supported=%d", ErrSchemaTooNew, version, currentSchemaVersion)
	}
	for version < currentSchemaVersion {
		next := version + 1
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("store: begin migration %d: %w", next, err)
		}
		if _, err := tx.Exec(migrations[next-1]); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: apply migration %d: %w", next, err)
		}
		if _, err := tx.Exec("INSERT INTO schema_migrations(version) VALUES (?)", next); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: record migration %d: %w", next, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("store: commit migration %d: %w", next, err)
		}
		version = next
	}
	return nil
}

// Close closes the database.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Metrics returns a point-in-time snapshot of Battle Result write phases.
func (s *SQLiteStore) Metrics() (SQLiteMetrics, error) {
	dbStats := s.db.Stats()
	s.metricsMu.Lock()
	metrics := SQLiteMetrics{
		JournalMode:          s.journalMode,
		Synchronous:          s.synchronous,
		WriteTransactions:    len(s.writes.commit),
		WriteWait:            latencyPercentiles(s.writes.wait),
		WriteSQL:             latencyPercentiles(s.writes.sql),
		WriteCommit:          latencyPercentiles(s.writes.commit),
		DatabaseWaitCount:    dbStats.WaitCount,
		DatabaseWaitDuration: dbStats.WaitDuration,
	}
	s.metricsMu.Unlock()
	info, err := os.Stat(s.path + "-wal")
	if errors.Is(err, os.ErrNotExist) {
		return metrics, nil
	}
	if err != nil {
		return SQLiteMetrics{}, fmt.Errorf("store: inspect WAL: %w", err)
	}
	metrics.WALBytes = info.Size()
	return metrics, nil
}

// CreateTrainer creates a stable Trainer and binds its first SSH Credential.
func (s *SQLiteStore) CreateTrainer(fingerprintHash string) (*game.Trainer, error) {
	id, err := newTrainerID()
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("store: begin Trainer creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec("INSERT INTO trainers(id) VALUES (?)", id); err != nil {
		return nil, fmt.Errorf("store: create Trainer: %w", err)
	}
	if _, err := tx.Exec(
		"INSERT INTO ssh_credentials(fingerprint_hash, trainer_id) VALUES (?, ?)",
		fingerprintHash, id,
	); err != nil {
		credentialErr := fmt.Errorf("store: create SSH Credential: %w", err)
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return nil, errors.Join(credentialErr, rollbackErr)
		}
		trainer, resolveErr := s.ResolveCredential(fingerprintHash)
		if resolveErr == nil {
			return trainer, nil
		}
		return nil, credentialErr
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: commit Trainer creation: %w", err)
	}
	return &game.Trainer{ID: id}, nil
}

// ResolveCredential returns the Trainer authenticated by fingerprintHash.
func (s *SQLiteStore) ResolveCredential(fingerprintHash string) (*game.Trainer, error) {
	return s.loadAndRewrite(func(tx *sql.Tx) rowScanner {
		return tx.QueryRow(
			`SELECT trainers.id, trainers.handle, trainers.wins, trainers.losses,
				trainers.save_version, trainers.save_payload
			FROM ssh_credentials
			JOIN trainers ON trainers.id = ssh_credentials.trainer_id
			WHERE ssh_credentials.fingerprint_hash = ?`,
			fingerprintHash,
		)
	})
}

// LoadTrainer returns a Trainer by stable ID.
func (s *SQLiteStore) LoadTrainer(id string) (*game.Trainer, error) {
	return s.loadAndRewrite(func(tx *sql.Tx) rowScanner {
		return tx.QueryRow(
			`SELECT id, handle, wins, losses, save_version, save_payload
			FROM trainers WHERE id = ?`,
			id,
		)
	})
}

// loadAndRewrite loads one Trainer and, when the loader remints or sanitizes
// the payload, persists the rewrite in the same transaction. Reading and
// rewriting in one tx closes the lost-update window: without it, a mutation
// committing between the read and the rewrite is silently reverted by the
// stale save written back here.
func (s *SQLiteStore) loadAndRewrite(query func(*sql.Tx) rowScanner) (*game.Trainer, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("store: begin load: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	trainer, rewritten, err := s.scanTrainer(query(tx))
	if err != nil {
		return nil, err
	}
	if rewritten {
		if err := persistSave(tx, trainer.ID, trainer.Save); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: commit load: %w", err)
	}
	return trainer, nil
}

// CompleteOnboarding writes a Trainer's first Save. It refuses to overwrite
// an existing Save (ErrAlreadyOnboarded) so a replayed onboarding flow can
// never clobber party or record data.
func (s *SQLiteStore) CompleteOnboarding(id string, save *game.Save) error {
	if save == nil {
		return errors.New("store: nil save")
	}
	if len(save.Collection) != 1 || save.Collection[0].Species == "" {
		return errors.New("store: onboarding requires one starter species")
	}
	starter, err := mintStarterMonster(s.content, save.Collection[0].Species)
	if err != nil {
		return err
	}
	payload, err := encodePayload(savePayload{
		Collection: []game.Monster{starter},
		Party:      [3]string{starter.ID, "", ""},
		Notices:    nil,
	})
	if err != nil {
		return err
	}
	result, err := s.db.Exec(
		`UPDATE trainers
		SET handle = ?, wins = ?, losses = ?, save_version = ?, save_payload = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND save_version IS NULL`,
		save.Handle, save.Wins, save.Losses, currentSaveVersion, payload, id,
	)
	if err != nil {
		return fmt.Errorf("store: complete onboarding: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: complete onboarding rows: %w", err)
	}
	if updated == 0 {
		var exists int
		if err := s.db.QueryRow(`SELECT 1 FROM trainers WHERE id = ?`, id).Scan(&exists); err == nil {
			return ErrAlreadyOnboarded
		}
		return ErrNotFound
	}
	return nil
}

// ResetTrainer clears mutable game state while preserving identity and credentials.
func (s *SQLiteStore) ResetTrainer(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin reset: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM activity_results WHERE trainer_id = ?`, id); err != nil {
		return fmt.Errorf("store: clear activity results: %w", err)
	}
	result, err := tx.Exec(
		`UPDATE trainers
		SET handle = NULL, wins = 0, losses = 0, save_version = NULL,
			save_payload = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		id,
	)
	if err != nil {
		return fmt.Errorf("store: reset trainer: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: reset trainer rows: %w", err)
	}
	if updated == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// RecordBattleResult atomically stores the result and updates both records.
func (s *SQLiteStore) RecordBattleResult(rec BattleRecord) (ResultRecords, error) {
	started := time.Now()
	tx, err := s.db.Begin()
	if err != nil {
		return ResultRecords{}, fmt.Errorf("store: begin battle result: %w", err)
	}
	acquired := time.Now()
	defer func() { _ = tx.Rollback() }()
	body, err := canonicalBattleBody(rec)
	if err != nil {
		return ResultRecords{}, err
	}
	applied, err := checkBattleResult(tx, rec, body)
	if err != nil {
		return ResultRecords{}, err
	}
	if applied {
		winner, err := s.loadTrainerFrom(tx, rec.Result.Winner)
		if err != nil {
			return ResultRecords{}, err
		}
		loser, err := s.loadTrainerFrom(tx, rec.Result.Loser)
		if err != nil {
			return ResultRecords{}, err
		}
		return ResultRecords{Winner: winner.Save, Loser: loser.Save}, nil
	}
	if _, err := tx.Exec(
		`INSERT INTO battle_results(id, winner_id, loser_id, reason, completed_at, result_body)
		VALUES (?, ?, ?, ?, ?, ?)`,
		rec.Result.ID, rec.Result.Winner, rec.Result.Loser, rec.Result.Reason,
		rec.Result.CompletedAt.UTC().Format(time.RFC3339Nano), body,
	); err != nil {
		return ResultRecords{}, fmt.Errorf("store: insert battle result: %w", err)
	}
	if err := updateRecord(tx, rec.Result.Winner, "wins"); err != nil {
		return ResultRecords{}, err
	}
	if err := updateRecord(tx, rec.Result.Loser, "losses"); err != nil {
		return ResultRecords{}, err
	}
	if rec.ApplyRewards {
		prior, err := countPairResults24h(tx, rec.Result.Winner, rec.Result.Loser, rec.Result.CompletedAt)
		if err != nil {
			return ResultRecords{}, err
		}
		decay := pvpDecayMultiplier(prior)
		winnerSave, err := s.loadSaveFrom(tx, rec.Result.Winner)
		if err != nil {
			return ResultRecords{}, err
		}
		loserSave, err := s.loadSaveFrom(tx, rec.Result.Loser)
		if err != nil {
			return ResultRecords{}, err
		}
		if err := applyPvPRewards(s.content, winnerSave, rec.WinnerActive, rec.WinnerReserve, true, decay, rec.Result.ID); err != nil {
			return ResultRecords{}, err
		}
		if err := applyPvPRewards(s.content, loserSave, rec.LoserActive, rec.LoserReserve, false, decay, rec.Result.ID); err != nil {
			return ResultRecords{}, err
		}
		if err := persistSave(tx, rec.Result.Winner, winnerSave); err != nil {
			return ResultRecords{}, err
		}
		if err := persistSave(tx, rec.Result.Loser, loserSave); err != nil {
			return ResultRecords{}, err
		}
	}
	winner, err := s.loadTrainerFrom(tx, rec.Result.Winner)
	if err != nil {
		return ResultRecords{}, err
	}
	loser, err := s.loadTrainerFrom(tx, rec.Result.Loser)
	if err != nil {
		return ResultRecords{}, err
	}
	sqlDone := time.Now()
	if err := tx.Commit(); err != nil {
		return ResultRecords{}, fmt.Errorf("store: commit battle result: %w", err)
	}
	committed := time.Now()
	s.metricsMu.Lock()
	s.writes.wait = appendSample(s.writes.wait, acquired.Sub(started))
	s.writes.sql = appendSample(s.writes.sql, sqlDone.Sub(acquired))
	s.writes.commit = appendSample(s.writes.commit, committed.Sub(sqlDone))
	s.metricsMu.Unlock()
	return ResultRecords{Winner: winner.Save, Loser: loser.Save, Applied: true}, nil
}

func latencyPercentiles(samples []time.Duration) LatencyPercentiles {
	return LatencyPercentiles{
		P50: latency.Percentile(samples, 0.50),
		P95: latency.Percentile(samples, 0.95),
		P99: latency.Percentile(samples, 0.99),
	}
}

func checkBattleResult(tx *sql.Tx, rec BattleRecord, body string) (bool, error) {
	var winner, loser, reason, completedAt, storedBody string
	err := tx.QueryRow(
		`SELECT winner_id, loser_id, reason, completed_at, result_body FROM battle_results WHERE id = ?`,
		rec.Result.ID,
	).Scan(&winner, &loser, &reason, &completedAt, &storedBody)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: check battle result: %w", err)
	}
	wantCompletedAt := rec.Result.CompletedAt.UTC().Format(time.RFC3339Nano)
	if winner != rec.Result.Winner || loser != rec.Result.Loser || reason != rec.Result.Reason ||
		completedAt != wantCompletedAt || storedBody != body {
		return false, ErrResultConflict
	}
	return true, nil
}

func countPairResults24h(tx *sql.Tx, a, b string, at time.Time) (int, error) {
	windowStart := at.Add(-24 * time.Hour).UTC().Format(time.RFC3339Nano)
	windowEnd := at.UTC().Format(time.RFC3339Nano)
	var count int
	err := tx.QueryRow(
		`SELECT COUNT(*) FROM battle_results
		WHERE completed_at >= ? AND completed_at < ?
		AND ((winner_id = ? AND loser_id = ?) OR (winner_id = ? AND loser_id = ?))`,
		windowStart, windowEnd, a, b, b, a,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: count pair results: %w", err)
	}
	return count, nil
}

func (s *SQLiteStore) loadSaveFrom(q trainerQueryer, id string) (*game.Save, error) {
	trainer, err := s.loadTrainerFrom(q, id)
	if err != nil {
		return nil, err
	}
	return cloneSave(trainer.Save), nil
}

func persistSave(tx *sql.Tx, id string, save *game.Save) error {
	payload, err := encodePayload(payloadFromSave(save))
	if err != nil {
		return err
	}
	result, err := tx.Exec(
		`UPDATE trainers SET save_payload = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND save_version IS NOT NULL`,
		payload, id,
	)
	if err != nil {
		return fmt.Errorf("store: persist save: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: persist save rows: %w", err)
	}
	if updated != 1 {
		return fmt.Errorf("store: persist save: %w", ErrNotFound)
	}
	return nil
}

func updateRecord(tx *sql.Tx, id, column string) error {
	query := "UPDATE trainers SET " + column + " = " + column + " + 1, " + //nolint:gosec // column is a compile-time constant ("wins"/"losses"), value is parameterized
		"updated_at = CURRENT_TIMESTAMP WHERE id = ? AND save_version IS NOT NULL"
	result, err := tx.Exec(query, id)
	if err != nil {
		return fmt.Errorf("store: update %s record: %w", column, err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update %s record rows: %w", column, err)
	}
	if updated != 1 {
		return fmt.Errorf("store: update %s record: %w", column, ErrNotFound)
	}
	return nil
}

type trainerQueryer interface {
	QueryRow(string, ...any) *sql.Row
}

func (s *SQLiteStore) loadTrainerFrom(q trainerQueryer, id string) (*game.Trainer, error) {
	trainer, _, err := s.scanTrainer(q.QueryRow(
		`SELECT id, handle, wins, losses, save_version, save_payload
		FROM trainers WHERE id = ?`,
		id,
	))
	return trainer, err
}

type rowScanner interface {
	Scan(...any) error
}

func (s *SQLiteStore) scanTrainer(row rowScanner) (*game.Trainer, bool, error) {
	var (
		id      string
		handle  sql.NullString
		wins    int
		losses  int
		version sql.NullInt64
		payload []byte
	)
	err := row.Scan(&id, &handle, &wins, &losses, &version, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, ErrNotFound
	}
	if err != nil {
		return nil, false, fmt.Errorf("store: load Trainer: %w", err)
	}
	trainer := &game.Trainer{ID: id}
	if !version.Valid {
		return trainer, false, nil
	}
	if version.Int64 > currentSaveVersion {
		return nil, false, fmt.Errorf("%w: Save=%d supported=%d", ErrSaveTooNew, version.Int64, currentSaveVersion)
	}
	if version.Int64 != currentSaveVersion {
		return nil, false, fmt.Errorf("%w: unsupported version %d", ErrCorruptSave, version.Int64)
	}
	saved, rewritten, err := decodePayload(payload)
	if err != nil {
		return nil, false, fmt.Errorf("%w for trainer %s: %w", ErrCorruptSave, id, err)
	}
	trainer.Save = saveFromPayload(handle.String, wins, losses, saved)
	if sanitizeSaveMoves(s.content, trainer.Save) {
		rewritten = true
	}
	if err := validateSave(s.content, trainer.Save); err != nil {
		return nil, false, err
	}
	return trainer, rewritten, nil
}

func newTrainerID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("store: generate Trainer ID: %w", err)
	}
	return hex.EncodeToString(id[:]), nil
}
