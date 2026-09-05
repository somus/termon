# Capture Gauge and tactical objectives - decided (TERM-44)

The Target Encounter uses a visible Capture Gauge instead of a consumable or a random capture roll. The Trainer earns Gauge by completing three tactical Capture Objectives during the Battle. Each objective is shown before the first action, can be completed once, and has a fixed award; the three awards sum to 100.

This keeps capture skillful without making the result depend on hidden rolls. A Trainer can read the requirements, choose a Battle line that satisfies them, and know that a full Gauge guarantees the target.

## Target Encounter loop

1. The server snapshots the target Family, the Trainer's Party and Battle Loadouts, the target's PvE band, and the deterministic encounter seed when the Target Encounter starts.
2. The server generates exactly three Capture Objectives from the target Family profile and the Party snapshot. It removes any objective whose eligibility predicate is false, then fills the slot from a deterministic fallback order. The final list is the list the client renders; there are no hidden substitutions after the fight begins.
3. The Capture Gauge starts at 0 for every Target Encounter. Preparation Encounter progress never carries into the target, so an earlier easy fight cannot create a guaranteed capture before the Trainer has made a target-specific decision.
4. The Battle resolves through the normal server-authoritative action loop. After each resolved turn, the server evaluates every incomplete objective against the resolved actions and Battle state. A newly satisfied objective becomes complete, awards its fixed Gauge amount once, and emits a progress event.
5. When Gauge reaches 100, the Expedition immediately commits a `captured` result. There is no second capture menu, accuracy roll, item, or currency check. If the same turn also reduces the Wild Monster to zero HP, objective completion is evaluated first, so a Gauge that reaches 100 on that turn still captures; a target that reaches zero while the Gauge remains below 100 produces `hunt_failed`.
6. Gauge never decreases, and repeating an already completed action cannot award more progress. A Target Encounter can therefore be won by good play or lost by overcommitting damage, but it cannot be stalled for unlimited Gauge.

The existing Battle event log remains the source of turn narration. Expedition state adds separate events for `capture_objective_completed`, `capture_gauge_changed`, `captured`, and `hunt_failed`; reconnecting clients replay the same committed state rather than recomputing it locally.

## Objective catalog

The launch catalog is five reusable IDs. The target Family profile chooses `measured_pressure` or `hold_the_line`. The generator then adds `show_move_variety`, the identity, and the first remaining eligible ID from `read_the_matchup`, `safe_switch`, and the complementary identity. Awards stay coarse so balance changes do not rewrite predicates.

| ID | Display name | Predicate | Award | Eligibility |
| --- | --- | --- | ---: | --- |
| `show_move_variety` | Use 3 different Moves | Resolve three turns using three distinct Move slugs from the active Party. | 30 | Every selected Monster has a four-Move loadout. |
| `read_the_matchup` | Land a super-effective Move | Resolve a Move with Type effectiveness at or above `2.0` against the Wild Monster. | 35 | A healthy Party Monster has a loaded Move with that effectiveness. |
| `safe_switch` | Safe switch | Complete a voluntary Switch into a healthy reserve with at least 50% HP. | 35 | The Target Encounter starts with at least two healthy Party Monsters. |
| `measured_pressure` | Measured pressure | Deal positive damage on two different turns while the Wild Monster is above 25% HP at the end of the second turn. | 35 | The PvE band leaves enough Capture HP for two resolved turns. |
| `hold_the_line` | Hold the line | After the Wild Monster acts, end a resolved turn with the active Monster above 50% HP. | 35 | At least one Party Monster survives one non-critical wild hit above 50% HP. |

The generator must select three distinct IDs that sum to 100. It must not substitute after the fight begins. Family profiles, fallback order, Capture HP, and the wild damage clamp are in [Capture Objective catalog](capture-catalog.md). Content validation rejects a Family profile that cannot produce three eligible IDs for a legal Party. Capture acceptance gates are in [Gameplay balance methodology](balance-methodology.md).

Objective completion is based on resolved Battle actions, not on client claims or raw button presses. A missed Move does not count as damage, a forced Replacement does not count as a voluntary Switch, and a target that is already below the predicate's HP floor cannot retroactively satisfy a pressure objective. These rules make progress auditable in the event log and prevent a Trainer from farming the target with harmless repeated inputs.

## Chosen terminal presentation

The selected presentation is the battle-first layout from the Charm prototype. The arena, sprite art, HP plates, narration box, Move grid, and TYPE pane stay in their current positions and keep their current visual language. The capture-specific surface is a lower information band between the arena and the action controls:

- The left panel shows `Gauge/100`, a fixed-width bar, the `target only` scope, and the latest progress or failure message.
- The right panel lists all three objectives with `[ ]` or `[x]`, the fixed `+N` award, and the full description for the focused row. The client never hides an objective after it completes.
- When Gauge is full, the status line changes to `Target captured immediately` and the target result is committed. In a Capture Lesson, Sable instead says the match is over, then names the captured Species, and FIGHT stays closed until the Progression overlay. [Expedition terminal view](expedition-view.md) puts that result, including `Collection +1`, in the same four-row Battle chrome as the rest of the run. It does not expose a probabilistic capture button.
- On failure, that same chrome names the unmet target outcome and states that completed-encounter XP is retained. The next Expedition starts at Preparation Encounter 1, as defined by the Expedition contract.

The prototype's `c` key was only a visual test affordance. The shipped flow does not require a capture input after the Gauge fills. `Tab` continues to open the existing Battle Log, where objective completion events appear alongside the turn that satisfied them.

## Balance invariants

- Every Target Encounter presents exactly three visible objectives whose awards sum to 100.
- An objective can transition from incomplete to complete at most once; Gauge is monotonic and capped at 100.
- The objective generator is deterministic for the run snapshot and rejects any Family profile that cannot produce three eligible IDs for a legal Party.
- At least one eligible objective must remain available after any single failed attempt, so a miss, resisted hit, or unlucky turn never makes a run mathematically unwinnable.
- Objectives cannot require a consumable, a random critical, a random miss, a hidden Move, a specific reserve that the Party does not own, or a target HP threshold that the target's PvE band cannot reach.
- A target defeat before Gauge completion fails capture but retains XP from every completed Battle, matching the Expedition reward contract.
- Capture mutation is idempotent by Expedition run identity, so reconnects and duplicate result delivery cannot create two individuals.

## Verification target

The implementation work following this decision must add focused tests for objective eligibility, one-shot awards, tie ordering when Gauge fills on a KO turn, deterministic generation from a run snapshot, reconnect replay, and content validation against [Capture Objective catalog](capture-catalog.md). It must also exercise the three presentation states in a terminal-sized Bubble Tea view: active progress, immediate capture, and hunt failure.
