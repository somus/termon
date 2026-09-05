package main

import (
	"context"
	"io"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/ssh"
	"charm.land/wish/v2/bubbletea"

	"termon.sh/internal/metrics"
	"termon.sh/internal/sessionoutput"
)

func TestOutputWorkerEndsWithChannelNotSharedTransport(t *testing.T) {
	operational := metrics.New()
	handler := withSessionOutput(func(ctx context.Context, sess ssh.Session, output *sessionoutput.Writer) *tea.Program {
		opts := append(bubbletea.MakeOptions(sess), tea.WithOutput(output), tea.WithContext(ctx))
		return tea.NewProgram(rejectModel{reason: "finished"}, opts...)
	}, operational)(func(ssh.Session) {})
	address := startProxyAwareSSHServer(t, handler)
	client := dialSSH(t, address, []byte("PROXY UNKNOWN\r\n"), newTestSigner(t))
	for range 3 {
		sess, err := client.NewSession()
		if err != nil {
			t.Fatal(err)
		}
		sess.Stdout, sess.Stderr = io.Discard, io.Discard
		if err := sess.RequestPty("xterm-256color", 40, 120, nil); err != nil {
			t.Fatal(err)
		}
		if err := sess.Shell(); err != nil {
			t.Fatal(err)
		}
		if err := sess.Wait(); err != nil {
			t.Fatal(err)
		}
		_ = sess.Close()
	}
}
