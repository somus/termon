package store

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"termon.sh/internal/content"
	"termon.sh/internal/game"
)

type activityTransition struct {
	save         *game.Save
	result       ActivityResult
	dailyMastery *ActivityResult
}

// planActivityTransition computes every mutation caused by a new activity.
// Adapters remain responsible for duplicate detection and atomically persisting
// the returned records and Save.
func planActivityTransition(
	set *content.Set,
	current *game.Save,
	rec ActivityRecord,
	seed string,
) (activityTransition, error) {
	ids := activityIDs{seed: seed}
	save := cloneSave(current)
	payload, err := canonicalActivityPayload(rec)
	if err != nil {
		return activityTransition{}, err
	}

	var capturedID string
	if rec.Capture != nil && rec.Outcome == OutcomeCaptured {
		capturedID = ids.nextID()
		mon, err := monsterForSpecies(set, rec.Capture.Species, capturedID)
		if err != nil {
			return activityTransition{}, err
		}
		save.Collection = append(save.Collection, mon)
		save.Notices = append(save.Notices, game.ProgressionNotice{
			ID: ids.nextID(), Kind: "capture_review", MonsterID: mon.ID, SourceKey: rec.NaturalKey,
		})
		if rec.Capture.FillParty {
			if slot := firstVacantPartySlot(save.Party); slot >= 0 {
				save.Party[slot] = mon.ID
			}
		}
	}

	if !rec.MasteryOnly && rec.Kind != KindDailyMastery {
		if err := applyActivityRewards(set, save, rec, rec.NaturalKey, ids.nextOpaqueID); err != nil {
			return activityTransition{}, err
		}
	}

	transition := activityTransition{
		save: save,
		result: ActivityResult{
			ID: ids.nextID(), NaturalKey: rec.NaturalKey, TrainerID: rec.TrainerID,
			Kind: rec.Kind, CompletedAt: rec.CompletedAt, Payload: payload,
			CapturedMonsterID: capturedID,
		},
	}
	if rec.Kind == KindDailyXP && rec.DailyParMet {
		masteryRec := rec
		masteryRec.Kind = KindDailyMastery
		masteryRec.NaturalKey = rec.TrainerID + ":daily-mastery:" + rec.CompletedAt.UTC().Format("2006-01-02")
		masteryRec.MasteryOnly = true
		masteryPayload, err := canonicalActivityPayload(masteryRec)
		if err != nil {
			return activityTransition{}, err
		}
		transition.dailyMastery = &ActivityResult{
			ID: ids.nextID(), NaturalKey: masteryRec.NaturalKey, TrainerID: rec.TrainerID,
			Kind: KindDailyMastery, CompletedAt: rec.CompletedAt, Payload: masteryPayload,
		}
	}

	if err := validateSave(set, save); err != nil {
		return activityTransition{}, err
	}
	return transition, nil
}

type activityIDs struct {
	seed string
	next uint64
}

func (s *activityIDs) nextID() string {
	s.next++
	sum := sha256.Sum256([]byte(s.seed + ":" + strconv.FormatUint(s.next, 10)))
	return hex.EncodeToString(sum[:16])
}

func (s *activityIDs) nextOpaqueID() (string, error) {
	return s.nextID(), nil
}
