// Package loadtest drives concurrent Trainers through the real Hub and Store.
package loadtest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/game"
	"termon.sh/internal/latency"
	"termon.sh/internal/server"
	"termon.sh/internal/store"
)

// Config defines one reproducible load run.
type Config struct {
	ContentDir        string
	Database          string
	Trainers          int
	Rounds            int
	SQLiteSynchronous string
}

// Report contains correctness, throughput, latency, CPU, and memory results.
type Report struct {
	Trainers             int           `json:"trainers"`
	Rounds               int           `json:"rounds"`
	Dojos                int           `json:"dojos"`
	PeakBattles          int           `json:"peak_battles"`
	CompletedBattles     int           `json:"completed_battles"`
	Failures             int           `json:"failures"`
	FirstError           string        `json:"first_error,omitempty"`
	Messages             uint64        `json:"messages"`
	SetupDuration        time.Duration `json:"setup_duration_ns"`
	Duration             time.Duration `json:"duration_ns"`
	P95Matchmaking       time.Duration `json:"p95_matchmaking_ns"`
	P50Commit            time.Duration `json:"p50_commit_ns"`
	P95Commit            time.Duration `json:"p95_commit_ns"`
	P99Commit            time.Duration `json:"p99_commit_ns"`
	WriteTransactions    int           `json:"write_transactions"`
	P50WriteWait         time.Duration `json:"p50_write_wait_ns"`
	P95WriteWait         time.Duration `json:"p95_write_wait_ns"`
	P99WriteWait         time.Duration `json:"p99_write_wait_ns"`
	P50WriteSQL          time.Duration `json:"p50_write_sql_ns"`
	P95WriteSQL          time.Duration `json:"p95_write_sql_ns"`
	P99WriteSQL          time.Duration `json:"p99_write_sql_ns"`
	P50WriteCommit       time.Duration `json:"p50_write_commit_ns"`
	P95WriteCommit       time.Duration `json:"p95_write_commit_ns"`
	P99WriteCommit       time.Duration `json:"p99_write_commit_ns"`
	DatabaseWaitCount    int64         `json:"database_wait_count"`
	DatabaseWaitDuration time.Duration `json:"database_wait_duration_ns"`
	PeakWALBytes         int64         `json:"peak_wal_bytes"`
	SQLiteJournalMode    string        `json:"sqlite_journal_mode"`
	SQLiteSynchronous    string        `json:"sqlite_synchronous"`
	BattlesPerSecond     float64       `json:"battles_per_second"`
	GOMAXPROCS           int           `json:"gomaxprocs"`
	CPUSeconds           float64       `json:"cpu_seconds"`
	CPUPercent           float64       `json:"cpu_percent"`
	PeakHeapBytes        uint64        `json:"peak_heap_bytes"`
	PeakSysBytes         uint64        `json:"peak_sys_bytes"`
	MemoryLimitBytes     uint64        `json:"memory_limit_bytes,omitempty"`
}

// Run onboards Trainers, globally pairs them, and commits every Battle Result.
func Run(ctx context.Context, cfg Config) (report Report, runErr error) {
	if cfg.Trainers < 2 || cfg.Trainers%2 != 0 {
		return Report{}, errors.New("loadtest: Trainers must be a positive even number")
	}
	if cfg.Rounds < 1 {
		return Report{}, errors.New("loadtest: Rounds must be positive")
	}
	set, err := content.Load(cfg.ContentDir)
	if err != nil {
		return Report{}, fmt.Errorf("loadtest: load content: %w", err)
	}
	synchronous := store.SQLiteSynchronousNormal
	switch strings.ToLower(cfg.SQLiteSynchronous) {
	case "", "normal":
	case "full":
		synchronous = store.SQLiteSynchronousFull
	default:
		return Report{}, errors.New("loadtest: SQLite synchronous must be full or normal")
	}
	saves, err := store.OpenSQLiteStore(cfg.Database, store.SQLiteOptions{Synchronous: synchronous})
	if err != nil {
		return Report{}, fmt.Errorf("loadtest: open Store: %w", err)
	}
	defer func() { _ = saves.Close() }()

	report = Report{
		Trainers: cfg.Trainers, Rounds: cfg.Rounds,
		GOMAXPROCS: runtime.GOMAXPROCS(0), MemoryLimitBytes: memoryLimit(),
	}
	stopMemory := sampleMemory()
	defer func() {
		report.PeakHeapBytes, report.PeakSysBytes = stopMemory()
	}()
	hub := server.NewHub(set, saves, server.Admission{OpenAccess: true, RegistrationsPerIP: -1})
	saves.UseContent(set)
	ids := make([]string, cfg.Trainers)
	detach := make([]func(), cfg.Trainers)
	var messages atomic.Uint64

	setupStarted := time.Now()
	setupErrors := concurrent(cfg.Trainers, func(i int) error {
		trainer, err := hub.Authenticate(fmt.Sprintf("load-credential-%06d", i), "10.0.0.9")
		if err != nil {
			return err
		}
		ids[i] = trainer.ID
		detach[i] = hub.Attach(trainer.ID, func(any) { messages.Add(1) }, func() {})
		if trainer.Save != nil {
			if err := hub.ResetTrainer(trainer.ID); err != nil {
				return err
			}
		}
		_, err = hub.CompleteOnboard(trainer.ID, fmt.Sprintf("load-%06d", i), "rootkit")
		if err != nil {
			return err
		}
		return fillParty(saves, trainer.ID)
	})
	report.SetupDuration = time.Since(setupStarted)
	defer func() {
		for _, drop := range detach {
			if drop != nil {
				drop()
			}
		}
	}()
	if len(setupErrors) > 0 {
		return Report{}, fmt.Errorf("loadtest: setup: %w", setupErrors[0])
	}

	stats := hub.Stats()
	report.Dojos = stats.Dojos
	initialMetrics, err := saves.Metrics()
	if err != nil {
		return report, err
	}
	report.SQLiteJournalMode = initialMetrics.JournalMode
	report.SQLiteSynchronous = initialMetrics.Synchronous
	startCPU := processCPU()
	loadStarted := time.Now()
	var matchmakingLatencies, commitLatencies []time.Duration
	for round := 0; round < cfg.Rounds; round++ {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		var latencyMu sync.Mutex
		errs := concurrent(cfg.Trainers/2, func(i int) error {
			a := ids[i*2]
			b := ids[i*2+1]
			started := time.Now()
			err := hub.StartMatch(a, b)
			latencyMu.Lock()
			matchmakingLatencies = append(matchmakingLatencies, time.Since(started))
			latencyMu.Unlock()
			return err
		})
		report.addFailures(errs)
		stats = hub.Stats()
		if stats.Battles > report.PeakBattles {
			report.PeakBattles = stats.Battles
		}

		battles := make(map[*battle.Battle]server.BattleMsg, cfg.Trainers/2)
		for _, id := range ids {
			msg, ok := hub.Resume(id).(server.BattleMsg)
			if !ok || msg.Battle == nil {
				report.addFailure(fmt.Errorf("trainer %s did not enter a Battle", id))
				continue
			}
			battles[msg.Battle] = msg
		}
		if len(battles) != cfg.Trainers/2 {
			report.addFailure(fmt.Errorf("round %d paired %d Battles, want %d", round+1, len(battles), cfg.Trainers/2))
		}
		matches := make([]server.BattleMsg, 0, len(battles))
		for _, msg := range battles {
			matches = append(matches, msg)
		}
		var commitMu sync.Mutex
		errs = concurrent(len(matches), func(i int) error {
			started := time.Now()
			err := hub.Forfeit(matches[i].You)
			commitMu.Lock()
			commitLatencies = append(commitLatencies, time.Since(started))
			commitMu.Unlock()
			return err
		})
		report.addFailures(errs)
		report.CompletedBattles += len(matches) - len(errs)
		if remaining := hub.Stats().Battles; remaining != 0 {
			report.addFailure(fmt.Errorf("round %d left %d Battles active", round+1, remaining))
		}
		metrics, metricsErr := saves.Metrics()
		if metricsErr != nil {
			return report, metricsErr
		}
		if metrics.WALBytes > report.PeakWALBytes {
			report.PeakWALBytes = metrics.WALBytes
		}
	}
	report.Duration = time.Since(loadStarted)
	report.Messages = messages.Load()
	report.P95Matchmaking = latency.Percentile(matchmakingLatencies, 0.95)
	report.P50Commit = latency.Percentile(commitLatencies, 0.50)
	report.P95Commit = latency.Percentile(commitLatencies, 0.95)
	report.P99Commit = latency.Percentile(commitLatencies, 0.99)
	writeMetrics, err := saves.Metrics()
	if err != nil {
		return report, err
	}
	report.WriteTransactions = writeMetrics.WriteTransactions
	report.P50WriteWait = writeMetrics.WriteWait.P50
	report.P95WriteWait = writeMetrics.WriteWait.P95
	report.P99WriteWait = writeMetrics.WriteWait.P99
	report.P50WriteSQL = writeMetrics.WriteSQL.P50
	report.P95WriteSQL = writeMetrics.WriteSQL.P95
	report.P99WriteSQL = writeMetrics.WriteSQL.P99
	report.P50WriteCommit = writeMetrics.WriteCommit.P50
	report.P95WriteCommit = writeMetrics.WriteCommit.P95
	report.P99WriteCommit = writeMetrics.WriteCommit.P99
	report.DatabaseWaitCount = writeMetrics.DatabaseWaitCount - initialMetrics.DatabaseWaitCount
	report.DatabaseWaitDuration = writeMetrics.DatabaseWaitDuration - initialMetrics.DatabaseWaitDuration
	if report.Duration > 0 {
		report.BattlesPerSecond = float64(report.CompletedBattles) / report.Duration.Seconds()
	}
	report.CPUSeconds = (processCPU() - startCPU).Seconds()
	if report.Duration > 0 && report.GOMAXPROCS > 0 {
		report.CPUPercent = report.CPUSeconds / report.Duration.Seconds() / float64(report.GOMAXPROCS) * 100
	}

	wins, losses := 0, 0
	for _, id := range ids {
		save, err := hub.Load(id)
		if err != nil {
			report.addFailure(err)
			continue
		}
		wins += save.Wins
		losses += save.Losses
	}
	if wins != report.CompletedBattles || losses != report.CompletedBattles {
		report.addFailure(fmt.Errorf("durable records are wins=%d losses=%d, want %d each", wins, losses, report.CompletedBattles))
	}
	return report, nil
}

func concurrent(count int, operation func(int) error) []error {
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := range count {
		wg.Go(func() {
			if err := operation(i); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	result := make([]error, 0, len(errs))
	for err := range errs {
		result = append(result, err)
	}
	return result
}

func (r *Report) addFailures(errs []error) {
	for _, err := range errs {
		r.addFailure(err)
	}
}

func (r *Report) addFailure(err error) {
	r.Failures++
	if r.FirstError == "" {
		r.FirstError = err.Error()
	}
}

func processCPU() time.Duration {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return 0
	}
	return timeval(usage.Utime) + timeval(usage.Stime)
}

func timeval(value syscall.Timeval) time.Duration {
	return time.Duration(value.Sec)*time.Second + time.Duration(value.Usec)*time.Microsecond
}

func sampleMemory() func() (uint64, uint64) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	var peakHeap, peakSys atomic.Uint64
	wg.Go(func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			peakHeap.Store(max(peakHeap.Load(), stats.HeapAlloc))
			peakSys.Store(max(peakSys.Load(), stats.Sys))
			select {
			case <-done:
				return
			case <-ticker.C:
			}
		}
	})
	return func() (uint64, uint64) {
		close(done)
		wg.Wait()
		return peakHeap.Load(), peakSys.Load()
	}
}

func memoryLimit() uint64 {
	for _, path := range []string{"/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory/memory.limit_in_bytes"} {
		raw, err := os.ReadFile(path) //nolint:gosec // load-test flag path, dev tool only
		if err != nil {
			continue
		}
		value := strings.TrimSpace(string(raw)) //nolint:gosec // load-test flag path, dev tool only
		if value == "max" {
			return 0
		}
		limit, err := strconv.ParseUint(value, 10, 64)
		if err == nil {
			return limit
		}
	}
	return 0
}

func fillParty(saves *store.SQLiteStore, trainerID string) error {
	for _, species := range []string{"emberbyte", "aquabit"} {
		tr, err := saves.LoadTrainer(trainerID)
		if err != nil {
			return err
		}
		if fullParty(tr.Save) {
			return nil
		}
		_, err = saves.RecordActivityResult(store.ActivityRecord{
			Kind: "lesson", NaturalKey: trainerID + ":load-fill:" + species,
			TrainerID: trainerID, Outcome: "captured",
			Capture:     &store.CaptureSpec{Species: species, FillParty: true},
			CompletedAt: time.Now(),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func fullParty(sv *game.Save) bool {
	if sv == nil {
		return false
	}
	for _, id := range sv.Party {
		if id == "" {
			return false
		}
	}
	return len(sv.Collection) >= 3
}
