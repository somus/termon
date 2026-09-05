// Command termon-ssh-load measures concurrent rendered SSH sessions.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"termon.sh/internal/sshload"
)

type output struct {
	Trainers      int     `json:"trainers"`
	Connected     int     `json:"connected"`
	PeakSessions  int     `json:"peak_sessions"`
	Failures      int     `json:"failures"`
	FirstError    string  `json:"first_error,omitempty"`
	RenderedBytes int64   `json:"rendered_bytes"`
	DurationMS    float64 `json:"duration_ms"`
	P50ConnectMS  float64 `json:"p50_connect_ms"`
	P95ConnectMS  float64 `json:"p95_connect_ms"`
	P99ConnectMS  float64 `json:"p99_connect_ms"`
}

func main() {
	address := flag.String("address", "127.0.0.1:2222", "SSH server address")
	trainers := flag.Int("trainers", 256, "concurrent SSH Trainers")
	hold := flag.Duration("hold", 2*time.Second, "duration to hold every rendered session")
	startupTimeout := flag.Duration("startup-timeout", 15*time.Second, "per-session startup timeout")
	timeout := flag.Duration("timeout", 2*time.Minute, "overall run timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	report, err := sshload.Run(ctx, sshload.Config{
		Address:        *address,
		Trainers:       *trainers,
		Hold:           *hold,
		StartupTimeout: *startupTimeout,
	})
	if err != nil {
		log.Fatal(err)
	}
	result := output{
		Trainers: report.Trainers, Connected: report.Connected,
		PeakSessions: report.PeakSessions, Failures: report.Failures,
		FirstError: report.FirstError, RenderedBytes: report.RenderedBytes,
		DurationMS:   report.Duration.Seconds() * 1000,
		P50ConnectMS: report.P50Connect.Seconds() * 1000,
		P95ConnectMS: report.P95Connect.Seconds() * 1000,
		P99ConnectMS: report.P99Connect.Seconds() * 1000,
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	_, _ = fmt.Fprintln(os.Stdout, string(encoded))
	if report.Failures > 0 {
		os.Exit(1)
	}
}
