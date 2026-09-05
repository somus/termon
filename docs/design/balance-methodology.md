# Gameplay balance methodology - decided (TERM-46)

Termon balances complete three-Monster teams around counterplay. Individual Species may have decisive favorable or unfavorable duels, but a legal reference team must be able to answer those matchups through Party construction, Move choice, or switching. A one-on-one result is diagnostic evidence, not an automatic balance failure.

This methodology governs Species stats, Movepools, Capture Objectives, Dojo policies, natural progression, and Normalized Battles. It changes no content by itself. Content changes require a reproducible gate failure against the versioned corpus below.

## Reproducible Balance Run

A Balance Run snapshots the content-pack revision, rules revision, simulator revision, Reference Teams, policy parameters, and a fixed corpus of 1,024 seeds. Every non-mirror scenario runs twice per seed with engine side and Party order exchanged. A run is invalid if any result depends on wall-clock time, map iteration order, an unrecorded random source, or client behavior.

The simulator uses the authoritative damage, Type, action-order, switch, faint, Replacement, normalization, and bot-policy rules. It records machine-readable per-battle results and prints a bounded terminal summary. A failed run reports the exact scenario, seed, teams, loadouts, actions, and first failed gate so the Battle can be replayed directly in a test.

Random outcomes are part of the corpus rather than averaged through unbounded Monte Carlo sampling. Changing the seed corpus is a reviewed rules change; adding a regression seed is allowed when it represents a legal state that the corpus missed.

## Reference Teams

The launch corpus starts with eight three-Family teams. A Family name resolves to the Species that the checkpoint Level and Evolution state require. Each Family appears once in the anchor set, so a globally acceptable average cannot hide a Family that was never exercised.

| Archetype | Evolution Families |
| --- | --- |
| Starter balance | Rootkit, Emberbyte, Aquabit |
| Alternate balance | Zaplet, Spamlet, Chippunk |
| Bulky control | Rootanami, Flowcell, Bloatware |
| Fast pressure | Sproutware, Wickware, Mistcache |
| Physical pressure | Thornpatch, Gushkit, Joulpup |
| Bruiser core | Cindernode, Amperent, Coghound |
| Mixed endurance | Mossmuff, Splashscreen, Surgetail |
| Specialist pressure | Scorchip, Wormate, Servoboar |

Every team runs with each of its three Monsters as lead. The corpus also creates one counterplay case per Family: that Family starts in a clearly unfavorable Type matchup while a healthy reserve has a favorable matchup. These cases measure whether switching provides a real answer rather than whether the disadvantaged active Monster can win alone.

At each checkpoint, a deterministic Reference Loadout selects up to four currently eligible Moves: the strongest neutral physical option, strongest neutral special option, most accurate option, and earliest-unlocked option, with duplicate choices removed and remaining slots filled by unlock level then Move slug. Balance work may add an authored loadout only to cover a distinct legal strategy; it cannot silently replace an anchor loadout that fails.

Three public-state policies exercise each team:

- **Pressure** selects the legal action with the best immediate expected damage.
- **Pivot** selects a Switch when it materially improves the next-turn survival and damage outlook, otherwise it uses the best immediate action.
- **Preservation** values keeping healthy reserves and avoiding a likely faint before immediate damage.

Each policy is deterministic before its injected tie-break. A balance conclusion must reproduce under at least two policies; a failure under only one policy is first treated as a policy or scenario issue.

## Progression checkpoints

Natural Battles run at Levels 1, 14, 24, 30, 40, and 50. The run additionally checks one Level below, at, and one Level above every Family's Evolution threshold. This catches a Move or stat spike hidden between the broad checkpoints.

Normalized Battles run at Queue Level 30 and a stat budget of 320 with Queue-eligible Battle Loadouts. They include all anchor-team pairings, mirrors, all three leads, all three policies, paired sides, and the fixed seed corpus. Natural scenarios use persistent eligible Moves and current Evolution state; normalization never supplies an otherwise ineligible Evolution.

Static validation runs before simulation and rejects a Species that has no legal Move at a natural checkpoint, fewer than four Queue-eligible Moves, a broken Evolution chain, an invalid stat budget, or a Reference Loadout that cannot be constructed.

## Competitive acceptance gates

The primary result is team-level win rate. Gates apply after paired side and order swaps.

| Gate | Acceptance threshold |
| --- | --- |
| Reference Team across the complete Normalized corpus | 40% to 60% wins |
| One non-mirror Reference Team matchup | 25% to 75% wins |
| Mirror matchup | 47% to 53% wins |
| Aggregate engine-side advantage | At most 3 percentage points |
| Neutral same-stage KO pace | Median of 3 to 5 landed hits **per faint** on non-super-effective KOs; no non-critical one-hit KO |
| Complete three-Monster Battle pace | Median of 6 to 15 resolved turns; 90th percentile at most 24 turns |
| Counterplay case | At least one legal Switch improves projected win rate by 10 percentage points or more |
| Move dominance | No Move exceeds 70% of choices when another legal Move is within 15% of its scored utility |

The 40-60 corpus band and 25-75 matchup band are both required. The first prevents an archetype from dominating the field; the second preserves strong identity without accepting team matchups decided almost entirely at entry. Individual one-on-one Family matchups do not use those win-rate gates because the Type chart deliberately creates hard counters.

A threshold failure must occur in the complete fixed run, not in a hand-selected seed. A severe deterministic invariant failure, including leaked hidden information, an illegal action, a non-terminating Battle, or an impossible Replacement, blocks immediately regardless of aggregate rates.

## Capture acceptance gates

Every one of the 24 target Family profiles runs against the checkpoint Party fixtures at Levels 1, 14, 24, 30, 40, and 50. Launch profiles, fallback order, and the PvE-band formula are in [Capture Objective catalog](capture-catalog.md). For every legal generated objective set:

- a scripted legal line must fill the Capture Gauge before target defeat without requiring a critical hit, miss, or favorable damage roll;
- the line must remain completable after any one non-objective Move misses;
- every selected objective must be eligible from the snapshotted Party and loadouts;
- the minimum-damage and maximum-damage paths must both preserve the objective ordering rule when Gauge completion and target defeat share a turn;
- at least one plausible over-aggressive line must still produce `hunt_failed`, so the Gauge tests play rather than merely participation.

Failure blocks that Family profile. Capture tuning may change its objective selection or PvE band but cannot weaken the global one-shot, visibility, determinism, or 100-point invariants.

## Dojo policy acceptance gates

Sparring policies use the same Reference Teams, matched natural Levels and Evolution stages, public-state boundary, loadouts, paired sides, and seeds. Against the Pivot reference policy, the Dojo Opponent's target win bands are:

| Tier | Target win rate |
| --- | ---: |
| Apprentice | 35% to 45% |
| Rival | 45% to 55% |
| Master | 55% to 65% |

Every team and checkpoint must preserve the ordering `Apprentice < Rival < Master`, and adjacent tiers must differ by at least 7 percentage points across the complete corpus. Rival must remain within 15% of its best one-turn score and Master within 5% of its best bounded two-turn score, as defined by the Dojo contract.

The run also records illegal-action count, hidden-information reads, switch frequency, repeated-action rate, and Decision Explanation reason coverage. Illegal actions and hidden-information reads must remain zero. Every authored Daily Challenge must be solvable within its published par under its fixed seed and must include at least one legal line that misses par, proving the Mastery Mark distinguishes execution.

## Tuning protocol

When a gate fails, preserve the failing replay and change one lever class at a time in this order:

1. Move unlock timing and Reference Loadout availability.
2. Movepool coverage, then Move power or accuracy.
3. Species stat distribution while preserving its named role and current stage budget.
4. Stage stat budget, Evolution threshold, or Type chart only as an explicit rules decision affecting the complete roster.

The first passing edit is compared with the prior full run. A change is rejected if it fixes the target but creates a new gate failure elsewhere. Designers do not tune from aggregate live win rate alone; live play may identify a missing scenario or regression seed, after which the reproducible corpus decides whether content changes.

Every accepted balance change records the failed gate, before-and-after summary, changed content fields, simulator revision, and corpus identity in its task. Launch Capture Family profiles and the Target Encounter PvE band are in [Capture Objective catalog](capture-catalog.md). Dojo Lesson targets, Sparring pool, policy coefficients, and Daily fixtures are in [Dojo Master policy and teams](dojo-policy.md).

## Prototype finding

The TERM-46 throwaway terminal harness loaded the then-current 72-Species, 24-Family, 42-Move content pack and ran 1,024 paired seeds across all 276 normalized one-on-one Family pairs. It found 259 pairs outside 40-60%, a landed-hit distribution of 1/3/5 at the 10th/median/90th percentiles, and several 100-0 Type-driven duels. That result rejected individual parity as the primary balance unit and established team counterplay as the governing model; it did not propose content changes from the incomplete one-on-one engine.

Implementation must promote this contract into a maintained simulator or deterministic integration suite before changing launch balance content. The first production run must populate the versioned Reference Team fixtures, per-battle replay artifacts, Capture profile matrix, and Dojo tier report described above.
