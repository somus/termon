package loadtest_test

import (
	"context"
	"path/filepath"
	"testing"

	"go.uber.org/goleak"

	"termon.sh/internal/loadtest"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestRunCompletesEveryBattleAcrossDojos(t *testing.T) {
	report, err := loadtest.Run(context.Background(), loadtest.Config{
		ContentDir: "../../content",
		Database:   filepath.Join(t.TempDir(), "termon.db"),
		Trainers:   34,
		Rounds:     2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Dojos != 2 || report.CompletedBattles != 34 || report.Failures != 0 {
		t.Fatalf("report = %+v, want 2 Dojos, 34 completed Battles, and no failures", report)
	}
	if report.PeakBattles != 17 {
		t.Fatalf("peak Battles = %d, want 17", report.PeakBattles)
	}
	if report.BattlesPerSecond <= 0 || report.P95Commit <= 0 {
		t.Fatalf("report has no throughput or latency: %+v", report)
	}
	if report.WriteTransactions != 34 {
		t.Fatalf("write transactions = %d, want 34", report.WriteTransactions)
	}
	if report.P95WriteWait <= 0 || report.P95WriteSQL <= 0 || report.P95WriteCommit <= 0 {
		t.Fatalf("report has no write-phase latency: %+v", report)
	}
	if report.DatabaseWaitCount == 0 || report.DatabaseWaitDuration <= 0 {
		t.Fatalf("report has no database pool wait: %+v", report)
	}
	if report.PeakWALBytes <= 0 {
		t.Fatalf("report has no WAL growth: %+v", report)
	}
	if report.SQLiteJournalMode != "wal" || report.SQLiteSynchronous != "normal" {
		t.Fatalf("default SQLite profile = %s/%s, want wal/normal", report.SQLiteJournalMode, report.SQLiteSynchronous)
	}
}

func TestRunReportsNormalSQLiteProfile(t *testing.T) {
	report, err := loadtest.Run(context.Background(), loadtest.Config{
		ContentDir:        "../../content",
		Database:          filepath.Join(t.TempDir(), "termon.db"),
		Trainers:          2,
		Rounds:            1,
		SQLiteSynchronous: "normal",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.SQLiteJournalMode != "wal" || report.SQLiteSynchronous != "normal" {
		t.Fatalf("SQLite profile = %s/%s, want wal/normal", report.SQLiteJournalMode, report.SQLiteSynchronous)
	}
}
