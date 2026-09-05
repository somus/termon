package server

import (
	"termon.sh/internal/game"
	"termon.sh/internal/store"
	"termon.sh/internal/telemetry"
)

func (h *Hub) recordActivityResult(rec store.ActivityRecord) (*game.Save, error) {
	save, err := h.saves.RecordActivityResult(rec)
	if err != nil {
		return nil, err
	}
	activityID := telemetry.DeterministicID("activity", rec.NaturalKey)
	h.recordEvent(telemetry.Event{
		ID:   telemetry.DeterministicID(telemetry.EventActivityEnded, rec.NaturalKey),
		Name: telemetry.EventActivityEnded, TrainerID: rec.TrainerID, ActivityID: activityID,
		Properties: map[string]any{
			"activity_kind": string(rec.Kind), "outcome": rec.Outcome,
			"sparring_tier": rec.SparringTier, "captured": rec.Capture != nil,
		},
	})
	return save, nil
}
