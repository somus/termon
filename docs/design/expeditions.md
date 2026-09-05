# Expedition contract

Expeditions are the repeatable solo route from the Dojo into the Collection. The route is short, targetable, and deterministic where ownership is concerned, while the Battles still ask the Trainer to play well. Ship Expeditions after Capture Lessons and three-active PvP, as specified in [Implementation rollout](implementation-rollout.md).

## Player-visible loop

1. The Trainer opens the Dojo Signal Board. Three Evolution Families are shown for the current server day. The board uses a content-owned, deterministic eight-day rotation, with three Families per day and every one of the 24 Families shown once per cycle. The card identifies the target Family, its base-stage Species, and the support-pool theme.
2. The Trainer selects one card and launches one Expedition. The server snapshots the target Family, target Species, support pool, and PvE band at launch, so a daily board change cannot invalidate a run already in progress. A Trainer may have one active Expedition at a time and may bring any Party of one to three battle-ready Monsters. Launch must refuse a Party Monster with fewer than four loaded Moves.
3. The route runs three single-active Battles against base-stage Wild Monsters:
   - **Preparation Encounter 1:** one distinct non-target Species from the selected target's curated support pool. It has no capture opportunity.
   - **Preparation Encounter 2:** a second distinct non-target Species from the same support pool. It also has no capture opportunity.
   - **Target Encounter:** the selected base-stage Species. This is the only encounter with a Capture Gauge and the only encounter that can add a Monster to the Collection.
4. The Party is fully healed and all fainted Monsters are revived between encounters. Damage and faints matter inside a Battle, but an Expedition does not become an attrition exercise. Solo Battles are untimed; the multiplayer Decision Clock does not apply.
5. The Capture Gauge is advanced by the reusable tactical objectives defined in [Capture Gauge and tactical objectives](capture-gauge.md). Family profiles and the Target Encounter PvE band are in [Capture Objective catalog](capture-catalog.md). When Gauge reaches 100%, the target is captured immediately and the Target Encounter ends. There is no consumable, random roll, or second capture check.
6. On success, the captured Monster is added as a new individual to the Collection. The current Party order is unchanged, so the Trainer chooses later whether to use the new Monster. The route then returns to the Dojo summary.

How the Dojo presents this loop at 100x32 is [Expedition terminal view](expedition-view.md).

## Target availability and support pools

The Signal Board is server-authoritative. The content pack owns one ordered list of the 24 Evolution Families; the server-day index selects three consecutive entries for each day of the eight-day cycle. The schedule has no hidden weighting and no Family is permanently unavailable. A board refresh changes only future launches, never a snapshotted run.

Each Family has a curated support pool of non-target, base-stage Species selected to teach the target's Type and useful counterplay. The server draws two distinct entries for the two Preparation Encounters. Support-pool composition is content data, not a new player-facing choice.

Preparation Encounters use ordinary natural combat. The Target Encounter uses the same `NaturalStat` formula, then applies Capture HP and the wild damage clamp from [Capture Objective catalog](capture-catalog.md) so the Gauge is a puzzle rather than a race to faint.

## Results and rewards

An encounter result is committed as soon as that Battle ends. XP for every completed encounter is retained even if the overall route later fails. A Target Encounter whose Wild Monster reaches zero HP before the Gauge fills is a `hunt_failed` result: the Trainer receives that completed Battle's XP, receives no captured Monster, and receives no successful-capture bonus.

Filling the Gauge produces a `captured` result. The Trainer receives the Target Encounter XP, the captured individual, and a small completion XP bonus. Exact XP integers, reserve shares, and the completion-bonus value belong to [XP, level curve, and normalized PvP](xp-progression.md); the Expedition contract does not add currency, items, or a daily resource cap.

The captured individual is initialized by [Individual Monster progression](progression.md) and persisted through [Progression persistence](progression-persistence.md). Captures are idempotent by Expedition run identity (`{run_id}:target`), so a reconnect or duplicate result cannot add the same individual twice. Duplicate Species are allowed: every successful run creates a distinct Monster even when the Trainer already owns that Species.

## Failure, abandonment, and reconnect

The following outcomes terminate the current Expedition without rolling back XP already committed for completed encounters:

- losing or forfeiting a Battle;
- explicitly abandoning the Expedition from the route UI;
- disconnecting beyond the existing 60-second same-process reconnect window; or
- a process restart while the Expedition is active.

On any of those outcomes the Trainer keeps completed-encounter XP, loses the target and route progress, receives no successful-capture bonus, and starts the next attempt at Preparation Encounter 1. There is no checkpoint, item cost, or other permanent failure penalty.

Within the same process, a reconnect during a Battle follows the [three-Monster Battle contract](party-battles.md): locked actions resolve, the event log remains authoritative, and the Trainer has 60 seconds to return. A reconnect between encounters resumes the snapshotted route under the same 60-second window. Expiry applies the same failure result as an abandoned run. A process restart discards the in-memory route; the already committed encounter rewards remain in the Save.

## State contract

| State | Entry | Terminal or next state |
| --- | --- | --- |
| `board` | Trainer is at the Dojo | `armed` after target selection |
| `armed` | Target and support pool are snapshotted | `preparation_1` |
| `preparation_1` / `preparation_2` | A single Wild Monster Battle starts | `recovery` on any completed result; `failed` on loss, forfeit, abandon, or reconnect expiry |
| `recovery` | Encounter result is committed | Next preparation or `target` |
| `target` | Capture Gauge is visible and active | `captured` when Gauge reaches 100%; `failed` when the target is defeated first or the run otherwise terminates |
| `captured` | Collection mutation and success rewards are committed | `completed` |
| `completed` / `failed` | Summary is shown | Return to `board` |

The server owns state transitions, target snapshots, reward idempotency, and Collection writes. The Battle engine remains responsible for synchronous combat resolution and receives the Expedition's Wild Monster and objective hooks as inputs. This keeps the Expedition loop compatible with PvP, Dojo Master, and the existing three-Monster Battle contract without changing their turn rules.
