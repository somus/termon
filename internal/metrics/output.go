package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type outputMetrics struct {
	pending prometheus.Gauge
	bytes   prometheus.Counter
	writes  prometheus.Histogram
	closed  *prometheus.CounterVec
	skipped prometheus.Counter
}

func (m *Metrics) registerOutput() {
	m.pending = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: namespace, Name: "ssh_output_pending_bytes", Help: "Application-owned output bytes, including active writes, across SSH sessions."})
	m.bytes = prometheus.NewCounter(prometheus.CounterOpts{Namespace: namespace, Name: "ssh_output_bytes_total", Help: "Terminal payload bytes written to SSH channels."})
	m.writes = prometheus.NewHistogram(prometheus.HistogramOpts{Namespace: namespace, Name: "ssh_output_write_seconds", Help: "SSH channel write duration, including flow-control waits.", Buckets: []float64{0.001, 0.005, 0.02, 0.1, 0.25, 1, 5, 10}})
	m.closed = prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: namespace, Name: "ssh_output_closed_total", Help: "Output workers closed by bounded reason."}, []string{"reason"})
	m.skipped = prometheus.NewCounter(prometheus.CounterOpts{Namespace: namespace, Name: "ssh_cosmetic_frames_skipped_total", Help: "Cosmetic render requests suppressed while SSH output was pending."})
	m.registry.MustRegister(m.pending, m.bytes, m.writes, m.closed, m.skipped)
}

// OutputPending accounts for queued and in-flight application bytes.
func (m *Metrics) OutputPending(delta int) { m.pending.Add(float64(delta)) }

// OutputWritten records payload throughput and channel-write latency.
func (m *Metrics) OutputWritten(bytes int, elapsed time.Duration) {
	m.bytes.Add(float64(bytes))
	m.writes.Observe(elapsed.Seconds())
}

// OutputClosed records a worker's bounded terminal reason, without identifiers.
func (m *Metrics) OutputClosed(reason string) { m.closed.WithLabelValues(reason).Inc() }

// CosmeticFrameSkipped records work avoided before terminal diff generation.
func (m *Metrics) CosmeticFrameSkipped() { m.skipped.Inc() }
