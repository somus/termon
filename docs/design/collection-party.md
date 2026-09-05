# Collection and Party terminal flow - decided (TERM-47)

Monster management uses a battle-adjacent Workbench at Termon's supported 100x32 minimum. It keeps the owned Collection, selected Monster, and ordered Party visible together, so changing a loadout or Party slot never loses the context that slot 1 is the opening lead. The full Species Index is a secondary tab rather than the primary management surface.

This document specifies presentation and interaction against the lifecycle in [Individual Monster progression](progression.md). Persistence is [Progression persistence](progression-persistence.md). It does not change progression or persistence rules.

## Entry and screen contract

Outside Battle, `p` opens Monster management from the Dojo. The retired Practice shortcut no longer owns that key; Dojo Master activities open by interacting with Master Sable. `Esc` returns to the Dojo. Active Battles, action reveals, and mandatory Replacement never open the Workbench. Remap `p` in slice 1 of [Implementation rollout](implementation-rollout.md).

The normal Workbench has three persistent panes inside the existing Termon page chrome:

```text
┌ Collection ─────┐┌ Selected Monster ─────────────────┐┌ Party ──────────┐
│ owned individual││ name, Species, Type, Level and XP ││ 1  LEAD         │
│ owned individual││ sprite, natural stats, next gains ││ 2               │
│ owned individual││ current contextual editor         ││ 3               │
└─────────────────┘└───────────────────────────────────┘└─────────────────┘
```

- **Collection pane:** one row per owned individual, never one row per Species. A row shows nickname or Species name, Level, Party slot when selected, and an attention mark for an unreviewed Move unlock or pending Evolution. Duplicate Species remain separate rows.
- **Selected Monster pane:** shows the individual identity through its nickname and Species, current Type, Level, XP progress, natural stats, Family path, sprite, current Battle Loadout, and the action or editor currently in focus. The opaque Monster ID is not user-facing.
- **Party pane:** always shows slots 1 through 3 in order, including empty slots. Each occupied slot shows the Monster, Level, Type, and compact Loadout summary. Slot 1 is labeled `LEAD` everywhere rather than relying on position alone.

At widths below 100 or heights below 32, the screen shows the existing minimum-terminal warning instead of collapsing panes until actions become ambiguous. The palette, borders, sprite rendering, selected cursor, Type colors, footer hints, and page chrome reuse the existing Battle and Dojo visual language.

## Browsing an uncapped Collection

The default Collection order is newest capture first. Party membership does not move an individual in the Collection. The cursor follows stable Monster identity when sorting, filtering, accepting Evolution, or changing Party order.

The Collection pane supports:

- `/` to search nickname or Species name;
- `f` to cycle `ALL`, `PARTY`, `BENCH`, and `ATTENTION` filters;
- `s` to cycle `RECENT`, `LEVEL`, and `SPECIES` sorting;
- arrows or `j`/`k` to move the selected individual;
- `Enter` to focus the selected Monster's actions;
- `Tab` to switch between `COLLECTION` and `SPECIES INDEX`.

An empty filter result preserves the query and shows `No owned Monsters match` plus the clear-filter key. It never changes Party state. A captured Monster appears at the top of `RECENT`, receives an attention mark until its capture summary is reviewed, and stays on the bench until explicitly assigned.

## Species Index

The Species Index lists all 72 launch Species grouped into their 24 Evolution Families. Content is public; unseen Species are not hidden behind silhouettes. Each entry shows Type, Family stage, and owned-individual count. A Family summary shows its complete Evolution path and thresholds without implying that one owned individual grants every stage.

Selecting a Species offers `Show owned`, which returns to the Collection tab with a Species filter. An owned count of zero is informational and exposes no capture shortcut; target availability remains on the Expedition Signal Board.

## Party editing

Party assignment is slot-directed rather than a toggle followed by a separate reorder mode.

1. Select an owned Monster and choose `Party` or press `p` inside the Workbench.
2. Choose slot 1, 2, or 3 from the always-visible Party pane. Slot 1 is described as `Opening lead` in the chooser.
3. If the selected Monster is on the bench and the target slot is occupied, show `Replace <name> in slot N?`; confirming benches the replaced Monster. Cancel changes nothing.
4. If the selected Monster is already in another Party slot, choosing an occupied slot swaps the two Party positions. No Monster leaves the Party, so this reorder needs no destructive confirmation.
5. `Remove from Party` benches the selected Party Monster. The Party may contain zero to three Monsters outside an activity.

Every change applies atomically to the ordered Party. Queue and direct Challenge entry require a Full Party; an Expedition or Dojo mode applies its own Party-size rule when launched. A failed eligibility check returns to the Workbench with the exact missing requirement and does not auto-fill a slot.

## Battle Loadout editing

Choosing `Moves` replaces the center dossier body with a two-column editor while the Collection and Party panes remain visible:

- the left column shows four numbered Battle Loadout slots, including empty slots;
- the right column shows the complete Move Library in deterministic unlock order, with equipped marks, Type, category, power, and accuracy;
- the footer explains the focused Move and available keys.

Select a Library Move, then select a Loadout slot. An empty slot fills immediately. Replacing an occupied slot previews `old -> new` and confirms once. A Move already equipped cannot occupy a second slot. Removing an equipped Move is allowed only when at least one other Move remains, preserving the one-to-four-Move invariant.

Edits affect the persistent Battle Loadout, and Normalized Battles use that Loadout directly. Before matchmaking, `FIND BATTLE` permits only roster selection and opening-order changes; Move changes return to the Workbench.

## Batched progression review

A Reward Packet is persisted before UI begins. Its result screen shows one Progression Summary covering every affected Party Monster in Party order:

- XP gained and resulting XP total;
- every crossed Level;
- every newly unlocked Move;
- each newly eligible Evolution;
- active or Reserve Share when that distinction affected the reward.

The summary offers `Review N` and `Return to Dojo`. Review visits actionable Monsters one at a time without undoing the already committed reward. Returning immediately leaves attention marks in the Collection.

### Move unlock notice

The notice states that the Move was added permanently to the Move Library and that the Battle Loadout did not change. `Edit loadout` opens the normal two-column editor with the new Move focused; `Keep loadout` acknowledges the notice without equipping it. Multiple Moves unlocked by one reward appear together for the same Monster.

### Evolution notice

The notice compares current and successor Species, role and natural stats at the Monster's persistent Level, newly eligible successor Moves, and the invariants that remain: individual identity, nickname, XP, Level, Library, Loadout, and Party slot. `Evolve` opens one irreversible confirmation; `Defer` returns to the review queue without confirmation.

Deferral clears only the current reward interruption. The Monster retains an `EVOLUTION READY` mark in both Collection and dossier until Evolution is accepted. If acceptance immediately makes another stage eligible, the next Evolution appears as a separate notice rather than chaining silently.

An unreviewed Move unlock is a Progression Notice until the Trainer chooses `Keep loadout` or finishes the Move editor. Pending Evolution is derived from Monster progression and remains visible independently of notice acknowledgement. [Progression persistence](progression-persistence.md) makes acknowledgement durable so reconnecting cannot lose or resurrect an unreviewed unlock. Keep loadout deletes the notice; Edit loadout writes the Loadout and deletes the notice in one Store operation.

## Navigation states

```text
Dojo
  -> Workbench.Collection
       -> Monster.Actions
            -> Party.SlotChoice -> ReplaceConfirm -> Collection
            -> Loadout.Editor -> ReplaceConfirm -> Monster.Actions
            -> Evolution.Review -> EvolveConfirm -> Monster.Actions
       -> SpeciesIndex -> ShowOwned -> Workbench.Collection
  -> Dojo

ActivityResult
  -> ProgressionSummary
       -> NoticeReview -> MoveEditor | EvolutionReview -> next Notice
       -> Dojo
```

`Esc` always backs out one uncommitted layer. It never reverses a confirmed Party or Loadout edit, accepted Evolution, or persisted reward. Reconnect restores the last durable game state and returns to the Dojo or Workbench; it does not attempt to resume an uncommitted slot chooser or confirmation dialog.

## Prototype result

The throwaway Bubble Tea prototype compared three 100x32 layouts using the real content pack, Species sprites, Moves, Type colors, and Termon chrome:

- **Workbench:** Collection, selected dossier/editor, and Party visible together.
- **Party first:** three large Party cards above a Collection bench.
- **Field guide:** Species Index as primary navigation with contextual Party actions.

Workbench was selected because it preserves individual, progression, Loadout, and opening-lead context during every edit. The Party-first version made browsing a growing Collection secondary, while the Field-guide version optimized completion tracking over frequent individual management. The selected design takes the Species Index from Field guide as a secondary tab rather than losing it.

## Required proof

Implementation must cover at least:

- rendering at 100x32 and rejecting smaller terminals;
- duplicate Species with distinct identity, progression, loadouts, and Party membership;
- cursor identity across search, filter, sort, capture insertion, and accepted Evolution;
- assigning to an empty slot, confirmed replacement, Party-slot swap, removal, and Full Party eligibility messaging;
- filling an empty Loadout slot, confirmed replacement, duplicate rejection, and refusal to remove the final Move;
- pre-Queue roster selection and ordering without Move editing;
- multi-Monster, multi-Level Progression Summaries in Party order;
- Move unlock acknowledgement, skipped review, reconnect, and later Collection review;
- deferred and accepted Evolution, including a second immediately eligible stage;
- `Esc` behavior at every uncommitted layer and no rollback of committed state.
