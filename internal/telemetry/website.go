package telemetry

import (
	"errors"
	"slices"

	"github.com/google/uuid"
)

// Website event names describe explicit actions on the public landing page.
const (
	EventWebsitePageViewed         = "website:page_view"
	EventWebsiteCommandCopied      = "website:command_copy"
	EventWebsiteDemoToggled        = "website:demo_toggle"
	EventWebsiteInstructionsOpened = "website:instructions_open"
)

var websiteOutcomes = map[string][]string{
	EventWebsitePageViewed:         {""},
	EventWebsiteCommandCopied:      {"success", "fallback"},
	EventWebsiteDemoToggled:        {"play", "pause"},
	EventWebsiteInstructionsOpened: {""},
}

// NewWebsiteEvent constructs an anonymous event with only fixed website properties.
func NewWebsiteEvent(visitID, name, outcome string) (Event, error) {
	event := Event{
		Name: name, VisitID: visitID,
		Properties: map[string]any{"page": "/"},
	}
	if outcome != "" {
		event.Properties["outcome"] = outcome
	}
	if err := validateWebsiteEvent(event); err != nil {
		return Event{}, err
	}
	return event, nil
}

func validateWebsiteEvent(event Event) error {
	id, err := uuid.Parse(event.VisitID)
	if err != nil || id == uuid.Nil || id.String() != event.VisitID {
		return errors.New("invalid website Visit ID")
	}
	if event.TrainerID != "" || event.SessionID != "" {
		return errors.New("website event contains gameplay identity")
	}
	if event.BattleID != "" || event.ActivityID != "" || event.ErrorID != "" {
		return errors.New("website event contains gameplay correlation")
	}
	for key, value := range event.Properties {
		switch key {
		case "page":
			if value != "/" {
				return errors.New("invalid website page")
			}
		case "outcome":
			if _, ok := value.(string); !ok {
				return errors.New("invalid website outcome")
			}
		default:
			return errors.New("unknown website property")
		}
	}
	if event.Properties["page"] != "/" {
		return errors.New("missing website page")
	}
	outcome, _ := event.Properties["outcome"].(string)
	if !slices.Contains(websiteOutcomes[event.Name], outcome) {
		return errors.New("invalid website event or outcome")
	}
	return nil
}
