// Package game holds trainer-owned state: Monsters, Parties, Saves.
package game

// Trainer is a stable player identity plus optional onboarded game state.
type Trainer struct {
	ID   string
	Save *Save
}

// Monster is a trainer-owned instance of a Species.
type Monster struct {
	ID               string   `json:"id"`
	Species          string   `json:"species"`
	Nickname         string   `json:"nickname"`
	XP               int64    `json:"xp"`
	Level            int      `json:"level"`
	MoveLibrary      []string `json:"move_library"`
	BattleLoadout    []string `json:"battle_loadout"`
	EvolutionPending bool     `json:"evolution_pending"`
}

// ProgressionNotice is an open attention item after a reward or capture.
type ProgressionNotice struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"` // move_unlock | capture_review
	MonsterID string   `json:"monster_id"`
	SourceKey string   `json:"source_key"`
	Moves     []string `json:"moves,omitempty"`
}

// Save is the mutable game state owned by a Trainer.
type Save struct {
	Handle     string // from trainers.handle
	Wins       int    // from trainers.wins
	Losses     int    // from trainers.losses
	Collection []Monster
	Party      [3]string // Monster IDs; "" vacant
	Notices    []ProgressionNotice
}
