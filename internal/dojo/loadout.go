package dojo

import (
	"fmt"

	"termon.sh/internal/content"
)

// ReferenceLoadout selects up to four eligible Moves per balance-methodology.md.
func ReferenceLoadout(set *content.Set, species string, level int) ([]string, error) {
	sp, ok := set.Species[species]
	if !ok {
		return nil, fmt.Errorf("dojo: unknown species %q", species)
	}
	var eligible []content.Move
	for _, e := range sp.Movepool {
		if e.Level <= level {
			mv, ok := set.Moves[e.Move]
			if !ok {
				return nil, fmt.Errorf("dojo: unknown move %q", e.Move)
			}
			eligible = append(eligible, mv)
		}
	}
	if len(eligible) < 1 {
		return nil, fmt.Errorf("dojo: %s has no moves at level %d", species, level)
	}
	if len(eligible) < 4 {
		out := make([]string, len(eligible))
		for i, m := range eligible {
			out[i] = m.Slug
		}
		return out, nil
	}
	pick := func(pred func(content.Move) bool, less func(a, b content.Move) bool) *content.Move {
		var best *content.Move
		for i := range eligible {
			m := eligible[i]
			if !pred(m) {
				continue
			}
			if best == nil || less(m, *best) {
				cp := m
				best = &cp
			}
		}
		return best
	}
	used := map[string]bool{}
	var out []string
	add := func(m *content.Move) {
		if m == nil || used[m.Slug] {
			return
		}
		used[m.Slug] = true
		out = append(out, m.Slug)
	}
	add(pick(func(m content.Move) bool { return m.Category == "physical" },
		func(a, b content.Move) bool { return a.Power > b.Power }))
	add(pick(func(m content.Move) bool { return m.Category == "special" },
		func(a, b content.Move) bool { return a.Power > b.Power }))
	add(pick(func(content.Move) bool { return true }, func(a, b content.Move) bool {
		if a.Accuracy != b.Accuracy {
			return a.Accuracy > b.Accuracy
		}
		return a.Slug < b.Slug
	}))
	add(pick(func(content.Move) bool { return true }, func(a, b content.Move) bool {
		la, lb := moveUnlock(sp, a.Slug), moveUnlock(sp, b.Slug)
		if la != lb {
			return la < lb
		}
		return a.Slug < b.Slug
	}))
	for _, m := range eligible {
		if len(out) == 4 {
			break
		}
		cp := m
		add(&cp)
	}
	return out, nil
}

func moveUnlock(sp content.Species, slug string) int {
	for _, e := range sp.Movepool {
		if e.Move == slug {
			return e.Level
		}
	}
	return 99
}

// FilterLoadoutMaxPower keeps only moves at or below maxPower.
func FilterLoadoutMaxPower(set *content.Set, loadout []string, maxPower float64) ([]string, error) {
	var out []string
	for _, slug := range loadout {
		mv, ok := set.Moves[slug]
		if !ok {
			return nil, fmt.Errorf("dojo: unknown move %q", slug)
		}
		if mv.Power <= maxPower {
			out = append(out, slug)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("dojo: no moves at or below power %v", maxPower)
	}
	return out, nil
}
