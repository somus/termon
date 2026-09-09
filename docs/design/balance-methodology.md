# Gameplay balance methodology - decided (TERM-46)

Termon balances complete three-Monster teams around counterplay. Individual Species may have decisive favorable or unfavorable duels, but a legal reference team must be able to answer those matchups through Party construction, Move choice, or switching. A one-on-one result is diagnostic evidence, not an automatic balance failure.

This methodology governs Species stats, Movepools, Capture Objectives, Dojo policies, natural progression, and historical normalized diagnostic fixtures. It changes no content by itself. Content changes require a reproducible gate failure against the versioned corpus below.

## Reproducible Balance Run

A Balance Run snapshots the content-pack revision, rules revision, simulator revision, Reference Teams, policy parameters, and a fixed corpus of 1,024 seeds. Every non-mirror scenario runs twice per seed with engine side and Party order exchanged. A run is invalid if any result depends on wall-clock time, map iteration order, an unrecorded random source, or client behavior.

The default `balancerun` mode is the fixed audit corpus. The opt-in matrix mode records its requested and completed coverage separately, and streams detailed JSONL outcomes when `-outcomes` is set while retaining only bounded aggregate evidence and replay samples in the report. Matrix policies are Pressure (maximum immediate expected HP loss), Pivot (the exact-best one-turn Rival score), and Preservation (minimum active-faint probability against a predicted public opponent response, then healthy reserves, then outgoing expected HP loss). The competitive-copy feature and its `-competitive` flag have been removed. Normalized fixtures remain historical diagnostics; live Queue and Challenge use earned progression, so these fixtures do not establish live PvP balance. Matrix coverage currently includes stage, loadout, lead, physical side/order, and policy axes, but does not claim counterplay switch/stay, full capture trajectories, or Dojo tier/Daily trajectory gates.

Reference-policy revision `reference-policies-v2` retains Pressure and Pivot and adopts Preservation response model v1. The user accepted this model after the retained cycle and a 299,916-battle one-seed matrix completed without a turn cap. This does not establish full-corpus balance acceptance.

Preservation first predicts the opponent's equally best actions under the original conservative survival-first score at the current position. It derives those candidates solely from public Move eligibility and public roster data, modeling unseen living reserves at full HP. It averages these tied responses uniformly, resolves predicted switches before our attack, and accounts for a predicted Move's exact faint probability and Speed-dependent KO cancellation. It then minimizes our averaged faint probability, maximizes healthy reserves, and maximizes potential outgoing HP loss, using injected RNG for remaining ties. The opponent prediction is a single nonrecursive step, not an equilibrium model. Potential outgoing damage retains the existing tie-break convention rather than estimating the chance a slower strike executes.

The old strongest-Move/no-switch model's switching cycles must not be attributed to the new version. Reports, reference battle outcomes and counterplay matrices record `reference_policy_revision` so policy versions remain distinguishable. [Gameplay lessons](gameplay-lessons.md) summarize the findings; the old model and raw comparison artifacts have been removed.


The simulator uses the authoritative damage, Type, action-order, switch, faint, Replacement, normalization, and bot-policy rules. It records machine-readable per-battle results and prints a bounded terminal summary. A failed run reports the exact scenario, seed, teams, loadouts, actions, and first failed gate so the Battle can be replayed directly in a test.

Random outcomes are part of the corpus rather than averaged through unbounded Monte Carlo sampling. Changing the seed corpus is a reviewed rules change; adding a regression seed is allowed when it represents a legal state that the corpus missed.

The default `audit` command preserves the original normalized fixture schedule for before/after comparison. Its non-mirror team records exclude mirrors. Every unordered non-mirror pair receives a separate 25–75% gate with a win count and denominator; mirror results measure physical engine-side advantage separately. Winner attribution follows the actual party/trainer mapping in each run. Passing this smaller schedule does not establish the complete methodology below.

Use `go run ./cmd/balancerun -content ./content -capture -fail-gates -report /tmp/balance.json -outcomes /tmp/balance-outcomes.jsonl` for the historical normalized control. JSONL output can be large. Snapshot identity includes the content revision, Git revision when available, seed corpus, policy and mode. For an uncommitted build, retain the source diff alongside the report; the Git commit alone cannot identify modified source. A turn cap, failed action, invalid Replacement or nonadvancing state is an incomplete run with failure context, never a completed loss or a retry-until-success result.

## CI regression baseline

CI and `scripts/check.sh` run the fixed corpus with Capture smoke checks, then compare the report with `.github/balance-baseline.json` using `go run ./cmd/checkbalance -report balance-report.json`. The baseline records commit `2ef6c52` from CI run `34356109810`: 196,608 battles, 13 failing non-mirror matchups, and a failing overall Reference Team win-rate range. These remain known balance debt; baseline acceptance does not mean the methodology gates pass.

A baselined matchup may improve toward the 25-75% band but may not worsen or fail in the opposite direction. The overall team minimum may not fall and its maximum may not rise beyond the recorded range. Previously passing matchups retain the normal band. Mirror, engine-side, knockout pace, Battle pace, illegal-action, and Capture gates must all pass. Missing gates, changed thresholds, or changes to seeds, rules, policy, teams, or corpus size fail the comparison. Content revisions may change so tuning can be evaluated against the same corpus.

The JSON report retains its original gate failures. `balancerun -fail-gates` remains available for strict methodology acceptance. Baseline changes require explicit review of the old and new reports; do not regenerate the baseline merely to make CI pass.

## Reference Teams

The launch corpus starts with eight three-Family teams. A Family name resolves to the Species that the checkpoint Level and Evolution state require. Each Family appears once in the anchor set, so a globally acceptable average cannot hide a Family that was never exercised.

| Archetype | Evolution Families |
| --- | --- |
| Starter balance | Rootkit, Emberbyte, Aquabit |
| Alternate balance | Zaplet, Spamlet, Chippunk |
| Bulky control | Taproot, Flowcell, Bloatware |
| Fast pressure | Sproutware, Wickware, Mistcache |
| Physical pressure | Thornpatch, Gushkit, Joulepup |
| Bruiser core | Cindernode, Ampcoil, Coghound |
| Mixed endurance | Mossmuff, Splashlotl, Surgetail |
| Specialist pressure | Scorchip, Wormate, Servoboar |

Every team runs with each of its three Monsters as lead. The corpus also creates one counterplay case per Family: that Family starts in a clearly unfavorable Type matchup while a healthy reserve has a favorable matchup. These cases measure whether switching provides a real answer rather than whether the disadvantaged active Monster can win alone.

At each checkpoint, a deterministic Reference Loadout selects up to four currently eligible Moves: the strongest neutral physical option, strongest neutral special option, most accurate option, and earliest-unlocked option, with duplicate choices removed and ties in accuracy and unlock level resolved by stable Move order, and remaining slots filled in authored Movepool order. Balance work may add an authored loadout only to cover a distinct legal strategy; it cannot silently replace an anchor loadout that fails.

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

Illegal actions fail with replay context. The public-information boundary is enforced by separate opponent types and hidden-Loadout/reserve-HP/pending-action tests; there is no runtime hidden-read counter. Matrix reports measure voluntary Switches and conditional Move choices. Complete Decision Explanation reason coverage remains an additional acceptance requirement rather than an implemented runtime gate. Every authored Daily Challenge must be solvable within its published par under its fixed seed and must include at least one legal objective-clearing win that misses par, proving the Mastery Mark distinguishes execution. A battle win that also misses the objective cannot isolate the par condition.

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

## Supporting evidence commands and limits

`go run ./cmd/gameplayevidence -suite capture -content ./content -report /tmp/capture.json` executes 4,608 authoritative trajectories: eight anchor Parties, six natural checkpoints, 24 target profiles, and four variants. The planner deliberately switches to an eligible super-effective attacker, tracks only generated incomplete objectives, and uses low-power unused Moves for variety. Min/max runs force ordinary hits without crits at each variance bound. The miss run attempts a previously used inaccurate Move once, verifies that the miss awards no objective, then continues. This checks one authored non-objective miss position per case, not every possible miss position required by the strongest reading of the acceptance contract. Wild actions use the live Apprentice policy with an independent deterministic policy stream. Over-aggressive lines must fail capture through target defeat.

Use `-suite dojo -seeds 1024` for all tier/team/checkpoint cells, `-suite counterplay -seeds 1024` for all 24 three-policy Switch/stay cases, and `-suite daily` for the seven fixed Daily fixtures. Each requires `-report`. These commands return nonzero for failed or unproven evidence. Daily witnesses are replayed from recorded action sequences before bounded beam search; stale witnesses are not accepted. Beam search is a proof finder, not an impossibility proof. It does not establish that a clear is independent of every critical hit or opponent miss; occurrence flags are reported separately.

Normalized matrix fixtures cover default, Reference, and damage-frontier recipes, not every possible team/loadout combination. Natural defaults keep onboarding's first four entries; Reference loadouts follow current-level eligibility. Frontier dominance compares Type, category, power and accuracy, retains nondominated choices before ranked fillers, and ranks damage against equal Defense 100. All 4,032 shipped competitive individual stage/loadout subsets of sizes one through four receive preparation validation tests; that is eligibility evidence, not exhaustive strategic matchup coverage.

## Naming-independent Move ordering

Move `order` values preserve deterministic selection when names or slugs change. The naming pass assigns the existing alphabetical ranks once, then uses those stable values for normalized loadouts, Reference Loadout ties, damage-frontier fixture ties, and Daily proof-search ties. Display-only report sorting may still use names or slugs. Reordering these ranks is a gameplay change requiring balance evidence; renaming a Move must preserve its rank. The reviewed baseline thresholds, rates, seeds, and corpus remain unchanged.
