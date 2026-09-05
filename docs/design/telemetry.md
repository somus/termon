# Telemetry and player statistics

Termon records meaningful domain transitions in structured JSON logs and can send product events, privacy-filtered operational logs, and deliberately selected exceptions to PostHog Cloud. Every remote path is asynchronous and never participates in gameplay transactions. SQLite remains authoritative for player-visible statistics, while Prometheus remains authoritative for operational rates and health.

## Correlation identifiers

Every authenticated connection receives a UUIDv7 Session ID. Structured events use these identifiers consistently:

- `trainer_id` is the stable opaque Trainer ID, PostHog `distinct_id`, and player-visible Support ID.
- `session_id` groups events from one SSH connection.
- `battle_id` and `activity_id` correlate immutable results.
- `error_id` is a short reference shown to a player when an unexpected operation fails.

Handles, SSH Credential fingerprints, source IPs, Save payloads, terminal input and output, and opponent identities never enter PostHog. Trainer IDs are pseudonymous data, so production logs and analytics still require access control and retention.

## Delivery

`internal/telemetry.Client` validates a fixed product-event catalog, emits a local JSON record, and enqueues the event with the PostHog Go SDK when configured. The SDK batches and retries in the background. A full queue, delivery failure, or shutdown timeout increments bounded Prometheus counters and never becomes a player-facing gameplay error.

When PostHog Logs is explicitly enabled, the same client adds an OpenTelemetry batch processor behind the process logger. Local JSON output retains diagnostic fields such as source addresses, while the remote handler removes source addresses, handles, Credential fingerprints, Save data, terminal content, and opponent identity before enqueueing. Its queue is bounded, exports time out, setup failure disables only remote logs, and shutdown shares the telemetry deadline.

Error Tracking receives only `ERROR` records that carry an opaque `trainer_id`. This captures unexpected authenticated operation failures with Session, Battle, Activity, error, operation, environment, and version correlation. The remote exception description uses the fixed log message rather than the underlying error text; full internal errors remain in local diagnostic logs. Expected player guidance and ordinary warnings are excluded. Exception autocapture remains disabled because it targets browser/mobile runtimes and would not define the server's error policy.

Configure PostHog with:

- `POSTHOG_API_KEY`: the project ingestion key; an empty value disables all PostHog delivery.
- `POSTHOG_HOST`: the region-matching ingestion host. It is required when the key is set.
- `POSTHOG_LOGS_ENABLED`: `true` or `1` opts into OTLP log delivery and its PostHog billing; false is the default.
- `TERMON_ENVIRONMENT`: a bounded environment name such as `production` or `development`.

The immutable image build embeds its release version, which is attached to events, logs, and exceptions. Production Compose defaults to US ingestion at `https://us.i.posthog.com`, matching both Termon projects. Development project ID is `386097`; production project ID is `594989`. Never share a project token between them or mix regions within a project.

## Product events

The initial catalog covers registration, SSH sessions, onboarding completion, Queue transitions, Challenge outcomes, multiplayer Battle starts and committed results, solo Activity Results, Expeditions, Workbench changes, and Stats views. Emit durable completion events after the Store commit and use deterministic event UUIDs for retryable Battle and Activity Results.

Do not emit render frames, Hub ticks, raw keypresses, or individual Move selections. Immutable Battle summaries retain bounded Move, switch, damage, faint, duration, participation, and Species facts for later statistics without creating a high-volume analytics stream.

## Player-visible statistics

The Workbench Stats tab reads `Store.TrainerStats` and `Store.WorldStats`; it never queries PostHog. Current SQLite data provides the Battle record, streaks, Collection size, captures, Expedition completions, Dojo clears, Mastery Marks, session count, playtime, registration date, and global totals.

Schema version 4 adds `session_results` and indexes for Trainer Battle and Activity history. Each Battle Result's versioned immutable statistics body preserves facts needed for future views, including favorite Species or Moves, accuracy, critical-hit rate, damage, switch tendencies, average duration, fastest victory, and performance by matchmaking path.

If these aggregate queries become expensive, add a rebuildable `trainer_stat_totals` cache. Battle Results, Activity Results, Session Results, and Saves remain the source of truth.

## Support workflow

Ask the player for their Support ID, any displayed error reference, and an approximate UTC time. Search `error_id` for the exact failure, `session_id` for surrounding activity, and `trainer_id` for cross-session history. The same Trainer ID locates the PostHog person timeline and SQLite Trainer row.

Expected guidance errors don't receive references. Unexpected errors are logged with operation, Trainer, Session, and error identifiers, while the TUI displays only the short reference and generic text.
