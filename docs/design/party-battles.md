# Three-Monster Battle contract

Status: implemented.

This contract extends the direct-damage combat rules in [combat.md](combat.md). The damage formula, accuracy, Type effectiveness, critical hits, variance, Speed ordering, event-log-driven clients, and sequential Move resolution remain unchanged unless this document says otherwise. Replace the 1v1 engine with this contract, in the order given by [Implementation rollout](implementation-rollout.md). Do not keep a parallel 1v1 engine.

## Entry and Party rules

- Every Trainer-versus-Trainer Battle is a Normalized Battle between two Full Parties of exactly three owned Monsters.
- This rule applies equally to global Queue matches and direct Challenge matches. There are no separate 1v1 or 2v2 PvP formats.
- A new Trainer chooses one starter and completes two Capture Lessons before PvP unlocks. The resulting three owned Monsters form the first Full Party; PvP never fills missing slots with loaners.
- Solo modes may construct scenarios with one to three Monsters per side. Their mode contracts decide the exact Party composition.
- Every Monster starts a new Battle at full HP. Damage persists when a Monster switches out, but no Battle HP persists after that Battle ends.

## Normalization

Queue and direct Challenge Battles use the same battle-only normalization. It never writes to a Monster's persistent Collection record, Move Library, Battle Loadout, XP, Level, pending Evolution, or Party order.

- The battle copy uses `QueueLevel = 30` for level-dependent calculations. A pending Evolution remains pending and cannot be triggered by entering Queue.
- Calculate each Monster's natural stats at Level 30, then rescale the five stats to the fixed `QueueStatBudget = 320`, the existing middle-stage stat total. A deterministic largest-remainder allocation preserves role proportions, keeps every stat at least 1, and makes the sum exactly 320.
- A Normalized Battle copies each Monster's persistent Battle Loadout. Every selected Move must be Queue-eligible at Level 30; otherwise Queue entry directs the Trainer to adjust the Loadout in the Workbench. The pre-Queue screen changes only roster membership and opening order.
- Species, Type, Evolution stage, role distribution, and selected Moves remain meaningful. Solo Expeditions and Dojo modes use natural persistent Levels instead of this copy.

Both sides receive XP from the persistent Monsters and the completed Battle Result, using the [XP, level curve, and normalized PvP](xp-progression.md) packet rules rather than normalized stats.

## Public and private information

At Battle start, both Trainers can see:

- all three opposing Species;
- their order-independent healthy or fainted status;
- the opposing active Monster, current HP, and maximum HP;
- Moves after those Moves have been used.

An opposing Monster's Battle Loadout and pending Battle Action remain hidden. A Trainer always sees their own Party order, HP, fainted status, and Battle Loadouts.

How that information is laid out in the terminal is [Three-Monster Battle terminal view](party-battle-view.md).

Party slot one is the opening active Monster. Trainers choose that lead by ordering the Party before entering PvP; there is no post-matchmaking lead-selection phase.

## State machine

```text
revealing
    |
    v
awaiting_actions
    |
    v
resolving_turn
    |-- no faint ----------------------> revealing
    |-- faint + healthy reserve ------> awaiting_replacement --> revealing
    `-- faint + no healthy reserve ---> battle_over

awaiting_actions | awaiting_replacement
    |-- forfeit -----------------------> battle_over
    |-- decision clock exhausted -----> battle_over
    `-- reconnect grace exhausted ----> battle_over
```

`revealing` is a bounded, server-owned playback window. Neither Decision Clock runs and no Battle Action is accepted until the window ends. Its duration is a combat tuning constant rather than client-reported readiness, so a client cannot stall the Battle by withholding an acknowledgement.

## Battle Actions

During `awaiting_actions`, each Trainer submits exactly one immutable hidden Battle Action:

- `Move`: use one Move from the active Monster's Battle Loadout; or
- `Switch`: replace the active Monster with one healthy reserve Monster.

A Trainer cannot select the active Monster, a fainted Monster, an absent Party slot, or a Move outside the active Monster's Battle Loadout. Once locked, the Battle Action cannot be changed. The opponent may know that an action is locked but never its kind or target.

The second lock resolves the turn synchronously:

1. All Switch actions resolve before any Move. If both sides switch, both switches are mechanically simultaneous and the event log uses stable engine-side order only for playback.
2. A Move selected against a switching opponent targets the incoming Monster.
3. When both sides selected Moves, the active Monster with higher Speed acts first; a tie uses the existing injected coin flip.
4. Each Move resolves completely before the next Move begins.
5. If the first Move makes the opposing active Monster faint, that Monster's locked Move is canceled.
6. If both sides switched, or all surviving Moves finish without a faint, the Battle enters `revealing` before the next action phase.

Direct damage remains sequential. Status effects, recoil, simultaneous attacks, and other sources of a double faint remain out of scope, so this contract has no draw path.

## Fainting and Replacement

When an active Monster faints:

- its HP remains zero and it cannot return during that Battle;
- if its Party has a healthy reserve, the Battle enters `awaiting_replacement`;
- only the affected Trainer selects a Replacement;
- the surviving Trainer cannot Move, switch, or pre-lock the next action;
- the Replacement is revealed and sent out, then a fresh `awaiting_actions` phase begins;
- if no healthy reserve exists, the opposing Trainer wins the Battle.

The surviving Trainer receives no free counter-switch and no preview advantage beyond the Species roster already public at Battle start.

## Decision Clock

Decision Clocks apply to Queue and direct Challenge Battles only.

- Each Trainer starts with 60 seconds.
- At the start of every `awaiting_actions` or applicable `awaiting_replacement` phase, that Trainer gains 10 seconds up to a 60-second cap.
- A Trainer's clock runs only while the Battle requires an unlocked choice from that Trainer.
- Locking an action stops that Trainer's clock.
- Both clocks pause during `revealing`.
- Exhausting the clock loses the entire Battle with the distinct `decision_timeout` end reason.

Expeditions, Lessons, Sparring, and Daily Challenges have no Decision Clock while the Trainer remains connected.

## Disconnect and replay

- Every mode retains its exact in-memory Battle for the existing 60-second reconnect grace.
- The reconnect grace runs independently from the Decision Clock; the disconnected Trainer's clock is paused.
- An action locked before disconnect remains immutable. If the opponent subsequently locks, the turn resolves normally.
- When new input from the disconnected Trainer is required, the Battle waits in its current action or Replacement phase until reconnect or grace expiry.
- Reconnect restores active slots, every Monster's HP and fainted status, the pending phase, Decision Clock banks, and accumulated events. A bounded catch-up reveal window runs before a required clock resumes.
- The opponent sees the existing reconnecting banner. Grace expiry loses the entire Battle with `disconnect_timeout`.
- Active Battles remain process-local. A process restart ends them without resumable Battle state; the production durability contract remains unchanged.

## Forfeit and terminal outcome

A Trainer wins by making all three opposing Monsters faint. Explicit forfeit, Decision Clock expiry, and reconnect-grace expiry lose the whole Battle regardless of remaining healthy Monsters.

Solo modes do not write multiplayer win/loss totals. Their later reward contracts decide what an incomplete or lost activity grants.

## Bot contract

Bots submit the same Move and Switch actions and use the same Replacement phase as Trainers. A bot policy may inspect only public Battle state plus its own private Party and Battle Loadouts. It cannot inspect the Trainer's pending hidden action. [Dojo Master modes and bot behavior](dojo-master.md) defines the Lesson, Apprentice, Rival, Master, and Daily policies and their Decision Explanations.

## Snapshots and events

The authoritative engine owns complete Party state rather than one `Side.Monster`. Every Battle-facing snapshot must be viewer-specific so private Loadouts and pending actions cannot leak.

Snapshots identify each Party member by stable Monster identity and slot and include Species, display name, active status, HP, fainted status, and only the Moves visible to that viewer. They also include the current phase, lock status, public roster, and Decision Clock state.

Events identify the acting Trainer, Monster, and Party slot, plus the target Monster when applicable. The event vocabulary extends the existing log with Battle start/send-out, switch, Replacement, and Decision Clock expiry events. Hidden selections produce no public event until resolution. Existing Move, damage, effectiveness, faint, forfeit, disconnect, and Battle-over events remain ordered and replayable.

The engine remains a synchronous domain module with injected randomness. The server owns wall-clock deadlines, reconnect scheduling, mode policy, bot invocation, and result persistence. TUI clients render viewer-specific snapshots and ordered events without reproducing Battle rules.

Normalized content and policy tuning use the paired seeds, Reference Teams, team win-rate bands, counterplay cases, and replay artifacts defined by [Gameplay balance methodology](balance-methodology.md). Individual one-on-one Type counters are diagnostics rather than the primary competitive acceptance unit.

## Required proof

Implementation must cover at least these deterministic scenarios:

- Move versus Move, Move versus Switch, and Switch versus Switch;
- invalid and duplicate selections;
- first-Move faint canceling the fainted Monster's action;
- mandatory Replacement with no opponent counteraction;
- damage preservation across multiple switches;
- victory only after the third faint;
- forfeit, Decision Clock expiry, and disconnect expiry with healthy reserves remaining;
- reconnect while awaiting an action, after locking, during reveal, and during Replacement;
- viewer snapshots never exposing an opposing Loadout or pending action;
- bot actions using public state only;
- event identity and replay across repeated switches and faints;
- relevant engine and server packages passing the race detector.
