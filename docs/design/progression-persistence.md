# Progression persistence contract - decided (TERM-51)

Trainer-owned progression lives in a Save payload. Outcomes that must stay unique across reconnects live in relational rows. The Store still exposes domain operations; the Hub remains the only writer. Reward math follows [Individual Monster progression](progression.md) and [XP, level curve, and normalized PvP](xp-progression.md). Presentation follows [Collection and Party terminal flow](collection-party.md). Ship this contract first, as slice 1 of [Implementation rollout](implementation-rollout.md).

## Storage split

| State | Storage | Restart behavior |
| --- | --- | --- |
| Collection, Party slots, Monster progression, nicknames, open Progression Notices | Save payload | Load with the Trainer |
| Handle and W/L totals | `trainers` columns | Unchanged from TERM-26 |
| Multiplayer Battle Result | `battle_results` | Immutable; duplicate Battle ID does not reapply W/L or XP |
| Solo Activity Result, first-clear, Mastery Mark | `activity_results` | Immutable per natural key |
| In-progress Battle or Expedition | Memory | Abandoned on process restart; already committed encounter XP stays in the Save |
| Uncommitted Workbench choosers and confirmation dialogs | Memory | Discarded; reconnect returns to the Dojo or Workbench |

Relational tables hold data that needs a unique constraint or a Dojo status query. The Save holds Trainer-private state that will keep evolving.

## Save payload

Handle and W/L stay relational. Save version remains 1. Replace the JSON payload in place. Do not bump `save_version` for this shape.

Saves written before Collection (party stored as an array of `{species, nickname, moves}` objects) still load: the Store converts them in place to Collection + Party IDs, mints Monster IDs, and treats the stored moves as both Move Library and Battle Loadout at Level 1. The rewritten payload is persisted on the next `LoadTrainer` / `ResolveCredential` so reconnect keeps those IDs. Genuinely unreadable blobs still fail as corrupt.

The JSON payload is:

```go
type savePayload struct {
    Collection []Monster            `json:"collection"`
    Party      [3]string            `json:"party"` // Monster IDs; "" is a vacant slot
    Notices    []ProgressionNotice  `json:"notices"`
}

type Monster struct {
    ID                 string   `json:"id"`
    Species            string   `json:"species"`
    Nickname           string   `json:"nickname"`
    XP                 int64    `json:"xp"`
    Level              int      `json:"level"`
    MoveLibrary        []string `json:"move_library"`
    BattleLoadout      []string `json:"battle_loadout"`
    EvolutionPending   bool     `json:"evolution_pending"`
}

type ProgressionNotice struct {
    ID        string   `json:"id"`
    Kind      string   `json:"kind"` // move_unlock | capture_review
    MonsterID string   `json:"monster_id"`
    SourceKey string   `json:"source_key"` // Battle ID or Activity Result natural key
    Moves     []string `json:"moves,omitempty"`
}
```

`Party` is always three slots. Slot 1 is the opening lead. A vacant slot is the empty string and does not shift later members forward. Filled IDs must be unique and must exist in `Collection`. Unknown future Save versions still refuse to load.

`Level` is derived from `XP` and validated on load. `BattleLoadout` is one to four unique slugs from `MoveLibrary`. `EvolutionPending` is derived from the current Species threshold and is not a Progression Notice.

An empty nickname means the Species name is shown. `SetNickname` accepts 1–16 Unicode letters, marks, digits, spaces, or hyphens after trimming ends. Any other rune is rejected. A blank (or whitespace-only) value clears the nickname.

The Store mints Monster IDs and notice IDs as opaque 16-byte hex strings, the same width as Trainer IDs. A Trainer Reset never reuses those IDs.

Open notices are the only notice rows. Acknowledgement deletes them. Skipping a Progression Summary leaves them in place. Reconnect cannot resurrect a deleted notice. Capture review uses `kind=capture_review`; Move unlocks use `kind=move_unlock` with every newly added slug on that Monster from that reward.

## Relational Activity Results

Schema version 3 adds:

```sql
CREATE TABLE activity_results (
    id TEXT PRIMARY KEY,
    natural_key TEXT NOT NULL UNIQUE,
    trainer_id TEXT NOT NULL REFERENCES trainers(id),
    kind TEXT NOT NULL,
    completed_at TEXT NOT NULL,
    payload TEXT NOT NULL,
    captured_monster_id TEXT
);
```

`natural_key` is the idempotency identity. `id` is an opaque log identifier. `payload` is canonical JSON used for conflict comparison: kind, outcome, active IDs, reserve IDs, capture species, `fill_party`, and mastery. It does not include derived XP integers.

| Kind | Natural key | Pays XP / capture |
| --- | --- | --- |
| `expedition` | `{run_id}:prep1`, `{run_id}:prep2`, `{run_id}:target` | Each completed encounter once. Capture is the `captured` target row, not a second key. |
| `lesson` | `{trainer_id}:lesson:{lesson_id}` | First successful completion and its capture. |
| `sparring` | `{trainer_id}:sparring:{tier}:{yyyy-mm-dd}` | First clear of that tier for the snapshotted Server Day. |
| `daily_xp` | `{trainer_id}:daily:{yyyy-mm-dd}` | First objective clear for that Server Day. |
| `daily_mastery` | `{trainer_id}:daily-mastery:{yyyy-mm-dd}` | Mastery Mark; may land on a later no-XP replay. |

`run_id` is an opaque ID the Hub generates at Expedition launch. There is no durable run row. The first durable use is the `prep1` commit. A process restart abandons the in-memory route; the next launch mints a new `run_id`.

Server Day is the UTC date snapshotted when the attempt starts, encoded `yyyy-mm-dd`.

A Daily first-clear that also meets par inserts `daily_xp` and `daily_mastery` in the same transaction. A later mastery-only replay inserts only `daily_mastery`.

## Store operations

Each mutation commits before the Hub broadcasts success. Persistence failure stays visible. The Hub does not report a reward, capture, or Workbench edit until the operation returns.

### Existing operations

- `CreateTrainer` — unchanged.
- `CompleteOnboarding` — writes Collection containing the Level 1 starter, Party `[starterID, "", ""]`, and empty notices.
- `ResetTrainer` — keeps Trainer ID, SSH Credentials, and multiplayer `battle_results`. Clears Handle, W/L, and the Save. Deletes that Trainer's `activity_results` rows so Lessons, Sparring, and Daily can pay again.
- `LoadTrainer` — returns Collection, Party, and open notices. Handle and W/L still come from columns. A payload that does not decode is `ErrCorruptSave`.

### `RecordBattleResult`

The Battle ID remains the idempotency key. The Hub also passes each side's active Monster IDs, reserve Monster IDs, and `ApplyRewards`.

In one transaction the Store:

1. If the Battle ID exists with the same result body, participation, and `ApplyRewards`, return both Saves without writing.
2. If the Battle ID exists with a different body, return `ErrResultConflict`.
3. Insert `battle_results`.
4. Increment winner W/L and loser W/L.
5. If `ApplyRewards` is false, persist no XP (forfeit, Decision Clock expiry, disconnect timeout).
6. Otherwise count that pair's prior completed Results in the rolling 24-hour window, apply [repeated-opponent decay](xp-progression.md), add shares, unlock Moves, set `EvolutionPending`, and append notices.

Duplicate delivery cannot pay twice. The Store does not parse Battle logs. Empty participation with `ApplyRewards=false` is the no-XP path; W/L still change.

### `RecordActivityResult`

One method covers Expedition encounters, Lessons, Sparring, Daily XP, and Mastery Marks.

The Hub passes `kind`, `natural_key`, trainer ID, active IDs, reserve IDs, optional capture (Species slug plus `fill_party`), and optional `mastery_only`. The Store applies the [activity reward table](xp-progression.md) from `kind` and participation.

- Duplicate `natural_key` with the same payload returns the existing Save and does not reapply.
- Duplicate `natural_key` with a different payload returns `ErrResultConflict` and writes nothing. A retried `hunt_failed` target is fine; a later `captured` on the same `{run_id}:target` is a conflict.
- A capture mints a Level 1 Monster, appends Collection, and adds a `capture_review` notice. `fill_party` (Capture Lessons) writes the new ID into the first vacant Party slot. Expedition captures leave Party unchanged.
- Daily first-clear may also insert `daily_mastery` in this transaction when par was met.

### Workbench operations

- `SetParty(trainerID, [3]string)` — each filled ID must exist in Collection; filled IDs are unique.
- `SetBattleLoadout(trainerID, monsterID, moves)` — one to four unique Library slugs; does not ack notices.
- `SetBattleLoadoutAndAck(trainerID, monsterID, moves, noticeIDs)` — same loadout write, and deletes those notice IDs in the same transaction. Missing IDs are no-ops. A crash cannot ack a Move unlock without the matching Loadout write.
- `AcknowledgeProgressionNotices(trainerID, noticeIDs)` — deletes those open notices (`Keep loadout` or capture-review dismiss). Missing IDs are no-ops.
- `SetNickname(trainerID, monsterID, nickname)` — validation above.
- `AcceptEvolution(trainerID, monsterID)` — requires `EvolutionPending`. Species, art, and future Movepool source change; ID, nickname, XP, Level, Library, Loadout, and Party slot stay. Successor Moves eligible at the current Level are added. New unlocks mint `move_unlock` notices. A newly eligible next stage stays pending and is not accepted in this call.

There is no `DeferEvolution` write. Deferral leaves `EvolutionPending` set.

### Reads

- `ActivityExists(trainerID, naturalKey) bool`
- `ActivityResult(trainerID, naturalKey) *ActivityResult`

The Dojo uses these for Lesson completion, today's Sparring first-clears, Daily first-clear, and Mastery Mark. First-clear flags are not copied into the Save.

## Reward application order

For every paying PvP or solo commit:

1. Insert the result identity (Battle ID or activity natural key) or return the duplicate/conflict outcome.
2. Compute integer shares (decay for PvP; activity table plus active/reserve for solo).
3. Add XP with the Level 50 clamp.
4. Recompute Level, unlock every crossed Movepool entry, set `EvolutionPending` when the current Species threshold is met.
5. Append open Progression Notices for new Move unlocks and new captures.
6. Commit the Save (and both Saves for PvP) before the Hub presents UI.

The captured target never receives the capturing Party's XP. Normalized Battles copy the persistent Library and Battle Loadout without writing either one.

## Invariants and verification

- Collection IDs are unique per Trainer. Party is length 3. Filled slots reference Collection and do not duplicate.
- `0 <= XP <= XPForLevel(50)` and `Level` matches `XP`. Loadout is a subset of Library, length 1–4.
- Open notices reference Collection IDs. Acknowledged notices are absent. `EvolutionPending` does not depend on notices.
- Each Battle ID and each activity natural key pays at most once. Conflicting bodies error instead of overwriting.
- Reset removes mutable progression and solo Activity Results; multiplayer Battle Results remain.
- Capture Lesson fills a Party slot; Expedition capture does not.
- `SetBattleLoadoutAndAck` is atomic; `AcknowledgeProgressionNotices` never writes a Loadout.

Implementation tests must cover onboarding Save payload, PvP reward vs forfeit, 24-hour decay counted inside the transaction, duplicate Battle ID, Expedition encounter commits including `hunt_failed` vs later `captured` conflict, Lesson capture Party fill, Expedition bench capture, Sparring and Daily first-clear plus mastery-only replay, notice skip/ack/reconnect, atomic loadout+ack, vacant Party slots, nickname validation, AcceptEvolution, and ResetTrainer cleanup of `activity_results`.

## Alternatives not selected

A Save-only ledger cannot enforce uniqueness when the Hub retries with a new opaque ID. Fully relational Monsters would move evolving Trainer-owned state out of the versioned payload TERM-26 already uses.

Embedding Monster copies in Party would drift from Collection. Compact 0–3 Party slices cannot represent a vacant lead with occupied reserves.

Persisting acknowledged notice history is unnecessary once deletion makes resurrection impossible. Deriving attention from Library size cannot distinguish skip-summary from ack.

Splitting W/L and XP across two transactions can record a winner without paying XP. Counting decay outside the Store can race a second Result for the same pair.

Opaque activity IDs without a natural unique key fail when a retry mints a new ID. Boolean flags on the Save cannot share the Battle Result conflict model.

Keeping solo Activity Results across Reset blocks Capture Lessons on the new Save. Denormalizing first-clears into the Save duplicates `activity_results`.
