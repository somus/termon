# Operate termond

## SSH listener

termond listens on `127.0.0.1:22` by default, which lets local clients connect with `ssh localhost`. Binding port 22 may require elevated network privileges, and a system SSH daemon may already occupy it. For local development, use `-listen 127.0.0.1:2222`; the production container listens as a non-root process on port 2222, and Docker publishes that port as TCP 22 only on the VPS public IPv4 address. Dokploy's Traefik independently owns TCP 443 for the public website, so `ssh termon.sh` and `https://termon.sh` share a hostname without protocol multiplexing.

Set `-listen 0.0.0.0:22` only for a direct public deployment with host firewall controls. Keep the loopback bind when a local proxy accepts public connections.

## SSH security

termond accepts public-key authentication only and derives Trainer identity from the authenticated session key after ownership proof. The server pins SHA-2-era key exchanges and MACs through `modernAlgorithms`; it doesn't mount SCP, SFTP, agent forwarding, or any middleware that reads client-selected file paths. Wish enforces a one-hour idle timeout, and termond closes sessions after 24 hours.

Keep the direct `golang.org/x/crypto` requirement in `go.mod` when updating Wish. Wish also pins that module, so Go's minimum version selection won't necessarily raise a vulnerable transport dependency when Wish changes. CI runs `govulncheck`, but dependency changes still need a manual comparison against current Go SSH advisories.

Players initially trust the generated Ed25519 host key through SSH's trust-on-first-use flow. Back up that key with the database and publish its fingerprint through a trusted channel before opening a public server. For the production container's persisted key, print the SHA-256 fingerprint with:

```sh
ssh-keygen -y -f /path/to/host-key | ssh-keygen -lf -
```

## Connection limits

termond accepts an average of one new SSH session per source IP per second, with a burst of five. The in-process limiter retains the 4,096 most recently seen IPs. This protects session setup and game resources; the host firewall or ingress must still limit TCP handshakes before Wish creates a session.

New-Trainer registration has a separate quota per source IP: 10 creations per one-hour window by default, with at most 4,096 sources tracked at once. When the tracking cap is full and no windows have expired yet, further new sources are denied until entries expire — memory stays bounded at the cost of refusing unseen addresses.

Tune the quota with `-registrations-per-ip` (0 selects the default; a negative value means unlimited, intended for load-test harnesses) and `-registration-window` (0 selects the one-hour default). Each denied attempt increments `termon_registrations_total{outcome="denied_quota"}` on the metrics endpoint.

Wish recovery is the outermost session middleware. A panic in one session is logged with its stack and does not terminate the server process.

On top of the per-source connection limiter, login smoothing admits SSH session starts at one global rate so a cold burst queues briefly instead of stampeding startup: `-login-rate` admissions per second (default 25), with `-login-burst` instant admissions before any delay applies (default 128 — a lone player never waits). A session whose expected wait exceeds `-login-max-wait` (default 45s) is closed instead of seated. Waiting sessions see a "dojo is filling up" notice once their expected wait passes roughly 250 ms. Every gated admission observes `termon_login_wait_seconds`, and each session closed for waiting too long increments `termon_login_drops_total`. Loopback peers bypass the gate when `-exempt-loopback-rate-limit` is set, so local load tooling measures raw capacity.

## Slow SSH clients

Gameplay terminal output uses one bounded writer per session. Pending payload,
including the currently blocked write, cannot exceed 256 KiB. Cosmetic painting
pauses while output is pending; input and authoritative updates still run. A
channel write that fails to complete in 10 seconds, an output-budget overflow,
or a write error closes the underlying SSH transport. This also disconnects any
other channels sharing that transport; independent Trainer connections aren't
closed. Static clients with no pending output still use the normal idle timeout.

A timeout means the server couldn't complete a write, not that it observed the
client's physical display. SSH window credit and socket buffers can hide a slow
reader until they fill. Already-generated ANSI deltas remain ordered; Termon
never drops bytes from a live terminal stream. On disconnection, durable progress
remains in SQLite and the normal active-Battle reconnect policy applies. See the
[output contract](design/render-caching.md#bounded-session-output--term-70).

Watch for sustained pending bytes, write durations above 250 ms, and increases in
`termon_ssh_output_closed_total{reason="stalled"}` or `{reason="overflow"}`.
`SSH output closed` warnings include only the existing opaque Session ID and a
fixed error description, not keypresses, frame contents, or client addresses.
Warnings run after session cleanup, not in the renderer or Hub path.

## Prometheus metrics

termond serves Prometheus metrics on `127.0.0.1:9090` by default. Override the port with `-metrics-listen`, but the address must remain on loopback:

```sh
go run ./cmd/termond -metrics-listen 127.0.0.1:9191
curl http://127.0.0.1:9191/metrics
```

The termond-specific series are:

- `termon_active_ssh_sessions`, `termon_trainers`, `termon_dojos`, `termon_queue_depth`, and `termon_active_battles` report current process-owned game state.
- `termon_battle_results_total{reason}` counts newly persisted multiplayer Battle Results.
- `termon_battle_result_persistence_duration_seconds` measures each persistence attempt, including failed attempts that will be retried.
- `termon_store_failures_total{operation}` counts unexpected Store failures. Expected lookup misses are excluded.
- `termon_sqlite_wait_total`, `termon_sqlite_wait_seconds_total`, and `termon_sqlite_wal_bytes` expose database pool contention and WAL growth.
- `termon_telemetry_events_total{destination,outcome}` counts structured-log and PostHog enqueue, delivery, rejection, and shutdown outcomes.
- `termon_ssh_output_pending_bytes` totals application-owned queued and active-write bytes across sessions; it excludes kernel and SSH peer buffers.
- `termon_ssh_output_bytes_total` and `termon_ssh_output_write_seconds` measure terminal payload throughput and channel-write duration, including flow-control waits.
- `termon_ssh_output_closed_total{reason}` counts output workers ending with `finished`, `canceled`, `stalled`, `overflow`, or `write_error`. A completed shell drains output as `finished`; transport cancellation is `canceled`.
- `termon_ssh_cosmetic_frames_skipped_total` counts cosmetic render requests deferred before ANSI diffing under output pressure.

The registry also exposes the standard Go runtime and process collectors. No metric contains a Trainer, Credential, Dojo, or Battle identifier.

The metrics listener shuts down with the SSH server. Failure to bind either listener prevents startup.

### Local profiling

The loopback metrics listener also serves `/debug/pprof/`. For a diagnostic run,
add `-profile-contention` to record every mutex and blocking event; leave it off
for normal operation because it adds profiling overhead. CPU and heap profiles
are available without that flag. The [latency procedure](load-baseline.md#in-session-latency-and-output-pressure)
includes collection commands. Profiles may contain sensitive process data: don't
publish the listener, upload raw heaps, or store them in the repository.

## Product analytics, logs, and support correlation

Set `POSTHOG_API_KEY`, `POSTHOG_HOST`, and `TERMON_ENVIRONMENT` to enable asynchronous PostHog product events and deliberate Error Tracking. An empty key disables PostHog while retaining local structured logs. The production Compose default uses US ingestion, matching the Termon PostHog organization; don't send a project token to an ingestion host in another region.

Set `POSTHOG_LOGS_ENABLED=true` to additionally batch privacy-filtered structured logs to PostHog's `/i/v1/logs` OTLP endpoint. This is an explicit billing switch and defaults off. Local JSON logs retain source addresses for abuse diagnosis; the remote stream removes them and the other forbidden fields defined in the telemetry contract. Verify delivery in Development before enabling it in Production, and alert on `termon_telemetry_events_total{destination="posthog_logs",outcome="delivery_failed"}`.

Player-visible statistics come from SQLite rather than PostHog. Ask a player reporting a problem for the Support ID shown in the Workbench Stats tab, any displayed error reference, and the approximate UTC time. Search logs by `error_id`, then `session_id` and `trainer_id`; the Trainer ID also locates the PostHog person timeline and SQLite row. See [Telemetry and player statistics](design/telemetry.md) for the data and privacy contract.

## Deployment topology

How termond learns each client's source address depends on two flags; pick one mode per deployment and set its flags deliberately.

**Direct mode** — termond's `-listen` address is reachable by players directly (bound to a public interface). Every TCP peer is its own address, so rate-limit and registration-quota buckets split naturally per client. No topology flags are needed.

**Proxied mode** — a local fronting proxy (HAProxy, nginx with `stream` + PROXY protocol out) terminates player connections and forwards them over loopback. Without help every proxied player shares the proxy's address: one ~5-burst connection bucket and one global 10-registrations-per-hour cap for everyone. Enable `-proxy-protocol` so termond expects a PROXY v1/v2 header on every accepted connection and recovers the real client address before any rate limiting or quota accounting runs:

```sh
go run ./cmd/termond -listen 127.0.0.1:2222 -proxy-protocol
```

In this mode only the fronting proxy may speak to termond directly; keep `-listen` on loopback or behind firewall rules. Headers from signature-bearing but malformed connections are refused rather than guessed at, so a broken proxy fails closed instead of misattributing sources.

Two combinations never make sense together: `-proxy-protocol` trusts the address a connection claims, while `-exempt-loopback-rate-limit` removes rate limits for loopback peers — combined, any process that can reach termond can forge an unlimited, unattributed source. Startup refuses that combination unless `-unsafe-topology-override` records the operator's decision. The boot log states which topology resolved (`proxied topology` vs `direct topology`) so misconfiguration is visible before players connect.

## Backups & recovery

Everything durable lives on the data volume: the SQLite database file plus its WAL/SHM sidecars (`termon.db`, `termon.db-wal`, `termon.db-shm`), and the SSH host key. Losing the host key changes the server identity every client sees, so back it up together with the database. A live copy of a WAL-mode database can be inconsistent, so every backup and restore must stop the container.

The [deployment runbook](deployment.md#configure-stopped-container-backups) defines the production schedule, S3 retention and access controls, exact Dokploy procedure, restore drill, and required evidence. Use that procedure rather than copying individual files from a running volume.

Under the default WAL + `synchronous=NORMAL` profile, an acknowledged result survives an application crash, but commits made since the last checkpoint may be lost if the OS crashes or the host loses power — the database is never corrupted, only rolled back to the last checkpoint. That profile is a measured decision recorded in [docs/design/durability.md](design/durability.md); run `-sqlite-sync full` if host-crash durability outweighs its commit-latency cost.
