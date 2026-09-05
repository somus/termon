// Package sshload measures concurrent rendered SSH sessions.
package sshload

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"termon.sh/internal/latency"
)

// Config defines one concurrent SSH session run.
type Config struct {
	Address        string
	Trainers       int
	Hold           time.Duration
	StartupTimeout time.Duration
}

// Report summarizes SSH authentication, PTY rendering, and concurrency.
type Report struct {
	Trainers      int
	Connected     int
	PeakSessions  int
	Failures      int
	FirstError    string
	RenderedBytes int64
	Duration      time.Duration
	P50Connect    time.Duration
	P95Connect    time.Duration
	P99Connect    time.Duration
}

type sessionResult struct {
	latency time.Duration
	bytes   int
	err     error
}

// Run opens independent SSH sessions, waits for terminal output, and holds
// every successful session concurrently before disconnecting them.
func Run(ctx context.Context, cfg Config) (Report, error) {
	if cfg.Address == "" {
		return Report{}, errors.New("sshload: address is required")
	}
	if cfg.Trainers < 1 {
		return Report{}, errors.New("sshload: Trainers must be positive")
	}
	if cfg.Hold <= 0 {
		return Report{}, errors.New("sshload: hold duration must be positive")
	}
	if cfg.StartupTimeout <= 0 {
		return Report{}, errors.New("sshload: startup timeout must be positive")
	}

	report := Report{Trainers: cfg.Trainers}
	started := time.Now()
	release := make(chan struct{})
	results := make(chan sessionResult, cfg.Trainers)
	var sessions sync.WaitGroup
	var active atomic.Int64
	var peak atomic.Int64
	for i := range cfg.Trainers {
		sessions.Go(func() {
			connectedAt := time.Now()
			client, session, rendered, err := connect(cfg, i)
			latency := time.Since(connectedAt)
			if err != nil {
				results <- sessionResult{latency: latency, err: err}
				return
			}
			current := active.Add(1)
			for {
				observed := peak.Load()
				if current <= observed || peak.CompareAndSwap(observed, current) {
					break
				}
			}
			results <- sessionResult{latency: latency, bytes: rendered}
			select {
			case <-release:
			case <-ctx.Done():
			}
			active.Add(-1)
			_ = session.Close()
			_ = client.Close()
		})
	}

	latencies := make([]time.Duration, 0, cfg.Trainers)
	for range cfg.Trainers {
		result := <-results
		if result.err != nil {
			report.Failures++
			if report.FirstError == "" {
				report.FirstError = result.err.Error()
			}
			continue
		}
		report.Connected++
		report.RenderedBytes += int64(result.bytes)
		latencies = append(latencies, result.latency)
	}
	report.PeakSessions = int(peak.Load())
	if report.Connected > 0 {
		timer := time.NewTimer(cfg.Hold)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
		}
	}
	close(release)
	sessions.Wait()
	report.Duration = time.Since(started)
	report.P50Connect = latency.Percentile(latencies, 0.50)
	report.P95Connect = latency.Percentile(latencies, 0.95)
	report.P99Connect = latency.Percentile(latencies, 0.99)
	if err := ctx.Err(); err != nil {
		return report, err
	}
	return report, nil
}

func connect(cfg Config, trainer int) (*gossh.Client, *gossh.Session, int, error) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("generate Trainer %d key: %w", trainer, err)
	}
	signer, err := gossh.NewSignerFromKey(privateKey)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("create Trainer %d signer: %w", trainer, err)
	}
	client, err := gossh.Dial("tcp", cfg.Address, &gossh.ClientConfig{
		User:            fmt.Sprintf("load-%06d", trainer),
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(signer)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec // ephemeral benchmark server on loopback
		Timeout:         cfg.StartupTimeout,
	})
	if err != nil {
		return nil, nil, 0, fmt.Errorf("connect Trainer %d: %w", trainer, err)
	}
	session, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		return nil, nil, 0, fmt.Errorf("start Trainer %d session: %w", trainer, err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		closeSession(client, session)
		return nil, nil, 0, fmt.Errorf("open Trainer %d output: %w", trainer, err)
	}
	if err := session.RequestPty("xterm-256color", 40, 120, gossh.TerminalModes{gossh.ECHO: 0}); err != nil {
		closeSession(client, session)
		return nil, nil, 0, fmt.Errorf("request Trainer %d PTY: %w", trainer, err)
	}
	if err := session.Shell(); err != nil {
		closeSession(client, session)
		return nil, nil, 0, fmt.Errorf("open Trainer %d shell: %w", trainer, err)
	}
	type readResult struct {
		data []byte
		err  error
	}
	read := make(chan readResult, 1)
	go func() {
		buffer := make([]byte, 512)
		n, readErr := stdout.Read(buffer)
		read <- readResult{data: buffer[:n], err: readErr}
	}()
	timer := time.NewTimer(cfg.StartupTimeout)
	defer timer.Stop()
	select {
	case result := <-read:
		if result.err != nil && !errors.Is(result.err, io.EOF) {
			closeSession(client, session)
			return nil, nil, 0, fmt.Errorf("read Trainer %d terminal: %w", trainer, result.err)
		}
		if !bytes.Contains(result.data, []byte("\x1b")) {
			closeSession(client, session)
			return nil, nil, 0, fmt.Errorf("trainer %d produced no terminal control output", trainer)
		}
		return client, session, len(result.data), nil
	case <-timer.C:
		closeSession(client, session)
		return nil, nil, 0, fmt.Errorf("trainer %d terminal startup timed out", trainer)
	}
}

func closeSession(client *gossh.Client, session *gossh.Session) {
	_ = session.Close()
	_ = client.Close()
}
