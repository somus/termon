# Dojo Master policy and teams - decided (TERM-55)

This is the content reference for [Dojo Master modes and bot behavior](dojo-master.md). It pins Capture Lesson targets, the Sparring roster pool and builder, policy coefficients, Decision Explanation reason codes, and the seven Daily Challenge fixtures.

Mode rules, XP, and the public-information boundary stay on that page. Capture Gauge IDs stay in [Capture Objective catalog](capture-catalog.md).

## Capture Lessons

Each starter has two authored targets. Lesson 1 is a one-Monster capture that teaches Moves, Speed, HP, Type, and the Gauge. Lesson 2 starts with the starter plus the first capture, teaches Switch, and captures the third Monster for a Full Party.

| Starter | Lesson 1 target | Lesson 2 target | Resulting Types |
| --- | --- | --- | --- |
| Rootkit | Mistcache | Chippunk | organic, coolant, silicon |
| Emberbyte | Sproutware | Zaplet | thermal, organic, current |
| Aquabit | Wickware | Spamlet | coolant, thermal, virus |

Lesson 1 is always super-effective for the starter. Lesson 2 is not super-effective for the starter and is super-effective for the Lesson 1 capture, so a voluntary Switch is the teaching line. The three Full Parties together include every Type. Each Party's STAB covers five of the six defender Types.

Authored objectives, not the Family generator:

| Lesson | Objectives |
| --- | --- |
| 1 | `show_move_variety`, `read_the_matchup`, `hold_the_line` |
| 2 | `show_move_variety`, `safe_switch`, `read_the_matchup` |

The wild target uses the Target Encounter Capture HP formula and wild damage clamp. Filling the Gauge captures and completes the Lesson. Knocking the target out first fails the attempt and retries the current Lesson. The success line uses minimum non-critical damage and does not require a miss, critical, or hidden action.

Lesson 2's success line never requires Replacement. If the active Monster faints, Replacement is mandatory and the Lesson remains completable with the remaining Monster. Intent before selection names Switch on Lesson 2 and names Replacement as the faint rule. After the first failure, the hint names the public matchup. After the second failure, it recommends one legal action and does not lock it.

The Dojo Opponent in both Lessons is a single Wild Monster using the Apprentice policy with no Switch (it has no reserve). Replaying a completed Lesson pays no XP and does not capture again.

## Sparring roster pool

Sparring rotates through every authored Evolution Family while preserving the same matchup and power budget across tiers. Each Type has this curated Family pool:

| Type | Families |
| --- | --- |
| organic | Mossmuff, Rootanami, Rootkit, Sproutware, Thornpatch |
| thermal | Cindernode, Emberbyte, Scorchip, Wickware |
| coolant | Aquabit, Flowcell, Gushkit, Mistcache, Splashscreen |
| current | Amperent, Joulpup, Surgetail, Zaplet |
| virus | Bloatware, Spamlet, Wormate |
| silicon | Chippunk, Coghound, Servoboar |

### Builder

Slots correspond one-to-one with the Trainer's snapshotted Party. From the Dojo Opponent's view, slot 1 is favorable, slot 2 is neutral, and slot 3 is unfavorable. The preview shows that assignment before the Trainer confirms.

A Type is favorable when the Dojo Species' Type is super-effective versus the player's slot Type. It is unfavorable when the player's Type is super-effective versus the Dojo Species. It is neutral when neither side is super-effective, including same-Type.

For each slot, collect every legal pool Family and sort the Family slugs. An offset derived from the ordered Party Type signature and slot starts that list; the UTC Server Day advances the offset once per day. Scan cyclically from the offset and select the first Family that isn't already used. If every legal Family is used, reuse the Family at the offset as a second individual.

The default daily roster is identical across Apprentice, Rival, and Master, so tier difficulty comes only from policy quality. After the first clear of a tier that day, the Trainer may request a practice remix. Each remix advances the same candidate rotation and pays no additional XP.

The Dojo Species' Evolution stage matches the player's Monster in that slot, not the pool Family's own threshold. The Dojo Level equals that slot's persistent Level. Loadouts use the Reference Loadout rule from [Gameplay balance methodology](balance-methodology.md).

Boot validation builds a legal roster for all 216 Type triples and rejects an unknown, duplicate, mistyped, or incomplete pool Family.

## Policy coefficients

All three Sparring tiers share legal-action enumeration, the public-state boundary, and an injected random source. They do not share a near-best band. Daily Challenges override the band as specified below.

Expected damage for scoring uses the combat formula with `NaturalStat`, hit chance `accuracy / 100`, critical chance `1/16`, and mean variance `0.925`. Unknown opponent Moves are the current Species' level-legal Movepool entries, never the Trainer's hidden Loadout. `P_ko` is `clamp(E[damage] / current_HP, 0, 1)`. Incoming survival is `1 - clamp(E[incoming] / self_HP, 0, 1)`. Matchup value is `+1` when the Dojo Type is super-effective versus the player, `-1` when the reverse is true, otherwise `0`.

### Apprentice

Move weight is `3.0` super-effective, `1.0` neutral, `0.5` resisted, matching the existing Practice weights. The policy may add a Switch with weight `1.0` only when the player's active Type is super-effective versus the Dojo active Type and the chosen reserve is not similarly disadvantaged. It samples in proportion to those weights. Replacement picks the healthy reserve with the highest Apprentice move-weight against the player's active Type; ties use the injected source.

### Rival

Score every legal Move and Switch on one resolved turn:

```text
score = 1.00 * E[damage] / opponent_max_HP
      + 0.50 * P_ko
      + 0.60 * incoming_survival
      + 0.25 * matchup_value
      - 0.80 * P_self_faint
```

A Switch uses zero outgoing damage this turn and evaluates incoming survival and matchup on the incoming Monster. The policy samples uniformly from actions whose score is at least `0.85` times the best score (the 15% band). Ties inside the band use the injected source.

### Master

Master uses the Rival one-turn terms, then adds a second turn. After the candidate action, the opponent is modeled as taking their best Rival one-turn reply among public legal actions. Master then takes its own best one-turn follow-up. The published score is:

```text
score = rival_one_turn(action)
      + 0.45 * E[rival_one_turn after the modeled reply]
```

The policy samples uniformly from actions whose score is at least `0.95` times the best score (the 5% band). It never reads future random values. Replacement uses the same two-turn score on each healthy reserve.

These coefficients nest: Apprentice reads Type weights, Rival reads one-turn outcomes, Master reads a bounded two-turn tree. Adjacent Sparring tiers must keep the win-rate gap in [Gameplay balance methodology](balance-methodology.md). Changing a coefficient is a reviewed balance edit; it must not change Levels, stats, or matchup budgets between tiers.

## Reason codes

The Battle view shows one primary reason. The Battle Log may list codes and normalized scores for every legal action considered. Codes are stable strings. They must not name hidden player Moves, pending Battle Actions, or unseeded future rolls.

| Code | When it is the primary reason |
| --- | --- |
| `move_se` | Chosen Move has Type effectiveness at or above `2.0` |
| `move_ko` | Chosen Move has the highest `P_ko` among legal Moves |
| `move_damage` | Chosen Move has the highest expected damage |
| `move_survive` | Chosen action maximizes incoming survival |
| `switch_matchup` | Switch improves matchup value |
| `switch_disadvantage` | Apprentice Switch out of a public Type disadvantage |
| `replace_score` | Forced Replacement using the tier's position score |
| `near_best` | Selected from the near-best band, not the unique top score |
| `tie_seed` | Injected random source broke a remaining tie |

The primary reason is the first matching code in that table for the selected action. Lessons may also show intent text before selection. Sparring and Daily Challenges explain only after the action resolves.

## Daily Challenge fixtures

The Server Day index `floor(unix_utc / 86400) % 7` selects one archetype. Every Trainer on that snapshot receives the same loaned Parties, default four-Move loadouts, Level 20, middle Evolution stage when the Family's threshold is at most 20 else base stage, starting order, objective, par, opponent policy, and seed.

Daily opponent policy uses the Rival or Master score formula with a **0% near-best band**: it always takes the unique best action and uses the seed only on true ties. That keeps par reproducible. Sparring keeps the 15% and 5% bands.

Loaned slots map to the Trainer's snapshotted persistent Party in order. Species and Moves are never written to the Collection.

| Day | ID | Player lead order | Opponent lead order | Objective | Par (turns) | Opponent score | Seed |
| ---: | --- | --- | --- | --- | ---: | --- | ---: |
| 0 | `type_read` | Emberbyte, Rootkit, Aquabit | Mossmuff, Bloatware, Servoboar | Win after resolving one Move with Type effectiveness at or above `2.0` | 10 | Rival, 0% band | 55001 |
| 1 | `safe_switch` | Rootkit, Aquabit, Emberbyte | Emberbyte, Cindernode, Scorchip | Win after a voluntary Switch from a disadvantaged active into a reserve that is not disadvantaged | 10 | Rival, 0% band | 55002 |
| 2 | `full_rotation` | Thornpatch, Gushkit, Joulpup | Flowcell, Amperent, Bloatware | Win after every loaned Monster resolves at least one turn | 12 | Rival, 0% band | 55003 |
| 3 | `tempo` | Scorchip, Wickware, Zaplet | Mossmuff, Bloatware, Servoboar | Win | 8 | Rival, 0% band | 55004 |
| 4 | `preservation` | Rootanami, Flowcell, Thornpatch | Gushkit, Joulpup, Sproutware | Win with at least two loaned Monsters healthy | 10 | Rival, 0% band | 55005 |
| 5 | `limited_toolkit` | Chippunk, Spamlet, Mistcache | Wormate, Cindernode, Rootkit | Win while using only Moves with Power at most 65 | 12 | Rival, 0% band | 55006 |
| 6 | `master_trial` | Emberbyte, Aquabit, Rootkit | Thornpatch, Scorchip, Flowcell | Win after one super-effective Move, one voluntary Switch, and every loaned Monster resolving a turn | 14 | Master, 0% band | 55007 |

A legal line exists for each objective under its seed without a required critical or miss. A legal line also exists that wins the Battle but misses par or the extra objective, so the Mastery Mark is not automatic. `tempo` treats any win as the objective clear; turns at or below par earn the Mark. `limited_toolkit` treats a Power-above-65 Move as an illegal Daily action; the engine still offers only legal Moves from the filtered set.

Unlimited replay keeps the same seed and loaned teams. Duplicate objective clears pay no XP. A later par-only clear still records the Mark.

## Content shape

Implementation stores these values as content validated at boot, not as literals inside the policy:

```text
content/dojo/lessons.json          starter -> lesson_1, lesson_2, objectives
content/dojo/sparring-pool.json    type -> families
content/dojo/policy.json           apprentice weights, rival and master coefficients, bands
content/dojo/daily/<id>.json       parties, level, objective id, par, seed, policy band
```

Unknown Family, missing pool Type, duplicate Daily ID, or a pool that cannot build F/N/U for a legal Type triple must refuse to start the server.

## Implementation notes

Sparring win-rate bands remain a Balance Run gate. This specification authors the teams and coefficients those runs must use. Lessons and Dailies must pass focused tests for the scripted success line, one injected miss, one failure path, reconnect idempotency, and Decision Explanations that contain only permitted inputs.
