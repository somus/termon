# Durability and production persistence - decided (TERM-26)

Termon uses embedded SQLite as its production persistence backend on the VPS. Durable Trainer data and completed Battle Results survive restarts; live coordination and active Battles do not. The database runs in WAL mode on a Dokploy named volume, and Dokploy backs up that volume while the container is stopped.

## Durability contract

The durability boundary keeps player-owned progress and completed outcomes while allowing the process-owned Hub to restart without reconstructing live sessions.

| State | Durable | Restart behavior |
|---|---:|---|
| Trainer identity | Yes | Loads by stable Trainer ID |
| SSH Credentials | Yes | Authenticate the same Trainer |
| Handle and W/L totals | Yes | Load from relational columns |
| Party, Collection, nicknames, progression, and open Progression Notices | Yes | Load from the versioned Save payload |
| Completed multiplayer Battle Result | Yes¹ | Remains immutable and prevents duplicate record updates |
| Solo Activity Result, first-clear, and Mastery Mark | Yes¹ | Remains unique on its natural key and prevents duplicate XP, capture, or marks |
| Active Battle | No | Abandoned without changing either record |
| Hidden move selections | No | Discarded with the active Battle |
| Battle event log | No | Discarded with the active Battle |
| Reconnect deadline | No | Applies only while the same termond process owns the Battle |
| Queue entry | No | Trainer reconnects to the Lobby |
| Challenge | No | Trainer reconnects to the Lobby |
| Lobby presence and emote | No | Recreated after reconnect |
| TUI screen and playback | No | Recreated from durable Trainer state |

A connection drop can resume an active Battle during the existing 60-second grace period only while the same termon process remains alive. A process restart or deployment abandons every active Battle, records no winner or loser, and returns authenticated Trainers to the Lobby.

¹ "Durable" here means surviving application crash and restart. The production WAL/NORMAL SQLite profile does not fsync every commit, so a result acknowledged in the seconds before an OS crash or power loss may be lost (never corrupted). See "SQLite durability profile" below for the measured tradeoff and the `-sqlite-sync full` opt-in.

## Trainer identity

A generated, opaque Trainer ID identifies the player independently of SSH. A hashed SSH public key fingerprint is an SSH Credential that authenticates that Trainer.

The schema permits multiple SSH Credentials per Trainer, but launch supports one credential and no linking, rotation, or recovery flow. Losing the private key means losing access until a separate secure recovery design ships. Termon never stores the raw public key fingerprint.

## SQLite schema boundary

Relational columns hold data that needs uniqueness, joins, constraints, or operational queries. A versioned JSON payload holds game state that will evolve with progression features.

| Storage | Data |
|---|---|
| `schema_migrations` | Applied relational schema versions |
| `trainers` | Trainer ID, Handle, W/L totals, Save version, Save payload, and timestamps (a NULL Handle/Save version marks a Trainer still in onboarding) |
| `ssh_credentials` | Hashed fingerprint and owning Trainer ID |
| `battle_results` | Unique Battle ID, participants, winner, loser, reason, and completion time |
| `activity_results` | Unique natural key, trainer, kind, payload, and optional captured Monster ID |
| `session_results` | Authenticated SSH Session ID, Trainer, start/end time, end reason, version, and resume target |
| Versioned Battle Result body | Participation, reward intent, and bounded statistics captured at Battle time |
| Versioned Save payload | Collection, three Party slots, Monster progression, nicknames, and open Progression Notices |

The production database does not retain `FingerprintHash` inside the Save payload. Identity belongs to `trainers` and `ssh_credentials`; the Save contains only Trainer-owned game state.

The previous JSON-per-Trainer store has no automatic migration because its data was pre-production. A future Cloudflare migration may implement the same persistence operations with Durable Objects, but it does not change this contract.

## Transaction boundaries

The persistence interface exposes domain operations rather than generic load-modify-write methods. Each operation commits before the Hub broadcasts success.

- `CreateTrainer` creates the stable Trainer ID and first SSH Credential in an onboarding-required state.
- `CompleteOnboarding` stores the chosen Handle and initial versioned Save payload.
- `ResetTrainer` preserves the Trainer ID and SSH Credentials, clears the Handle, Save, and W/L totals, deletes that Trainer's solo Activity Results, and returns the Trainer to onboarding. Immutable historical Battle Results remain stored.
- `RecordBattleResult` inserts one immutable Battle Result, updates both Trainers' W/L totals, and when eligible applies both Reward Packets to both Saves in the same transaction. The unique Battle ID makes retries idempotent; a duplicate returns the existing outcome without incrementing either record or paying XP twice.
- `RecordActivityResult` inserts one immutable solo Activity Result and applies that Trainer's capture, XP, first-clear, or Mastery Mark in the same transaction. The unique natural key makes retries idempotent; a conflicting body is rejected.

Workbench edits (`SetParty`, loadout, nickname, notice acknowledgement, AcceptEvolution) are separate domain operations on the Save. The full progression persistence contract is [Progression persistence](progression-persistence.md).

Dojo Master and Expedition never call `RecordBattleResult`. An active Battle has no persistence transaction because process-restart recovery is outside the product contract.

Persistence failures remain visible to the Hub. The Hub must not report a durable result, clear reconciliation state, or send updated records until the domain operation commits successfully. TERM-27 defines retry and surfaced-error behavior around this boundary.

## Concurrency ownership

The single process-owned Hub is the only caller that mutates game state. SQLite transactions provide the durable serialization boundary, so the VPS implementation does not add revision columns, compare-and-swap loops, or row locks.

A future backend with multiple writers must preserve the same domain operations and idempotency guarantees. It may add revisions, row locks, or a single Durable Object coordinator when that deployment architecture exists; those backend mechanisms do not leak into Battle or TUI packages.

## Schema and Save migrations

Termon applies numbered, forward-only SQLite migrations transactionally at startup. Startup fails before accepting SSH sessions when a migration fails or when the database schema is newer than the binary supports.

Each Save payload carries a version so a newer binary can refuse an unknown future payload. Collection and progression replace the payload at Save version 1. They do not bump that version. The application does not provide down migrations. Schema version 3 shipped `activity_results`. Schema version 4 adds `session_results` plus Trainer-history indexes; the Save remains at version 1. Battle statistics extend the existing immutable result body without changing its relational schema.

A previous binary can roll back safely only when it declares support for the current relational schema and Save version. Otherwise, rollback requires stopping writes and restoring the pre-upgrade Dokploy backup. The deployment runbook must state compatibility before each release.

## Dokploy storage and backups

SQLite runs in WAL mode with the database on a Docker named volume. The SSH host key also lives on persistent named-volume storage so a deployment does not change the server identity.

Dokploy Volume Backups copy the volume to the configured S3 destination. Enable **Turn off Container** for each database backup so termond cannot write while Dokploy copies the SQLite database and WAL files. Dokploy warns that a live volume backup can be inconsistent; the stopped-container mode trades brief scheduled downtime for a coherent restore point. See [Dokploy Volume Backups](https://docs.dokploy.com/docs/core/volume-backups).

Dokploy owns the schedule, retention, encryption, and S3 destination configuration. TERM-14 must verify one backup and restore drill before launch, document the resulting downtime, and test that Trainer data, Battle Results, migrations, and the SSH host key recover correctly. The step-by-step operator procedure for stopping, copying, and restoring this volume lives in [Backups & recovery](../operations.md#backups--recovery).

## SQLite durability profile

The production profile is WAL + `synchronous=NORMAL` (`-sqlite-sync normal`, the default). This is a measured decision, not an oversight: under FULL, 512 concurrent Trainers exceeded the 500 ms commit-p95 target, while NORMAL roughly halved commit p95 and p99 at every load level with zero correctness failures — see docs/load-baseline.md for the numbers. The tradeoff is that an acknowledged Battle Result survives application crashes but may be lost on OS crash or power loss (never corrupted).

Operators who need host-crash durability can run `-sqlite-sync full`, accepting the measured commit-latency regression. Connection-scoped PRAGMAs (`foreign_keys`, `busy_timeout`, `synchronous`) travel in the DSN, so every connection database/sql creates enforces them; the store refuses to start if the effective profile does not match.

## Alternatives not selected

An early JSON-per-Trainer store (since deleted) could not commit a multiplayer result across two Trainers: one atomic rename covers a single file, and coordinating multiple files would recreate transaction and recovery behavior poorly.

Embedded SQLite supplies atomic multi-Trainer transactions, constraints, WAL, migrations, and a single-volume deployment without adding a network service. SQLite documents its [transaction guarantees](https://www.sqlite.org/lang_transaction.html) and [WAL behavior](https://www.sqlite.org/wal.html).

SQLite-backed Durable Objects provide transactional, strongly consistent storage and point-in-time recovery, but using them from the VPS would add a remote Worker API before Cloudflare raw TCP deployment is ready. They remain a future backend behind the same domain operations. See [Cloudflare Durable Object storage](https://developers.cloudflare.com/durable-objects/api/sqlite-storage-api/).
