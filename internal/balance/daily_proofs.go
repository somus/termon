package balance

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
)

//go:embed testdata/daily-proofs.json
var dailyProofData []byte

type recordedDailyLine struct {
	ID        string          `json:"id"`
	Seed      uint64          `json:"seed"`
	WithinPar bool            `json:"within_par"`
	Choices   []battle.Action `json:"choices"`
}

// recordedDailyProof revalidates discovered lines against current engine and
// content. A stale line is never evidence; bounded search can replace it.
func recordedDailyProof(set *content.Set, fixture dojo.DailyFixture, withinPar bool) (DailyProofLine, bool, error) {
	var lines []recordedDailyLine
	if err := json.Unmarshal(dailyProofData, &lines); err != nil {
		return DailyProofLine{}, false, fmt.Errorf("daily proof fixtures: %w", err)
	}
	for _, line := range lines {
		if line.ID != fixture.ID || line.Seed != fixture.Seed || line.WithinPar != withinPar {
			continue
		}
		replay, err := replayDaily(set, fixture, line.Choices)
		if err == nil && dailyLineMatches(replay, withinPar) {
			return replay.line, true, nil
		}
	}
	return DailyProofLine{}, false, nil
}
