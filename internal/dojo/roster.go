package dojo

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/game"
)

// SparringSlot is one Dojo Opponent in slot order for preview and battle setup.
type SparringSlot struct {
	Slot    int
	Role    string // favorable | neutral | unfavorable
	Family  string
	Species string
	Level   int
	Loadout []string
	Type    string
}

// BuildSparringRoster constructs the three-slot Dojo Party for a natural Party
// snapshot. Cycle changes the deterministic candidate rotation; the server uses
// the UTC Server Day plus a no-reward rematch number.
func BuildSparringRoster(set *content.Set, party []game.Monster, cycle int64) ([]SparringSlot, error) {
	if set == nil {
		return nil, errors.New("dojo: nil content")
	}
	if len(party) != 3 {
		return nil, fmt.Errorf("dojo: want 3 party monsters, got %d", len(party))
	}
	playerTypes := make([]string, 3)
	playerStages := make([]int, 3)
	levels := make([]int, 3)
	for i, m := range party {
		sp, ok := set.Species[m.Species]
		if !ok {
			return nil, fmt.Errorf("dojo: unknown species %q", m.Species)
		}
		playerTypes[i] = sp.Type
		playerStages[i] = StageIndex(set, m.Species)
		levels[i] = max(m.Level, 1)
	}
	families, err := buildRosterFamilies(set, playerTypes, cycle)
	if err != nil {
		return nil, err
	}
	roles := []string{"favorable", "neutral", "unfavorable"}
	out := make([]SparringSlot, 3)
	for i, fam := range families {
		species, err := SpeciesAtStage(set, fam, playerStages[i])
		if err != nil {
			return nil, err
		}
		loadout, err := ReferenceLoadout(set, species, levels[i])
		if err != nil {
			return nil, err
		}
		out[i] = SparringSlot{
			Slot: i + 1, Role: roles[i], Family: fam, Species: species,
			Level: levels[i], Loadout: loadout, Type: set.Species[species].Type,
		}
	}
	return out, nil
}

func buildRosterFamilies(set *content.Set, playerTypes []string, cycle int64) ([]string, error) {
	roles := []string{"F", "N", "U"}
	used := map[string]bool{}
	out := make([]string, 3)
	for i, role := range roles {
		cands := candidateFamilies(set, role, playerTypes[i])
		if len(cands) == 0 {
			return nil, fmt.Errorf("slot %d role %s type %s: no pool family", i, role, playerTypes[i])
		}
		start := rosterOffset(playerTypes, i, cycle, len(cands))
		picked := ""
		for step := range len(cands) {
			fam := cands[(start+step)%len(cands)]
			if !used[fam] {
				picked = fam
				break
			}
		}
		if picked == "" {
			picked = cands[start]
		}
		used[picked] = true
		out[i] = picked
	}
	return out, nil
}

func candidateFamilies(set *content.Set, role, playerType string) []string {
	var out []string
	for _, t := range candidateTypes(set, role, playerType) {
		out = append(out, SparringPool[t]...)
	}
	slices.Sort(out)
	return out
}

func rosterOffset(playerTypes []string, slot int, cycle int64, count int) int {
	base := slot % count
	for _, r := range strings.Join(playerTypes, "/") {
		base = (base*31 + int(r)) % count
	}
	shift := int(cycle % int64(count))
	if shift < 0 {
		shift += count
	}
	return (base + shift) % count
}

func candidateTypes(set *content.Set, role, playerType string) []string {
	var out []string
	for t := range set.Types {
		f := set.Effectiveness(t, playerType) >= battle.SuperEffectiveAt
		u := set.Effectiveness(playerType, t) >= battle.SuperEffectiveAt
		n := !f && !u
		ok := false
		switch role {
		case "F":
			ok = f
		case "U":
			ok = u
		case "N":
			ok = n
		}
		if ok {
			out = append(out, t)
		}
	}
	slices.Sort(out)
	return out
}
