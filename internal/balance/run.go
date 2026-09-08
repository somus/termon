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
	Set            *content.Set
	Seeds          []uint64
	SeedBase       uint64
	Policy         dojo.PolicyConfig
	MaxTurns       int
	Rules          string
	ContentID      string
	NormalizedOnly bool
	TeamLimit      int // 0 = all teams
	CaptureSmoke   bool
	FailGates      bool
	Outcomes       io.Writer
	GitRevision    string
}

// Snapshot records run identity for reproducibility.
type Snapshot struct {
	BoundaryEvidence string            `json:"boundary_evidence"`
	ContentRevision  string            `json:"content_revision"`
	RulesRevision    string            `json:"rules_revision"`
	PackageIdentity  string            `json:"package_identity"`
	ReferenceTeams   []ReferenceTeam   `json:"reference_teams"`
	Policy           dojo.PolicyConfig `json:"policy"`
	SeedBase         uint64            `json:"seed_base"`
	SeedCount        int               `json:"seed_count"`
	GitRevision      string            `json:"git_revision"`
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

	teams := ReferenceTeams
	if cfg.TeamLimit > 0 && cfg.TeamLimit < len(teams) {
		teams = teams[:cfg.TeamLimit]
	}

	out := &RunOutput{
		Snapshot: Snapshot{
			BoundaryEvidence: "PolicyFoe excludes private loadouts and reserve HP; covered by battle policy-view regression tests, not a runtime hidden-read counter",
			ContentRevision:  cfg.ContentID,
			RulesRevision:    cfg.Rules,
			PackageIdentity:  PackageIdentity,
			ReferenceTeams:   teams,
			Policy:           cfg.Policy,
			SeedBase:         cfg.SeedBase,
			SeedCount:        len(cfg.Seeds),
			GitRevision:      revisionOrUnknown(cfg.GitRevision),
		},
	}

	return runAudit(cfg, teams, out)
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
