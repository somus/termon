package balance

import (
	"fmt"
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
		LeadB:       0,
		Level:       0,
		EngineSideA: true,
	}
}

// RunScenario executes one scenario.
func RunScenario(cfg Config, sc Scenario) (*BattleOutcome, error) {
	policy := cfg.Policy
	if policy.Tier == "" {
		policy = DefaultPolicy()
	}
	maxTurns := cfg.MaxTurns
	if maxTurns < 1 {
		maxTurns = DefaultMaxTurns
	}
	aTrainer, bTrainer := sideBName, sideAName
	if sc.EngineSideA {
		aTrainer, bTrainer = sideAName, sideBName
	}
	teamLeft, teamRight := sc.TeamA, sc.TeamB
	if !sc.EngineSideA {
		teamLeft, teamRight = sc.TeamB, sc.TeamA
	}
	swap := sc.PartyOrderSwapped
	partyA, err := BuildNormalizedParty(cfg.Set, teamLeft, sc.LeadA, aTrainer, swap)
	if err != nil {
		return nil, err
	}
	leadB := sc.LeadB
	if leadB == 0 && sc.LeadA >= 0 {
		leadB = sc.LeadA
	}
	partyB, err := BuildNormalizedParty(cfg.Set, teamRight, leadB, bTrainer, swap)
	if err != nil {
		return nil, err
	}
	out, err := Simulate(cfg.Set, partyA, partyB, sc.Seed, policy, maxTurns)
	if err != nil {
		return nil, err
	}
	out.Scenario = sc.Name
	out.EngineSideA = sc.EngineSideA
	out.PartyOrderSwapped = sc.PartyOrderSwapped
	out.TeamA = sc.TeamA
	out.TeamB = sc.TeamB
	out.SideA = partyA
	out.SideB = partyB
	return out, nil
}

// PairedNormalizedRuns executes the side+order paired non-mirror contract.
func PairedNormalizedRuns(cfg Config, teamA, teamB ReferenceTeam, lead int, seed uint64) []*BattleOutcome {
	base := NormalizedScenario(
		fmt.Sprintf("normalized/%s-vs-%s/lead-%d", teamA.Name, teamB.Name, lead),
		teamA, teamB, lead,
	)
	base.Seed = seed
	runs := []*BattleOutcome{
		mustRun(cfg, base),
	}
	second := base
	second.EngineSideA = false
	second.PartyOrderSwapped = true
	second.Name = base.Name + "/paired"
	runs = append(runs, mustRun(cfg, second))
	return runs
}

func mustRun(cfg Config, sc Scenario) *BattleOutcome {
	out, err := RunScenario(cfg, sc)
	if err != nil {
		panic(err)
	}
	return out
}

// IsTeamWin reports whether teamA won.
func IsTeamWin(out *BattleOutcome, teamA ReferenceTeam) bool {
	winner := WinningTeam(out)
	return winner.Name == teamA.Name
}

// WinningTeam returns the ReferenceTeam that won, if any.
func WinningTeam(out *BattleOutcome) ReferenceTeam {
	if out.Winner == sideAName {
		if out.EngineSideA {
			return out.TeamA
		}
		return out.TeamB
	}
	if out.Winner == sideBName {
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
