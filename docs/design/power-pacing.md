# Level-scaled power evidence

The September 2026 combat change targets about three landed neutral hits at comparable Levels and Evolution stages. Move `power` is now a ceiling: effective power starts at 40% at Level 1 and grows linearly to 100% at Level 50, rounded down. The damage divisor changes from 5 to 2.72 and Type advantage from 2 to 1.5. Attack, Defense, STAB, critical chance and variance retain their roles. See [combat](combat.md) for the formula.

## Natural progression

The new deterministic audit covers all 24 Families at every Level from 1 through 50, resolving reachable Evolutions and comparing ordered same-stage pairs. Each attacker uses the eligible Move with the greatest expected damage including accuracy. Once selected, expected hits to faint condition on landing, include criticals and integer rounding, and account for overkill through a remaining-HP recurrence. This is direct-damage pacing, not a claim about full Battle length, switching strategy, or every possible loadout.

| Level | Neutral mean landed hits | Advantage mean landed hits |
| --- | --- | --- |
| 1 | 3.32 | 2.37 |
| 3 | 3.30 | 2.36 |
| 14 | 3.18 | 2.28 |
| 24 | 3.17 | 2.29 |
| 30 | 3.18 | 2.29 |
| 40 | 3.49 | 2.51 |
| 50 | 3.39 | 2.45 |

Across all 50 Levels, neutral means range from 2.97 to 3.49, with zero non-critical neutral one-hit KOs in the audited pairs. The gate requires 2.5–3.5 at every Level. Favorable matchups can still produce one-hit KOs against fragile targets, and criticals can shorten fights.

The early-game regression uses Level-3 Aquabit's Jumbo Wave against Level-3 Emberbyte: effective power is 31 rather than 75, and ordinary damage is 32–38 against 45 HP, rather than the previous 56–65. An authoritative engine test checks both variance endpoints and survival.

The capture gameplay-evidence run also passes all 4,608 trajectories, covering minimum and maximum damage, a scripted miss, and over-aggressive target defeat. Capture HP calculations and bot forecasts use the same level-scaled power as the engine.

## Daily challenge calibration

All fourteen recorded Daily winning lines were revalidated or regenerated against the new formula. Six challenges retain their rosters, seeds, policies and pars. The original limited-toolkit fixture no longer produced a winning proof within the bounded search. Its Organic opponent changes from Rootkit to Taproot, with par adjusted from 12 to 10; the player Party, Rival policy, seed 55006 and power-ceiling restriction of 65 are unchanged. The revised fixture has verified objective-valid wins in 10 and 14 turns, preserving both mastery and over-par outcomes. Search failure for the original fixture is not a proof that no winning line exists.

## Historical normalized control

Both reports used the same 1,024 seeds, eight Reference Teams, and 196,608 Battles. The old report used `party-battles-v2` at source revision `348d498`; the new report uses `level-scaled-power-v3` with this source change. Normalized fixtures retain their historical 320-point total and Level 30. Live Queue and Challenge use natural earned stats, so their pacing gate now comes from the natural audit. The old normalized per-faint measurement remains visible in the report, including its failure.

| Measurement | Before | After |
| --- | --- | --- |
| Failing non-mirror matchup bands | 13 | 15 |
| Overall team win-rate range | 20.7–77.9% | 18.4–71.2% |
| Normalized neutral median hits per faint | 3 | 2, with non-critical one-hit KOs |
| Normalized Battle median / p90 turns | 12 / 16 | 9 / 12 |
| Illegal actions | 0 | 0 |
| Capture eligibility fixtures passing | 144 / 144 | 144 / 144 |

Four previously failing matchup bands now pass, while six previously passing bands fail: starter-balance versus mixed-endurance; alternate-balance versus fast-pressure and bruiser-core; bulky-control versus mixed-endurance; bruiser-core versus specialist-pressure; and mixed-endurance versus specialist-pressure. This is a pacing change with unresolved team-balance debt, not an overall balance improvement.

The baseline records the new rules and these measured failures so future changes cannot silently worsen them. Strict `balancerun -fail-gates` still fails on team balance. The opt-in matrix retains its historical per-faint gates and can also fail on normalized pacing; the default audit's natural pacing gate does not claim that matrix passes.
