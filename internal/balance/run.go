package balance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
)

// Config controls one Balance Run.
type Config struct {
	Set             *content.Set
	Seeds           []uint64
	SeedBase        uint64
	Policy          dojo.PolicyConfig
	MaxTurns        int
	Rules           string
	ContentID       string
	NormalizedOnly  bool
	TeamLimit       int // 0 = all teams
	CaptureSmoke    bool
	FailGates       bool
	Outcomes        io.Writer
	GitRevision     string
	ReferencePolicy string
	Mode            string
}

// Snapshot records run identity for reproducibility.
type Snapshot struct {
	BoundaryEvidence        string            `json:"boundary_evidence"`
	ContentRevision         string            `json:"content_revision"`
	RulesRevision           string            `json:"rules_revision"`
	PackageIdentity         string            `json:"package_identity"`
	ReferenceTeams          []ReferenceTeam   `json:"reference_teams"`
	Policy                  dojo.PolicyConfig `json:"policy"`
	SeedBase                uint64            `json:"seed_base"`
	SeedCount               int               `json:"seed_count"`
	GitRevision             string            `json:"git_revision"`
	ReferencePolicy         string            `json:"reference_policy,omitempty"`
	ReferencePolicyRevision string            `json:"reference_policy_revision"`
	Mode                    string            `json:"mode"`
	Coverage                MatrixCoverage    `json:"coverage,omitzero"`
}

// RunOutput is the machine-readable Balance Run result.
type RunOutput struct {
	Snapshot     Snapshot           `json:"snapshot"`
	BattlesRun   int                `json:"battles_run"`
	Gates        []GateResult       `json:"gates"`
	FailedGates  []FailedGateReport `json:"failed_gates,omitempty"`
	FirstFailure string             `json:"first_failed_gate,omitempty"`
	Passed       bool               `json:"passed"`
	CaptureSmoke *CaptureSmoke      `json:"capture_smoke,omitempty"`
	Matrix       []MatrixEvidence   `json:"matrix,omitempty"`
}

// MatrixCoverage states exactly what the opt-in matrix exercised.
type MatrixCoverage struct {
	Complete           bool     `json:"complete"`
	CompletedBattles   int      `json:"completed_battles"`
	Policies           int      `json:"policies,omitempty"`
	NormalizedStages   int      `json:"normalized_stages,omitempty"`
	NaturalLevels      int      `json:"natural_levels,omitempty"`
	LoadoutSets        int      `json:"loadout_sets,omitempty"`
	IndependentLeads   int      `json:"independent_leads,omitempty"`
	PhysicalPlacements int      `json:"physical_placements,omitempty"`
	RemainingAxes      []string `json:"remaining_axes,omitempty"`
}

// MatrixEvidence holds one policy's normalized gate evidence and bounded samples.
type MatrixEvidence struct {
	Policy            string           `json:"policy"`
	Kind              string           `json:"kind"`
	Stage             string           `json:"stage"`
	Loadout           string           `json:"loadout"`
	Level             int              `json:"level"`
	Battles           int              `json:"battles"`
	VoluntarySwitches int              `json:"voluntary_switches"`
	MoveChoices       []MoveChoiceRow  `json:"move_choices"`
	MoveGates         []MoveChoiceGate `json:"move_gates"`
	Gates             []GateResult     `json:"gates"`
	Samples           []BattleOutcome  `json:"samples"`
}

// Run executes the configured Balance Run corpus.
func Run(cfg Config) (*RunOutput, error) {
	if cfg.Set == nil {
		return nil, errors.New("balance: nil content")
	}
	if len(cfg.Seeds) == 0 {
		cfg.Seeds = CorpusSeeds(DefaultSeedBase, DefaultCorpusSize)
	}
	if cfg.SeedBase == 0 {
		cfg.SeedBase = DefaultSeedBase
	}
	if cfg.Rules == "" {
		cfg.Rules = RulesRevision
	}
	if cfg.ContentID == "" {
		return nil, errors.New("balance: content revision required")
	}
	if cfg.Policy.Tier == "" {
		cfg.Policy = DefaultPolicy()
	}
	if cfg.Mode == "" {
		cfg.Mode = "audit"
	}
	if cfg.Mode != "audit" && cfg.Mode != "matrix" {
		return nil, fmt.Errorf("balance: unknown mode %q", cfg.Mode)
	}

	teams := ReferenceTeams
	if cfg.TeamLimit > 0 && cfg.TeamLimit < len(teams) {
		teams = teams[:cfg.TeamLimit]
	}

	out := &RunOutput{
		Snapshot: Snapshot{
			BoundaryEvidence:        "PolicyFoe excludes private loadouts and reserve HP; covered by battle policy-view regression tests, not a runtime hidden-read counter",
			ContentRevision:         cfg.ContentID,
			RulesRevision:           cfg.Rules,
			PackageIdentity:         PackageIdentity,
			ReferenceTeams:          teams,
			Policy:                  cfg.Policy,
			SeedBase:                cfg.SeedBase,
			SeedCount:               len(cfg.Seeds),
			GitRevision:             revisionOrUnknown(cfg.GitRevision),
			ReferencePolicy:         cfg.ReferencePolicy,
			ReferencePolicyRevision: dojo.ReferencePolicyRevision,
			Mode:                    cfg.Mode,
		},
	}

	if cfg.Mode == "matrix" {
		if err := runMatrix(cfg, teams, out); err != nil {
			out.FirstFailure = "incomplete_scenario"
			return out, err
		}
		return finishMatrix(cfg, out)
	}
	return runAudit(cfg, teams, out)
}

func finishMatrix(cfg Config, out *RunOutput) (*RunOutput, error) {
	if cfg.CaptureSmoke {
		smoke, err := RunCaptureSmoke(cfg.Set)
		if err != nil {
			return out, err
		}
		out.CaptureSmoke = smoke
	}
	out.Passed = true
	if out.CaptureSmoke != nil {
		gate := evalCaptureSmoke(out.CaptureSmoke)
		out.Gates = append(out.Gates, gate)
		if !gate.Passed {
			out.Passed = false
			out.FirstFailure = gate.Name
		}
	}
	for _, evidence := range out.Matrix {
		for _, gate := range evidence.MoveGates {
			if !gate.Passed {
				out.Passed = false
				out.FirstFailure = "conditional_move_dominance"
				break
			}
		}
		for _, gate := range evidence.Gates {
			if !gate.Passed {
				out.Passed = false
				out.FirstFailure = gate.Name
				break
			}
		}
		if !out.Passed {
			break
		}
	}
	if cfg.FailGates && !out.Passed {
		return out, fmt.Errorf("balance: gate failed: %s", out.FirstFailure)
	}
	return out, nil
}

func runAudit(cfg Config, teams []ReferenceTeam, out *RunOutput) (*RunOutput, error) {
	var results []*BattleOutcome
	for _, seed := range cfg.Seeds {
		for i, teamA := range teams {
			for j, teamB := range teams {
				if j < i {
					continue
				}
				for lead := range 3 {
					if teamA.Name == teamB.Name {
						sc := MirrorScenario(teamA, lead, seed)
						res, err := RunScenario(cfg, sc)
						if res != nil {
							results = append(results, res)
							out.BattlesRun++
							if streamErr := writeOutcome(cfg.Outcomes, res, cfg.Policy); streamErr != nil {
								return out, streamErr
							}
						}
						if err != nil {
							out.Gates = EvaluateGates(results, out.CaptureSmoke)
							out.FailedGates = BuildFailedReports(results, out.Gates)
							return out, err
						}
						continue
					}
					paired, err := PairedNormalizedRuns(cfg, teamA, teamB, lead, seed)
					for _, res := range paired {
						results = append(results, res)
						out.BattlesRun++
						if streamErr := writeOutcome(cfg.Outcomes, res, cfg.Policy); streamErr != nil {
							return out, streamErr
						}
					}
					if err != nil {
						out.Gates = EvaluateGates(results, out.CaptureSmoke)
						out.FailedGates = BuildFailedReports(results, out.Gates)
						return out, err
					}
				}
			}
		}
	}

	if cfg.CaptureSmoke {
		smoke, err := RunCaptureSmoke(cfg.Set)
		if err != nil {
			return out, err
		}
		out.CaptureSmoke = smoke
	}

	return finishRun(cfg, out, results)
}

func finishRun(cfg Config, out *RunOutput, results []*BattleOutcome) (*RunOutput, error) {
	gates := EvaluateGates(results, out.CaptureSmoke)
	out.Gates = gates
	for _, g := range gates {
		if !g.Passed {
			out.FirstFailure = g.Name
			break
		}
	}
	out.FailedGates = BuildFailedReports(results, gates)
	out.Passed = out.FirstFailure == ""

	if cfg.FailGates && !out.Passed {
		return out, fmt.Errorf("balance: gate failed: %s", out.FirstFailure)
	}
	return out, nil
}

func revisionOrUnknown(revision string) string {
	if revision == "" {
		return "unknown"
	}
	return revision
}

// FormatSummary prints a bounded terminal summary.
func FormatSummary(out *RunOutput) string {
	type row struct {
		Snapshot Snapshot     `json:"snapshot"`
		Battles  int          `json:"battles_run"`
		Passed   bool         `json:"passed"`
		Gates    []GateResult `json:"gates"`
		First    string       `json:"first_failed_gate,omitempty"`
	}
	payload, err := json.MarshalIndent(row{
		Snapshot: out.Snapshot,
		Battles:  out.BattlesRun,
		Passed:   out.Passed,
		Gates:    out.Gates,
		First:    out.FirstFailure,
	}, "", "  ")
	if err != nil {
		return fmt.Sprintf("summary encode error: %v", err)
	}
	return string(payload)
}

// SortGateResults orders gate results by name for stable output.
func SortGateResults(gates []GateResult) {
	slices.SortFunc(gates, func(a, b GateResult) int { return strings.Compare(a.Name, b.Name) })
}
