// Package telemetry records privacy-bounded product events without coupling
// gameplay correctness to an external analytics provider.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	posthog "github.com/posthog/posthog-go"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/attribute"
	otlploghttp "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

// Published product-event names. Keep names stable once production data exists.
const (
	EventTrainerRegistered   = "trainer:registration_create"
	EventSessionStarted      = "session:ssh_start"
	EventSessionEnded        = "session:ssh_end"
	EventOnboardingCompleted = "onboarding:save_complete"
	EventQueueJoined         = "queue:entry_join"
	EventQueueCancelled      = "queue:entry_cancel"
	EventQueuePaired         = "queue:entry_pair"
	EventChallengeIssued     = "challenge:invitation_create"
	EventChallengeEnded      = "challenge:invitation_end"
	EventBattleStarted       = "battle:match_start"
	EventBattleEnded         = "battle:match_end"
	EventActivityEnded       = "activity:attempt_end"
	EventExpeditionStarted   = "expedition:run_start"
	EventWorkbenchChanged    = "workbench:change_save"
	EventStatsViewed         = "profile:stats_view"
)

var eventNames = map[string]struct{}{
	EventTrainerRegistered: {}, EventSessionStarted: {}, EventSessionEnded: {},
	EventOnboardingCompleted: {}, EventQueueJoined: {}, EventQueueCancelled: {},
	EventQueuePaired: {}, EventChallengeIssued: {}, EventChallengeEnded: {}, EventBattleStarted: {},
	EventBattleEnded: {}, EventActivityEnded: {}, EventExpeditionStarted: {},
	EventWorkbenchChanged: {}, EventStatsViewed: {},
}

var forbiddenProperties = map[string]struct{}{
	"handle": {}, "fingerprint": {}, "fingerprint_hash": {}, "ip": {},
	"source": {}, "source_ip": {}, "terminal_input": {}, "terminal_output": {},
	"save": {}, "save_payload": {}, "opponent_id": {},
}

// Event is one meaningful product transition. Properties must stay bounded and
// must not contain player-authored text or identifying transport data.
type Event struct {
	ID         string
	Name       string
	Timestamp  time.Time
	TrainerID  string
	SessionID  string
	BattleID   string
	ActivityID string
	ErrorID    string
	Properties map[string]any
}

// Recorder accepts events without returning provider failures to gameplay.
type Recorder interface {
	Record(Event)
}

// Observer receives bounded delivery outcomes for Prometheus.
type Observer interface {
	ObserveTelemetry(destination, outcome string)
}

// Config controls the optional PostHog adapter and common event properties.
type Config struct {
	APIKey      string
	Host        string
	Environment string
	AppVersion  string
	LogsEnabled bool
}

// Client fans validated events out to structured logs and optional PostHog.
type Client struct {
	logger      *slog.Logger
	posthog     posthog.Client
	logProvider *sdklog.LoggerProvider
	observer    Observer
	common      map[string]any
}

// New creates a telemetry client. An empty API key leaves PostHog disabled.
func New(logger *slog.Logger, cfg Config, observer Observer) (*Client, error) {
	c := &Client{
		logger: logger, observer: observer,
		common: map[string]any{"environment": cfg.Environment, "app_version": cfg.AppVersion},
	}
	if cfg.APIKey == "" {
		return c, nil
	}
	if cfg.Host == "" {
		return nil, errors.New("telemetry: POSTHOG_HOST is required when POSTHOG_API_KEY is set")
	}
	ph, err := posthog.NewWithConfig(cfg.APIKey, posthog.Config{
		Endpoint: cfg.Host,
		DefaultEventProperties: posthog.Properties{
			"environment": cfg.Environment,
			"app_version": cfg.AppVersion,
		},
		Callback:        callback{observer: observer, logger: logger},
		Logger:          posthogLogger{logger: logger},
		ShutdownTimeout: 8 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("telemetry: create PostHog client: %w", err)
	}
	c.posthog = ph
	if cfg.LogsEnabled {
		if err := c.enableLogs(context.Background(), cfg); err != nil {
			// Operational telemetry must never prevent the SSH game from starting.
			if logger != nil {
				logger.Warn("PostHog Logs disabled after setup failure", "err", err)
			}
			c.observe("posthog_logs", "setup_failed")
		}
	}
	return c, nil
}

// WrapLogger preserves the configured local handler, mirrors privacy-filtered
// records to PostHog Logs when enabled, and captures only Error-level records
// carrying an opaque trainer_id in PostHog Error Tracking.
func (c *Client) WrapLogger(base *slog.Logger) *slog.Logger {
	if c == nil || base == nil || c.posthog == nil {
		return base
	}
	local := posthog.NewSlogCaptureHandler(base.Handler(), c.posthog,
		posthog.WithMinCaptureLevel(slog.LevelError),
		posthog.WithDistinctIDFn(func(_ context.Context, record slog.Record) string {
			return recordString(record, "trainer_id")
		}),
		posthog.WithFingerprintFn(func(_ context.Context, record slog.Record) *string {
			operation := recordString(record, "operation")
			if operation == "" {
				return nil
			}
			fingerprint := record.Message + ":" + operation
			return &fingerprint
		}),
		posthog.WithDescriptionExtractor(safeErrorDescription{}),
		posthog.WithPropertiesFn(c.errorProperties),
	)
	if c.logProvider == nil {
		return slog.New(local)
	}
	remote := filteringHandler{next: otelslog.NewHandler("termond",
		otelslog.WithLoggerProvider(c.logProvider))}
	return slog.New(fanoutHandler{handlers: []slog.Handler{local, observedHandler{
		next: remote, observer: c.observer,
	}}})
}

func (c *Client) enableLogs(ctx context.Context, cfg Config) error {
	exporter, err := otlploghttp.New(ctx,
		otlploghttp.WithEndpointURL(strings.TrimRight(cfg.Host, "/")+"/i/v1/logs"),
		otlploghttp.WithHeaders(map[string]string{"Authorization": "Bearer " + cfg.APIKey}),
	)
	if err != nil {
		return fmt.Errorf("create OTLP log exporter: %w", err)
	}
	res, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("service.name", "termond"),
		attribute.String("deployment.environment.name", cfg.Environment),
		attribute.String("service.version", cfg.AppVersion),
	))
	if err != nil {
		_ = exporter.Shutdown(ctx)
		return fmt.Errorf("create log resource: %w", err)
	}
	observed := observedExporter{Exporter: exporter, observer: c.observer}
	c.logProvider = sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(observed,
			sdklog.WithMaxQueueSize(2048),
			sdklog.WithExportMaxBatchSize(256),
			sdklog.WithExportTimeout(5*time.Second),
		)),
	)
	return nil
}

// Record validates, logs, and then enqueues one event. Invalid or full-queue
// events are dropped and surfaced through logs and bounded metrics.
func (c *Client) Record(event Event) {
	if c == nil {
		return
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.ID == "" {
		event.ID = NewID()
	}
	if err := validate(event); err != nil {
		c.observe("telemetry", "invalid")
		if c.logger != nil {
			c.logger.Error("telemetry event rejected", "event", event.Name, "err", err)
		}
		return
	}
	attrs := eventAttrs(event, c.common)
	if c.logger != nil {
		c.logger.LogAttrs(context.Background(), slog.LevelInfo, "domain event", attrs...)
	}
	c.observe("log", "recorded")
	if c.posthog == nil {
		return
	}
	properties := make(posthog.Properties, len(event.Properties)+4)
	maps.Copy(properties, event.Properties)
	if event.SessionID != "" {
		properties["$session_id"] = event.SessionID
	}
	if event.BattleID != "" {
		properties["battle_id"] = event.BattleID
	}
	if event.ActivityID != "" {
		properties["activity_id"] = event.ActivityID
	}
	if event.ErrorID != "" {
		properties["error_id"] = event.ErrorID
	}
	if err := c.posthog.Enqueue(posthog.Capture{
		Uuid: event.ID, DistinctId: event.TrainerID, Event: event.Name,
		Timestamp: event.Timestamp, Properties: properties,
	}); err != nil {
		c.observe("posthog", "enqueue_failed")
		if c.logger != nil {
			c.logger.Warn("PostHog event enqueue failed", "event", event.Name, "err", err)
		}
		return
	}
	c.observe("posthog", "enqueued")
}

// Close flushes queued logs and PostHog events until ctx expires.
func (c *Client) Close(ctx context.Context) error {
	if c == nil {
		return nil
	}
	var logErr, posthogErr error
	if c.logProvider != nil {
		if err := c.logProvider.Shutdown(ctx); err != nil {
			c.observe("posthog_logs", "shutdown_failed")
			logErr = fmt.Errorf("telemetry: close PostHog Logs: %w", err)
		}
	}
	if c.posthog != nil {
		if err := c.posthog.CloseWithContext(ctx); err != nil {
			c.observe("posthog", "shutdown_failed")
			posthogErr = fmt.Errorf("telemetry: close PostHog: %w", err)
		}
	}
	return errors.Join(logErr, posthogErr)
}

// NewID returns a UUIDv7 suitable for sessions and ordinary events.
func NewID() string {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.NewString()
	}
	return id.String()
}

// DeterministicID makes retries of the same committed domain result converge.
func DeterministicID(parts ...string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(strings.Join(parts, "\x00"))).String()
}

func validate(event Event) error {
	if _, ok := eventNames[event.Name]; !ok {
		return fmt.Errorf("unknown event name %q", event.Name)
	}
	if event.TrainerID == "" {
		return errors.New("missing Trainer ID")
	}
	if _, err := uuid.Parse(event.ID); err != nil {
		return fmt.Errorf("invalid event ID: %w", err)
	}
	if event.SessionID != "" {
		if _, err := uuid.Parse(event.SessionID); err != nil {
			return fmt.Errorf("invalid session ID: %w", err)
		}
	}
	for key := range event.Properties {
		if _, forbidden := forbiddenProperties[strings.ToLower(key)]; forbidden {
			return fmt.Errorf("forbidden property %q", key)
		}
	}
	return nil
}

func eventAttrs(event Event, common map[string]any) []slog.Attr {
	attrs := []slog.Attr{
		slog.String("event", event.Name), slog.String("event_id", event.ID),
		slog.String("trainer_id", event.TrainerID),
	}
	for _, pair := range []struct{ key, value string }{
		{"session_id", event.SessionID},
		{"battle_id", event.BattleID},
		{"activity_id", event.ActivityID},
		{"error_id", event.ErrorID},
	} {
		if pair.value != "" {
			attrs = append(attrs, slog.String(pair.key, pair.value))
		}
	}
	keys := make([]string, 0, len(common)+len(event.Properties))
	all := make(map[string]any, len(common)+len(event.Properties))
	for key, value := range common {
		if value != "" {
			all[key] = value
		}
	}
	maps.Copy(all, event.Properties)
	for key := range all {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		attrs = append(attrs, slog.Any(key, all[key]))
	}
	return attrs
}

func (c *Client) observe(destination, outcome string) {
	if c.observer != nil {
		c.observer.ObserveTelemetry(destination, outcome)
	}
}

type safeErrorDescription struct{}

func (safeErrorDescription) ExtractDescription(record slog.Record) string {
	return record.Message
}

func (c *Client) errorProperties(_ context.Context, record slog.Record) posthog.Properties {
	properties := posthog.Properties{}
	for key, value := range c.common {
		if value != "" {
			properties[key] = value
		}
	}
	for _, key := range []string{"session_id", "battle_id", "activity_id", "error_id", "operation"} {
		if value := recordString(record, key); value != "" {
			properties[key] = value
		}
	}
	return properties
}

func recordString(record slog.Record, key string) string {
	var value string
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == key {
			value = attr.Value.String()
			return false
		}
		return true
	})
	return value
}

type fanoutHandler struct{ handlers []slog.Handler }

func (h fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h fanoutHandler) Handle(ctx context.Context, record slog.Record) error {
	var errs []error
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, record.Level) {
			errs = append(errs, handler.Handle(ctx, record.Clone()))
		}
	}
	return errors.Join(errs...)
}

func (h fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithAttrs(attrs)
	}
	return fanoutHandler{handlers: handlers}
}

func (h fanoutHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithGroup(name)
	}
	return fanoutHandler{handlers: handlers}
}

type filteringHandler struct{ next slog.Handler }

func (h filteringHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h filteringHandler) Handle(ctx context.Context, record slog.Record) error {
	filtered := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		if attr, ok := filterAttr(attr); ok {
			filtered.AddAttrs(attr)
		}
		return true
	})
	return h.next.Handle(ctx, filtered)
}

func (h filteringHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	filtered := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		if attr, ok := filterAttr(attr); ok {
			filtered = append(filtered, attr)
		}
	}
	return filteringHandler{next: h.next.WithAttrs(filtered)}
}

func filterAttr(attr slog.Attr) (slog.Attr, bool) {
	if _, forbidden := forbiddenProperties[strings.ToLower(attr.Key)]; forbidden {
		return slog.Attr{}, false
	}
	attr.Value = attr.Value.Resolve()
	if attr.Value.Kind() != slog.KindGroup {
		return attr, true
	}
	group := attr.Value.Group()
	filtered := make([]slog.Attr, 0, len(group))
	for _, child := range group {
		if child, ok := filterAttr(child); ok {
			filtered = append(filtered, child)
		}
	}
	attr.Value = slog.GroupValue(filtered...)
	return attr, true
}

func (h filteringHandler) WithGroup(name string) slog.Handler {
	return filteringHandler{next: h.next.WithGroup(name)}
}

type observedHandler struct {
	next     slog.Handler
	observer Observer
}

func (h observedHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h observedHandler) Handle(ctx context.Context, record slog.Record) error {
	if h.observer != nil {
		h.observer.ObserveTelemetry("posthog_logs", "recorded")
	}
	return h.next.Handle(ctx, record)
}

func (h observedHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return observedHandler{next: h.next.WithAttrs(attrs), observer: h.observer}
}

func (h observedHandler) WithGroup(name string) slog.Handler {
	return observedHandler{next: h.next.WithGroup(name), observer: h.observer}
}

type observedExporter struct {
	sdklog.Exporter
	observer Observer
}

func (e observedExporter) Export(ctx context.Context, records []sdklog.Record) error {
	err := e.Exporter.Export(ctx, records)
	if e.observer != nil {
		outcome := "delivered"
		if err != nil {
			outcome = "delivery_failed"
		}
		for range records {
			e.observer.ObserveTelemetry("posthog_logs", outcome)
		}
	}
	return err
}

type callback struct {
	observer Observer
	logger   *slog.Logger
}

func (c callback) Success(posthog.APIMessage) {
	if c.observer != nil {
		c.observer.ObserveTelemetry("posthog", "delivered")
	}
}

func (c callback) Failure(_ posthog.APIMessage, err error) {
	if c.observer != nil {
		c.observer.ObserveTelemetry("posthog", "delivery_failed")
	}
	if c.logger != nil {
		c.logger.Warn("PostHog event delivery failed", "err", err)
	}
}

type posthogLogger struct{ logger *slog.Logger }

func (l posthogLogger) Debugf(format string, args ...any) {
	l.log(slog.LevelDebug, format, args...)
}

func (l posthogLogger) Logf(format string, args ...any) {
	l.log(slog.LevelInfo, format, args...)
}

func (l posthogLogger) Warnf(format string, args ...any) {
	l.log(slog.LevelWarn, format, args...)
}

func (l posthogLogger) Errorf(format string, args ...any) {
	l.log(slog.LevelError, format, args...)
}

func (l posthogLogger) log(level slog.Level, format string, args ...any) {
	if l.logger != nil {
		l.logger.Log(context.Background(), level, fmt.Sprintf(format, args...), "module", "posthog")
	}
}
