# PostHog observability for Termon

PostHog Cloud is a good fit for Termon's product analytics, provided Termon emits explicit server-side domain events. PostHog's browser autocapture and visual Session Replay don't observe a Wish/Bubble Tea SSH terminal, so they can't replace gameplay instrumentation, Prometheus, SQLite, or server logs ([autocapture](https://posthog.com/docs/product-analytics/autocapture), [Session Replay](https://posthog.com/docs/session-replay/start-here)).

The recommended first release uses one process-wide PostHog Go client, opaque Trainer IDs as `distinct_id`, and one UUIDv7 `$session_id` per authenticated SSH connection. It keeps product telemetry best-effort and outside gameplay correctness: SQLite remains authoritative, Prometheus remains operational, and structured logs remain the diagnostic record.

## What PostHog can measure

Termon can build product metrics from manually captured events because PostHog Insights support trends, funnels, retention, paths, stickiness, lifecycle, and SQL, while dashboards collect saved insights and apply shared date, event, person, and cohort filters ([insights](https://posthog.com/docs/product-analytics/insights), [dashboards](https://posthog.com/docs/product-analytics/dashboards)).

- **Registrations:** Count `trainer:registration_create` events, emitted only after `CreateTrainer` succeeds. Keep registration denials in Prometheus because they describe admission pressure rather than a created Trainer.
- **Unique Trainers and DAU:** Aggregate a deliberate activity event such as `session:ssh_start` by unique users with a daily interval. PostHog counts one user once per period even if that Trainer emits the event repeatedly ([trend aggregations](https://posthog.com/docs/product-analytics/trends/aggregations)).
- **WAU and MAU:** Use PostHog's rolling active-user aggregations over the same activity event; WAU looks back 7 days and MAU looks back 30 days for each plotted period ([trend aggregations](https://posthog.com/docs/product-analytics/trends/aggregations)).
- **Sessions:** Emit `session:ssh_start` and `session:ssh_end`, and put the same `$session_id` on every event from that connection. Server SDKs don't create session IDs automatically; a custom ID must be unique to one user and session, be UUIDv7, start no later than the first event, and cover no more than 24 hours ([sessions](https://posthog.com/docs/data/sessions)). A reconnect is a new SSH session even when it reattaches to the same Battle.
- **Onboarding:** Build a sequential funnel from registration through the two required Capture Lessons. Funnels can report conversion, drop-off, and time to convert, and can filter or break down by event and person properties ([funnels](https://posthog.com/docs/product-analytics/funnels)).
- **Retention and cohorts:** Define activation as `onboarding:lesson_complete{lesson_number=2}` and return activity as `session:ssh_start` or a stricter gameplay event. Retention compares a start event with a later return event over hourly, daily, weekly, or monthly periods; cohorts can represent behavioral or property-based groups and filter trends, funnels, retention, paths, and dashboards ([retention](https://posthog.com/docs/product-analytics/retention), [cohorts](https://posthog.com/docs/data/cohorts)).

The canonical Termon lifecycle is SSH Credential authentication, compulsory onboarding, Dojo or Lobby activity, Queue or Challenge matchmaking, and a three-Monster Battle ([matchmaking](../design/matchmaking.md), [onboarding](../design/onboarding-storyboard.md)). Analytics names should describe those domain transitions rather than TUI screens or keystrokes.

## Recommended event catalog

Use fixed lowercase names and fixed property keys. PostHog recommends snake case, present-tense verbs, stable names, and variable values in properties rather than dynamically generated event names ([product analytics best practices](https://posthog.com/docs/product-analytics/best-practices)).

| Event | Emit when | Event properties |
| --- | --- | --- |
| `trainer:registration_create` | `CreateTrainer` succeeds | `registration_access`, `app_version` |
| `session:ssh_start` | Authentication and session attachment succeed | `$session_id`, `is_new_trainer`, `resume_target` |
| `session:ssh_end` | The connection detaches | `$session_id`, `duration_seconds`, `end_reason`, `is_displaced` |
| `onboarding:flow_start` | A Trainer without a Save enters first-run | `$session_id`, `attempt_number` |
| `onboarding:starter_select` | The Trainer confirms a starter | `$session_id`, `starter_species` |
| `onboarding:save_complete` | `CompleteOnboarding` commits the initial Save | `$session_id`, `starter_species` |
| `onboarding:lesson_start` | A required Capture Lesson starts | `$session_id`, `lesson_number`, `party_size`, `attempt_number` |
| `onboarding:lesson_fail` | A Lesson retries | `$session_id`, `lesson_number`, `failure_reason`, `gauge_value` |
| `onboarding:lesson_complete` | `RecordActivityResult` commits the Lesson capture | `$session_id`, `lesson_number`, `turn_count`, `duration_seconds` |
| `queue:entry_join` | The Trainer enters the Queue | `$session_id`, `party_stage_mix` |
| `queue:entry_cancel` | The Trainer leaves before pairing | `$session_id`, `wait_seconds` |
| `queue:entry_pair` | The Queue pairs a Trainer | `$session_id`, `wait_seconds` |
| `challenge:invitation_end` | A Challenge reaches an outcome | `$session_id`, `outcome` (`accepted`, `declined`, `expired`) |
| `battle:match_start` | A multiplayer Battle becomes active | `$session_id`, `battle_id`, `entry_path` (`queue`, `challenge`) |
| `battle:match_end` | `RecordBattleResult` first commits | `$session_id`, `battle_id`, `entry_path`, `result`, `reason`, `turn_count`, `duration_seconds`, `move_count`, `switch_count` |
| `activity:attempt_end` | Sparring or a Daily Challenge ends | `$session_id`, `activity_kind`, `outcome`, `tier`, `turn_count`, `is_first_clear`, `is_mastery` |
| `expedition:run_start` | The Signal Board launches an Expedition | `$session_id`, `target_family`, `server_day` |
| `expedition:encounter_end` | An Activity Result commits for an encounter | `$session_id`, `phase`, `outcome`, `turn_count`, `capture_gauge` |
| `workbench:change_save` | A Party, Battle Loadout, nickname, or Evolution mutation commits | `$session_id`, `change_kind` |

Emit one `battle:match_end` per participating Trainer with that Trainer's `result`; this makes user-level win, completion, and retention analysis direct. Reuse the same `battle_id` only as a correlation property. Don't assume it gives exactly-once ingestion: PostHog's event UUID deduplication is eventual, and its documentation warns that custom UUID handling has caveats ([events and deduplication](https://posthog.com/docs/data/events#event-deduplication)).

Don't emit every Move or TUI keypress by default. The summary counters on `battle:match_end` answer normal product questions with much lower event volume, while the deterministic Balance Run remains the source for combat tuning. Add a sampled `battle:action_select` event only when a specific live-balance question justifies its cost and privacy surface.

Every event should also carry bounded common properties: `schema_version`, `app_version`, `environment`, and `$session_id` when a connection exists. Mark test Trainers with an `is_test_user` person property so dashboards can exclude them. Use `$set_once` on registration for immutable acquisition properties and `$set` only for durable Trainer attributes needed by cohorts; PostHog distinguishes person properties from event-specific properties and uses identified users across sessions and insights ([identifying users](https://posthog.com/docs/product-analytics/identify)).

## Go capture and delivery

Create one `posthog.Client` in `cmd/termond`, capture connection start and end at the SSH session boundary, and inject a narrow analytics interface into the Hub for domain events; the TUI should never call PostHog directly. Enqueue events only after the authoritative transition succeeds. In particular, emit onboarding and activity completion after the Store commit, and emit `battle:match_end` only when `RecordBattleResult` reports that the immutable result was newly applied. The Hub remains Termon's only application coordinator, matching the existing durability contract ([durability](../design/durability.md), [progression persistence](../design/progression-persistence.md)).

The official Go SDK queues calls in memory and flushes batches asynchronously, so capture doesn't need to block a Battle or hold the Hub mutex ([Go library](https://posthog.com/docs/libraries/go)). Its current defaults are a 100-message batch, a 5-second flush interval, a 10,000-message in-memory queue, and four total delivery attempts; a full queue drops the newest event and returns `ErrQueueFull` ([PostHog Go v1.25.1 configuration source](https://github.com/PostHog/posthog-go/blob/a5e539c68b8471a86dd746a4bff64601340f4e9f/config.go#L157-L275)). Retries cover network errors, HTTP 408, 429, and 5xx responses, honor a longer `Retry-After`, and use bounded exponential backoff ([PostHog Go v1.25.1 delivery source](https://github.com/PostHog/posthog-go/blob/a5e539c68b8471a86dd746a4bff64601340f4e9f/posthog.go#L1544-L1700), [backoff source](https://github.com/PostHog/posthog-go/blob/a5e539c68b8471a86dd746a4bff64601340f4e9f/backoff.go#L27-L45)).

Configure a callback and check every `Enqueue` error. Export enqueue drops, upload failures, and shutdown failures as bounded Prometheus counters; don't turn analytics delivery failures into player-facing errors. During shutdown, stop accepting SSH sessions, let Hub workers finish, then call `CloseWithContext` inside the existing shutdown budget. The SDK drains queued messages and in-flight batches until the context expires, after which messages can be lost ([PostHog Go v1.25.1 shutdown source](https://github.com/PostHog/posthog-go/blob/a5e539c68b8471a86dd746a4bff64601340f4e9f/posthog.go#L1416-L1510)).

This is intentionally at-most-best-effort at the process boundary. If product reporting later requires delivery across crashes, add a transactional SQLite analytics outbox and acknowledge rows only from the SDK success callback; don't make PostHog part of a gameplay transaction.

## Historical import

Backfill only facts SQLite already owns: Trainer creation timestamps, immutable Battle Results, and Activity Results. PostHog requires historical events to include the original ISO 8601 timestamp and `distinct_id`; the documented historical pipeline uses the Python SDK or batch API with `historical_migration=true`, and events must be at least 48 hours old ([historical migrations](https://posthog.com/docs/migrate)). Test a bounded sample in a separate project, checkpoint the SQLite cursor, and make the importer resumable before importing production history.

Don't reconstruct sessions, Lesson attempts, Queue waits, Battle turns, or abandoned in-memory Battles from current tables; Termon's durability contract deliberately doesn't retain those facts. Start those measurements when live instrumentation ships rather than inventing historical precision.

## Logs and error tracking

Keep Termon's `slog` output in the deployment's structured log store as the primary diagnostic record. It retains operational context and warnings that shouldn't become product events, while Prometheus supplies alertable rates and latency. PostHog Logs can ingest OpenTelemetry logs and correlate them with analytics, but adopting it would create another billed copy of server logs ([Logs](https://posthog.com/docs/logs/start-here), [Logs pricing](https://posthog.com/docs/logs/pricing)). Revisit it only if jumping from a Trainer's product event to nearby logs is worth that duplication.

Use PostHog Error Tracking selectively for actionable unexpected errors at player-impacting boundaries. The Go SDK can capture an exception directly or wrap `slog`; the wrapper captures warning and higher records by default and associates them with a Trainer only when a distinct-ID function returns one ([Go Error Tracking](https://posthog.com/docs/error-tracking/installation/go)). Don't wrap the existing logger without filtering: expected quota denials, disconnects, and retry warnings would become exception events and duplicate ordinary logs. Error Tracking bills ingested `$exception` events, so reserve it for faults that need grouping, stack traces, assignment, or product-impact analysis ([Error Tracking pricing](https://posthog.com/docs/error-tracking/pricing)).

## Privacy and residency

Use the opaque stable Trainer ID as `distinct_id`; never send a Handle, raw or hashed SSH Credential fingerprint, source IP, terminal dimensions, terminal input/output, free text, or opponent identity. Species, Evolution Family, Move, mode, reason, and bounded counts are acceptable gameplay properties because they describe game content rather than a person's identity. The Go SDK disables GeoIP by default for server-side capture, but source-side omission remains the safer control because PostHog says data that must not reach its servers should be omitted during collection ([PostHog Go v1.25.1 configuration source](https://github.com/PostHog/posthog-go/blob/a5e539c68b8471a86dd746a4bff64601340f4e9f/config.go#L88-L92), [privacy controls](https://posthog.com/docs/privacy)).

Choose the Cloud region before sending data because the ingestion host must match it: `https://us.i.posthog.com` for US Cloud or `https://eu.i.posthog.com` for EU Cloud ([capture API](https://posthog.com/docs/api/capture)). PostHog recommends EU Cloud, hosted in Frankfurt, when robust GDPR residency is required; Cloud also supports pre-storage transformations, discarding stored client IPs, person/event deletion, and access controls ([data storage](https://posthog.com/docs/privacy/data-storage)). Termon still owns consent, disclosure, retention policy, and deletion mapping from Trainer ID to PostHog person.

A Trainer Reset must not mint a new PostHog identity because Termon preserves the Trainer and SSH Credentials. If Termon later adds account deletion, delete or anonymize the corresponding PostHog person and events as part of that workflow; PostHog notes that event deletion is asynchronous ([data storage](https://posthog.com/docs/privacy/data-storage#data-deletion)).

## Pricing, quotas, and hosting choice

PostHog prices Product Analytics by captured event, Logs by ingested GB, and Error Tracking by ingested `$exception` event ([Product Analytics pricing](https://posthog.com/docs/product-analytics/pricing), [Logs pricing](https://posthog.com/docs/logs/pricing), [Error Tracking pricing](https://posthog.com/docs/error-tracking/pricing)). The current Cloud pricing page lists monthly free allowances of 1 million analytics events, 10 GB of logs, and 100,000 exceptions; allowances reset monthly, and billing limits can cap paid usage ([pricing](https://posthog.com/pricing)). The compact catalog above should remain well inside the analytics allowance until Termon has substantial use; per-action capture and duplicated logs are the likely cost multipliers.

Use Cloud, with EU or US residency chosen deliberately. PostHog describes self-hosting as unsupported, continuously shipped without tagged releases or guarantees, limited to free-plan features, and the operator's responsibility for deployment, scaling, backups, and security; its documented starting host is 4 vCPU, 16 GB RAM, and more than 30 GB storage ([self-hosting](https://posthog.com/docs/self-host)). That stack is disproportionate beside Termon's single binary and SQLite deployment unless a hard requirement forbids third-party processing and the team accepts operating PostHog itself.

## System-of-record split

Each store has one job, which prevents analytics convenience from weakening Termon's operational or durability contracts.

| System | Keep here |
| --- | --- |
| **PostHog** | Identified product events, SSH-session correlation, onboarding funnels, unique Trainers and DAU/WAU/MAU, feature adoption, retention, and behavioral cohorts. |
| **Prometheus** | Active SSH sessions, Trainers, Dojos, Queue depth, active Battles, registration/admission outcomes, Queue waits, Store failures and latency, SQLite contention/WAL size, Go/process health, and new PostHog enqueue/upload/drop counters. Never add Trainer, Credential, Dojo, Battle, or session IDs as labels. |
| **SQLite** | Trainer identity, SSH Credentials, Save, Handle and W/L totals, immutable Battle Results, Activity Results, timestamps, and any future analytics outbox. It remains authoritative for registrations, progression, and completed outcomes. |
| **Structured log storage** | Startup/shutdown, security and topology decisions, stack-bearing failures, retry detail, request/session diagnostics, and incident evidence. Apply access control and retention independently of product analytics. |

## Source validation

Every external URL cited above was requested with redirects enabled. The table records the final response rather than assuming a link's path is current.

| Requested URL | Final status | Final redirect target |
| --- | ---: | --- |
| <https://github.com/PostHog/posthog-go/blob/a5e539c68b8471a86dd746a4bff64601340f4e9f/backoff.go#L27-L45> | 200 | — |
| <https://github.com/PostHog/posthog-go/blob/a5e539c68b8471a86dd746a4bff64601340f4e9f/config.go#L88-L92> | 200 | — |
| <https://github.com/PostHog/posthog-go/blob/a5e539c68b8471a86dd746a4bff64601340f4e9f/config.go#L157-L275> | 200 | — |
| <https://github.com/PostHog/posthog-go/blob/a5e539c68b8471a86dd746a4bff64601340f4e9f/posthog.go#L1416-L1510> | 200 | — |
| <https://github.com/PostHog/posthog-go/blob/a5e539c68b8471a86dd746a4bff64601340f4e9f/posthog.go#L1544-L1700> | 200 | — |
| <https://posthog.com/docs/api/capture> | 200 | — |
| <https://posthog.com/docs/data/cohorts> | 200 | — |
| <https://posthog.com/docs/data/events#event-deduplication> | 200 | — |
| <https://posthog.com/docs/data/sessions> | 200 | — |
| <https://posthog.com/docs/error-tracking/installation/go> | 200 | — |
| <https://posthog.com/docs/error-tracking/pricing> | 200 | — |
| <https://posthog.com/docs/libraries/go> | 200 | — |
| <https://posthog.com/docs/logs/pricing> | 200 | — |
| <https://posthog.com/docs/logs/start-here> | 200 | — |
| <https://posthog.com/docs/migrate> | 200 | — |
| <https://posthog.com/docs/privacy> | 200 | — |
| <https://posthog.com/docs/privacy/data-storage> | 200 | — |
| <https://posthog.com/docs/privacy/data-storage#data-deletion> | 200 | — |
| <https://posthog.com/docs/product-analytics/autocapture> | 200 | — |
| <https://posthog.com/docs/product-analytics/best-practices> | 200 | — |
| <https://posthog.com/docs/product-analytics/dashboards> | 200 | — |
| <https://posthog.com/docs/product-analytics/funnels> | 200 | — |
| <https://posthog.com/docs/product-analytics/identify> | 200 | — |
| <https://posthog.com/docs/product-analytics/insights> | 200 | — |
| <https://posthog.com/docs/product-analytics/pricing> | 200 | — |
| <https://posthog.com/docs/product-analytics/retention> | 200 | — |
| <https://posthog.com/docs/product-analytics/trends/aggregations> | 200 | — |
| <https://posthog.com/docs/self-host> | 200 | — |
| <https://posthog.com/docs/session-replay/start-here> | 200 | <https://posthog.com/docs/session-replay> |
| <https://posthog.com/pricing> | 200 | — |

## Uncertainties and decisions to confirm

These points require a launch-time product or operational decision.

- PostHog's public pricing and SDK defaults can change. Pin a reviewed `posthog-go` version in `go.mod`, rerun the pricing calculator before launch, and set a billing limit. The current Go source exposes `HistoricalMigration`, but the migration guide still prescribes Python or the batch API, so use the documented import path until PostHog documents Go for this workflow.
- The repository doesn't currently define analytics consent or an account-deletion product flow. Product and legal owners must decide those policies before production capture.
- The recommended live stream accepts loss during process crashes. Add a SQLite outbox only if analytics completeness becomes a stated contract.
- PostHog sessions model each SSH connection, not a browser visit or a reconnect-spanning play period. If product reporting needs a longer logical play period, add a separate `play_period_id` property rather than violating `$session_id` constraints.
