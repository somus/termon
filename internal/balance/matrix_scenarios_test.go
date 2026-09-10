package balance_test

import (
	"slices"
	"testing"

	"termon.sh/internal/balance"
	"termon.sh/internal/dojo"
)

func TestEnumerateMatrixScenariosCoversStagesAndPlacements(t *testing.T) {
	set := loadContent(t)
	placements, normalized := 0, 0
	seen := map[string]bool{}
	err := balance.EnumerateMatrixScenarios(set, balance.ReferenceTeams, func(sc balance.Scenario) error {
		placements++
		if sc.Kind == "normalized" {
			normalized++
			for _, side := range []struct {
				team balance.ReferenceTeam
				lead int
			}{{sc.TeamA, sc.LeadA}, {sc.TeamB, sc.LeadB}} {
				party, err := balance.FixtureParty(set, side.team, side.lead, 30, "test", false, true, sc.Stage, sc.Loadout)
				if err != nil {
					return err
				}
				for _, member := range party.Members {
					seen[member.Monster.Species] = true
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if placements == 0 || normalized == 0 {
		t.Fatal("missing matrix scenarios")
	}
	if len(seen) != 72 {
		missing := []string{}
		for slug := range set.Species {
			if !seen[slug] {
				missing = append(missing, slug)
			}
		}
		t.Fatalf("normalized species=%d, want 72; missing=%v", len(seen), missing)
	}
}

func TestNaturalMatrixLevelsIncludeThresholdBoundaries(t *testing.T) {
	set := loadContent(t)
	levels := balance.NaturalMatrixLevels(set)
	for _, checkpoint := range []int{1, 14, 24, 30, 40, 50} {
		if !slices.Contains(levels, checkpoint) {
			t.Fatalf("missing checkpoint %d", checkpoint)
		}
	}
	for _, species := range set.Species {
		if species.EvolvesTo != nil {
			for _, level := range []int{species.EvolvesTo.Level - 1, species.EvolvesTo.Level, species.EvolvesTo.Level + 1} {
				if !slices.Contains(levels, level) {
					t.Fatalf("missing threshold level %d", level)
				}
			}
		}
	}
}

func TestNaturalMatrixEnumeratesDeferredBaseStage(t *testing.T) {
	set := loadContent(t)
	found := false
	err := balance.EnumerateMatrixScenarios(set, balance.ReferenceTeams[:1], func(sc balance.Scenario) error {
		if sc.Kind != "natural" || sc.Level != 50 || sc.Stage != "base" {
			return nil
		}
		party, err := balance.FixtureParty(set, sc.TeamA, sc.LeadA, sc.Level, "test", sc.PartyOrderSwapped, false, sc.Stage, sc.Loadout)
		if err != nil {
			return err
		}
		if party.Members[0].Monster.Species == "rootkit" {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("level-50 deferred base fixture missing")
	}
}

func TestMatrixIncompleteScenarioRetainsCompletedCoverage(t *testing.T) {
	out, err := balance.Run(balance.Config{
		Set: loadContent(t), ContentID: "test-pack", Mode: "matrix",
		NormalizedOnly: true, ReferencePolicy: "preservation",
		Seeds: []uint64{14180207640020093695, 1}, TeamLimit: 1, MaxTurns: 9,
	})
	if err == nil || out == nil {
		t.Fatalf("expected retained partial report, got %v, %v", out, err)
	}
	if out.Passed || out.Snapshot.Coverage.Complete || out.FirstFailure != "incomplete_scenario" {
		t.Fatalf("incomplete report claims success: passed=%v coverage=%+v first=%q", out.Passed, out.Snapshot.Coverage, out.FirstFailure)
	}
	if out.Snapshot.Coverage.CompletedBattles < 1 || out.BattlesRun != out.Snapshot.Coverage.CompletedBattles+1 {
		t.Fatalf("attempted=%d completed=%d; want completed battles plus one incomplete attempt", out.BattlesRun, out.Snapshot.Coverage.CompletedBattles)
	}
	if len(out.Snapshot.ReferenceTeams) != 1 || len(out.FailedGates) != 1 {
		t.Fatalf("wrong retained teams/failures: %d/%d", len(out.Snapshot.ReferenceTeams), len(out.FailedGates))
	}
}

func TestPreservationResponseEndsRetainedCycle(t *testing.T) {
	sc := balance.NormalizedScenario("retained-starter-cycle", balance.ReferenceTeams[0], balance.ReferenceTeams[0], 0)
	sc.Stage, sc.Loadout = balance.FixtureStageBase, balance.FixtureLoadoutDefault
	sc.Seed = 14180207640020093695
	out, err := balance.RunScenario(balance.Config{Set: loadContent(t), ReferencePolicy: dojo.ReferencePreservation}, sc)
	if err != nil {
		t.Fatal(err)
	}
	if out.Winner == "" || out.Turns > 9 || out.IllegalActions != 0 {
		t.Fatalf("retained cycle: winner=%q turns=%d illegal=%d", out.Winner, out.Turns, out.IllegalActions)
	}
	if out.ReferencePolicyRevision != dojo.ReferencePolicyRevision {
		t.Fatalf("missing reference-policy revision: %q", out.ReferencePolicyRevision)
	}
}
