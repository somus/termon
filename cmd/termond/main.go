// Command termond is the termon.sh game server: a single Go binary serving
// the game over SSH (Charmbracelet Wish) with Bubble Tea TUIs per session.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/ssh"
	"charm.land/wish/v2"
	"charm.land/wish/v2/activeterm"
	"charm.land/wish/v2/bubbletea"
	"charm.land/wish/v2/logging"
	wishratelimiter "charm.land/wish/v2/ratelimiter"
	wishrecover "charm.land/wish/v2/recover"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/time/rate"

	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
	"termon.sh/internal/identity"
	"termon.sh/internal/metrics"
	"termon.sh/internal/server"
	"termon.sh/internal/sessionoutput"
	"termon.sh/internal/store"
	"termon.sh/internal/telemetry"
	"termon.sh/internal/tui"
	"termon.sh/internal/website"
)

var appVersion = "dev"

const (
	defaultSSHListen = "127.0.0.1:22"
	sessionRateLimit = rate.Limit(1)
	sessionRateBurst = 5
	sessionRateIPs   = 4096
	idleTimeout      = time.Hour
	maxSessionAge    = 24 * time.Hour
)

type config struct {
	contentDir         string
	database           string
	listen             string
	hostKey            string
	metricsListen      string
	websiteListen      string
	openAccess         bool
	exemptLoopback     bool
	proxyProtocol      bool
	sqliteSync         string
	logLevel           string
	logFormat          string
	registrationsPerIP int
	registrationWindow time.Duration
	loginRate          float64
	loginBurst         int
	loginMaxWait       time.Duration
	posthogAPIKey      string
	posthogHost        string
	posthogLogs        bool
	environment        string
}

// newLogger builds the process logger from CLI flags: level via
// slog.Level text parsing, format as text (default) or JSON, always
// writing to standard error.
func newLogger(level, format string) (*slog.Logger, error) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("invalid -log-level %q: %w", level, err)
	}
	opts := &slog.HandlerOptions{Level: lvl}
	switch format {
	case "text":
		return slog.New(slog.NewTextHandler(os.Stderr, opts)), nil
	case "json":
		return slog.New(slog.NewJSONHandler(os.Stderr, opts)), nil
	default:
		return nil, fmt.Errorf("invalid -log-format %q: want \"text\" or \"json\"", format)
	}
}

func main() {
	contentDir := flag.String("content", "content", "content pack directory")
	database := flag.String("database", "data/termon.db", "SQLite database path")
	listen := flag.String("listen", defaultSSHListen, "SSH listen address")
	hostKey := flag.String("host-key", ".ssh/termond_ed25519", "SSH host key path")
	metricsListen := flag.String("metrics-listen", "127.0.0.1:9090", "loopback Prometheus listen address")
	websiteListen := flag.String("website-listen", "", "public landing page listen address (empty disables it)")
	openAccess := flag.Bool("open-access", true, "let unknown SSH keys register a new Trainer")
	exemptLoopback := flag.Bool("exempt-loopback-rate-limit", false, "skip rate limits for loopback clients (local dev and load-test tooling only)")
	proxyProtocol := flag.Bool("proxy-protocol", false, "expect HAProxy/nginx PROXY v1/v2 headers on accepted connections and trust the client address they carry (enable when a local fronting proxy speaks the PROXY protocol)")
	proxyListenerIsolated := flag.Bool("proxy-listener-isolated", false, "declare that network policy restricts the listener to the trusted PROXY-protocol sender")
	topologyOverride := flag.Bool("unsafe-topology-override", false, `allow the otherwise-refused combination of -proxy-protocol and -exempt-loopback-rate-limit; any process that can reach termond can then forge an exempt, unattributed source`)
	sqliteSync := flag.String("sqlite-sync", "normal", `SQLite synchronous mode: "normal" (default; production latency profile, see docs/load-baseline.md) or "full" (host-crash durable; roughly doubles commit p95/p99)`)
	logLevel := flag.String("log-level", "info", "operational log verbosity: debug, info, warn, or error")
	logFormat := flag.String("log-format", "text", `operational log output format: "text" (default) or "json"`)
	registrationsPerIP := flag.Int("registrations-per-ip", 0, "new Trainers a source IP may create per registration window: 0 uses the package default (10 per hour), negative means unlimited (load-test harnesses)")
	registrationWindow := flag.Duration("registration-window", 0, "registration quota window per source IP; 0 uses the package default (1h)")
	loginRate := flag.Float64("login-rate", defaultLoginRatePerSecond, "SSH session starts admitted per second globally (login smoothing)")
	loginBurst := flag.Int("login-burst", defaultLoginBurst, "SSH session starts admitted instantly before login smoothing delays kick in")
	loginMaxWait := flag.Duration("login-max-wait", defaultLoginMaxWait, "how long an SSH session may wait for login admission before being closed")
	profileContention := flag.Bool("profile-contention", false, "record every mutex and blocking event for loopback pprof (diagnostic overhead; default off)")
	healthcheckURL := flag.String("healthcheck-url", "", "probe a running termond readiness endpoint and exit")
	flag.Parse()

	if *healthcheckURL != "" {
		if err := probeReadiness(*healthcheckURL); err != nil {
			log.Fatal(err)
		}
		return
	}

	if *profileContention {
		runtime.SetMutexProfileFraction(1)
		runtime.SetBlockProfileRate(1)
	}

	logger, err := newLogger(*logLevel, *logFormat)
	if err != nil {
		_, _ = fmt.Fprintln(flag.CommandLine.Output(), err)
		flag.Usage()
		os.Exit(2)
	}

	if err := validateTopology(*proxyProtocol, *exemptLoopback, *topologyOverride); err != nil {
		_, _ = fmt.Fprintln(flag.CommandLine.Output(), err)
		flag.Usage()
		os.Exit(2)
	}
	logTopologyAdvisory(logger, *listen, *proxyProtocol, *proxyListenerIsolated, *exemptLoopback)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, config{
		contentDir:     *contentDir,
		database:       *database,
		listen:         *listen,
		hostKey:        *hostKey,
		metricsListen:  *metricsListen,
		websiteListen:  *websiteListen,
		openAccess:     *openAccess,
		exemptLoopback: *exemptLoopback,
		proxyProtocol:  *proxyProtocol,

		sqliteSync: *sqliteSync,
		logLevel:   *logLevel,
		logFormat:  *logFormat,

		registrationsPerIP: *registrationsPerIP,
		registrationWindow: *registrationWindow,
		loginRate:          *loginRate,
		loginBurst:         *loginBurst,
		loginMaxWait:       *loginMaxWait,
		posthogAPIKey:      os.Getenv("POSTHOG_API_KEY"),
		posthogHost:        os.Getenv("POSTHOG_HOST"),
		posthogLogs:        envEnabled("POSTHOG_LOGS_ENABLED"),
		environment:        os.Getenv("TERMON_ENVIRONMENT"),
	}); err != nil {
		log.Fatal(err)
	}
}

func envEnabled(name string) bool {
	value := strings.TrimSpace(os.Getenv(name))
	return value == "1" || strings.EqualFold(value, "true")
}

// validateTopology refuses a flag combination that silently weakens source
// identity. Trusting PROXY headers while exempting loopback from rate
// limits means any process that can reach termond directly can forge a
// fresh loopback-exempt identity per connection; the override flag records
// that the operator weighed this deliberately.
func validateTopology(proxyProtocol, exemptLoopback, override bool) error {
	if !proxyProtocol || !exemptLoopback || override {
		return nil
	}
	return errors.New("refusing to start: -proxy-protocol together with -exempt-loopback-rate-limit lets any reachable client forge an unlimited, unattributed source; pass -unsafe-topology-override to accept this")
}

// logTopologyAdvisory explains the source-address topology the flag set
// resolved to, so operators can see at boot which mode their deployment is
// in before players share (or split) rate-limit and quota buckets.
func logTopologyAdvisory(logger *slog.Logger, listen string, proxyProtocol, proxyListenerIsolated, exemptLoopback bool) {
	loopback := isLoopbackListen(listen)
	switch {
	case proxyProtocol && proxyListenerIsolated:
		logger.Info("proxied topology: listener is isolated to the trusted proxy",
			"listen", listen,
			"hint", "keep the listener unpublished and restrict its network to the fronting proxy")
	case proxyProtocol && loopback:
		logger.Info("proxied topology: client addresses come from PROXY protocol headers",
			"listen", listen,
			"hint", "only the fronting proxy should be able to reach this address")
	case proxyProtocol:
		logger.Warn("proxied topology on a public address: every peer's PROXY header is trusted",
			"listen", listen,
			"hint", "bind -listen to loopback behind the fronting proxy")
	case loopback && exemptLoopback:
		logger.Info("direct topology: loopback clients skip rate limits",
			"listen", listen,
			"hint", "local dev and load-test tooling only")
	case loopback:
		logger.Warn("clients behind a local proxy share one rate-limit bucket",
			"listen", listen,
			"hint", "use -proxy-protocol if your proxy speaks the PROXY protocol, or -exempt-loopback-rate-limit for local tooling")
	}
}

func run(ctx context.Context, cfg config) error {
	restrictFileMode()
	logger, err := newLogger(cfg.logLevel, cfg.logFormat)
	if err != nil {
		return err
	}
	set, err := content.Load(cfg.contentDir)
	if err != nil {
		return err
	}
	if err := dojo.ValidateBoot(set); err != nil {
		return err
	}
	saves, err := store.OpenSQLiteStore(cfg.database, store.SQLiteOptions{
		Synchronous: store.SQLiteSynchronous(cfg.sqliteSync),
	})
	if err != nil {
		return err
	}
	saves.UseContent(set)
	defer func() { _ = saves.Close() }()
	operational := metrics.New()
	events, err := telemetry.New(logger, telemetry.Config{
		APIKey: cfg.posthogAPIKey, Host: cfg.posthogHost,
		Environment: cfg.environment, AppVersion: appVersion,
		LogsEnabled: cfg.posthogLogs,
	}, operational)
	if err != nil {
		return err
	}
	logger = events.WrapLogger(logger)
	hub := server.NewHub(set, operational.WrapStore(saves), server.Admission{
		OpenAccess:         cfg.openAccess,
		RegistrationsPerIP: cfg.registrationsPerIP,
		RegistrationWindow: cfg.registrationWindow,
	})
	hub.Instrument(operational, logger)
	hub.RecordEvents(events)
	operational.RegisterRuntime(hub, saves)

	runtimeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	opts := []ssh.Option{
		wish.WithAddress(cfg.listen),
		wish.WithHostKeyPath(cfg.hostKey),
		wish.WithIdleTimeout(idleTimeout),
		wish.WithMaxTimeout(maxSessionAge),
		wish.WithPublicKeyAuth(func(_ ssh.Context, key ssh.PublicKey) bool {
			return acceptableKey(key)
		}),
	}
	if cfg.proxyProtocol {
		// ssh.Server.Serve wraps our listener so each accepted connection's
		// PROXY v1/v2 header is parsed before any wish middleware or
		// hub.Authenticate(..., source) reads its RemoteAddr: sessions then
		// carry the proxied client's address instead of the proxy's.
		opts = append(opts, ssh.EnableProxyProtocol())
	}
	opts = append(opts, wish.WithMiddleware(sessionMiddleware(
		withSessionOutput(func(ctx context.Context, sess ssh.Session, output *sessionoutput.Writer) *tea.Program {
			return handleSession(ctx, sess, set, hub, logger, operational, output)
		}, operational),
		newLoginGate(cfg.loginRate, cfg.loginBurst, cfg.loginMaxWait, cfg.exemptLoopback, operational),
		cfg.exemptLoopback,
	)...),
		modernAlgorithms,
	)
	s, err := wish.NewServer(opts...)
	if err != nil {
		return err
	}

	sshListener, err := net.Listen("tcp", cfg.listen)
	if err != nil {
		return fmt.Errorf("listen for SSH: %w", err)
	}
	metricsListener, err := listenLoopback(cfg.metricsListen)
	if err != nil {
		_ = sshListener.Close()
		return err
	}
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", operational.Handler())
	metricsMux.Handle("/readyz", readinessHandler(saves))
	registerPprof(metricsMux)
	metricsServer := &http.Server{
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	var webServer *http.Server
	var webListener net.Listener
	if cfg.websiteListen != "" {
		handler, err := website.New(s.HostSigners[0].PublicKey(), func() int { return hub.Stats().ActiveSessions })
		if err != nil {
			_ = sshListener.Close()
			_ = metricsListener.Close()
			return fmt.Errorf("prepare website: %w", err)
		}
		webListener, err = net.Listen("tcp", cfg.websiteListen)
		if err != nil {
			_ = sshListener.Close()
			_ = metricsListener.Close()
			return fmt.Errorf("listen for website: %w", err)
		}
		webServer = &http.Server{
			Handler: handler, ReadHeaderTimeout: 5 * time.Second,
			WriteTimeout: 30 * time.Second, IdleTimeout: time.Minute,
		}
		fmt.Printf("termond: website http://%s\n", webListener.Addr())
	}

	fmt.Printf("termond: %d types, %d moves, %d species · ssh %s · metrics %s\n",
		len(set.Types), len(set.Moves), len(set.Species), sshListener.Addr(), metricsListener.Addr())
	errs := make(chan error, 3)
	sshReady := newReadyListener(sshListener)
	metricsReady := newReadyListener(metricsListener)
	var workers sync.WaitGroup
	if webServer != nil {
		workers.Go(func() { errs <- webServer.Serve(webListener) })
	}
	workers.Add(3)
	go func() {
		defer workers.Done()
		tickHub(runtimeCtx, hub, 100*time.Millisecond)
	}()
	go func() {
		defer workers.Done()
		errs <- s.Serve(sshReady)
	}()
	go func() {
		defer workers.Done()
		errs <- metricsServer.Serve(metricsReady)
	}()

	serveErr := waitUntilServing(sshReady.ready, metricsReady.ready, errs)
	if serveErr == nil {
		select {
		case <-ctx.Done():
		case serveErr = <-errs:
		}
	}
	cancel()
	shut, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	sshShutdownErr := s.Shutdown(shut)
	if sshShutdownErr != nil {
		_ = s.Close()
	}
	metricsShutdownErr := metricsServer.Shutdown(shut)
	if metricsShutdownErr != nil {
		_ = metricsServer.Close()
	}
	var webShutdownErr error
	if webServer != nil {
		webShutdownErr = webServer.Shutdown(shut)
		if webShutdownErr != nil {
			_ = webServer.Close()
		}
	}
	workers.Wait()
	telemetryErr := events.Close(shut)
	return errors.Join(
		unexpectedServeError(serveErr),
		sshShutdownErr,
		metricsShutdownErr,
		webShutdownErr,
		telemetryErr,
	)
}

func readinessHandler(saves *store.SQLiteStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		health, err := saves.Readiness(r.Context(), false)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "not_ready", "error": err.Error(), "database": health})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ready", "database": health})
	})
}

func probeReadiness(url string) error {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("readiness probe: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if readErr != nil {
		return fmt.Errorf("readiness probe: read response: %w", readErr)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("readiness probe: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.Status != "ready" {
		return fmt.Errorf("readiness probe: invalid ready response: %s", strings.TrimSpace(string(body)))
	}
	_, err = fmt.Fprintln(os.Stdout, strings.TrimSpace(string(body)))
	return err
}

func sessionMiddleware(program wish.Middleware, gate *loginGate, exemptLoopback bool) []wish.Middleware {
	sessionLimiter := wishratelimiter.NewRateLimiter(
		sessionRateLimit, sessionRateBurst, sessionRateIPs,
	)
	if exemptLoopback {
		// Local dev/load-test tooling only: loopback clients skip limits.
		sessionLimiter = localFriendlyLimiter{sessionLimiter}
	}
	return []wish.Middleware{
		program,
		gate.middleware(),
		activeterm.Middleware(),
		wishratelimiter.Middleware(sessionLimiter),
		logging.Middleware(),
		wishrecover.Middleware(),
	}
}

// modernAlgorithms pins key-exchange and MAC negotiation to SHA-2-era
// algorithms. x/crypto's defaults still include SHA-1 entries
// (diffie-hellman-group14-sha1, hmac-sha1-96) for old-client
// compatibility; nothing connects to termond with those, and offering
// them only widens downgrade surface. See docs/operations.md#ssh-security.
func modernAlgorithms(s *ssh.Server) error {
	s.ServerConfigCallback = func(ssh.Context) *gossh.ServerConfig {
		return &gossh.ServerConfig{
			KeyExchanges: []string{
				gossh.KeyExchangeMLKEM768X25519,
				gossh.KeyExchangeCurve25519,
				gossh.KeyExchangeECDHP256,
				gossh.KeyExchangeECDHP384,
				gossh.KeyExchangeECDHP521,
				gossh.KeyExchangeDH14SHA256,
			},
			MACs: []string{
				gossh.HMACSHA256ETM,
				gossh.HMACSHA512ETM,
				gossh.HMACSHA256,
				gossh.HMACSHA512,
			},
		}
	}
	return nil
}

// localFriendlyLimiter exempts loopback clients so local dev tooling
// (termon-ssh-load, scripted duels) is never throttled.
type localFriendlyLimiter struct {
	inner wishratelimiter.RateLimiter
}

func (l localFriendlyLimiter) Allow(s ssh.Session) error {
	if isLoopbackAddr(s.RemoteAddr()) {
		return nil
	}
	return l.inner.Allow(s)
}

func isLoopbackAddr(addr net.Addr) bool {
	tcp, ok := addr.(*net.TCPAddr)
	return ok && tcp.IP.IsLoopback()
}

// isLoopbackListen reports whether the server binds a loopback address,
// which usually means a local fronting proxy terminates client connections.
func isLoopbackListen(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback() || host == "localhost"
}

func sourceAddr(sess ssh.Session) string {
	host, _, err := net.SplitHostPort(sess.RemoteAddr().String())
	if err != nil {
		return sess.RemoteAddr().String()
	}
	return host
}

// acceptableKey reports whether a presented public key may authenticate.
// Identity is derived from the key fingerprint, so refusing an algorithm
// locks out every Trainer who registered with one — the deny list is kept
// to legacy algorithms considered broken (DSA, wire name "ssh-dss") rather
// than a broad allowlist.
func acceptableKey(key ssh.PublicKey) bool {
	return key != nil && key.Type() != "ssh-dss"
}

type readyListener struct {
	net.Listener
	ready chan struct{}
	once  sync.Once
}

func newReadyListener(listener net.Listener) *readyListener {
	return &readyListener{Listener: listener, ready: make(chan struct{})}
}

func (l *readyListener) Accept() (net.Conn, error) {
	l.once.Do(func() { close(l.ready) })
	return l.Listener.Accept()
}

func waitUntilServing(sshReady, metricsReady <-chan struct{}, errs <-chan error) error {
	for sshReady != nil || metricsReady != nil {
		select {
		case <-sshReady:
			sshReady = nil
		case <-metricsReady:
			metricsReady = nil
		case err := <-errs:
			return err
		}
	}
	return nil
}

// registerPprof exposes net/http/pprof handlers next to /metrics on the
// loopback metrics listener, for sustained-load CPU and heap profiling.
func registerPprof(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
}

func listenLoopback(address string) (net.Listener, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("metrics listen address: %w", err)
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, fmt.Errorf("metrics listen address must be loopback: %s", address)
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen for metrics: %w", err)
	}
	return listener, nil
}

func unexpectedServeError(err error) error {
	if err == nil || errors.Is(err, ssh.ErrServerClosed) || errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func tickHub(ctx context.Context, hub *server.Hub, interval time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			hub.Tick(now)
		}
	}
}

func handleSession(ctx context.Context, sess ssh.Session, set *content.Set, hub *server.Hub, logger *slog.Logger, operational *metrics.Metrics, output *sessionoutput.Writer) *tea.Program {
	opts := append(bubbletea.MakeOptions(sess), tea.WithContext(ctx), tea.WithOutput(output))
	key := sess.PublicKey()
	if key == nil {
		_, _ = io.WriteString(output, "public key authentication required\n")
		return tea.NewProgram(rejectModel{reason: "public key authentication required"}, opts...)
	}
	hash := identity.Hash(key.Marshal())
	trainer, err := hub.Authenticate(hash, sourceAddr(sess))
	if errors.Is(err, server.ErrRegistrationDisabled) {
		_, _ = io.WriteString(output, "registration is closed\n")
		return tea.NewProgram(rejectModel{reason: "registration is closed"}, opts...)
	}
	if errors.Is(err, server.ErrTooManyRegistrations) {
		_, _ = io.WriteString(output, "too many new trainers from your address; try again later\n")
		return tea.NewProgram(rejectModel{reason: "too many new trainers from your address; try again later"}, opts...)
	}
	if err != nil {
		if logger != nil {
			logger.Error("failed to load save", "err", err)
		}
		_, _ = io.WriteString(output, "failed to load save\n")
		return tea.NewProgram(rejectModel{reason: "failed to load save"}, opts...)
	}
	sessionID := telemetry.NewID()
	if err := hub.StartSession(trainer.ID, sessionID, appVersion); err != nil && logger != nil {
		logger.Error("failed to record session start", "trainer_id", trainer.ID, "session_id", sessionID, "err", err)
	}
	m := tui.New(trainer.ID, trainer.Save, set, hub).WithOutputPressure(
		func() bool { return output.Pending() != 0 }, operational.CosmeticFrameSkipped)
	p := tea.NewProgram(m, opts...)
	detach := hub.AttachSession(trainer.ID, sessionID, func(msg any) { p.Send(msg) }, func() { p.Quit() })
	go func() {
		detachWhenDone(ctx, detach)
		output.Wait()
		if err := output.Err(); logger != nil && (errors.Is(err, sessionoutput.ErrStalled) || errors.Is(err, sessionoutput.ErrOverflow)) {
			logger.Warn("SSH output closed", "session_id", sessionID, "err", err)
		}
	}()
	return p
}

func detachWhenDone(ctx context.Context, detach func()) {
	<-ctx.Done()
	detach()
}

type rejectModel struct{ reason string }

func (m rejectModel) Init() tea.Cmd { return tea.Quit }
func (m rejectModel) Update(tea.Msg) (tea.Model, tea.Cmd) {
	return m, tea.Quit
}

func (m rejectModel) View() tea.View {
	if m.reason == "" {
		return tea.NewView("connection rejected")
	}
	return tea.NewView(m.reason)
}
