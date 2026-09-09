package server

import (
	"testing"
	"time"

	"termon.sh/internal/battle"
	"termon.sh/internal/dojo"
	"termon.sh/internal/game"
)

func TestDailyPolicyUsesSharedFixtureSeed(t *testing.T) {
	h := testHub(t)
	for _, fixture := range dojo.DailyFixtures {
		t.Run(fixture.ID, func(t *testing.T) {
			player, opponent, err := dojo.DailyParties(h.set, fixture)
			if err != nil {
				t.Fatal(err)
			}
			bt, err := battle.New(h.set, player, opponent, battle.Seeded(fixture.Seed))
			if err != nil {
				t.Fatal(err)
			}
			mode := newDailyMode(fixture, time.Time{}, dojo.TierConfig(fixture.PolicyTier), &game.Save{})
			for _, replacement := range []bool{false, true} {
				first := mode.policyRNG(&match{bt: bt}, replacement)
				replay := mode.policyRNG(&match{bt: bt}, replacement)
				proof := dojo.DailyPolicyRNG(fixture.Seed, bt.Turn(), replacement)
				for range 8 {
					want := proof.Float64()
					if first.Float64() != want || replay.Float64() != want {
						t.Fatal("live Daily policy and repeated proof use different random streams")
					}
				}
			}
		})
	}
}
