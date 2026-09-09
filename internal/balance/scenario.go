package balance

import (
	"fmt"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
)

// Scenario is one balance-run battle fixture.
type Scenario struct {
	Name              string
	Kind              string // normalized | mirror | natural
	TeamA             ReferenceTeam
	TeamB             ReferenceTeam
	LeadA             int
	LeadB             int
	Level             int
	Seed              uint64
	EngineSideA       bool
	PartyOrderSwapped bool
	Stage             string
	Loadout           string
}

// NormalizedScenario builds a normalized team-vs-team scenario name.
func NormalizedScenario(name string, teamA, teamB ReferenceTeam, lead int) Scenario {
	kind := "normalized"
	if teamA.Name == teamB.Name {
		kind = "mirror"
	}
	return Scenario{
		Name:        name,
		Kind:        kind,
		TeamA:       teamA,
		TeamB:       teamB,
		LeadA:       lead,
		LeadB:       lead,
		Level:       0,
		EngineSideA: true,
	}
}

// RunScenario executes one scenario.
func RunScenario(cfg Config, sc Scenario) (*BattleOutcome, error) {
	prepared, err := prepareScenario(cfg, sc)
	if err != nil {
		return nil, err
	}
	return runPreparedScenario(cfg, sc, prepared)
}

type preparedScenario struct{ partyA, partyB battle.Party }

func prepareScenario(cfg Config, sc Scenario) (preparedScenario, error) {
	aTrainer, bTrainer := sideBName, sideAName
	if sc.EngineSideA {
		aTrainer, bTrainer = sideAName, sideBName
	}
	teamLeft, teamRight := sc.TeamA, sc.TeamB
	leadLeft, leadRight := sc.LeadA, sc.LeadB
	if !sc.EngineSideA {
		teamLeft, teamRight = sc.TeamB, sc.TeamA
		leadLeft, leadRight = sc.LeadB, sc.LeadA
	}
	swap := sc.PartyOrderSwapped
	build := BuildNormalizedParty
	if sc.Kind == "natural" {
		build = func(set *content.Set, team ReferenceTeam, lead int, trainer string, swapped bool) (battle.Party, error) {
			return BuildNaturalParty(set, team, lead, sc.Level, trainer, swapped)
		}
	}
	partyA, err := build(cfg.Set, teamLeft, leadLeft, aTrainer, swap)
	if err != nil {
		return preparedScenario{}, fmt.Errorf("balance: build side A for %s: %w", sc.Name, err)
	}
	partyB, err := build(cfg.Set, teamRight, leadRight, bTrainer, swap)
	if err != nil {
		return preparedScenario{}, fmt.Errorf("balance: build side B for %s: %w", sc.Name, err)
	}
	return preparedScenario{partyA, partyB}, nil
}

func runPreparedScenario(cfg Config, sc Scenario, prepared preparedScenario) (*BattleOutcome, error) {
	policy := cfg.Policy
	if policy.Tier == "" {
		policy = DefaultPolicy()
	}
	maxTurns := cfg.MaxTurns
	if maxTurns < 1 {
		maxTurns = DefaultMaxTurns
	}
	partyA, partyB := prepared.partyA, prepared.partyB
	out, err := Simulate(cfg.Set, partyA, partyB, sc.Seed, policy, maxTurns)
	if out != nil {
		out.Scenario = sc.Name
		out.Kind = sc.Kind
		out.Stage = sc.Stage
		out.LoadoutVariant = sc.Loadout
		out.EngineSideA = sc.EngineSideA
		out.PartyOrderSwapped = sc.PartyOrderSwapped
		out.TeamA = sc.TeamA
		out.TeamB = sc.TeamB
		out.SideA = partyA
		out.SideB = partyB
	}
	if err != nil {
		return out, err
	}
	return out, nil
}

// PairedNormalizedRuns executes the side and Party-order paired non-mirror contract.
func PairedNormalizedRuns(cfg Config, teamA, teamB ReferenceTeam, lead int, seed uint64) ([]*BattleOutcome, error) {
	base := NormalizedScenario(
		fmt.Sprintf("normalized/%s-vs-%s/lead-%d", teamA.Name, teamB.Name, lead),
		teamA, teamB, lead,
	)
	base.Seed = seed
	first, err := RunScenario(cfg, base)
	if err != nil {
		if first == nil {
			return nil, err
		}
		return []*BattleOutcome{first}, err
	}
	runs := []*BattleOutcome{first}
	second := base
	second.EngineSideA = false
	second.PartyOrderSwapped = true
	second.Name = base.Name + "/paired"
	secondOut, err := RunScenario(cfg, second)
	if err != nil {
		if secondOut != nil {
			runs = append(runs, secondOut)
		}
		return runs, err
	}
	return append(runs, secondOut), nil
}

// IsTeamWin reports whether teamA won.
func IsTeamWin(out *BattleOutcome, teamA ReferenceTeam) bool {
	winner := WinningTeam(out)
	return winner.Name == teamA.Name
}

// WinningTeam returns the ReferenceTeam that won, if any.
func WinningTeam(out *BattleOutcome) ReferenceTeam {
	if out == nil {
		return ReferenceTeam{}
	}
	if out.Winner == out.SideA.Trainer {
		if out.EngineSideA {
			return out.TeamA
		}
		return out.TeamB
	}
	if out.Winner == out.SideB.Trainer {
		if out.EngineSideA {
			return out.TeamB
		}
		return out.TeamA
	}
	return ReferenceTeam{}
}

// EngineSideWin reports whether engine side 0 (first party in battle.New) won.
func EngineSideWin(out *BattleOutcome) bool {
	return out.Winner == out.SideA.Trainer
}

// MirrorScenario is a normalized mirror match.
func MirrorScenario(team ReferenceTeam, lead int, seed uint64) Scenario {
	sc := NormalizedScenario(
		fmt.Sprintf("mirror/%s/lead-%d", team.Name, lead),
		team, team, lead,
	)
	sc.Seed = seed
	return sc
}

// NaturalScenario builds one natural-level matrix fixture.
func NaturalScenario(name string, teamA, teamB ReferenceTeam, leadA, leadB, level int) Scenario {
	return Scenario{Name: name, Kind: "natural", TeamA: teamA, TeamB: teamB, LeadA: leadA, LeadB: leadB, Level: level, EngineSideA: true}
}
