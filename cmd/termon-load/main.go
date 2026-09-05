// Command termon-load runs a concurrent Hub and SQLite capacity scenario.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"termon.sh/internal/loadtest"
)

type output struct {
	Trainers          int     `json:"trainers"`
	Rounds            int     `json:"rounds"`
	Dojos             int     `json:"dojos"`
	PeakBattles       int     `json:"peak_battles"`
	CompletedBattles  int     `json:"completed_battles"`
	Failures          int     `json:"failures"`
	FirstError        string  `json:"first_error,omitempty"`
	Messages          uint64  `json:"messages"`
	SetupMS           float64 `json:"setup_ms"`
	DurationSeconds   float64 `json:"duration_seconds"`
	P95MatchmakingMS  float64 `json:"p95_matchmaking_ms"`
	P50CommitMS       float64 `json:"p50_commit_ms"`
	P95CommitMS       float64 `json:"p95_commit_ms"`
	P99CommitMS       float64 `json:"p99_commit_ms"`
	WriteTransactions int     `json:"write_transactions"`
	P50WriteWaitMS    float64 `json:"p50_write_wait_ms"`
	P95WriteWaitMS    float64 `json:"p95_write_wait_ms"`
	P99WriteWaitMS    float64 `json:"p99_write_wait_ms"`
	P50WriteSQLMS     float64 `json:"p50_write_sql_ms"`
	P95WriteSQLMS     float64 `json:"p95_write_sql_ms"`
	P99WriteSQLMS     float64 `json:"p99_write_sql_ms"`
	P50WriteCommitMS  float64 `json:"p50_write_commit_ms"`
	P95WriteCommitMS  float64 `json:"p95_write_commit_ms"`
	P99WriteCommitMS  float64 `json:"p99_write_commit_ms"`
	DatabaseWaitCount int64   `json:"database_wait_count"`
	DatabaseWaitMS    float64 `json:"database_wait_ms"`
	PeakWALMiB        float64 `json:"peak_wal_mib"`
	SQLiteJournalMode string  `json:"sqlite_journal_mode"`
	SQLiteSynchronous string  `json:"sqlite_synchronous"`
	BattlesPerSecond  float64 `json:"battles_per_second"`
	GOMAXPROCS        int     `json:"gomaxprocs"`
	CPUSeconds        float64 `json:"cpu_seconds"`
	CPUPercent        float64 `json:"cpu_percent_of_limit"`
	PeakHeapMiB       float64 `json:"peak_heap_mib"`
	PeakSysMiB        float64 `json:"peak_sys_mib"`
	MemoryLimitMiB    float64 `json:"memory_limit_mib,omitempty"`
}

func main() {
	trainers := flag.Int("trainers", 32, "concurrent Trainers; must be even")
	rounds := flag.Int("rounds", 10, "Battle rounds per Trainer")
	contentDir := flag.String("content", "content", "content pack directory")
	database := flag.String("database", "", "SQLite path; defaults to a temporary database")
	synchronous := flag.String("sqlite-synchronous", "normal", "SQLite WAL synchronization: full or normal")
	flag.Parse()

	path, cleanup, err := databasePath(*database)
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()
	report, err := loadtest.Run(context.Background(), loadtest.Config{
		ContentDir:        *contentDir,
		Database:          path,
		Trainers:          *trainers,
		Rounds:            *rounds,
		SQLiteSynchronous: *synchronous,
	})
	if err != nil {
		log.Fatal(err)
	}
	encoded, err := json.MarshalIndent(toOutput(report), "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	_, _ = fmt.Fprintln(os.Stdout, string(encoded))
	if report.Failures > 0 {
		os.Exit(1)
	}
}

func databasePath(configured string) (string, func(), error) {
	if configured != "" {
		return configured, func() {}, nil
	}
	dir, err := os.MkdirTemp("", "termon-load-*")
	if err != nil {
		return "", nil, fmt.Errorf("create load directory: %w", err)
	}
	return filepath.Join(dir, "termon.db"), func() { _ = os.RemoveAll(dir) }, nil
}

func toOutput(report loadtest.Report) output {
	const mebibyte = 1024 * 1024
	return output{
		Trainers: report.Trainers, Rounds: report.Rounds, Dojos: report.Dojos,
		PeakBattles: report.PeakBattles, CompletedBattles: report.CompletedBattles,
		Failures: report.Failures, FirstError: report.FirstError, Messages: report.Messages,
		SetupMS: report.SetupDuration.Seconds() * 1000, DurationSeconds: report.Duration.Seconds(),
		P95MatchmakingMS:  report.P95Matchmaking.Seconds() * 1000,
		P50CommitMS:       report.P50Commit.Seconds() * 1000,
		P95CommitMS:       report.P95Commit.Seconds() * 1000,
		P99CommitMS:       report.P99Commit.Seconds() * 1000,
		WriteTransactions: report.WriteTransactions,
		P50WriteWaitMS:    report.P50WriteWait.Seconds() * 1000,
		P95WriteWaitMS:    report.P95WriteWait.Seconds() * 1000,
		P99WriteWaitMS:    report.P99WriteWait.Seconds() * 1000,
		P50WriteSQLMS:     report.P50WriteSQL.Seconds() * 1000,
		P95WriteSQLMS:     report.P95WriteSQL.Seconds() * 1000,
		P99WriteSQLMS:     report.P99WriteSQL.Seconds() * 1000,
		P50WriteCommitMS:  report.P50WriteCommit.Seconds() * 1000,
		P95WriteCommitMS:  report.P95WriteCommit.Seconds() * 1000,
		P99WriteCommitMS:  report.P99WriteCommit.Seconds() * 1000,
		DatabaseWaitCount: report.DatabaseWaitCount,
		DatabaseWaitMS:    report.DatabaseWaitDuration.Seconds() * 1000,
		PeakWALMiB:        float64(report.PeakWALBytes) / mebibyte,
		SQLiteJournalMode: report.SQLiteJournalMode,
		SQLiteSynchronous: report.SQLiteSynchronous,
		BattlesPerSecond:  report.BattlesPerSecond,
		GOMAXPROCS:        report.GOMAXPROCS, CPUSeconds: report.CPUSeconds, CPUPercent: report.CPUPercent,
		PeakHeapMiB:    float64(report.PeakHeapBytes) / mebibyte,
		PeakSysMiB:     float64(report.PeakSysBytes) / mebibyte,
		MemoryLimitMiB: float64(report.MemoryLimitBytes) / mebibyte,
	}
}
