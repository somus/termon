# XP, level curve, and normalized PvP - decided (TERM-50)

This decision supplies the integer progression and reward rules referenced by [Individual Monster progression](progression.md), [Three-Monster Battle contract](party-battles.md), and [Expedition contract](expeditions.md). It is calibrated for mixed play: about eight short PvP Battles or six ten-minute Expeditions per active hour.

## Level thresholds

XP is cumulative. `LevelForXP` returns the greatest level whose threshold is at or below the Monster's XP. XP is an integer, monotonic, and clamped at the Level 50 threshold.

```text
XPForLevel(1) = 0

1 < level <= 24: XPForLevel(level) = 90 * (level - 1)
24 < level <= 40: XPForLevel(level) = 2070 + 240 * (level - 24)
40 < level <= 50: XPForLevel(level) = 5910 + 390 * (level - 40)
```

The useful anchors are:

| Level | XP threshold | Design purpose |
| ---: | ---: | --- |
| 14 | 1,170 | Earliest first Evolution |
| 24 | 2,070 | Latest first Evolution |
| 30 | 3,510 | Normalized Battle level and earliest final Evolution |
| 32 | 3,990 | Early final-Evolution checkpoint |
| 34 | 4,470 | Mid final-Evolution checkpoint |
| 36 | 4,950 | Mid final-Evolution checkpoint |
| 38 | 5,430 | Late final-Evolution checkpoint |
| 40 | 5,910 | Latest final Evolution |
| 50 | 9,810 | Progression cap |

The jump in XP per level after 24 makes the final-stage stretch visible without making the first Evolution feel slow. A reward may cross several thresholds; every crossed Move and Evolution check is processed in order. Reaching Level 50 clamps XP to 9,810 and produces no further progression.

## Reward packets

Each completed activity creates one idempotent reward packet identified by its Battle or activity result. The server adjusts the packet once, then applies the resulting share to each eligible Monster.

| Activity result | Base XP | Additional active-participant bonus | Successful completion condition |
| --- | ---: | ---: | --- |
| PvP Battle | 130 | Winner +25 | Completed Battle Result; both sides receive the base |
| Expedition Preparation Encounter | 40 | None | Battle completes, whether won or lost |
| Expedition Target Encounter | 65 | None | Battle completes, including `hunt_failed` |
| Expedition capture completion | 0 | +35 | Gauge fills and the Target Encounter captures |
| Capture Lesson | 90 | None | First successful completion of that Lesson |
| Apprentice Sparring | 65 | None | First clear for the snapshotted Server Day |
| Rival Sparring | 90 | None | First clear for the snapshotted Server Day |
| Master Sparring | 130 | None | First clear for the snapshotted Server Day |
| Daily Challenge | 180 | None | First objective clear for the snapshotted Server Day |

An active participant is a Monster that enters and resolves at least one turn. A Party reserve that never enters receives the reserve share below. The winner bonus and Expedition capture completion bonus go only to active participants. A captured Target Monster is created at Level 1 and receives none of the Expedition's XP.

An incomplete activity, explicit forfeit, disconnect timeout, or abandoned Expedition pays nothing for that unfinished activity. A completed Target Encounter that ends `hunt_failed` pays its 65 XP but has no capture or completion bonus. Completed Lesson replays, Sparring clears after that tier's daily first clear, and Daily Challenge replays after the first objective clear pay no XP. Daily loaned slots map active or reserve participation to the same persistent Party slots as defined in [Dojo Master modes and bot behavior](dojo-master.md).

## Repeated-opponent decay

PvP completion and winner rewards are reduced for repeated play against the same Trainer, so two friends cannot create a profitable loop by rematching forever. Count completed PvP Results between the pair in a rolling 24-hour window, regardless of who won:

| Prior completed Results in the window | Decay multiplier |
| ---: | ---: |
| 0-1 | 1.00 |
| 2-3 | 0.75 |
| 4 or more | 0.50 |

The multiplier is symmetric and applies to both sides. For a result, compute `adjustedBase = floor(130 * multiplier)` and `adjustedWinner = floor(25 * multiplier)`. An active loser receives `adjustedBase`; an active winner receives `adjustedBase + adjustedWinner`; a reserve receives `floor(adjustedBase * 0.40)`. A Result leaving the 24-hour window reduces the pair's count again. There is no global daily XP cap, and a new opponent always starts at the full multiplier.

## Reserve share and participation

The reserve share is 40% of the already-decayed base XP, rounded down. It is paid to a Party slot that never becomes active in that activity. The share is not paid to a Monster outside the selected Party, and it is not retroactively changed when another Monster faints. Entering after a voluntary Switch or mandatory Replacement changes that Monster from reserve to active for the current activity and pays the full base share after the next completed result.

The reward application order is deterministic:

1. Commit the result identity and participant set once.
2. Compute the decayed base and bonuses, then each Monster's active or reserve share.
3. Add XP with the Level 50 clamp.
4. Recompute levels, unlock every crossed Move, and mark eligible Evolution as pending.
5. Persist the complete Save before showing progression prompts.

This order keeps reconnects and duplicate result delivery from paying twice and guarantees that an Evolution prompt never appears before the corresponding XP and Move Library state is durable. [Progression persistence](progression-persistence.md) commits that identity, decay, both Saves, and notices in one Store operation.

## Natural stats

Solo Battles use the Species base stats and the following level multiplier for each of HP, Attack, Defense, Special Attack, and Speed. Target Encounter Capture HP and the wild damage clamp overlay this formula; see [Capture Objective catalog](capture-catalog.md).

```text
NaturalStat(base, level) = max(1, floor(base * (100 + 2 * (level - 1)) / 100))
```

Level 1 therefore uses the content stat unchanged, while Level 50 is just under twice the base value. Stats are derived from the current Species after an accepted Evolution; XP, Level, and Monster identity remain unchanged.

## Normalized Queue and Challenge Battles

Queue and direct Challenge Battles use a battle-only normalized copy. The persistent Monster, Move Library, Battle Loadout, XP, Level, pending Evolution, and Party order are never mutated by normalization. Queue reopens already normalized, in the same slice as the first paying XP. See [Implementation rollout](implementation-rollout.md).

- The copy uses `QueueLevel = 30` for level-dependent battle calculations. This level is high enough to include the first final-stage threshold while staying below the late-game cap.
- First calculate natural stats at Level 30, then rescale each Monster to `QueueStatBudget = 320`, the existing middle-stage stat total. Preserve each Species' role proportions and use a deterministic largest-remainder allocation so the five stats sum exactly to 320 and each stat is at least 1. This removes grind-based stat gaps while retaining Species, Type, Evolution stage, and matchup identity.
- The copy uses the Monster's persistent Battle Loadout. Each Move must come from the current Species' Movepool at or below Level 30, or be an inherited Library Move whose original unlock level is at or below 30. Loadout edits happen in the Workbench; Queue entry may only change roster membership and opening order. A pending Evolution is not accepted by entering Queue.
- Both sides start at full normalized HP. XP rewards still use the real persistent Monsters and the reward packet above, not the normalized copy.

The Level-30 eligibility check removes a veteran's later Move-unlock advantage from the competitive ruleset, while Species stage, role distribution, Types, and the Trainer's persistent Battle Loadout remain meaningful. Solo Expeditions and Dojo modes use natural persistent Levels, so progression still changes the main capture-and-training loop.

## Calibration result

The throwaway terminal prototype used the thresholds and reward packets above. Its output was:

| Milestone | PvP loser at 8/h | PvP winner at 8/h | Successful Expedition at 6/h |
| --- | ---: | ---: | ---: |
| First Evolution range, Levels 14-24 | 1.12-1.99 h | 0.94-1.67 h | 1.08-1.92 h |
| Final Evolution range, Levels 30-40 | 3.38-5.68 h | 2.83-4.77 h | 3.25-5.47 h |
| Level 50 | 9.43 h | 7.91 h | 9.08 h |

The curve therefore targets the agreed pacing of first Evolution in 1-2 active hours, final Evolution in 4-6, and Level 50 in 8-10 for ordinary mixed play. Same-opponent decay deliberately extends those times for a closed rematch loop, while varied opponents and Expedition targets retain the target pace.

## Invariants and verification

- `0 <= XP <= XPForLevel(50)` and `1 <= Level <= 50`; Level is always derived consistently from XP.
- Every completed result pays at most once, and every reward share is integer and non-negative.
- A Monster's active share requires at least one resolved turn; an unused reserve receives exactly 40% of the adjusted base, rounded down.
- Queue normalization never mutates persistent progression, never triggers Evolution, and always produces five stats summing to 320.
- Normalized Battles reject a Battle Loadout containing a Move above the Level 30 threshold and never write a persistent unlock.
- Repeated-opponent decay counts completed Results symmetrically in a rolling 24-hour window and never reduces rewards below 50% of the base packet.
- Dojo first-clear rewards are keyed idempotently by Lesson or by Sparring tier and Server Day; a Daily Challenge first clear is keyed by Server Day.
- Content validation must guarantee at least four current Species Movepool entries at or below Level 30 so every normalized Monster can form a legal four-Move Queue Loadout.

Implementation tests must cover threshold boundaries, multi-level rewards, Level 50 clamping, active versus reserve shares, winner and completion bonuses, rolling decay buckets, failed Target Encounter rewards, normalized stat sums and rounding, normalized Battle Loadout filtering, and idempotent result replay.
