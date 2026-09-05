package main

import (
	"bytes"
	context "context"
	"crypto/ed25519"
	crand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/ssh"
	gossh "golang.org/x/crypto/ssh"

	"termon.sh/internal/content"
	"termon.sh/internal/identity"
	"termon.sh/internal/server"
	"termon.sh/internal/store"
)

func TestIsLoopbackAddr(t *testing.T) {
	loopback := net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22022}
	ipv6Loopback := net.TCPAddr{IP: net.ParseIP("::1"), Port: 22022}
	external := net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 51000}

	for name, tc := range map[string]struct {
		addr net.Addr
		want bool
	}{
		"ipv4 loopback": {&loopback, true},
		"ipv6 loopback": {&ipv6Loopback, true},
		"external":      {&external, false},
		"other type":    {nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := isLoopbackAddr(tc.addr); got != tc.want {
				t.Fatalf("isLoopbackAddr = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewLogger(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		level    string
		format   string
		wantErr  string
		wantLvl  slog.Level
		wantJSON bool
	}{
		{name: "defaults are text at info", level: "info", format: "text", wantLvl: slog.LevelInfo},
		{name: "json format", level: "debug", format: "json", wantLvl: slog.LevelDebug, wantJSON: true},
		{name: "level names are case-insensitive", level: "ERROR", format: "text", wantLvl: slog.LevelError},
		{name: "unknown level", level: "chatty", format: "text", wantErr: "-log-level"},
		{name: "empty level", level: "", format: "json", wantErr: "-log-level"},
		{name: "unknown format", level: "warn", format: "csv", wantErr: "-log-format"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger, err := newLogger(test.level, test.format)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("newLogger(%q, %q) error = %v, want it to mention %s",
						test.level, test.format, err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("newLogger(%q, %q): %v", test.level, test.format, err)
			}
			if !logger.Enabled(ctx, test.wantLvl) || logger.Enabled(ctx, test.wantLvl-1) {
				t.Fatalf("logger honors %v only, got different threshold", test.wantLvl)
			}
			if test.wantJSON {
				if _, ok := logger.Handler().(*slog.JSONHandler); !ok {
					t.Fatalf("handler = %T, want *slog.JSONHandler", logger.Handler())
				}
				return
			}
			if _, ok := logger.Handler().(*slog.TextHandler); !ok {
				t.Fatalf("handler = %T, want *slog.TextHandler", logger.Handler())
			}
		})
	}
}

func TestReadinessHandlerAndProbe(t *testing.T) {
	saves, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "termon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = saves.Close() })

	server := httptest.NewServer(readinessHandler(saves))
	t.Cleanup(server.Close)
	if err := probeReadiness(server.URL); err != nil {
		t.Fatalf("probeReadiness: %v", err)
	}

	request, err := http.NewRequest(http.MethodPost, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST readiness status = %d, want %d", response.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestProbeReadinessRejectsUnhealthyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	err := probeReadiness(server.URL)
	if err == nil || !strings.Contains(err.Error(), "database unavailable") {
		t.Fatalf("probeReadiness error = %v, want response body", err)
	}
}

func TestModernAlgorithms(t *testing.T) {
	srv := &ssh.Server{}
	if err := modernAlgorithms(srv); err != nil {
		t.Fatalf("modernAlgorithms: %v", err)
	}
	cfg := srv.ServerConfigCallback(nil)
	if cfg == nil {
		t.Fatal("ServerConfigCallback not set")
	}
	for _, sha1 := range []string{
		gossh.InsecureKeyExchangeDH14SHA1,
		gossh.InsecureKeyExchangeDH1SHA1,
		gossh.InsecureKeyExchangeDHGEXSHA1,
		gossh.InsecureHMACSHA196,
		gossh.HMACSHA1,
	} {
		if slices.Contains(cfg.KeyExchanges, sha1) || slices.Contains(cfg.MACs, sha1) {
			t.Errorf("negotiation lists contain %q", sha1)
		}
	}
}

func TestEnvEnabled(t *testing.T) {
	for _, test := range []struct {
		value string
		want  bool
	}{
		{"", false}, {"false", false}, {"0", false}, {"true", true}, {"TRUE", true}, {"1", true},
	} {
		t.Setenv("TERMON_TEST_ENABLED", test.value)
		if got := envEnabled("TERMON_TEST_ENABLED"); got != test.want {
			t.Errorf("envEnabled(%q) = %v, want %v", test.value, got, test.want)
		}
	}
}

func TestValidateTopology(t *testing.T) {
	tests := []struct {
		name           string
		proxyProtocol  bool
		exemptLoopback bool
		override       bool
		wantErr        bool
	}{
		{name: "direct mode needs nothing"},
		{name: "proxied mode alone is fine", proxyProtocol: true},
		{name: "loopback exemption alone is fine", exemptLoopback: true},
		{name: "conflict refuses to start", proxyProtocol: true, exemptLoopback: true, wantErr: true},
		{name: "override admits the conflict", proxyProtocol: true, exemptLoopback: true, override: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTopology(test.proxyProtocol, test.exemptLoopback, test.override)
			if got := err != nil; got != test.wantErr {
				t.Fatalf("validateTopology error = %v, want error %v", err, test.wantErr)
			}
			if test.wantErr && !strings.Contains(err.Error(), "-unsafe-topology-override") {
				t.Fatalf("error %q does not name the override flag", err)
			}
		})
	}
}

func TestLogTopologyAdvisory(t *testing.T) {
	tests := []struct {
		name           string
		listen         string
		proxyProtocol  bool
		exemptLoopback bool
		wantMsg        string
		wantSilent     bool
	}{
		{
			name:          "proxied on loopback resolves the shared bucket",
			listen:        "127.0.0.1:2222",
			proxyProtocol: true,
			wantMsg:       "proxied topology",
		},
		{
			name:          "proxied on a public address warns about trusted headers",
			listen:        "0.0.0.0:2222",
			proxyProtocol: true,
			wantMsg:       "public address",
		},
		{
			name:           "direct loopback with exemption says so",
			listen:         "localhost:2222",
			exemptLoopback: true,
			wantMsg:        "skip rate limits",
		},
		{
			name:    "direct loopback without proxy support shares a bucket",
			listen:  "127.0.0.1:2222",
			wantMsg: "share one rate-limit bucket",
		},
		{
			name:       "direct public listen stays silent",
			listen:     "203.0.113.5:2222",
			wantSilent: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, nil))
			logTopologyAdvisory(logger, test.listen, test.proxyProtocol, test.exemptLoopback)
			out := buf.String()
			if test.wantSilent {
				if out != "" {
					t.Fatalf("expected silence, got %q", out)
				}
				return
			}
			if !strings.Contains(out, test.wantMsg) {
				t.Fatalf("advisory %q does not mention %q", out, test.wantMsg)
			}
		})
	}
}

// startProxyAwareSSHServer serves a minimal charm.land/ssh server with PROXY
// protocol enabled, mirroring what termond wires through wish when
// -proxy-protocol is set. Each session's recovered source address and key
// hash are handed to handle; the returned addr accepts raw TCP clients.
func startProxyAwareSSHServer(t *testing.T, handle func(sess ssh.Session)) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &ssh.Server{
		Handler:             handle,
		EnableProxyProtocol: true,
		// Production always requires a public key; requiring one here keeps
		// sessions carrying their credential exactly like termond's.
		PublicKeyHandler: func(ssh.Context, ssh.PublicKey) bool { return true },
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(l) }()
	t.Cleanup(func() {
		_ = srv.Close()
		if err := <-done; err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			t.Errorf("ssh server: %v", err)
		}
	})
	return l.Addr().String()
}

// dialSSH connects like a proxied client: header (may be nil) is written
// to the raw TCP stream before the SSH handshake begins.
func dialSSH(t *testing.T, addr string, header []byte, signer gossh.Signer) *gossh.Client {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(header) > 0 {
		if _, err := conn.Write(header); err != nil {
			t.Fatal(err)
		}
	}
	cc, chans, reqs, err := gossh.NewClientConn(conn, addr, &gossh.ClientConfig{
		User:            "trainer",
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(signer)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		t.Fatal(err)
	}
	client := gossh.NewClient(cc, chans, reqs)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func newTestSigner(t *testing.T) gossh.Signer {
	t.Helper()
	_, key, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func runSession(client *gossh.Client) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()
	return session.Run("")
}

// proxyV2Header builds a minimal PROXY v2 header claiming srcIP as source.
func proxyV2Header(srcIP [4]byte, srcPort uint16) []byte {
	const signature = "\x0d\x0a\x0d\x0a\x00\x0d\x0a\x51\x55\x49\x54\x0a" // 12-byte v2 signature
	header := make([]byte, 16+12)                                        // signature + ver/cmd/family/length + address pair
	copy(header, signature)
	header[12] = 0x21                 // version 2, command PROXY
	header[13] = 0x11                 // transport TCP, family IPv4
	header[14], header[15] = 0x00, 12 // address pair length
	copy(header[16:20], srcIP[:])
	copy(header[20:24], []byte{192, 0, 2, 9}) // destination
	binary.BigEndian.PutUint16(header[24:26], srcPort)
	binary.BigEndian.PutUint16(header[26:28], 2222)
	return header
}

func TestProxyProtocolRecoversClientSource(t *testing.T) {
	const (
		v1Source = "203.0.113.10"
		v2Source = "198.51.100.77"
	)
	cases := []struct {
		name   string
		header []byte
		wantIP string
	}{
		{
			name:   "v1 header replaces the peer address",
			header: []byte("PROXY TCP4 " + v1Source + " 198.51.100.2 40000 2222\r\n"),
			wantIP: v1Source,
		},
		{
			name:   "v2 header replaces the peer address",
			header: proxyV2Header([4]byte{198, 51, 100, 77}, 41000),
			wantIP: v2Source,
		},
		{
			name:   "no header falls back to the real peer",
			wantIP: "127.0.0.1",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := make(chan net.Addr, 1)
			addr := startProxyAwareSSHServer(t, func(sess ssh.Session) {
				got <- sess.RemoteAddr()
			})
			client := dialSSH(t, addr, test.header, newTestSigner(t))
			if err := runSession(client); err != nil {
				t.Fatalf("session run: %v", err)
			}
			select {
			case remote := <-got:
				host, _, err := net.SplitHostPort(remote.String())
				if err != nil {
					t.Fatal(err)
				}
				if host != test.wantIP {
					t.Fatalf("recovered source = %s, want %s", host, test.wantIP)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("handler never ran")
			}
		})
	}
}

func TestProxyProtocolGarbageHeaderFailsClosed(t *testing.T) {
	addr := startProxyAwareSSHServer(t, func(ssh.Session) {
		t.Error("session established despite malformed PROXY header")
	})
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	// Signature-bearing but unparseable: the wrapper must refuse the
	// connection rather than guess at a source.
	if _, err := io.WriteString(conn, "PROXY not-a-header\r\n"); err != nil {
		t.Fatal(err)
	}
	signer := newTestSigner(t)
	_, _, _, err = gossh.NewClientConn(conn, addr, &gossh.ClientConfig{
		User:            "trainer",
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(signer)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	})
	if err == nil {
		t.Fatal("SSH handshake succeeded over a malformed PROXY header")
	}
}

func TestProxiedSourcesGetIndependentBuckets(t *testing.T) {
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	// One registration per source per window: a shared bucket would deny
	// the second trainer entirely.
	hub := server.NewHub(set, store.NewMemoryStore(), server.Admission{
		OpenAccess: true, RegistrationsPerIP: 1,
	})

	type outcome struct {
		source string
		err    error
	}
	results := make(chan outcome, 3)
	addr := startProxyAwareSSHServer(t, func(sess ssh.Session) {
		key := sess.PublicKey()
		hash := identity.Hash(key.Marshal())
		_, err := hub.Authenticate(hash, sourceAddr(sess))
		results <- outcome{source: sourceAddr(sess), err: err}
	})

	runAs := func(ip string, port int) outcome {
		client := dialSSH(t, addr,
			[]byte(fmt.Sprintf("PROXY TCP4 %s 198.51.100.2 %d 2222\r\n", ip, port)),
			newTestSigner(t))
		if err := runSession(client); err != nil {
			t.Fatalf("session run for %s: %v", ip, err)
		}
		select {
		case r := <-results:
			return r
		case <-time.After(5 * time.Second):
			t.Fatalf("handler never ran for %s", ip)
			return outcome{}
		}
	}

	first := runAs("203.0.113.10", 40001)
	if first.err != nil || first.source != "203.0.113.10" {
		t.Fatalf("first registration from source A = %+v", first)
	}
	second := runAs("203.0.113.10", 40002)
	if !errors.Is(second.err, server.ErrTooManyRegistrations) {
		t.Fatalf("second registration from source A = %+v, want quota denial", second)
	}
	third := runAs("203.0.113.11", 40003)
	if third.err != nil || third.source != "203.0.113.11" {
		t.Fatalf("first registration from source B = %+v, want its own bucket", third)
	}
}
