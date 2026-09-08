package balance

import (
	"testing"

	"termon.sh/internal/battle"
)

func TestNeutralPaceUsesEveryContributingHit(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		firstStage           int
		firstSE              bool
		wantSE, wantMismatch bool
	}{
		{"earlier super effective hit", 1, true, true, false},
		{"earlier cross stage hit", 0, false, false, true},
		{"entirely neutral same stage", 1, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := []battle.Event{{Kind: battle.EventMoveUsed, MonsterID: "first"}}
			if tc.firstSE {
				events = append(events, battle.Event{Kind: battle.EventSuperEffective})
			}
			events = append(events, battle.Event{Kind: battle.EventDamageDealt, TargetID: "target"}, battle.Event{Kind: battle.EventMoveUsed, MonsterID: "second"}, battle.Event{Kind: battle.EventDamageDealt, TargetID: "target"}, battle.Event{Kind: battle.EventFainted, MonsterID: "target"})
			got := collectFaintPaces(events, map[string]int{"first": tc.firstStage, "second": 1, "target": 1})
			if len(got) != 1 || got[0].Hits != 2 || got[0].SuperEffective != tc.wantSE || got[0].StageMismatch != tc.wantMismatch {
				t.Fatalf("wrong history classification: %+v", got)
			}
		})
	}
}
