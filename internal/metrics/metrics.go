// Package metrics exposes bounded operational metrics for termond.
package metrics

import (
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"termon.sh/internal/game"
	"termon.sh/internal/server"
	"termon.sh/internal/store"
)

const namespace = "termon"

// Metrics owns termond's isolated Prometheus registry and Store instrumentation.
type Metrics struct {
	registry             *prometheus.Registry
	battleResults        *prometheus.CounterVec
	battleResultDuration prometheus.Histogram
	storeFailures        *prometheus.CounterVec
	registrations        *prometheus.CounterVec
	displacements        prometheus.Counter
	challenges           *prometheus.CounterVec
	queueJoins           *prometheus.CounterVec
	queueWait            prometheus.Histogram
	loginWait            prometheus.Histogram
	loginDrops           prometheus.Counter
	telemetryEvents      *prometheus.CounterVec
	runtimeRegistered    bool
}

// New creates an isolated registry with Go, process, and termond collectors.
func New() *Metrics {
	registry := prometheus.NewRegistry()
	m := &Metrics{
		registry: registry,
		battleResults: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "battle_results_total",
			Help:      "Persisted multiplayer Battle Results.",
		}, []string{"reason"}),
		battleResultDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "battle_result_persistence_duration_seconds",
			Help:      "Time spent persisting one multiplayer Battle Result attempt.",
			Buckets:   prometheus.DefBuckets,
		}),
		storeFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "store_failures_total",
			Help:      "Store operation failures.",
		}, []string{"operation"}),
		registrations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "registrations_total",
			Help:      "New-Trainer registration attempts by outcome.",
		}, []string{"outcome"}),
		displacements: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "session_displacements_total",
			Help:      "Connections displaced by a newer session for the same Trainer.",
		}),
		challenges: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "challenges_total",
			Help:      "Challenge flow by outcome.",
		}, []string{"outcome"}),
		queueJoins: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "queue_transitions_total",
			Help:      "Queue membership transitions by outcome.",
		}, []string{"outcome"}),
		queueWait: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "queue_wait_seconds",
			Help:      "Time a Trainer waited in the Queue before being paired.",
			Buckets:   prometheus.ExponentialBuckets(0.5, 2, 8), // 0.5s .. 128s
		}),
		loginWait: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "login_wait_seconds",
			Help:      "Time an SSH session spent waiting at the login gate before admission.",
			Buckets:   prometheus.ExponentialBuckets(0.25, 2, 9), // 0.25s .. 64s
		}),
		loginDrops: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "login_drops_total",
			Help:      "SSH sessions closed for waiting past the login admission budget.",
		}),
		telemetryEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "telemetry_events_total",
			Help:      "Telemetry events by bounded destination and delivery outcome.",
		}, []string{"destination", "outcome"}),
	}
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.battleResults,
		m.battleResultDuration,
		m.storeFailures,
		m.registrations,
		m.displacements,
		m.challenges,
		m.queueJoins,
		m.queueWait,
		m.loginWait,
		m.loginDrops,
		m.telemetryEvents,
	)
	return m
}

// ObserveRegistration records a registration attempt outcome.
func (m *Metrics) ObserveRegistration(outcome string) {
	m.registrations.WithLabelValues(outcome).Inc()
}

// ObserveDisplacement records a seat takeover by a newer connection.
func (m *Metrics) ObserveDisplacement() {
	m.displacements.Inc()
}

// ObserveChallenge records a challenge-flow outcome.
func (m *Metrics) ObserveChallenge(outcome string) {
	m.challenges.WithLabelValues(outcome).Inc()
}

// ObserveQueueJoin records a queue membership transition.
func (m *Metrics) ObserveQueueJoin(outcome string) {
	m.queueJoins.WithLabelValues(outcome).Inc()
}

// ObserveQueueWait records how long one Trainer waited before pairing.
func (m *Metrics) ObserveQueueWait(wait time.Duration) {
	m.queueWait.Observe(wait.Seconds())
}

// ObserveLoginWait records how long one SSH session waited at the login gate.
func (m *Metrics) ObserveLoginWait(wait time.Duration) {
	m.loginWait.Observe(wait.Seconds())
}

// ObserveLoginDrop records an SSH session closed for waiting past the login budget.
func (m *Metrics) ObserveLoginDrop() {
	m.loginDrops.Inc()
}

// ObserveTelemetry records one bounded logging or analytics delivery outcome.
func (m *Metrics) ObserveTelemetry(destination, outcome string) {
	m.telemetryEvents.WithLabelValues(destination, outcome).Inc()
}

// RegisterRuntime adds current Hub and SQLite measurements to the registry.
func (m *Metrics) RegisterRuntime(hub *server.Hub, sqlite *store.SQLiteStore) {
	if m.runtimeRegistered {
		panic("metrics: runtime collectors already registered")
	}
	m.runtimeRegistered = true
	m.registry.MustRegister(runtimeCollector{hub: hub, sqlite: sqlite})
}

// runtimeCollector emits all runtime gauges from ONE hub.Stats() snapshot per
// scrape, so the reported values are mutually consistent and the hub lock is
// taken once instead of once per gauge. Unchecked collector: descriptors are
// created per Collect.
type runtimeCollector struct {
	hub    *server.Hub
	sqlite *store.SQLiteStore
}

func (runtimeCollector) Describe(chan<- *prometheus.Desc) {}

func (c runtimeCollector) Collect(ch chan<- prometheus.Metric) {
	gauge := func(name, help string, v float64) {
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(namespace+"_"+name, help, nil, nil),
			prometheus.GaugeValue, v,
		)
	}
	counter := func(name, help string, v float64) {
		ch <- prometheus.MustNewConstMetric(
			prometheus.NewDesc(namespace+"_"+name, help, nil, nil),
			prometheus.CounterValue, v,
		)
	}
	stats := c.hub.Stats()
	gauge("active_ssh_sessions", "Current authenticated SSH sessions.", float64(stats.ActiveSessions))
	gauge("trainers", "Current Trainers seated across all Dojos.", float64(stats.Trainers))
	gauge("dojos", "Current Dojos.", float64(stats.Dojos))
	gauge("queue_depth", "Current Trainers waiting in the Queue.", float64(stats.Queued))
	gauge("active_battles", "Current Battles, including Practice Battles.", float64(stats.Battles))

	sm := sqliteMetrics(c.sqlite)
	counter("sqlite_wait_total", "SQLite database pool waits.", float64(sm.DatabaseWaitCount))
	counter("sqlite_wait_seconds_total", "Time spent waiting for the SQLite database pool.", sm.DatabaseWaitDuration.Seconds())
	gauge("sqlite_wal_bytes", "Current SQLite write-ahead log size.", float64(sm.WALBytes))
}

func sqliteMetrics(sqlite *store.SQLiteStore) store.SQLiteMetrics {
	current, err := sqlite.Metrics()
	if err != nil {
		return store.SQLiteMetrics{}
	}
	return current
}

// Handler returns the Prometheus scrape handler for this registry.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// WrapStore instruments Store latency, results, and failures.
func (m *Metrics) WrapStore(next store.Store) store.Store {
	return instrumentedStore{next: next, metrics: m}
}

type instrumentedStore struct {
	next    store.Store
	metrics *Metrics
}

func (s instrumentedStore) ResolveCredential(hash string) (*game.Trainer, error) {
	trainer, err := s.next.ResolveCredential(hash)
	s.observeFailure("resolve_credential", err)
	return trainer, err
}

func (s instrumentedStore) CreateTrainer(hash string) (*game.Trainer, error) {
	trainer, err := s.next.CreateTrainer(hash)
	s.observeFailure("create_trainer", err)
	return trainer, err
}

func (s instrumentedStore) LoadTrainer(id string) (*game.Trainer, error) {
	trainer, err := s.next.LoadTrainer(id)
	s.observeFailure("load_trainer", err)
	return trainer, err
}

func (s instrumentedStore) CompleteOnboarding(id string, save *game.Save) error {
	err := s.next.CompleteOnboarding(id, save)
	s.observeFailure("complete_onboarding", err)
	return err
}

func (s instrumentedStore) ResetTrainer(id string) error {
	err := s.next.ResetTrainer(id)
	s.observeFailure("reset_trainer", err)
	return err
}

func (s instrumentedStore) RecordBattleResult(rec store.BattleRecord) (store.ResultRecords, error) {
	started := time.Now()
	records, err := s.next.RecordBattleResult(rec)
	s.metrics.battleResultDuration.Observe(time.Since(started).Seconds())
	s.observeFailure("record_battle_result", err)
	if err == nil && records.Applied {
		s.metrics.battleResults.WithLabelValues(rec.Result.Reason).Inc()
	}
	return records, err
}

func (s instrumentedStore) RecordActivityResult(rec store.ActivityRecord) (*game.Save, error) {
	save, err := s.next.RecordActivityResult(rec)
	s.observeFailure("record_activity_result", err)
	return save, err
}

func (s instrumentedStore) SetParty(trainerID string, party [3]string) (*game.Save, error) {
	save, err := s.next.SetParty(trainerID, party)
	s.observeFailure("set_party", err)
	return save, err
}

func (s instrumentedStore) SetBattleLoadout(trainerID, monsterID string, moves, noticeIDs []string) (*game.Save, error) {
	save, err := s.next.SetBattleLoadout(trainerID, monsterID, moves, noticeIDs)
	s.observeFailure("set_battle_loadout", err)
	return save, err
}

func (s instrumentedStore) AcknowledgeProgressionNotices(trainerID string, noticeIDs []string) (*game.Save, error) {
	save, err := s.next.AcknowledgeProgressionNotices(trainerID, noticeIDs)
	s.observeFailure("acknowledge_progression_notices", err)
	return save, err
}

func (s instrumentedStore) SetNickname(trainerID, monsterID, nickname string) (*game.Save, error) {
	save, err := s.next.SetNickname(trainerID, monsterID, nickname)
	s.observeFailure("set_nickname", err)
	return save, err
}

func (s instrumentedStore) AcceptEvolution(trainerID, monsterID string) (*game.Save, error) {
	save, err := s.next.AcceptEvolution(trainerID, monsterID)
	s.observeFailure("accept_evolution", err)
	return save, err
}

func (s instrumentedStore) ActivityExists(trainerID, naturalKey string) (bool, error) {
	ok, err := s.next.ActivityExists(trainerID, naturalKey)
	s.observeFailure("activity_exists", err)
	return ok, err
}

func (s instrumentedStore) ActivityResult(trainerID, naturalKey string) (*store.ActivityResult, error) {
	res, err := s.next.ActivityResult(trainerID, naturalKey)
	s.observeFailure("activity_result", err)
	return res, err
}

func (s instrumentedStore) StartSession(rec store.SessionRecord) error {
	err := s.next.StartSession(rec)
	s.observeFailure("start_session", err)
	return err
}

func (s instrumentedStore) EndSession(sessionID string, endedAt time.Time, reason string) error {
	err := s.next.EndSession(sessionID, endedAt, reason)
	s.observeFailure("end_session", err)
	return err
}

func (s instrumentedStore) TrainerStats(trainerID string) (store.TrainerStats, error) {
	stats, err := s.next.TrainerStats(trainerID)
	s.observeFailure("trainer_stats", err)
	return stats, err
}

func (s instrumentedStore) WorldStats() (store.WorldStats, error) {
	stats, err := s.next.WorldStats()
	s.observeFailure("world_stats", err)
	return stats, err
}

func (s instrumentedStore) observeFailure(operation string, err error) {
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.metrics.storeFailures.WithLabelValues(operation).Inc()
	}
}
