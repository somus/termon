# Capture Objective catalog - decided (TERM-54)

This is the content reference for [Capture Gauge and tactical objectives](capture-gauge.md). It pins the five launch IDs, the generator, one profile per Evolution Family, and the Target Encounter PvE band. Predicates and Gauge rules stay on that page.

Launch content has no off-Type coverage Moves. Matchup eligibility is therefore the Party's Types versus the Wild Species' Type.

## Objective IDs

The generator must emit three distinct IDs whose awards sum to 100. IDs are stable strings. Display names may change without changing IDs.

| ID | Display name | Award |
| --- | --- | ---: |
| `show_move_variety` | Use 3 different Moves | 30 |
| `read_the_matchup` | Land a super-effective Move | 35 |
| `safe_switch` | Safe switch | 35 |
| `measured_pressure` | Measured pressure | 35 |
| `hold_the_line` | Hold the line | 35 |

No objective requires a consumable, a random critical, a random miss, a hidden Move, or a reserve the Party does not own. A miss does not count as damage. A forced Replacement does not count as `safe_switch`. Repeating a completed action awards nothing.

## Generator

Each Family profile stores one identity ID: `measured_pressure` or `hold_the_line`. The global fallback order is:

1. `show_move_variety`
2. The profile identity
3. `read_the_matchup`
4. `safe_switch`
5. The complementary identity (`hold_the_line` if identity is `measured_pressure`, and the reverse)

Walk that list. Skip any ID that fails its eligibility predicate or is already selected. Stop at three IDs. The rendered list is this selection in this order. The server must not substitute after the Target Encounter starts.

Eligibility at snapshot time:

| ID | Eligible when |
| --- | --- |
| `show_move_variety` | Every selected Party Monster has four loaded Moves and at least three distinct slugs |
| `read_the_matchup` | A healthy Party Monster has a loaded Move with Type effectiveness at or above `2.0` versus the Wild Species |
| `safe_switch` | The Target Encounter starts with at least two healthy Party Monsters |
| `measured_pressure` | The Party has a damaging Move, and two minimum-variance hits from the weakest such Move leave the Wild Monster above 25% Capture HP |
| `hold_the_line` | At least one Party Monster remains above 50% HP after one maximum-variance, non-critical wild hit |

Expedition launch must refuse a Party Monster with fewer than four loaded Moves. That keeps `show_move_variety` eligible for every legal run.

## Eligibility by Party shape

Every base-stage default loadout is four same-Type Moves. A one-Monster Party therefore has `read_the_matchup` only when that Monster's Type is super-effective versus the target.

| Party shape | Typical generated set |
| --- | --- |
| One Monster, same Type as the target | `show_move_variety`, identity, complement |
| One Monster, super-effective versus the target | `show_move_variety`, identity, `read_the_matchup` |
| Two or three Monsters, at least one super-effective Move | `show_move_variety`, identity, `read_the_matchup` |
| Two or three Monsters, no super-effective Move | `show_move_variety`, identity, `safe_switch` |

The eight Reference Teams from [Gameplay balance methodology](balance-methodology.md) cover mixed-Type three-Monster Parties. Solo fixtures cover every Family as a one-Monster hunter.

## Family profiles

Identity follows Family role. Defensive and bulky Families teach `hold_the_line`. Fast and pressure Families teach `measured_pressure`. Twelve Families use each identity.

| Family | Type | Identity |
| --- | --- | --- |
| Rootkit | organic | `hold_the_line` |
| Sproutware | organic | `measured_pressure` |
| Thornpatch | organic | `hold_the_line` |
| Mossmuff | organic | `hold_the_line` |
| Rootanami | organic | `hold_the_line` |
| Emberbyte | thermal | `measured_pressure` |
| Cindernode | thermal | `hold_the_line` |
| Scorchip | thermal | `measured_pressure` |
| Wickware | thermal | `measured_pressure` |
| Aquabit | coolant | `measured_pressure` |
| Flowcell | coolant | `hold_the_line` |
| Gushkit | coolant | `measured_pressure` |
| Mistcache | coolant | `measured_pressure` |
| Splashscreen | coolant | `hold_the_line` |
| Zaplet | current | `measured_pressure` |
| Joulpup | current | `measured_pressure` |
| Amperent | current | `hold_the_line` |
| Surgetail | current | `hold_the_line` |
| Spamlet | virus | `measured_pressure` |
| Bloatware | virus | `hold_the_line` |
| Wormate | virus | `hold_the_line` |
| Chippunk | silicon | `measured_pressure` |
| Coghound | silicon | `measured_pressure` |
| Servoboar | silicon | `hold_the_line` |

Content stores `{ "family": "<base slug>", "identity": "<id>" }`. The fallback order is not per-Family. Boot validation must reject a profile whose identity is unknown, duplicated, or unable to produce three eligible IDs for every legal Party fixture.

## Target Encounter PvE band

Preparation Encounters use ordinary natural combat from [XP, level curve, and normalized PvP](xp-progression.md). They do not use Capture HP or the wild damage clamp. The Target Encounter is a Gauge puzzle on top of the same damage formula.

The Wild Species is always the target Family's base stage. Its Level is `max(1, min(50, max(party Levels)))`. Stats use `NaturalStat`. The Wild Battle Loadout is the Species' first four Movepool entries.

Capture HP is computed from the snapshotted Party, not from a per-Family constant. A stronger Party therefore faces a bulkier target. For each Party Monster, using loaded Moves versus the Wild Species, no critical, variance 0.85 and 1.00:

- `max_i` is that Monster's strongest maximum-variance hit
- `line_i` is the sum of its two weakest minimum-variance hits plus, when it has a super-effective Move, that Move's minimum-variance hit; otherwise the three weakest minimum-variance hits
- `low` is the maximum over the Party of `max_i + 1` and `line_i + 1`
- `high` is eight times the Party's strongest maximum-variance hit
- Capture HP is `min(high, low + that strongest hit)`

This keeps a scripted three-hit line from knocking out the target, forbids a non-critical one-shot, and still lets an all-out line knock the target out within eight hits.

Wild Target Encounter damage uses the combat formula, then clamps to `max(1, floor((defenderMaxHP - 1) / 5))`. Five resolved wild hits therefore cannot faint a Monster that started the fight at full HP. `hold_the_line` remains eligible without a random miss: one clamped hit is always less than 50% of max HP.

## Capture lines

A legal capture line, used by tests and the planning harness, is:

1. If `safe_switch` is selected, Switch into a healthy reserve. Prefer the reserve that keeps the most HP after one clamped wild hit.
2. Use the two weakest loaded Moves. After any one of those Moves misses, retry it. The line must still complete.
3. If `read_the_matchup` is selected, use a super-effective Move as the third landed Move.
4. Evaluate objective completion before faint when both happen on the same turn.

An over-aggressive line repeats the strongest loaded Move at maximum non-critical variance and never switches Moves. Variety cannot complete. The target reaches zero HP with Gauge below 100, which is `hunt_failed`.

Repeating an already completed action must not increase Gauge. The generator must reject a candidate list that repeats an ID or does not sum to 100.

## Gate corpus

The planning harness `go run ./cmd/capturecatalog` loads the content pack and checks 5,760 cases: 24 target Families, checkpoints 1, 14, 24, 30, 40, and 50, and 40 Party fixtures per pair (every Family as a solo hunter, plus each Reference Team as a duo and a trio). Party Species use the Evolution stage required at that Level. Loadouts use the Reference Loadout rule from the balance methodology.

Every case must:

- generate three distinct eligible IDs that sum to 100
- fill Gauge on the scripted line at minimum non-critical damage, including one injected miss
- produce `hunt_failed` when the strongest Move is spammed at maximum non-critical damage
- keep min-damage and max-damage paths consistent with Gauge-before-KO ordering

Focused checks that implementation tests must keep:

| Check | Result in the launch pack |
| --- | --- |
| Level 1 Rootkit versus Emberbyte | Variety, pressure, hold the line; Gauge fills |
| Level 1 Scorchip versus Emberbyte (no matchup) | Variety, pressure, hold the line; Gauge fills |
| Level 1 starter trio versus Bloatware | Variety, hold the line, matchup; Switch is available but unused |
| Level 50 Infernalink versus Servoboar | Variety, hold the line, pressure; no one-shot |
| Level 50 starter trio versus Mossmuff | Variety, hold the line, matchup; Gauge fills |

Do not weaken the one-shot, visibility, determinism, or 100-point invariants to pass a new Species. Change identity or the shared PvE-band formula instead.

## Implementation notes

Expedition Battles must apply `NaturalStat` from the XP contract. Target Encounter Capture HP and the wild damage clamp are Expedition hooks. They must not apply to Queue Battles, Challenges, or Dojo Sparring.

Boot validation must refuse to start when any Family profile cannot generate three eligible IDs for a legal Party, when two IDs collide, or when awards would not sum to 100.
