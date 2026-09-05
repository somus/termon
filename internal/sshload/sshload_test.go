package sshload_test

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"charm.land/ssh"
	"charm.land/wish/v2"
	"go.uber.org/goleak"

	"termon.sh/internal/sshload"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestRunHoldsConcurrentRenderedSessions(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server, err := wish.NewServer(
		wish.WithHostKeyPath(filepath.Join(t.TempDir(), "host-key")),
		wish.WithPublicKeyAuth(func(_ ssh.Context, key ssh.PublicKey) bool { return key != nil }),
		func(server *ssh.Server) error {
			server.Handler = func(session ssh.Session) {
				_, _ = io.WriteString(session, "\x1b[2Jrendered terminal session\r\n")
				<-session.Context().Done()
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Close()
		<-serveDone
	})

	report, err := sshload.Run(context.Background(), sshload.Config{
		Address:        listener.Addr().String(),
		Trainers:       4,
		Hold:           20 * time.Millisecond,
		StartupTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Connected != 4 || report.PeakSessions != 4 || report.Failures != 0 {
		t.Fatalf("report = %+v, want 4 concurrent rendered sessions and no failures", report)
	}
	if report.P95Connect <= 0 || report.RenderedBytes < 4 {
		t.Fatalf("report lacks connection latency or rendered output: %+v", report)
	}
}
