package balance_test

import (
	"reflect"
	"testing"

	"termon.sh/internal/balance"
	"termon.sh/internal/onboard"
)

func TestFixturePartyBuildsEveryNormalizedStageAndLoadout(t *testing.T) {
	set := loadContent(t)
	for _, team := range balance.ReferenceTeams {
		for _, stage := range []string{"base", "middle", "final"} {
			for _, loadout := range []string{"default", "reference", "frontier"} {
				party, err := balance.FixtureParty(set, team, 0, 30, "fixture", false, true, stage, loadout)
				if err != nil {
					t.Fatalf("%s %s %s: %v", team.Name, stage, loadout, err)
				}
				if len(party.Members) != 3 {
					t.Fatalf("%s: members=%d", team.Name, len(party.Members))
				}
				for _, member := range party.Members {
					if len(member.Monster.BattleLoadout) != 4 {
						t.Fatalf("%s %s %s: loadout=%v", team.Name, stage, loadout, member.Monster.BattleLoadout)
					}
				}
			}
		}
	}
}

func TestNaturalDefaultUsesOnboardingBaselineNotReference(t *testing.T) {
	set := loadContent(t)
	team := balance.ReferenceTeams[0]
	defaultParty, err := balance.FixtureParty(set, team, 0, 50, "fixture", false, false, "base", "default")
	if err != nil {
		t.Fatal(err)
	}
	referenceParty, err := balance.FixtureParty(set, team, 0, 50, "fixture", false, false, "base", "reference")
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := onboard.DefaultLoadout(set, defaultParty.Members[0].Monster.Species)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(defaultParty.Members[0].Monster.BattleLoadout, baseline.BattleLoadout) {
		t.Fatalf("natural default = %v, want onboarding baseline %v", defaultParty.Members[0].Monster.BattleLoadout, baseline.BattleLoadout)
	}
	if len(defaultParty.Members[0].Monster.BattleLoadout) != 4 {
		t.Fatalf("natural default loadout = %v, want four baseline moves", defaultParty.Members[0].Monster.BattleLoadout)
	}
	if reflect.DeepEqual(defaultParty.Members[0].Monster.BattleLoadout, referenceParty.Members[0].Monster.BattleLoadout) {
		t.Fatalf("natural default and reference unexpectedly match: %v", defaultParty.Members[0].Monster.BattleLoadout)
	}
}

func TestFixturePartyAllowsDeferredNaturalStages(t *testing.T) {
	set := loadContent(t)
	party, err := balance.FixtureParty(set, balance.ReferenceTeams[0], 0, 50, "fixture", false, false, "base", "frontier")
	if err != nil {
		t.Fatal(err)
	}
	if party.Members[0].Monster.Species != "rootkit" {
		t.Fatalf("species=%q", party.Members[0].Monster.Species)
	}
}

func TestFixturePartyRejectsUnknownAndUnreachableVariants(t *testing.T) {
	set := loadContent(t)
	team := balance.ReferenceTeams[0]
	if _, err := balance.FixtureParty(set, team, 0, 1, "fixture", false, false, "final", "default"); err == nil {
		t.Fatal("unreachable natural final accepted")
	}
	if _, err := balance.FixtureParty(set, team, 0, 30, "fixture", false, true, "unknown", "default"); err == nil {
		t.Fatal("unknown stage accepted")
	}
	if _, err := balance.FixtureParty(set, team, 0, 30, "fixture", false, true, "base", "unknown"); err == nil {
		t.Fatal("unknown loadout accepted")
	}
	unknown := team
	unknown.Families[0] = "missing"
	if _, err := balance.FixtureParty(set, unknown, 0, 30, "fixture", false, true, "base", "default"); err == nil {
		t.Fatal("unknown family accepted")
	}
	if _, err := balance.FixtureParty(set, team, 0, 51, "fixture", false, true, "base", "default"); err == nil {
		t.Fatal("out-of-range level accepted")
	}
}
