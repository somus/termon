package balance

import (
	"fmt"

	"termon.sh/internal/capture"
	"termon.sh/internal/content"
)

// CaptureSmoke summarizes capture generator eligibility smoke checks.
type CaptureSmoke struct {
	Total    int  `json:"total"`
	Eligible int  `json:"eligible"`
	Failed   int  `json:"failed"`
	Passed   bool `json:"passed"`
}

// RunCaptureSmoke verifies capture.Generate for reference parties at checkpoints.
func RunCaptureSmoke(set *content.Set) (*CaptureSmoke, error) {
	smoke := &CaptureSmoke{}
	for _, team := range ReferenceTeams {
		for _, level := range NaturalCheckpoints {
			party, err := naturalPartyFighters(set, team, 0, level)
			if err != nil {
				return nil, err
			}
			for _, family := range team.Families {
				smoke.Total++
				_, err = capture.Generate(set, party, family)
				if err != nil {
					smoke.Failed++
					continue
				}
				smoke.Eligible++
				_ = party
			}
		}
	}
	smoke.Passed = smoke.Failed == 0 && smoke.Eligible > 0
	return smoke, nil
}

func naturalPartyFighters(set *content.Set, team ReferenceTeam, lead int, level int) ([]capture.PartyFighter, error) {
	bp, err := BuildNaturalParty(set, team, lead, level, "capture-smoke", false)
	if err != nil {
		return nil, err
	}
	var fighters []capture.PartyFighter
	for _, m := range bp.Members {
		fighters = append(fighters, capture.PartyFighter{
			Species: m.Monster.Species,
			Level:   m.Monster.Level,
			Loadout: append([]string(nil), m.Monster.BattleLoadout...),
		})
	}
	return fighters, nil
}

// CaptureSmokeSummary returns a one-line status string.
func CaptureSmokeSummary(s *CaptureSmoke) string {
	if s == nil {
		return ""
	}
	return fmt.Sprintf("capture smoke: %d/%d eligible", s.Eligible, s.Total)
}
