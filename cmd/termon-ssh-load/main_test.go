package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"charm.land/ssh"
	"charm.land/wish/v2"
)

func TestCommandReportsRenderedSSHSessions(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", ".",
		"-address", listener.Addr().String(),
		"-trainers", "2",
		"-hold", "20ms",
	)
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatalf("decode command output: %v\n%s", err, output)
	}
	if report["connected"] != float64(2) || report["peak_sessions"] != float64(2) || report["failures"] != float64(0) {
		t.Fatalf("command report = %s, want 2 concurrent sessions and no failures", output)
	}
}
