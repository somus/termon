package main

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCommandReportsSQLiteWritePhases(t *testing.T) {
	// go run may compile the load command with the race-enabled parent test
	// process competing for CPU on constrained CI runners.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", ".",
		"-trainers", "2",
		"-rounds", "1",
		"-content", "../../content",
		"-database", filepath.Join(t.TempDir(), "termon.db"),
	)
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatalf("decode command output: %v\n%s", err, output)
	}
	for _, field := range []string{
		"write_transactions",
		"p95_write_wait_ms",
		"p95_write_sql_ms",
		"p95_write_commit_ms",
		"database_wait_ms",
		"peak_wal_mib",
	} {
		if _, ok := report[field]; !ok {
			t.Fatalf("command output lacks %q: %s", field, output)
		}
	}
	if report["sqlite_journal_mode"] != "wal" || report["sqlite_synchronous"] != "normal" {
		t.Fatalf("command SQLite profile = %v/%v, want wal/normal", report["sqlite_journal_mode"], report["sqlite_synchronous"])
	}
}
