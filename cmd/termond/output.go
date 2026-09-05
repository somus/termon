package main

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"charm.land/ssh"
	"charm.land/wish/v2"
	"charm.land/wish/v2/bubbletea"
	gossh "golang.org/x/crypto/ssh"

	"termon.sh/internal/metrics"
	"termon.sh/internal/sessionoutput"
)

// withSessionOutput owns output until the Program finishes, not until the SSH
// connection ends: a client can keep a transport alive across shell channels.
func withSessionOutput(handler func(context.Context, ssh.Session, *sessionoutput.Writer) *tea.Program, operational *metrics.Metrics) wish.Middleware {
	return func(next ssh.Handler) ssh.Handler {
		return func(sess ssh.Session) {
			ctx, cancel := context.WithCancel(sess.Context())
			defer cancel()
			// Raw transport closure releases both channel-credit waits and TCP
			// writes. Sending an SSH close packet cannot guarantee the latter.
			conn := sess.Context().Value(ssh.ContextKeyConn).(gossh.Conn)
			output := sessionoutput.New(ctx, sess, func() { _ = conn.Close() },
				sessionoutput.DefaultLimit, sessionoutput.DefaultStall, operational)
			defer output.Finish()
			bubbletea.MiddlewareWithProgramHandler(func(s ssh.Session) *tea.Program {
				return handler(ctx, s, output)
			})(func(s ssh.Session) {
				output.Finish()
				cancel()
				next(s)
			})(sess)
		}
	}
}
