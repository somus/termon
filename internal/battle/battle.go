// Package battle owns the three-Monster combat engine and the vocabulary shared
// with TUI clients. Spec: docs/design/party-battles.md, docs/design/combat.md.
package battle

import (
	"termon.sh/internal/game"
)

// PartyMember is one Monster fielded in a Battle Party copy.
type PartyMember struct {
	Monster game.Monster // must have ID, Species, BattleLoadout, Level
	MaxHP   int          // when > 0, overrides natural MaxHP (Capture HP)
	Stats   *[5]int      // optional hp, atk, def, spa, spe override (normalized PvP)
}

// Party is one Trainer's side: 1–3 Monsters with no vacant slots.
type Party struct {
	Trainer             string
	Members             []PartyMember // slot 0 is the opening lead
	ClampOutgoingDamage bool          // caps this side's damage (wild Target clamp)
}

// ActionKind classifies a hidden Battle Action.
type ActionKind string

// Battle action kinds.
const (
	ActionMove   ActionKind = "move"
	ActionSwitch ActionKind = "switch"
)

// Action is one immutable hidden choice during awaiting_actions.
type Action struct {
	Kind     ActionKind
	Move     string // loadout slug when Kind=move
	SwitchTo string // monster ID when Kind=switch
}

// State is a Battle's phase in the state machine.
type State string

// Battle states in transition order.
const (
	StateRevealing           State = "revealing"
	StateAwaitingActions     State = "awaiting_actions"
	StateResolvingTurn       State = "resolving_turn"
	StateAwaitingReplacement State = "awaiting_replacement"
	StateOver                State = "battle_over"
)

// EndReason is why a Battle reached battle_over.
type EndReason string

// Reasons a battle can end.
const (
	EndKO                EndReason = "ko"
	EndForfeit           EndReason = "forfeit"
	EndDisconnectTimeout EndReason = "disconnect_timeout"
	EndDecisionTimeout   EndReason = "decision_timeout"
)

// EventKind classifies one entry in the ordered battle log.
type EventKind string

// Kinds of battle log events.
const (
	EventTurnStarted      EventKind = "turn_started"
	EventSendOut          EventKind = "send_out"
	EventSwitched         EventKind = "switched"
	EventReplacement      EventKind = "replacement"
	EventMoveUsed         EventKind = "move_used"
	EventMissed           EventKind = "missed"
	EventCriticalHit      EventKind = "critical_hit"
	EventSuperEffective   EventKind = "super_effective"
	EventNotVeryEffective EventKind = "not_very_effective"
	EventDamageDealt      EventKind = "damage_dealt"
	EventFainted          EventKind = "fainted"
	EventForfeit          EventKind = "forfeit"
	EventDecisionTimeout  EventKind = "decision_timeout"
	EventBattleOver       EventKind = "battle_over"
)

// Event is one entry in the ordered battle log both clients render from.
type Event struct {
	Turn      int       `json:"turn"`
	Actor     string    `json:"actor,omitempty"` // acting trainer
	MonsterID string    `json:"monster_id,omitempty"`
	Slot      int       `json:"slot,omitempty"`
	TargetID  string    `json:"target_id,omitempty"`
	MoveSlug  string    `json:"move_slug,omitempty"`
	Kind      EventKind `json:"kind"`
	Text      string    `json:"text,omitempty"`
	Damage    int       `json:"damage,omitempty"` // HP lost by TargetID on EventDamageDealt
}

// Fighter is a convenience snapshot of one Trainer's active Monster.
type Fighter struct {
	Trainer string
	ID      string // Monster ID of the fielded member
	Name    string
	Species string
	Type    string
	HP      int
	MaxHP   int
	Moves   []string
}

// SnapshotMember is one Monster on the viewer's Party in a viewer snapshot.
type SnapshotMember struct {
	Slot    int
	ID      string
	Species string
	Name    string
	HP      int
	MaxHP   int
	Fainted bool
	Active  bool
	Loadout []string // only populated for the viewer's own Party
}

// SnapshotFoe is public roster information for one opposing Monster.
type SnapshotFoe struct {
	Species string
	Name    string
	ID      string
	Fainted bool
	Active  bool
	HP      int // only for the active Monster; 0 for reserves
	MaxHP   int // only for the active Monster; 0 for reserves
}

// FieldedHP is the plate HP of the Monster a side had fielded at a replay
// position.
type FieldedHP struct {
	MonsterID string
	HP        int
	MaxHP     int
}

// Snapshot is a viewer-specific Battle view for TUI rendering.
type Snapshot struct {
	Phase               State
	Turn                int
	Winner              string
	Reason              EndReason
	YourParty           []SnapshotMember
	FoeRoster           []SnapshotFoe
	YouLocked           bool
	YouLockKind         ActionKind
	FoeLocked           bool
	ReplacementRequired bool
}

// HealthyReserves returns viewer Party members that can be switched in.
func (s Snapshot) HealthyReserves() []SnapshotMember {
	var out []SnapshotMember
	for _, m := range s.YourParty {
		if !m.Active && !m.Fainted && m.HP > 0 {
			out = append(out, m)
		}
	}
	return out
}

// YourPartyActiveLoadout returns the active Monster's Battle Loadout for the viewer.
func (s Snapshot) YourPartyActiveLoadout() []string {
	for _, m := range s.YourParty {
		if m.Active {
			return append([]string(nil), m.Loadout...)
		}
	}
	return nil
}
