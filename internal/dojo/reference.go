package dojo

import (
	"errors"
	"math"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
)

// ReferencePolicyRevision identifies the reference-policy scoring contract.
const ReferencePolicyRevision = "reference-policies-v2"

// Reference policy names are recorded by balance scenarios.
const (
	ReferencePressure     = "pressure"
	ReferencePivot        = "pivot"
	ReferencePreservation = "preservation"
)

// ChooseReferenceAction chooses a deterministic reference-policy action using
// only the policy view exposed by the battle engine.
func ChooseReferenceAction(
	set *content.Set,
	view battle.PolicyView,
	name string,
	rng battle.Rand,
) (battle.Action, DecisionExplanation, error) {
	if set == nil || rng == nil {
		return battle.Action{}, DecisionExplanation{}, errors.New("dojo: nil argument")
	}
	candidates, err := enumerateCandidates(set, view, TierConfig(TierRival))
	if err != nil {
		return battle.Action{}, DecisionExplanation{}, err
	}
	if len(candidates) == 0 {
		return battle.Action{}, DecisionExplanation{}, errors.New("dojo: no legal actions")
	}

	switch name {
	case ReferencePressure:
		return choosePressure(set, view, candidates, rng)
	case ReferencePivot:
		return choosePivot(set, view, candidates, rng)
	case ReferencePreservation:
		return choosePreservation(set, view, candidates, rng)
	default:
		return battle.Action{}, DecisionExplanation{}, errors.New("dojo: unknown reference policy " + name)
	}
}

func choosePressure(set *content.Set, view battle.PolicyView, candidates []policyCandidate, rng battle.Rand) (battle.Action, DecisionExplanation, error) {
	active := activeMember(view.Self)
	best := -1.0
	pool := []policyCandidate{}
	scored := make([]policyCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		score := 0.0
		if candidate.action.Kind == battle.ActionMove {
			score = actionExpectedHPLoss(set, active, view.FoeActive, candidate.action)
		}
		candidate.score = score
		scored = append(scored, candidate)
		if score > best+1e-9 {
			best, pool = score, []policyCandidate{candidate}
		} else if math.Abs(score-best) <= 1e-9 {
			pool = append(pool, candidate)
		}
	}
	picked := chooseTied(pool, rng)
	return picked.action, referenceExplanation(ReferencePressure, "maximum immediate expected HP loss", picked, scored), nil
}

func choosePivot(set *content.Set, view battle.PolicyView, candidates []policyCandidate, rng battle.Rand) (battle.Action, DecisionExplanation, error) {
	best := math.Inf(-1)
	pool := []policyCandidate{}
	scored := make([]policyCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		candidate.score = rivalOneTurn(set, view, candidate.action, actionMember(view.Self, candidate.action))
		scored = append(scored, candidate)
		if candidate.score > best+1e-9 {
			best, pool = candidate.score, []policyCandidate{candidate}
		} else if math.Abs(candidate.score-best) <= 1e-9 {
			pool = append(pool, candidate)
		}
	}
	picked := chooseTied(pool, rng)
	return picked.action, referenceExplanation(ReferencePivot, "best one-turn Rival score", picked, scored), nil
}

type preservationScore struct {
	koProbability  float64
	reserves       int
	expectedHPLoss float64
}

func preservationActionScore(set *content.Set, view battle.PolicyView, action battle.Action, self battle.PolicyMember) preservationScore {
	ko := foeKOProbability(set, view.FoeActive, self)
	if action.Kind == battle.ActionMove {
		ownKO := ownKOProbability(set, self, view.FoeActive, action.Move)
		if self.Spe > view.FoeActive.Spe {
			ko *= 1 - ownKO
		} else if self.Spe == view.FoeActive.Spe {
			ko *= 1 - 0.5*ownKO
		}
	}
	return preservationScore{
		koProbability:  ko,
		reserves:       healthyReserves(view.Self, self.ID),
		expectedHPLoss: actionExpectedHPLoss(set, self, view.FoeActive, action),
	}
}

func preservationBetter(a, b preservationScore) bool {
	if math.Abs(a.koProbability-b.koProbability) > 1e-9 {
		return a.koProbability < b.koProbability
	}
	if a.reserves != b.reserves {
		return a.reserves > b.reserves
	}
	return a.expectedHPLoss > b.expectedHPLoss+1e-9
}

func preservationEqual(a, b preservationScore) bool {
	return math.Abs(a.koProbability-b.koProbability) <= 1e-9 && a.reserves == b.reserves && math.Abs(a.expectedHPLoss-b.expectedHPLoss) <= 1e-9
}

func foeKOProbability(set *content.Set, foe battle.PolicyFoe, self battle.PolicyMember) float64 {
	best := 0.0
	for _, slug := range battle.PolicyMovepool(set, foe) {
		mv := set.Moves[slug]
		base := battle.DamageBase(mv.Power, attackStat(opponentMember(foe), mv.Category), self.Def, mv.Type, foe.Type, set.Effectiveness(mv.Type, self.Type))
		best = max(best, battle.KOProbability(base, mv.Accuracy, self.HP))
	}
	return best
}

func ownKOProbability(set *content.Set, self battle.PolicyMember, foe battle.PolicyFoe, slug string) float64 {
	mv, ok := set.Moves[slug]
	if !ok {
		return 0
	}
	base := battle.DamageBase(mv.Power, attackStat(self, mv.Category), foe.Def, mv.Type, self.Type, set.Effectiveness(mv.Type, foe.Type))
	return battle.KOProbability(base, mv.Accuracy, foe.HP)
}

func actionExpectedHPLoss(set *content.Set, self battle.PolicyMember, foe battle.PolicyFoe, action battle.Action) float64 {
	if action.Kind != battle.ActionMove {
		return 0
	}
	mv, ok := set.Moves[action.Move]
	if !ok {
		return 0
	}
	base := battle.DamageBase(mv.Power, attackStat(self, mv.Category), foe.Def, mv.Type, self.Type, set.Effectiveness(mv.Type, foe.Type))
	return battle.ExpectedHPLoss(base, mv.Accuracy, foe.HP)
}

func attackStat(member battle.PolicyMember, category string) int {
	if category == "special" {
		return member.SpA
	}
	return member.Atk
}

func actionMember(party []battle.PolicyMember, action battle.Action) battle.PolicyMember {
	if action.Kind == battle.ActionSwitch {
		for _, member := range party {
			if member.ID == action.SwitchTo {
				return member
			}
		}
	}
	return activeMember(party)
}

func healthyReserves(party []battle.PolicyMember, activeID string) int {
	count := 0
	for _, member := range party {
		if member.ID != activeID && !member.Fainted && member.HP > 0 {
			count++
		}
	}
	return count
}

func chooseTied(candidates []policyCandidate, rng battle.Rand) policyCandidate {
	if len(candidates) == 1 {
		return candidates[0]
	}
	return candidates[min(int(rng.Float64()*float64(len(candidates))), len(candidates)-1)]
}

func referenceExplanation(name, reason string, picked policyCandidate, candidates []policyCandidate) DecisionExplanation {
	explanation := DecisionExplanation{Tier: name, PrimaryReason: reason, ReasonCode: name, Selected: picked.action}
	for _, candidate := range candidates {
		explanation.Considered = append(explanation.Considered, ScoredActionSummary{Kind: candidate.action.Kind, Move: candidate.action.Move, SwitchTo: candidate.action.SwitchTo, Score: candidate.score})
	}
	return explanation
}
