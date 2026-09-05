# Individual Monster progression - decided (TERM-48)

An individual Monster is a persistent Collection item with its own identity, cumulative XP, Level, Move Library, Battle Loadout, and optional pending Evolution. Captures and starters begin at Level 1; the integer thresholds, rewards, natural stat curve, and normalized PvP rules are fixed in [XP, level curve, and normalized PvP](xp-progression.md). Implement the lifecycle in the slice order in [Implementation rollout](implementation-rollout.md).

## Persistent lifecycle

The versioned Save, Party slots, Progression Notices, and Store operations are [Progression persistence](progression-persistence.md). The runtime Monster record is:

```go
type Monster struct {
    ID               string   // opaque server-generated individual identity
    Species          string
    Nickname         string
    XP               int64    // cumulative XP, capped at the Level 50 threshold
    Level            int      // derived from XP and validated on load, 1 through 50
    MoveLibrary      []string // permanently unlocked Move slugs
    BattleLoadout    []string // at most four Move slugs selected outside Battle
    EvolutionPending bool     // the current Species has an accepted level threshold
}
```

`ID` is generated once when the Monster is created and survives Evolution, Party reordering, and reconnects. It is never reused after a Trainer Reset. The Collection is uncapped; two captures of the same Species always create two distinct IDs.

### Creation

- A starter and every captured Wild Monster enter at Level 1 with `XP = 0`.
- A new Monster's initial Move Library and Battle Loadout are the first four entries in its Species' Movepool, matching the current `DefaultLoadout` behavior. Those four entries are battle-ready immediately even when their content rows carry later learning levels; the listed levels govern additional entries after the baseline set.
- The initial Loadout is the first four Library entries in Movepool order. A Trainer may change it only outside Battle.
- A captured Monster is appended to the Collection and does not change the current Party. Capture itself awards no XP to the new individual; the Party that completed the Target Encounter receives that encounter's reward.
- The two onboarding Capture Lessons therefore add two Level 1 Monsters and leave the starter's progression separate, while still producing the required Full Party before Queue access.

### XP and Level

XP is cumulative and monotonic. `XPForLevel(1)` is zero; `Level` is the greatest level from 1 through 50 whose threshold is at or below the Monster's XP. At Level 50, XP is clamped to the Level 50 threshold and later rewards produce no further progression. A single reward may cross several thresholds; the runtime processes every crossed level in order rather than dropping overflow or skipping unlocks. Exact thresholds and natural stat scaling are defined in [XP, level curve, and normalized PvP](xp-progression.md).

The activity source determines whether an XP reward exists:

| Activity | Reward eligibility | Monster share |
| --- | --- | --- |
| Queue or direct Challenge Battle | A completed Battle Result | Monsters active for at least one resolved turn receive the full base reward; Party reserves that never enter receive the smaller reserve share. Completion, winner, reserve, and repeated-opponent rules are defined in [XP, level curve, and normalized PvP](xp-progression.md). |
| Expedition encounter | Every completed encounter, including a Target Encounter that ends `hunt_failed` | The active participant receives the full encounter reward; Party reserves use the reserve share. The newly captured target starts at Level 1 and receives none. |
| Dojo Master Capture Lesson | First successful completion of that Lesson | Active participants receive 90 XP; unused reserves receive the normal reserve share. A completed Lesson replay pays none. |
| Dojo Master Sparring | First clear of the selected tier for the snapshotted Server Day | Apprentice, Rival, and Master pay 65, 90, and 130 base XP respectively. Later clears that day pay none. |
| Daily Challenge | First objective clear for the snapshotted Server Day | Loaned slots map active and reserve participation to the same persistent Party slots and apply the 180 XP base packet. Mastery and replays add no XP. |

An incomplete Battle, explicit forfeit, disconnect timeout, or abandoned Expedition awards no reward for that unfinished activity. A failed Target Encounter is different: its Battle completed and therefore pays that encounter's XP, while the route and capture result are lost as defined in the Expedition contract. XP is committed once by the activity or Battle Result identity, so a reconnect or duplicate result cannot pay twice.

The server applies rewards in this order:

1. Commit the activity outcome and its participant set once.
2. Add each eligible Monster's share, clamping at the Level 50 threshold.
3. Recompute Level, unlock newly eligible Moves, and mark any newly eligible Evolution as pending.
4. Persist the complete Save before presenting progression UI. Evolution prompts are shown only after all Monsters in the reward have been processed.

The client presents one batched Progression Summary after persistence, then offers per-Monster Move and Evolution review. Skipping review returns to the Dojo without losing the reward and leaves durable attention state in the Collection. The complete terminal contract is defined in [Collection and Party terminal flow](collection-party.md).

Participation is based on Battle state, not on a button press. A Monster that enters after a forced Replacement or voluntary Switch and resolves at least one turn is a full participant even if it later faints. A reserve that never enters receives only the reserve share, and a Monster outside the selected Party receives nothing.

## Move Library and Loadout

When a reward raises a Monster to a new Level, the server scans the current Species' Movepool and permanently adds every not-yet-known entry whose learning level is at or below the new Level. Crossing several levels unlocks all eligible entries in content order. A newly unlocked Move never silently replaces an equipped Move: the Library grows, the four-slot Battle Loadout stays stable, and the Collection screen offers an explicit replacement flow outside Battle.

Evolution treats the Library as an inherited progression, not a reset. On acceptance:

- Keep every known Move and the current four-slot Loadout, including an inherited Move that is not listed in the successor's Movepool. This prevents a level reward or a deferred Evolution from unexpectedly deleting a player's chosen plan.
- Add every successor Movepool entry eligible at the Monster's current Level, then continue unlocking later entries at their content levels.
- Preserve the Monster ID, nickname, XP, Level, and Party position. The Species, art, base stats, and future Movepool source change permanently.

Content validation must guarantee that every Species has at least four baseline entries and that inherited Moves still resolve to valid global Move slugs. A successor may add new Moves or omit an old one without making an evolved Monster unable to battle.

## Evolution flow

Each Species has at most one successor and each Family's thresholds are strictly increasing. After a reward raises a Monster to or above its current Species' `evolves_to.level`, the server sets `EvolutionPending` and emits an explainable prompt with the successor name, stat-role summary, and newly eligible Moves.

The Trainer may accept or defer the prompt. Deferring is indefinite: the Monster keeps its current Species, stats, art, and Movepool unlock schedule while it continues gaining XP. The prompt is repeated in the Collection and after later rewards, but it never blocks Battle, Party edits, or Queue entry. Evolution is never triggered by a normalized PvP Level and never occurs during an active Battle.

Accepting an Evolution permanently changes only the Species identity and derived Species data. XP, Level, ID, nickname, Library, Loadout, Party position, and accumulated Battle history remain intact. The server adds successor Moves before returning to the Collection. If content ever permits the current Level to cross another successor threshold, prompts are presented one Evolution at a time after the first acceptance rather than chaining silently.

## Duplicate captures and reset

Every successful Expedition capture creates a new individual, even when the Species already exists in the Collection or Party. The capture mutation is idempotent by Expedition run identity, so reconnects cannot add the same individual twice. Collection order is append-only at capture time; Party order is unchanged until the Trainer edits it.

`Trainer Reset` clears the mutable Save: Collection, Party, Monster IDs, XP, Levels, Move Libraries, Loadouts, nicknames, pending Evolutions, and W/L totals. It preserves the Trainer ID, SSH Credentials, and immutable historical Battle Results. Re-onboarding creates a new Level 1 starter and new individual IDs; no prior Monster identity or progression can leak into the fresh Save.

## Invariants and verification

- Every persisted Monster has one unique ID within its Trainer, `0 <= XP <= XPForLevel(50)`, and `1 <= Level <= 50` with Level derived consistently from XP.
- Every Party entry points to a Collection ID, contains at most three Monsters, and retains order across Battle, Evolution, reconnect, and reward application.
- Every Battle Loadout has one to four unique Move slugs from the Monster's Move Library; changing the Loadout never removes Library knowledge.
- Each eligible activity outcome pays XP at most once, and a captured target never receives the capturing Party's XP retroactively.
- Level-up processing is deterministic and unlocks every crossed Movepool threshold, including thresholds crossed by one large reward.
- Evolution prompts are idempotent, deferrable, and processed after reward persistence; accepting an Evolution cannot erase learned Moves or create a duplicate individual.
- Reset removes all mutable progression while preserving identity and historical result records.

Implementation tests must cover creation at Level 1, baseline four-Move initialization, participant versus reserve XP, Level 50 clamping, multi-threshold unlocks, deferred and accepted Evolution, inherited Loadout Moves, duplicate-capture idempotency, reconnect replay, and ResetTrainer cleanup. Exact XP values, reserve percentage, winner bonus, repeated-opponent decay, and level-stat constants are specified in [XP, level curve, and normalized PvP](xp-progression.md).
