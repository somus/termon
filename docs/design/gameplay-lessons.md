# Gameplay and balance lessons

Use these lessons when changing content, Dojo behavior or balance tests. They summarize the September 2026 investigation; the raw reports and rejected prototypes were removed. Historical observations are not release certification. Current rules live in the [balance methodology](balance-methodology.md), [Dojo policy](dojo-policy.md) and [Party Battle contract](party-battles.md).

## Measure the right behavior

- Attribute wins through the actual Party/trainer mapping after side swaps. Exclude mirrors from team win rates and evaluate every non-mirror pair separately. The original accounting concealed large disparities even though the live engine selected the correct winner.
- Record the source, policy, seeds and completed coverage with each result. A green normalized fixture run cannot establish balance for earned-Level PvP. Exercise natural Levels around Evolution and Move unlocks, alternative loadouts, independent leads, sides and reserve order.
- Count all contributing hits when measuring neutral KO pace. An earlier super-effective hit or a different-stage attacker can invalidate a sample even when the finishing Move is neutral.
- Treat turn caps and invalid actions as incomplete runs. Keep focused regression cases and bounded diagnostic runs; expand the corpus when it answers a specific unresolved question. More battles cannot repair a biased policy or wrong fixture.

## Separate policy quality from gameplay quality

Opponent models must use public information, with tests showing that private loadouts, reserve HP and pending actions cannot affect their inputs. A counter initialized to zero does not prove that boundary.

Expected damage must match the engine's hit, critical, variance and rounding rules. Near-best sampling must handle negative scores and sample the entire permitted band. Master forecasts must model switches before attacks, Speed order and faint cancellation; a mean-damage forecast still approximates outcomes near KO thresholds.

The original Preservation test policy repeatedly switched because its model assumed the opponent always attacked. Predicting a public opponent response removed the retained cycle without changing combat stats. That improves a test opponent, not human PvP or the live Dojo tiers.

Dojo roster composition can dominate difficulty even when both sides use the same policy. Evaluate complete Parties across teams, Levels and tiers. Two roster heuristics improved average bot win rates but introduced new per-matchup failures, so neither replaced the existing roster builder.

## Preserve meaningful player choices

Keep earned progression authoritative in PvP: entering a Battle must not grant Levels, Moves or Evolutions. Making every Family available daily removes a calendar restriction on team building without granting progression.

The roster's visual identities were more distinct than its combat roles. Avoid cutting families from a single-policy result. Several same-Type damage Moves overlap, and a later unlock can be worse when it uses the Monster's weaker attacking stat. Evaluate power, accuracy, category and unlock timing together. Low-power Moves can still matter for capture, while reliable hits can matter near a KO.

Describe mechanics literally. Both attack categories use the same Defense, so a special attacker is not a special-resistant wall. The reviewed Type chart has no half-damage resistances; a switch can escape super-effective damage into neutral damage.

Guard/Pierce demonstrated defensive prediction and counterplay, but also introduced a new failing matchup. Equal total failure counts hid that regression. The prototype did not establish better human enjoyment and was removed.

## Keep evidence proportional to the claim

Capture eligibility does not prove a successful capture trajectory. A failed planner can reflect a missed objective or switch rather than an engine defect. Check successful lines, damage bounds, a recovered miss and over-aggressive failure separately.

Daily Challenges need a shared deterministic seed for bot decisions as well as combat. Fourteen recorded proofs covered an objective-clearing within-par and over-par line for each of seven fixtures. A win that misses the objective does not isolate the par requirement.

Automated play and replay verification establish behavior, not fun, clarity or retention. Human validation remains separate. Matchup and Dojo difficulty failures were not resolved by the cleanup; use fresh balance runs before making new numerical claims.
