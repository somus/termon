package dojo

import (
	"math"
	"slices"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
)

// masterScore uses a two-turn mean-damage forecast. It models the opponent's
// best public Rival reply, then our best Rival follow-up. It never samples
// future rolls; Speed ties average both possible first-turn orders.
func masterScore(set *content.Set, view battle.PolicyView, act battle.Action, self battle.PolicyMember) float64 {
	first := rivalOneTurn(set, view, act, self)
	parties := modeledParties(set, view)
	replyParties := [2][]battle.PolicyMember{slices.Clone(parties[0]), parties[1]}
	if act.Kind == battle.ActionSwitch {
		activate(replyParties[0], act.SwitchTo)
	}
	replies := bestRivalActions(set, policyView(replyParties, 1))
	if len(replies) == 0 {
		return first + 0.45*terminalPositionScore(parties)
	}
	future := 0.0
	for _, reply := range replies {
		forecasts := battle.ForecastTurn(set, parties, [2]battle.Action{act, reply})
		for _, forecast := range forecasts {
			future += forecast.Weight * bestFollowUp(set, forecast.Parties) / float64(len(replies))
		}
	}
	return first + 0.45*future
}

func modeledParties(set *content.Set, view battle.PolicyView) [2][]battle.PolicyMember {
	foes := view.FoeRoster
	if len(foes) == 0 {
		foes = []battle.PolicyFoe{view.FoeActive}
	}
	opponents := make([]battle.PolicyMember, 0, len(foes))
	for _, foe := range foes {
		m := opponentMember(foe)
		m.Loadout = battle.PolicyMovepool(set, foe)
		if !m.Active && !m.Fainted {
			// Reserve HP is not public. Model an unseen reserve at full HP.
			m.HP = m.MaxHP
		}
		opponents = append(opponents, m)
	}
	return [2][]battle.PolicyMember{slices.Clone(view.Self), opponents}
}

func policyView(parties [2][]battle.PolicyMember, side int) battle.PolicyView {
	view := battle.PolicyView{Self: parties[side]}
	for _, m := range parties[1-side] {
		foe := battle.PolicyFoe{
			ID: m.ID, Species: m.Species, Type: m.Type, Level: m.Level,
			MaxHP: m.MaxHP, Atk: m.Atk, Def: m.Def, SpA: m.SpA, Spe: m.Spe,
			Active: m.Active, Fainted: m.Fainted,
			PublicMovepool: m.PublicMovepool,
		}
		if foe.Active {
			foe.HP = m.HP
			view.FoeActive = foe
		}
		view.FoeRoster = append(view.FoeRoster, foe)
	}
	return view
}

func bestRivalActions(set *content.Set, view battle.PolicyView) []battle.Action {
	if activeMember(view.Self).Fainted || view.FoeActive.Fainted {
		return nil
	}
	candidates, err := enumerateCandidates(set, view, PolicyConfig{Tier: TierRival})
	if err != nil {
		return nil
	}
	best := math.Inf(-1)
	actions := make([]battle.Action, 0, len(candidates))
	for _, c := range candidates {
		if c.score > best+1e-9 {
			best = c.score
			actions = actions[:0]
		}
		if math.Abs(c.score-best) < 1e-9 {
			actions = append(actions, c.action)
		}
	}
	return actions
}

func bestFollowUp(set *content.Set, parties [2][]battle.PolicyMember) float64 {
	if score := terminalPositionScore(parties); score != 0 {
		return score
	}
	for side := range parties {
		if activeMember(parties[side]).Fainted {
			chooseForecastReplacement(set, parties, side)
		}
	}
	view := policyView(parties, 0)
	candidates, err := enumerateCandidates(set, view, PolicyConfig{Tier: TierRival})
	if err != nil {
		return -3
	}
	best := math.Inf(-1)
	for _, candidate := range candidates {
		best = max(best, candidate.score)
	}
	return best
}

func chooseForecastReplacement(set *content.Set, parties [2][]battle.PolicyMember, side int) {
	view := policyView(parties, side)
	best, target := math.Inf(-1), ""
	for _, m := range parties[side] {
		if m.Fainted || m.HP <= 0 {
			continue
		}
		score := replacementScore(set, view, PolicyConfig{Tier: TierRival}, m)
		if score > best {
			best, target = score, m.ID
		}
	}
	activate(parties[side], target)
}

func activate(party []battle.PolicyMember, id string) {
	for i := range party {
		party[i].Active = party[i].ID == id
	}
}

func terminalPositionScore(parties [2][]battle.PolicyMember) float64 {
	var living [2]int
	for side, members := range parties {
		for _, m := range members {
			if !m.Fainted && m.HP > 0 {
				living[side]++
			}
		}
	}
	if living[0] == 0 {
		return -3
	}
	if living[1] == 0 {
		return 3
	}
	return 0
}
