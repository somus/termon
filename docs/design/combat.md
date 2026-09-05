# Combat Engine v1 — decided (TERM-6)

Direct-damage rules for a single Move-versus-Move exchange. Switch, Replacement, and three-Monster Parties are specified in [Three-Monster Battle contract](party-battles.md). The 1v1 engine is replaced in [Implementation rollout](implementation-rollout.md).

## Turn loop

1. Both clients render move menus; selections are hidden until both lock in.
2. Execution order: higher Speed first; tie → coin flip.
3. Each move resolves fully before the other side acts: accuracy roll → damage → faint check.
4. If the first attacker KOs the defender, the second move never executes; battle ends. No double-KO, no draws in v1.

## Damage

```
base = floor(Power × A ÷ D ÷ 5) + 2
dmg  = base × STAB × TypeEff × Crit × Variance
```

- `A` = attack or sp_attack of the attacker, chosen by move category; `D` = defender defense.
- STAB = 1.5 when move type equals the attacker's species type, else 1.0.
- TypeEff = product of attacker-side chart lookups against the defender's type (sparse map, absent = 1.0).
- Crit = 1/16 chance → 1.5 ("A critical hit!").
- Variance = uniform 0.85–1.00 per hit.
- Miss (roll > accuracy): turn consumed, zero damage, `missed` event.

Target pacing at launch stats: 3–5 hits per KO.

## State machine

```
awaiting_selections → resolving_turn → faint? → battle_over
                                   └─ no → awaiting_selections
battle_over(reason: ko | forfeit | disconnect_timeout)
```

Disconnect mid-battle: 60-second grace keyed on Trainer ID; reconnect with the Trainer's SSH Credential rejoins the same Battle view while the process remains alive. Timeout or explicit forfeit ends it; one transaction records the immutable Battle Result and updates both W/L totals. Active Battles do not survive a process restart. See [durability and production persistence](durability.md).

## Event log

Every resolution step appends ordered events both clients render from; clients are dumb views over the log (this is what makes reconnect trivial):

`turn_started`, `move_used`, `missed`, `critical_hit`, `super_effective`, `not_very_effective`, `damage_dealt`, `fainted`, `forfeit`, `battle_over`

## Dojo Opponent

The original weighted-random Practice Bot becomes the Apprentice Sparring policy: super-effective weight 3×, neutral 1×, resisted 0.5×, plus the limited public-matchup Switch rule. Rival and Master add published one-turn and bounded two-turn policies. Information limits, teams, rewards, explanations, and all three Dojo modes are defined in [Dojo Master modes and bot behavior](dojo-master.md).

## Constants

All tunables live in one file (`combat constants`): crit rate/chance, STAB multiplier, variance bounds, damage divisors, grace period. Playtesting adjusts numbers without touching engine code.

## Deferred

Priority/speed buckets, status effects, items, multi-hit moves, draw handling.
