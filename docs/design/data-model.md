# Data Model — decided (TERM-8)

Canonical terms live in CONTEXT.md. This doc pins the concrete content and Save shapes. [Durability and production persistence](durability.md) defines the Trainer, credential, and Save boundary.

## Decisions

- Format: **JSON**, one file per entity, filename = slug, cross-references by slug string. No numeric ids anywhere.
- Typing: **one Type per Species, one Type per Move** in v1.
- Stats: **five** base stats — hp, attack, defense, sp_attack, speed. Move category (physical/special) picks attack vs sp_attack on the offensive side; both hit defense.
- Move knowledge: an individual Monster permanently unlocks Moves into its Move Library as it levels, and equips at most four of them in a Battle Loadout. New Monsters start with the first four Movepool entries as the ready-to-battle baseline, matching onboarding's current default loadout; the progression rules live in [Individual Monster progression](progression.md).
- Evolution: a Species may contain one `evolves_to` rule with a target Species and required Monster level. Families are linear and contain at most three stages. The content rules are validated at boot; the individual lifecycle and deferred prompt are defined in [Individual Monster progression](progression.md).
- Progression presentation: the [Collection and Party terminal flow](collection-party.md) batches persisted reward changes into a Progression Summary and requires durable acknowledgement for unreviewed Move unlock notices. The versioned Save and Store operations are [Progression persistence](progression-persistence.md).
- Effectiveness: attacker-side sparse map per Type; missing pairs resolve to 1.0.
- Validation at boot: every referenced slug must resolve to an existing file; accuracy 0–100; power ≥ 0; at least 4 current Species Movepool entries must be eligible at or below normalized Level 30; unique slugs. Malformed content refuses to start the server.

## File layout

```
content/
  species/<slug>.json
  moves/<slug>.json
  types/<slug>.json
  art/<slug>.json
```

## Go structs

```go
type Stats struct {
	HP       int `json:"hp"`
	Attack   int `json:"attack"`
	Defense  int `json:"defense"`
	SpAttack int `json:"sp_attack"`
	Speed    int `json:"speed"`
}

type MovepoolEntry struct {
	Move  string `json:"move"`  // slug reference into content/moves/
	Level int    `json:"level"` // learned-at; dormant until XP lands
}

type Evolution struct {
	Species string `json:"species"` // target Species slug
	Level   int    `json:"level"`   // required Monster level
}

type Species struct {
	Slug      string          `json:"slug"`
	Name      string          `json:"name"`
	Flavor    string          `json:"flavor"`
	Type      string          `json:"type"` // single Type slug
	BaseStats Stats           `json:"base_stats"`
	Movepool  []MovepoolEntry `json:"movepool"`
	EvolvesTo *Evolution      `json:"evolves_to,omitempty"`
	ArtPath   string          `json:"art"`
}

type Move struct {
	Slug     string  `json:"slug"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`     // single Type slug
	Category string  `json:"category"` // physical | special
	Power    float64 `json:"power"`
	Accuracy float64 `json:"accuracy"` // 0–100
}

type TypeDef struct {
	Slug    string             `json:"slug"`
	Name    string             `json:"name"`
	Matchup map[string]float64 `json:"matchup"` // defender slug -> multiplier; absent = 1.0
}

type Monster struct { // trainer-owned instance; persistence fields are in progression-persistence.md
    ID       string   `json:"id"`
    Species  string   `json:"species"`
    Nickname string   `json:"nickname"`
    XP       int64    `json:"xp"`
    Level    int      `json:"level"`
    Library  []string `json:"move_library"`
    Moves    []string `json:"moves"` // ≤4 slugs from Library; persisted as battle_loadout
    PendingEvolution bool `json:"evolution_pending"`
}
```

SQLite maps SSH Credentials to stable Trainer IDs and stores identity, records, Battle Results, and Activity Results relationally. Handle and W/L totals remain relational columns. The versioned Save payload is Collection, three Party slots, and open Progression Notices, defined in [Progression persistence](progression-persistence.md).

## Example files

```json
// content/species/zaplet.json
{
  "slug": "zaplet",
  "name": "Zaplet",
  "flavor": "A static-charged hatchling that crackles when startled.",
  "type": "current",
  "base_stats": { "hp": 44, "attack": 50, "defense": 40, "sp_attack": 52, "speed": 62 },
  "movepool": [
    { "move": "floating_pin", "level": 1 },
    { "move": "debounce", "level": 1 },
    { "move": "interrupt", "level": 1 },
    { "move": "irq_handler", "level": 1 },
    { "move": "nmi", "level": 15 },
    { "move": "kernel_trap", "level": 31 }
  ],
  "evolves_to": { "species": "voltalon", "level": 15 },
  "art": "art/zaplet.json"
}
```

```json
// content/moves/floating_pin.json
{
  "slug": "floating_pin",
  "name": "Floating Pin",
  "type": "current",
  "category": "special",
  "power": 40,
  "accuracy": 100
}
```

```json
// content/types/current.json
{
  "slug": "current",
  "name": "Current",
  "matchup": { "coolant": 2.0, "silicon": 2.0 }
}
```

## Evolution validation

- Evolution targets must resolve to a Species in the same content pack.
- A Species has at most one predecessor and one successor, so families are linear.
- Evolution levels start at 2 and increase through a multi-stage family.
- Families cannot contain cycles or more than three stages.

The accepted families, thresholds, stats, and lore are recorded in [evolution.md](evolution.md).

## Deferred model extensions

Dual typings and status effects remain deferred. Capture Gauge, XP, Move Library, Evolution deferral, and Save/Store runtime are implemented according to [Capture Gauge and tactical objectives](capture-gauge.md), [Individual Monster progression](progression.md), and [Progression persistence](progression-persistence.md). The completed gameplay sequence remains recorded in [Implementation rollout](implementation-rollout.md).
