package dojo

import (
	"errors"
	"math"
	"slices"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
)

const meanVariance = 0.925

// Tier names for Sparring and Daily opponent policy.
const (
	TierApprentice = "apprentice"
	TierRival      = "rival"
	TierMaster     = "master"
)

// PolicyConfig selects tier scoring and near-best sampling band.
type PolicyConfig struct {
	Tier         string
	NearBestBand float64 // 0 selects unique best; 0.15 = rival sparring; 0.05 = master sparring
}

// TierConfig returns the default Sparring band for a published tier.
func TierConfig(tier string) PolicyConfig {
	switch tier {
	case TierRival:
		return PolicyConfig{Tier: TierRival, NearBestBand: 0.15}
	case TierMaster:
		return PolicyConfig{Tier: TierMaster, NearBestBand: 0.05}
	default:
		return PolicyConfig{Tier: TierApprentice, NearBestBand: 0}
	}
}

// ScoredActionSummary is one considered action for the Battle Log.
type ScoredActionSummary struct {
	Kind     battle.ActionKind
	Move     string
	SwitchTo string
	Score    float64
	Weight   float64
}

// DecisionExplanation documents a resolved Dojo policy choice.
type DecisionExplanation struct {
	Tier          string
	PrimaryReason string
	ReasonCode    string
	Selected      battle.Action
	Considered    []ScoredActionSummary
}

type policyCandidate struct {
	action battle.Action
	score  float64
	weight float64
}

// ChoosePolicyAction picks a legal action for the Dojo Opponent.
func ChoosePolicyAction(set *content.Set, view battle.PolicyView, cfg PolicyConfig, rng battle.Rand) (battle.Action, DecisionExplanation, error) {
	if set == nil || rng == nil {
		return battle.Action{}, DecisionExplanation{}, errors.New("dojo: nil argument")
	}
	active := activeMember(view.Self)
	if active.ID == "" {
		return battle.Action{}, DecisionExplanation{}, errors.New("dojo: no active monster")
	}
	cands, err := enumerateCandidates(set, view, cfg)
	if err != nil {
		return battle.Action{}, DecisionExplanation{}, err
	}
	if len(cands) == 0 {
		return battle.Action{}, DecisionExplanation{}, errors.New("dojo: no legal actions")
	}
	var picked policyCandidate
	switch cfg.Tier {
	case TierApprentice:
		picked = sampleWeighted(cands, rng)
	default:
		picked = sampleNearBest(cands, cfg.NearBestBand, rng)
	}
	exp := buildExplanation(set, view, cfg, cands, picked)
	return picked.action, exp, nil
}

// ChooseReplacement picks a forced Replacement reserve for the Dojo Opponent.
func ChooseReplacement(set *content.Set, view battle.PolicyView, cfg PolicyConfig, rng battle.Rand) (string, DecisionExplanation, error) {
	if set == nil || rng == nil {
		return "", DecisionExplanation{}, errors.New("dojo: nil argument")
	}
	var reserves []battle.PolicyMember
	for _, m := range view.Self {
		if !m.Active && !m.Fainted && m.HP > 0 {
			reserves = append(reserves, m)
		}
	}
	if len(reserves) == 0 {
		return "", DecisionExplanation{}, errors.New("dojo: no healthy reserves")
	}
	var cands []policyCandidate
	for _, m := range reserves {
		score := replacementScore(set, view, cfg, m)
		cands = append(cands, policyCandidate{
			action: battle.Action{Kind: battle.ActionSwitch, SwitchTo: m.ID},
			score:  score,
		})
	}
	picked := sampleNearBest(cands, 0, rng)
	exp := DecisionExplanation{
		Tier: cfg.Tier, ReasonCode: "replace_score",
		PrimaryReason: "Replacement: best reserve for this tier",
		Selected:      picked.action,
	}
	for _, c := range cands {
		exp.Considered = append(exp.Considered, ScoredActionSummary{
			Kind: battle.ActionSwitch, SwitchTo: c.action.SwitchTo, Score: c.score,
		})
	}
	return picked.action.SwitchTo, exp, nil
}

func enumerateCandidates(set *content.Set, view battle.PolicyView, cfg PolicyConfig) ([]policyCandidate, error) {
	active := activeMember(view.Self)
	var out []policyCandidate
	for _, slug := range active.Loadout {
		out = append(out, policyCandidate{
			action: battle.Action{Kind: battle.ActionMove, Move: slug},
			score:  scoreAction(set, view, cfg, battle.Action{Kind: battle.ActionMove, Move: slug}, active, active),
			weight: apprenticeMoveWeight(set, slug, view.FoeActive.Type),
		})
	}
	for _, m := range view.Self {
		if m.Active || m.Fainted || m.HP <= 0 {
			continue
		}
		act := battle.Action{Kind: battle.ActionSwitch, SwitchTo: m.ID}
		score := scoreAction(set, view, cfg, act, m, active)
		weight := 0.0
		if cfg.Tier == TierApprentice && apprenticeSwitchAllowed(set, view, m) {
			weight = 1.0
		}
		if cfg.Tier != TierApprentice || weight > 0 {
			out = append(out, policyCandidate{action: act, score: score, weight: weight})
		}
	}
	if cfg.Tier == TierApprentice {
		// Keep only moves plus eligible switches; re-filter switches with zero weight.
		var filtered []policyCandidate
		for _, c := range out {
			if c.action.Kind == battle.ActionMove || c.weight > 0 {
				filtered = append(filtered, c)
			}
		}
		out = filtered
	}
	if len(out) == 0 {
		return nil, errors.New("dojo: no candidates")
	}
	return out, nil
}

func apprenticeSwitchAllowed(set *content.Set, view battle.PolicyView, reserve battle.PolicyMember) bool {
	foeType := view.FoeActive.Type
	selfType := activeMember(view.Self).Type
	if set.Effectiveness(foeType, selfType) < battle.SuperEffectiveAt {
		return false
	}
	return set.Effectiveness(foeType, reserve.Type) < battle.SuperEffectiveAt
}

func apprenticeMoveWeight(set *content.Set, moveSlug, defenderType string) float64 {
	mv, ok := set.Moves[moveSlug]
	if !ok {
		return 1.0
	}
	return apprenticeWeight(set.Effectiveness(mv.Type, defenderType))
}

func scoreAction(set *content.Set, view battle.PolicyView, cfg PolicyConfig, act battle.Action, self, currentActive battle.PolicyMember) float64 {
	switch cfg.Tier {
	case TierMaster:
		return masterScore(set, view, act, self, currentActive)
	case TierRival:
		return rivalOneTurn(set, view, act, self, currentActive)
	default:
		if act.Kind == battle.ActionSwitch {
			return float64(apprenticeWeight(matchupValue(set, self.Type, view.FoeActive.Type)))
		}
		return apprenticeMoveWeight(set, act.Move, view.FoeActive.Type)
	}
}

func replacementScore(set *content.Set, view battle.PolicyView, cfg PolicyConfig, reserve battle.PolicyMember) float64 {
	switch cfg.Tier {
	case TierMaster:
		return masterScore(set, view, battle.Action{Kind: battle.ActionSwitch, SwitchTo: reserve.ID}, reserve, activeMember(view.Self))
	case TierRival:
		return rivalOneTurn(set, view, battle.Action{Kind: battle.ActionSwitch, SwitchTo: reserve.ID}, reserve, activeMember(view.Self))
	default:
		best := 0.0
		for _, slug := range reserve.Loadout {
			w := apprenticeMoveWeight(set, slug, view.FoeActive.Type)
			if w > best {
				best = w
			}
		}
		return best
	}
}

func rivalOneTurn(set *content.Set, view battle.PolicyView, act battle.Action, self, currentActive battle.PolicyMember) float64 {
	outgoing := 0.0
	pKO := 0.0
	if act.Kind == battle.ActionMove {
		outgoing = expectedDamage(set, self, view.FoeActive, act.Move)
		pKO = clampUnit(outgoing / float64(max(view.FoeActive.HP, 1)))
	}
	incoming := expectedIncoming(set, view.FoeActive, self)
	survival := 1 - clampUnit(incoming/float64(max(self.MaxHP, 1)))
	match := matchupValue(set, self.Type, view.FoeActive.Type)
	pSelfFaint := clampUnit(incoming / float64(max(currentActive.HP, 1)))
	return 1.00*outgoing/float64(max(view.FoeActive.MaxHP, 1)) +
		0.50*pKO +
		0.60*survival +
		0.25*match -
		0.80*pSelfFaint
}

// masterScore is the Master Sparring policy: a bounded two-turn expectimax over
// public information (dojo-master.md). It scores the candidate action this turn,
// then models the foe's best reply from the foe's public legal movepool and adds
// the resulting position value. For switch candidates self already carries the
// incoming reserve; for move candidates it is the active.
func masterScore(set *content.Set, view battle.PolicyView, act battle.Action, self, currentActive battle.PolicyMember) float64 {
	first := rivalOneTurn(set, view, act, self, currentActive)

	// Modeled position after my action: a move chips the foe, a switch does not.
	foe := view.FoeActive
	if act.Kind == battle.ActionMove {
		foe.HP = max(foe.HP-int(expectedDamage(set, self, view.FoeActive, act.Move)), 0)
	}
	if foe.HP <= 0 {
		// The foe falls before it can reply; the one-turn terms already reward the KO.
		return first + 0.50
	}

	// The foe's best reply from public information: its loadout and reserves are
	// not observable, so bound the reply by its full legal movepool.
	replyDamage := 0.0
	for _, slug := range battle.LevelLegalMovepool(set, foe.Species, foe.Level) {
		if d := expectedDamage(set, foe, self, slug); d > replyDamage {
			replyDamage = d
		}
	}
	pReplyKO := clampUnit(replyDamage / float64(max(self.MaxHP, 1)))
	survival := 1 - pReplyKO
	chip := clampUnit(1 - float64(foe.HP)/float64(max(foe.MaxHP, 1)))

	second := 0.45*chip + 0.30*survival - 0.90*pReplyKO
	return first + 0.55*second
}

func expectedDamage(set *content.Set, atk, def battle.PolicyMember, moveSlug string) float64 {
	mv, ok := set.Moves[moveSlug]
	if !ok {
		return 0
	}
	attack := float64(atk.Atk)
	defense := float64(def.Def)
	if mv.Category == "special" {
		attack = float64(atk.SpA)
		defense = float64(def.Def)
	}
	base := mv.Power*attack/defense/battle.DamageDivisor + 2
	dmg := base
	if mv.Type == atk.Type {
		dmg *= battle.STABMultiplier
	}
	dmg *= set.Effectiveness(mv.Type, def.Type)
	dmg *= (float64(mv.Accuracy) / 100) * (1 + 1.0/battle.CritChance*battle.CritMultiplier) * meanVariance
	return dmg
}

func expectedIncoming(set *content.Set, foe, self battle.PolicyMember) float64 {
	pool := battle.LevelLegalMovepool(set, foe.Species, foe.Level)
	best := 0.0
	for _, slug := range pool {
		d := expectedDamage(set, foe, self, slug)
		if d > best {
			best = d
		}
	}
	return best
}

func matchupValue(set *content.Set, selfType, foeType string) float64 {
	if set.Effectiveness(selfType, foeType) >= battle.SuperEffectiveAt {
		return 1
	}
	if set.Effectiveness(foeType, selfType) >= battle.SuperEffectiveAt {
		return -1
	}
	return 0
}

func clampUnit(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}

func activeMember(party []battle.PolicyMember) battle.PolicyMember {
	for _, m := range party {
		if m.Active {
			return m
		}
	}
	if len(party) > 0 {
		return party[0]
	}
	return battle.PolicyMember{}
}

func sampleWeighted(cands []policyCandidate, rng battle.Rand) policyCandidate {
	var total float64
	for _, c := range cands {
		w := c.weight
		if w <= 0 {
			w = c.score
		}
		if w <= 0 {
			w = 1
		}
		total += w
	}
	r := rng.Float64() * total
	var cum float64
	for _, c := range cands {
		w := c.weight
		if w <= 0 {
			w = c.score
		}
		if w <= 0 {
			w = 1
		}
		cum += w
		if r < cum {
			return c
		}
	}
	return cands[len(cands)-1]
}

func sampleNearBest(cands []policyCandidate, band float64, rng battle.Rand) policyCandidate {
	best := cands[0].score
	for _, c := range cands[1:] {
		if c.score > best {
			best = c.score
		}
	}
	threshold := best
	if band > 0 {
		threshold = best * (1 - band)
	}
	var pool []policyCandidate
	for _, c := range cands {
		if band == 0 {
			if c.score >= best-1e-9 {
				pool = append(pool, c)
			}
		} else if c.score >= threshold-1e-9 {
			pool = append(pool, c)
		}
	}
	if len(pool) == 0 {
		pool = cands
	}
	slices.SortFunc(pool, func(a, b policyCandidate) int {
		if a.score < b.score {
			return 1
		}
		if a.score > b.score {
			return -1
		}
		return 0
	})
	top := pool[0].score
	var ties []policyCandidate
	for _, c := range pool {
		if math.Abs(c.score-top) < 1e-9 {
			ties = append(ties, c)
		}
	}
	if len(ties) == 1 {
		return ties[0]
	}
	idx := int(rng.Float64() * float64(len(ties)))
	if idx >= len(ties) {
		idx = len(ties) - 1
	}
	return ties[idx]
}

func buildExplanation(set *content.Set, view battle.PolicyView, cfg PolicyConfig, cands []policyCandidate, picked policyCandidate) DecisionExplanation {
	exp := DecisionExplanation{Tier: cfg.Tier, Selected: picked.action}
	for _, c := range cands {
		exp.Considered = append(exp.Considered, ScoredActionSummary{
			Kind: c.action.Kind, Move: c.action.Move, SwitchTo: c.action.SwitchTo,
			Score: c.score, Weight: c.weight,
		})
	}
	code := reasonCode(set, view, picked)
	exp.ReasonCode = code
	exp.PrimaryReason = primaryReasonText(code, picked.action)
	return exp
}

func reasonCode(set *content.Set, view battle.PolicyView, picked policyCandidate) string {
	if picked.action.Kind == battle.ActionSwitch {
		if apprenticeSwitchAllowed(set, view, memberByID(view.Self, picked.action.SwitchTo)) {
			return "switch_disadvantage"
		}
		return "switch_matchup"
	}
	moveSlug := picked.action.Move
	mv, ok := set.Moves[moveSlug]
	if !ok {
		return "move_damage"
	}
	if set.Effectiveness(mv.Type, view.FoeActive.Type) >= battle.SuperEffectiveAt {
		return "move_se"
	}
	bestKO, bestDmg := -1.0, -1.0
	for _, c := range []policyCandidate{picked} {
		if c.action.Kind != battle.ActionMove {
			continue
		}
		d := expectedDamage(set, activeMember(view.Self), view.FoeActive, c.action.Move)
		ko := clampUnit(d / float64(max(view.FoeActive.HP, 1)))
		if ko > bestKO {
			bestKO = ko
		}
		if d > bestDmg {
			bestDmg = d
		}
	}
	_ = bestKO
	_ = bestDmg
	return "move_damage"
}

func memberByID(party []battle.PolicyMember, id string) battle.PolicyMember {
	for _, m := range party {
		if m.ID == id {
			return m
		}
	}
	return battle.PolicyMember{}
}

func primaryReasonText(code string, act battle.Action) string {
	switch code {
	case "move_se":
		return "Chose a super-effective Move"
	case "move_ko":
		return "Chose the highest KO chance"
	case "move_damage":
		if act.Kind == battle.ActionMove {
			return "Chose strong expected damage"
		}
		return "Chose the best action"
	case "move_survive":
		return "Prioritized surviving incoming damage"
	case "switch_matchup":
		return "Switched for a better matchup"
	case "switch_disadvantage":
		return "Switched out of a Type disadvantage"
	case "replace_score":
		return "Replacement: best reserve for this tier"
	case "near_best":
		return "Sampled from near-best actions"
	case "tie_seed":
		return "Random tie-break among equal scores"
	default:
		return "Dojo policy choice"
	}
}
