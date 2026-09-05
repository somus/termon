// Package content defines the game's data model and loads the read-only
// content packs (species, moves, types) from disk. Shapes follow
// docs/design/data-model.md.
package content

// Stats are a Species' five base stats.
type Stats struct {
	HP       int `json:"hp"`
	Attack   int `json:"attack"`
	Defense  int `json:"defense"`
	SpAttack int `json:"sp_attack"`
	Speed    int `json:"speed"`
}

// MovepoolEntry is one Move a Species can learn, with the level it is
// learned at (dormant until XP ships).
type MovepoolEntry struct {
	Move  string `json:"move"`
	Level int    `json:"level"`
}

// Evolution identifies the Species and Monster level for one evolution.
type Evolution struct {
	Species string `json:"species"`
	Level   int    `json:"level"`
}

// Species is the template for a kind of monster.
type Species struct {
	Slug      string          `json:"slug"`
	Name      string          `json:"name"`
	Flavor    string          `json:"flavor"`
	Type      string          `json:"type"`
	BaseStats Stats           `json:"base_stats"`
	Movepool  []MovepoolEntry `json:"movepool"`
	EvolvesTo *Evolution      `json:"evolves_to,omitempty"`
	ArtPath   string          `json:"art"`
}

// Move is a single combat action: typed, categorized, powered, accurate.
type Move struct {
	Slug     string  `json:"slug"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Category string  `json:"category"` // physical | special
	Power    float64 `json:"power"`
	Accuracy float64 `json:"accuracy"` // 0-100
}

// TypeDef is an element type with its attacker-side effectiveness matchups.
type TypeDef struct {
	Slug    string             `json:"slug"`
	Name    string             `json:"name"`
	Matchup map[string]float64 `json:"matchup"` // defender slug -> multiplier; absent = 1.0
}
