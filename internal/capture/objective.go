// Package capture evaluates Capture Gauge progress for Target Encounters and Lessons.
package capture

// ObjectiveID is a stable capture objective identifier.
type ObjectiveID string

// Launch objective IDs from capture-catalog.md.
const (
	ShowMoveVariety  ObjectiveID = "show_move_variety"
	ReadTheMatchup   ObjectiveID = "read_the_matchup"
	SafeSwitch       ObjectiveID = "safe_switch"
	MeasuredPressure ObjectiveID = "measured_pressure"
	HoldTheLine      ObjectiveID = "hold_the_line"
)

// Objective is one visible capture requirement with a fixed award.
type Objective struct {
	ID          ObjectiveID
	DisplayName string
	Award       int
	Description string
}

var catalog = map[ObjectiveID]Objective{
	ShowMoveVariety: {
		ID: ShowMoveVariety, DisplayName: "Use 3 different Moves", Award: 30,
		Description: "Resolve three turns using three distinct Moves from the active Party.",
	},
	ReadTheMatchup: {
		ID: ReadTheMatchup, DisplayName: "Land a super-effective Move", Award: 35,
		Description: "Hit with a Move that is 2× Type against the Wild Monster.",
	},
	SafeSwitch: {
		ID: SafeSwitch, DisplayName: "Safe switch", Award: 35,
		Description: "Complete a voluntary Switch into a healthy reserve with at least 50% HP.",
	},
	MeasuredPressure: {
		ID: MeasuredPressure, DisplayName: "Measured pressure", Award: 35,
		Description: "Deal positive damage on two different turns while the Wild Monster is above 25% HP at the end of the second turn.",
	},
	HoldTheLine: {
		ID: HoldTheLine, DisplayName: "Hold the line", Award: 35,
		Description: "After the Wild Monster acts, end a resolved turn with the active Monster above 50% HP.",
	},
}

// ObjectiveByID returns the catalog entry for id.
func ObjectiveByID(id ObjectiveID) (Objective, bool) {
	o, ok := catalog[id]
	return o, ok
}

// AuthoredLessonObjectives returns fixed lesson objective lists from dojo-policy.md.
func AuthoredLessonObjectives(lesson int) []ObjectiveID {
	switch lesson {
	case 1:
		return []ObjectiveID{ShowMoveVariety, ReadTheMatchup, HoldTheLine}
	case 2:
		return []ObjectiveID{ShowMoveVariety, SafeSwitch, ReadTheMatchup}
	default:
		return nil
	}
}
