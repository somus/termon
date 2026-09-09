package balance

import (
	"fmt"
	"slices"

	"termon.sh/internal/dojo"
)

// runMatrix keeps independent stage/loadout/policy/checkpoint strata. Full
// events stream before aggregation; memory does not grow with the seed corpus.
func runMatrix(cfg Config, teams []ReferenceTeam, out *RunOutput) error {
	policies := []string{dojo.ReferencePressure, dojo.ReferencePivot, dojo.ReferencePreservation}
	if cfg.ReferencePolicy != "" {
		policies = []string{cfg.ReferencePolicy}
	}
	levels := len(NaturalMatrixLevels(cfg.Set))
	if cfg.NormalizedOnly {
		levels = 0
	}
	out.Snapshot.Coverage = MatrixCoverage{Policies: len(policies), NormalizedStages: 3, NaturalLevels: levels, LoadoutSets: 3, IndependentLeads: 9, PhysicalPlacements: 4, RemainingAxes: []string{"counterplay switch/stay scenarios", "capture trajectory matrix", "Dojo tier and Daily trajectory gates"}}
	type stratum struct {
		evidence MatrixEvidence
		counts   *outcomeAccumulator
	}
	groups := map[string]*stratum{}
	finish := func() {
		keys := make([]string, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			group := groups[key]
			group.evidence.Gates = group.counts.Gates()
			group.evidence.MoveChoices = group.counts.choices.Rows()
			group.evidence.VoluntarySwitches = group.counts.choices.VoluntarySwitches
			group.evidence.MoveGates = group.counts.choices.Gates()
			for _, sample := range group.counts.FailedSamples() {
				group.evidence.Samples = append(group.evidence.Samples, *sample)
			}
			out.Matrix = append(out.Matrix, group.evidence)
		}
	}
	for _, policy := range policies {
		policyCfg := cfg
		policyCfg.ReferencePolicy = policy
		err := EnumerateMatrixScenarios(cfg.Set, teams, func(sc Scenario) error {
			if cfg.NormalizedOnly && sc.Kind == "natural" {
				return nil
			}
			kind := "normalized"
			if sc.Kind == "natural" {
				kind = "natural"
			}
			key := fmt.Sprintf("%s/%s/%s/%s/%d", policy, kind, sc.Stage, sc.Loadout, sc.Level)
			group := groups[key]
			if group == nil {
				group = &stratum{evidence: MatrixEvidence{Policy: policy, Kind: kind, Stage: sc.Stage, Loadout: sc.Loadout, Level: sc.Level}, counts: newOutcomeAccumulator()}
				groups[key] = group
			}
			sc.Name = fmt.Sprintf("%s/%s", policy, sc.Name)
			prepared, err := prepareScenario(policyCfg, sc)
			if err != nil {
				return err
			}
			for _, seed := range cfg.Seeds {
				sc.Seed = seed
				result, runErr := runPreparedScenario(policyCfg, sc, prepared)
				if result != nil {
					out.BattlesRun++
					if err := writeOutcome(cfg.Outcomes, result, cfg.Policy); err != nil {
						return err
					}
				}
				if runErr != nil {
					if result != nil {
						out.FailedGates = append(out.FailedGates, *outcomeReport("incomplete_scenario", result))
					}
					return fmt.Errorf("%s seed %d: %w", sc.Name, seed, runErr)
				}
				group.counts.Observe(result)
				group.evidence.Battles++
				out.Snapshot.Coverage.CompletedBattles++
				if kind == "natural" {
					clear(group.counts.samples)
				}
			}
			return nil
		})
		if err != nil {
			finish()
			return err
		}
	}
	finish()
	out.Snapshot.Coverage.Complete = true
	return nil
}
