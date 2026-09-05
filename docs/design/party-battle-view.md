# Three-Monster Battle terminal view - decided (TERM-52)

Queue and Challenge Battles keep the current arena: sprites, HP plates, narration, Move grid, and TYPE pane. The Party contract from [Three-Monster Battle](party-battles.md) is added as two always-on roster strips stacked around that arena, not as a second layout.

This document specifies presentation. It does not change hidden-action rules, Decision Clocks, or what a snapshot may contain.

The production Battle view uses the accepted Layout A arrangement.

## Stacked frame at 100x32

The supported minimum stays 100 columns by 32 rows. Smaller terminals keep the existing too-small warning. Roster UI is never painted through sprite art.

```text
foe roster strip
┌ foe plate ┐                         foe sprite
│ name  HP  │
└───────────┘
you sprite                         ┌ you plate ┐
                                   │ name  HP  │
                                   └───────────┘
you roster strip
┌ chrome: prompt or moves or waiting or send-out ┐
```

The foe strip sits above the arena. The you strip sits between the arena and the four-row chrome. Clocks live on those strips so plate size stays as it is today.

`Tab` still opens the Battle Log. The log remains the place for full event playback, including send-out, Switch, and Replacement.

## Roster strips

Both strips stay visible for the whole Battle, including reveal playback and Replacement.

**Your strip** uses Party order. Each occupied slot shows a healthy or fainted mark, the slot number, the display name, and current HP. Slot 1 is the opening lead. An empty solo slot is omitted rather than drawn as a blank PvP vacancy.

**The foe strip** lists the three public Species with healthy or fainted marks. It does not show Party slot numbers, reserve HP, or unused Moves. The active Species is already named on the foe plate; the strip still includes it so the full public roster is one line.

Marks are `●` healthy and `×` fainted. A fainted name stays on the strip until the Battle ends.

When you lock, your strip shows `LOCKED` plus the kind (`MOVE` or `SWITCH`). The foe strip shows only `LOCKED`. The chrome becomes `Waiting for opponent…` and does not name your action again.

## Commands

The two-row chrome grows a third root command. The three labels share one row so the chrome stays four rows tall:

`FIGHT` `SWITCH` `RUN`

- `FIGHT` opens the existing four-Move grid and TYPE pane.
- `SWITCH` lists healthy reserves. The active Monster and fainted Monsters are listed as unselectable. Locking a Switch hides its target from the opponent.
- `RUN` forfeits, same as today's forfeit path.

During `awaiting_replacement`, the chrome is a send-out list for the affected Trainer only. The surviving Trainer sees waiting chrome and cannot pre-lock. After the Replacement is revealed, a fresh `FIGHT` / `SWITCH` / `RUN` phase begins.

Reveal playback uses the existing narration box and sprite poses. Clocks on both strips pause for that window.

## Decision Clocks

Queue and Challenge clocks render on the roster strips as `m:ss`. Your clock pulses when it is at or below 10 seconds and still running. A locked or paused clock is shown but does not pulse. Solo Expeditions, Lessons, Sparring, and Daily Challenges omit the clock digits.

The existing reconnecting banner still appears above or in the chrome. It is separate from the lock badge.

## Solo sides smaller than three

Lessons, Sparring, Expeditions, and Daily Challenges may field one to three Monsters per side. Each strip lists only the Species that side actually sent. A one-Monster Lesson does not draw two empty foe marks.

## Rejected arrangements

Dual benches (prototype B) spent the width on party columns and shrank the arena until the approved sprites no longer matched the live 1v1 screen.

Overlay trays (prototype C) kept the arena almost unchanged but reduced the public foe roster to `●●×` ticks. That hid Species names the contract already makes public at Battle start.

## Verification target

The implementation that follows this decision must keep viewer-specific snapshots: foe reserve HP, unused Loadouts, and pending action kind never appear on the foe strip or in waiting chrome. It must also exercise, at 100x32, the root command row, Switch selection, lock-without-kind, Replacement send-out, paused clocks during reveal, and a solo one-Monster strip.
