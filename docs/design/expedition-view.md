# Expedition terminal view - decided (TERM-53)

The Signal Board and Expedition run keep the production Battle chrome: sprites and plates in the arena, and every prompt, result, and command in the four-row action box at the bottom. The Trainer never leaves that reading line when the route moves from the board into prep, the Target Encounter, or a summary.

This document specifies presentation against the loop in [Expedition contract](expeditions.md). It does not change snapshots, rewards, or Capture Objective rules. Gauge layout stays [Capture Gauge and tactical objectives](capture-gauge.md).

The production view uses the accepted Layout A arrangement inside the existing Battle chrome and Signal Board arena.

## Stacked frame at 100x32

The supported minimum stays 100 columns by 32 rows. Smaller terminals keep the existing too-small warning. UI is never painted through sprite art.

```text
arena: Family cards, or sprites and HP plates
[Target Encounter only: Capture Gauge band]
┌ chrome: two inner lines, same box as Battle ┐
│ narration or result                           │
│ commands                                      │
└───────────────────────────────────────────────┘
```

The chrome is the same widget Battle already uses for intro text, `What will X do?`, the Move grid, and `you won!`. It stays four rows tall (two inner lines plus the existing border). Line 1 is narration. Line 2 is commands, on one row, matching `FIGHT` `SWITCH` `RUN`.

A separate route strip, a Dojo-only text page, and a left-hand dossier are not part of this layout. Route progress is a prefix on chrome line 1 (`PREP 1/3`, `TARGET`) so the eye does not leave the action box.

## Arena

**Signal Board and launch.** Three Family cards sit centered in the arena. Each card shows the full base-stage sprite, Family name, Type, and support-pool theme. The sprite is trimmed of empty padding so heads and feet stay in frame; it is never a center crop of the torso. Those labels stay on the card the way HP stays on a plate. They are not a second narration stream.

**Encounters, recovery, capture, hunt failure, and reconnect.** The production arena stays: sprites, HP plates, and no Decision Clock. Recovery, `captured`, `hunt_failed`, and reconnect keep the arena visible and put the outcome in the chrome. The captured Species remains on screen when chrome reports `Collection +1`.

**Abandon.** The three board cards return in the arena. The chrome reports the lost target and kept XP.

## Chrome by state

| State | Line 1 | Line 2 |
| --- | --- | --- |
| `board` | Today's Families, server-day index, and the eight-day cycle | The three Family names; cursor matches the selected card |
| `armed` | Launch the selected Family. Party is ready. Prep has no capture. | `START` `BACK` `ABANDON` |
| `preparation_1` / `preparation_2` | `PREP n/3`, the Wild Species, and `No capture.` | `FIGHT` `SWITCH` `RUN` |
| `recovery` | The committed encounter, that the Party is healed, and kept XP | `PREP 2` or `TARGET`, plus `ABANDON` |
| `target` | `TARGET`, the Wild Species, and that the Gauge is live | `FIGHT` `SWITCH` `RUN` |
| `captured` | `CAPTURED`, `Collection +1`, Species, Level 1, and XP total | `DOJO` `WORKBENCH` |
| `hunt_failed` | Hunt failed, frozen Gauge, no capture, kept XP | `DOJO` |
| `abandoned` / reconnect expiry | Abandoned, lost target, kept completed XP, next run starts at prep 1 | `DOJO` |
| reconnect (in window) | Remaining seconds, next encounter, expiry abandons the route | `WAITING` |

If line 1 cannot name every reward share, keep the headline and the XP total here. Party-by-Party XP, Move unlocks, and Evolution notices stay on the [Collection and Party](collection-party.md) Progression Summary after `WORKBENCH` or after return to the Dojo.

`Tab` still opens the Battle Log during encounters. Objective completion events belong there, as in the Capture Gauge spec.

## Capture Gauge

On the Target Encounter only, the Gauge band sits between the arena and this chrome. It does not replace the action box. Prep encounters omit the band. When Gauge reaches 100, chrome line 1 becomes the `captured` line and the Target Encounter ends. There is no capture button.

## Rejected arrangements

A full-page Dojo text panel for recovery and results (early prototype A) put summaries in a different reading region than Battle. That is the thing this decision removes.

Spatial Dojo (prototype B) kept the notice-board floor for the board, then had to leave the floor for every fight so the Gauge could not hide. The board and the run did not share one chrome.

Run dossier (prototype C) pinned Gauge and Collection on a left rail. That rail stole columns from the arena until the sprites no longer matched the live Battle screen.

## Verification target

The implementation that follows this decision must render, at 100x32, the board, launch, a Preparation Encounter, recovery, a Target Encounter with the Gauge band, `captured` with `Collection +1` in the chrome, `hunt_failed`, abandon, and reconnect, all using the same four-row action box. Family names on the board cards and HP plates may repeat chrome line 1. Body copy must not appear in a second text region above that box.
