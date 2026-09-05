# Dojo Master modes and bot behavior - decided (TERM-45)

Master Sable replaces the current single Chippunk Practice fight with three distinct solo modes: two required Capture Lessons, repeatable Sparring at three visible tiers, and one standardized Daily Challenge. Every Dojo Opponent uses the same Battle Action and Replacement interfaces as a Trainer and obeys the public-information boundary in [Three-Monster Battle contract](party-battles.md). Delete Practice when the 1v1 engine is replaced. Open Lessons before Sparring and Daily, as specified in [Implementation rollout](implementation-rollout.md).

## Shared bot boundary

A Dojo Opponent may inspect only:

- the public Battle snapshot, including active Species, public HP, revealed Moves, roster health, and resolved events;
- its own private Party, HP, Battle Loadouts, and scenario instructions;
- the selected mode, published tier, snapshotted Server Day, and injected random source.

It cannot inspect the Trainer's pending hidden Battle Action, unrevealed Battle Loadout, persistent data outside the Battle snapshot, future accuracy or damage rolls, or the engine's next random value. An unrevealed player Move is modeled only as an unknown candidate from public Species content, never as the Trainer's actual hidden Loadout.

The policy receives a complete set of legal Move, Switch, or Replacement actions and returns one action plus a structured Decision Explanation. The action is locked through the same engine interface as a Trainer action. The server owns policy invocation and timing; the engine remains synchronous and policy-free.

Every mode has no Decision Clock while the Trainer remains connected. The existing 60-second same-process reconnect grace applies. Disconnect expiry, explicit forfeit, or process restart ends the current attempt without an unfinished-activity reward.

## Capture Lessons

The old standalone onboarding Practice fight is removed from the required path. After the starter joins, Master Sable introduces himself, explains that a Full Party of three is required before Queue or Challenge, and launches the two authored Capture Lessons before free Lobby play:

1. The first Lesson teaches Move selection, Speed, HP, Type matchups, and the Capture Gauge. Its starter-aware target becomes the second owned Monster.
2. The second Lesson starts with those two owned Monsters, teaches voluntary Switch and mandatory Replacement, and captures the third Monster needed for a Full Party.

Targets come from a curated starter-specific table in [Dojo Master policy and teams](dojo-policy.md). The two additions give the starting Party three Types and at least five super-effective answers on the Type chart. Lessons do not pick targets dynamically at runtime.

Lessons may reveal the Dojo Opponent's intent before action selection because teaching is their purpose. The prompt names the concept and the relevant public fact without selecting an action for the Trainer. After a failed attempt:

- fully heal and revive both sides and restart only the current Lesson;
- keep a capture already committed by the earlier Lesson;
- apply no loss record, item cost, XP reward, or other penalty;
- after the first failure, highlight the relevant matchup or Battle rule;
- after the second failure, recommend one legal action while leaving selection to the Trainer.

Each Lesson's first successful completion pays 90 base XP through the normal active and reserve shares. The newly captured Monster starts at Level 1 and receives none of the completed Lesson's XP. Replaying a completed Lesson is optional and pays no XP or additional capture.

## Sparring

Sparring replaces the legacy no-XP Practice surface. It uses the Trainer's snapshotted natural Party and persistent Levels. The Trainer chooses a published Sparring Tier, reviews the complete opponent roster and rules, then confirms with the current Party order locked for that attempt.

### Party-aware roster

The roster builder rotates through a curated Family pool and matches each player slot's persistent Level and Evolution stage. Across the three corresponding slots it must produce exactly one favorable, one neutral, and one unfavorable Type matchup from the Dojo Opponent's perspective. It cannot improve stats, Levels, Move availability, or matchup count for a higher tier. Difficulty comes from policy quality.

The ordered Party Type signature and UTC Server Day select one deterministic roster shared by Apprentice, Rival, and Master. After clearing a tier that day, the Trainer may rotate to another balanced roster for no-reward practice. Pool Families, rotation, slot assignment, and stage matching are in [Dojo Master policy and teams](dojo-policy.md).

The preview shows all three opposing Species, their slot order, Levels, Evolution stages, roster rotation, and the selected tier's policy summary. Starting Sparring snapshots both Parties, so changing the persistent Party later cannot change an active attempt.

### Visible tiers

| Tier | Policy contract | First clear XP per Server Day |
| --- | --- | ---: |
| Apprentice | Scores Moves with the existing effectiveness weights of 3.0 for super-effective, 1.0 for neutral, and 0.5 for resisted. It may Switch only from a public Type disadvantage into a healthy reserve that is not disadvantaged. Weighted choice keeps it readable and imperfect. | 65 |
| Rival | Scores every legal Move and Switch by one resolved turn of expected damage, KO chance, incoming survival, and resulting matchup. It samples from actions within 15% of the best score. | 90 |
| Master | Uses bounded two-turn expectimax over public legal-action possibilities, expected accuracy, crit, and variance rather than future random values. It samples from actions within 5% of the best score. | 130 |

Every tier uses the same public-state restriction and injected randomness. A forced Replacement chooses the healthy reserve with the highest public one-turn position score for that tier; tied candidates use the injected random source.

A clear requires defeating all three Dojo Opponent Monsters. Loss, forfeit, disconnect expiry, and incomplete attempts pay no XP. Sparring snapshots the Server Day when the attempt starts; a completion after UTC midnight still belongs to that day. Each tier's first clear for that Server Day pays its listed base XP using normal active and 40% reserve shares. Later clears that day remain fully replayable but pay no XP. Sparring never changes multiplayer W/L totals.

## Daily Challenge

The Daily Challenge is a standardized tactical puzzle rather than a natural-Party duel. The Server Day is the UTC calendar date captured when the attempt starts. Every Trainer on that Server Day receives identical loaned Parties, Loadouts, starting order, objective, par, opponent policy, and random seed.

Each loaned Party slot maps to the same slot in the Trainer's snapshotted persistent Party. If the loaner enters and resolves at least one turn, the mapped Monster receives the active share; an unused loaner maps to the 40% reserve share. Loaned Species and Moves are never added to the Collection or Move Library.

The deterministic seven-day cycle contains:

1. **Type Read:** win while satisfying the published super-effective matchup objective.
2. **Safe Switch:** win after switching from a disadvantaged matchup into a safe reserve.
3. **Full Rotation:** win after every loaned Monster resolves at least one turn.
4. **Tempo:** win within the published turn par.
5. **Preservation:** win while keeping the published number of loaned Monsters healthy.
6. **Limited Toolkit:** win while obeying the published Move-use restriction.
7. **Master Trial:** combine matchup, Switch, participation, and turn constraints in one finale.

The exact teams, restrictions, par values, seeds, and Daily opponent bands are in [Dojo Master policy and teams](dojo-policy.md). They cannot depend on an unseeded critical hit, miss, or hidden action.

The first objective clear for a Server Day pays 180 base XP through the mapped active and reserve shares. Meeting par records a Mastery Mark and grants no extra XP. A Trainer may retry without limit to earn the Mark or practice the puzzle, but duplicate objective clears cannot repay XP. An attempt completed after UTC midnight still belongs to the Server Day snapshotted at its start.

## Decision explanations and terminal feedback

Lessons may show intent before selection. Sparring and Daily Challenges preserve hidden actions and explain a choice only after it resolves.

The Battle view shows one short primary reason, for example `Switched: resisted your revealed Thermal pressure`. Reason codes and score factors are in [Dojo Master policy and teams](dojo-policy.md). The Battle Log stores the policy tier, legal actions considered, normalized action scores, selected action, primary factors, and seeded tie or near-best selection. It never records or renders information the policy was forbidden to inspect.

The Dojo interaction must expose:

- Lesson completion and current hint step;
- Sparring tiers, today's first-clear status, XP value, roster preview, and policy summary;
- today's Daily objective, par, first-clear status, Mastery Mark, and mapped owned Party slots;
- a result summary separating XP, capture, first clear, replay, and Mastery outcomes.

The existing battle arena, sprites, HP presentation, Move grid, and Battle Log remain the base interface. Mode status and the primary Decision Explanation fit in the narration and lower status regions rather than replacing the Battle screen.

## Persistence and idempotency

[Progression persistence](progression-persistence.md) stores:

- completion of each required Capture Lesson and its idempotent capture result;
- Sparring first clears keyed by Trainer, tier, and Server Day;
- Daily first clear and Mastery Mark keyed by Trainer and Server Day;
- the activity result identity and mapped participation used for XP.

An activity result commits capture, XP, first-clear state, and Mastery state atomically. Reconnect replay and duplicate result delivery cannot award a second capture, XP packet, first clear, or Mark.

## Required proof

Implementation must cover at least:

- both Capture Lessons for each starter profile, including first and second failure hints;
- capture and XP idempotency across reconnect and duplicate completion;
- legal Move, Switch, and Replacement choices for all three policies;
- public-state isolation, including an unrevealed player Move and pending hidden Battle Action;
- Apprentice weighting, Rival 15% near-best selection, and Master bounded two-turn selection under an injected random source;
- roster construction matching Levels and Evolution stages with one favorable, neutral, and unfavorable slot;
- Sparring first-clear reward and no-XP replay across a UTC boundary;
- all seven Daily objective evaluators, fixed seeds, slot-to-owned-Monster XP mapping, par-only Mastery, and unlimited replay;
- Decision Explanations containing only permitted inputs;
- no Dojo mode changing multiplayer W/L totals;
- same-process reconnect grace and no reward after forfeit, timeout, or process restart.

Exact Species rosters, scenario values, policy score coefficients, reason codes, and Daily par values are in [Dojo Master policy and teams](dojo-policy.md). They must pass [Gameplay balance methodology](balance-methodology.md) Balance Runs before those numbers change.
