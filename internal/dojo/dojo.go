// Package dojo holds Dojo Master lesson tables and opponent policies.
package dojo

import (
	"errors"
	"fmt"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/game"
)

// BotTrainer is the battle-side trainer ID for Dojo wild opponents.
const BotTrainer = "dojo:opponent"

// LessonTarget returns the wild Species slug for starter and lesson number.
func LessonTarget(starter string, lesson int) (string, error) {
	table, ok := lessonTargets[starter]
	if !ok {
		return "", fmt.Errorf("dojo: unknown starter %q", starter)
	}
	if lesson < 1 || lesson > len(table) {
		return "", fmt.Errorf("dojo: invalid lesson %d", lesson)
	}
	return table[lesson-1], nil
}

var lessonTargets = map[string][]string{
	"rootkit":   {"mistcache", "chippunk"},
	"emberbyte": {"sproutware", "zaplet"},
	"aquabit":   {"wickware", "spamlet"},
}

// StarterFromSave returns the species slug in Party slot 1.
func StarterFromSave(save *game.Save) (string, error) {
	if save == nil || save.Party[0] == "" {
		return "", errors.New("dojo: no party lead")
	}
	m, ok := game.MonsterByID(save, save.Party[0])
	if !ok {
		return "", errors.New("dojo: missing lead")
	}
	return m.Species, nil
}

// ChooseApprenticeMove samples a move with SE/neutral/NVE weights (3.0/1.0/0.5).
func ChooseApprenticeMove(set *content.Set, loadout []string, defenderType string, rng battle.Rand) (string, error) {
	if set == nil || rng == nil {
		return "", errors.New("dojo: nil argument")
	}
	if len(loadout) == 0 {
		return "", errors.New("dojo: empty loadout")
	}
	weights := make([]float64, len(loadout))
	var total float64
	for i, slug := range loadout {
		mv, ok := set.Moves[slug]
		if !ok {
			return "", fmt.Errorf("dojo: unknown move %q", slug)
		}
		w := apprenticeWeight(set.Effectiveness(mv.Type, defenderType))
		weights[i] = w
		total += w
	}
	r := rng.Float64() * total
	var cum float64
	for i, w := range weights {
		cum += w
		if r < cum {
			return loadout[i], nil
		}
	}
	return loadout[len(loadout)-1], nil
}

func apprenticeWeight(eff float64) float64 {
	switch {
	case eff >= battle.SuperEffectiveAt:
		return 3.0
	case eff > 0 && eff < battle.ResistedBelow:
		return 0.5
	default:
		return 1.0
	}
}

// WildLoadout is the first four Movepool entries for a Species.
func WildLoadout(set *content.Set, species string) ([]string, error) {
	sp, ok := set.Species[species]
	if !ok {
		return nil, fmt.Errorf("dojo: unknown species %q", species)
	}
	if len(sp.Movepool) < 4 {
		return nil, fmt.Errorf("dojo: %s movepool too short", species)
	}
	out := make([]string, 4)
	for i := range 4 {
		out[i] = sp.Movepool[i].Move
	}
	return out, nil
}
