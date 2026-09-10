package dojo

import (
	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/game"
)

// Preservation predicts a baseline opponent: predict a baseline Preservation opponent from
// public information, average its equally best actions, then optimize survival.
// This is one nonrecursive prediction, not an equilibrium or hidden-action read.
func choosePreservation(set *content.Set, view battle.PolicyView, candidates []policyCandidate, rng battle.Rand) (battle.Action, DecisionExplanation, error) {
	parties := modeledParties(set, view)
	foeView := policyView(parties, 1)
	var foeCandidates []policyCandidate
	for _, member := range foeView.Self {
		if member.Fainted || member.HP <= 0 {
			continue
		}
		if !member.Active {
			foeCandidates = append(foeCandidates, policyCandidate{action: battle.Action{Kind: battle.ActionSwitch, SwitchTo: member.ID}})
			continue
		}
		for _, slug := range member.Loadout {
			foeCandidates = append(foeCandidates, policyCandidate{action: battle.Action{Kind: battle.ActionMove, Move: slug}})
		}
	}
	// Synthetic public views can expose no available action. They imply no
	// modeled incoming attack, when no public Moves are available.
	if len(foeCandidates) == 0 {
		foeCandidates = append(foeCandidates, policyCandidate{})
	}
	var predicted []policyCandidate
	var bestFoe preservationScore
	for _, candidate := range foeCandidates {
		score := preservationActionScore(set, foeView, candidate.action, actionMember(foeView.Self, candidate.action))
		if len(predicted) == 0 || preservationBetter(score, bestFoe) {
			bestFoe = score
			predicted = []policyCandidate{candidate}
		} else if preservationEqual(score, bestFoe) {
			predicted = append(predicted, candidate)
		}
	}
	var best preservationScore
	var pool, scored []policyCandidate
	scores := make([]preservationScore, len(candidates))
	for i, candidate := range candidates {
		self := actionMember(view.Self, candidate.action)
		score := preservationScore{reserves: healthyReserves(view.Self, self.ID)}
		for _, reply := range predicted {
			target := actionMember(foeView.Self, reply.action)
			foe := battle.PolicyFoe{ID: target.ID, Species: target.Species, Type: target.Type, HP: target.HP, MaxHP: target.MaxHP, Atk: target.Atk, Def: target.Def, SpA: target.SpA, Spe: target.Spe, PublicMovepool: []string{reply.action.Move}}
			incoming := 0.0
			if reply.action.Kind == battle.ActionMove {
				incoming = foeKOProbability(set, foe, self)
			}
			outgoing, ownKO := 0.0, 0.0
			if candidate.action.Kind == battle.ActionMove {
				mv := set.Moves[candidate.action.Move]
				base := battle.DamageBase(game.MovePower(mv.Power, self.Level), attackStat(self, mv.Category), foe.Def, mv.Type, self.Type, set.Effectiveness(mv.Type, foe.Type))
				outgoing = battle.ExpectedHPLoss(base, mv.Accuracy, foe.HP)
				ownKO = battle.KOProbability(base, mv.Accuracy, foe.HP)
				if self.Spe > foe.Spe {
					incoming *= 1 - ownKO
				} else if self.Spe == foe.Spe {
					incoming *= 1 - 0.5*ownKO
				}
			}
			score.koProbability += incoming / float64(len(predicted))
			score.expectedHPLoss += outgoing / float64(len(predicted))
		}
		scores[i] = score
		candidate.score = score.expectedHPLoss
		scored = append(scored, candidate)
		if len(pool) == 0 || preservationBetter(score, best) {
			best = score
			pool = []policyCandidate{candidate}
		} else if preservationEqual(score, best) {
			pool = append(pool, candidate)
		}
	}
	picked := chooseTied(pool, rng)
	explanation := referenceExplanation(ReferencePreservation, "survival against one predicted baseline Preservation response; unknown reserves modeled at full HP", picked, scored)
	for i := range explanation.Considered {
		explanation.Considered[i].KOProbability = &scores[i].koProbability
		explanation.Considered[i].HealthyReserves = &scores[i].reserves
		explanation.Considered[i].ExpectedHPLoss = &scores[i].expectedHPLoss
	}
	return picked.action, explanation, nil
}
