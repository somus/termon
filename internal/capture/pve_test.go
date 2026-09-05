package capture_test

import (
	"path/filepath"
	"testing"

	"termon.sh/internal/battle"
	"termon.sh/internal/capture"
	"termon.sh/internal/content"
	"termon.sh/internal/game"
	"termon.sh/internal/onboard"
)

func loadCaptureSet(t *testing.T) *content.Set {
	t.Helper()
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func firstFour(sp content.Species) []string {
	out := make([]string, 0, 4)
	for _, e := range sp.Movepool {
		if len(out) == 4 {
			break
		}
		out = append(out, e.Move)
	}
	return out
}

func TestClampWildDamageNeverKO(t *testing.T) {
	for _, hp := range []int{8, 40, 55, 100, 297} {
		got := capture.ClampWildDamage(hp, 10_000)
		if got >= hp {
			t.Errorf("clamp vs HP %d = %d, must leave the defender alive", hp, got)
		}
		if got < 1 {
			t.Errorf("clamp vs HP %d = %d, want at least 1", hp, got)
		}
	}
}

func TestTargetHPExceedsMaxNonCritHit(t *testing.T) {
	set := loadCaptureSet(t)
	for _, starter := range onboard.StarterSlugs {
		sp := set.Species[starter]
		loadout := firstFour(sp)
		party := []capture.PartyFighter{{Species: starter, Level: 1, Loadout: loadout}}
		for slug, wild := range set.Species {
			if wild.EvolvesTo == nil || isEvolutionTarget(set, slug) {
				continue
			}
			hp := capture.TargetHP(set, party, wild, 1)
			mx := 0
			for _, mvSlug := range loadout {
				d := maxNonCritHit(set, sp, 1, set.Moves[mvSlug], wild, 1)
				if d > mx {
					mx = d
				}
			}
			if hp <= mx {
				t.Errorf("%s vs %s Capture HP %d <= max non-crit hit %d", starter, wild.Slug, hp, mx)
			}
		}
	}
}

func TestGenerateAllBaseFamilies(t *testing.T) {
	set := loadCaptureSet(t)
	var party []capture.PartyFighter
	for _, starter := range onboard.StarterSlugs {
		sp := set.Species[starter]
		party = append(party, capture.PartyFighter{
			Species: starter, Level: 1, Loadout: firstFour(sp),
		})
	}
	for slug, sp := range set.Species {
		if sp.EvolvesTo == nil || isEvolutionTarget(set, slug) {
			continue
		}
		if _, err := capture.Generate(set, party, slug); err != nil {
			t.Errorf("Generate(%s): %v", slug, err)
		}
	}
}

func isEvolutionTarget(set *content.Set, slug string) bool {
	for _, sp := range set.Species {
		if sp.EvolvesTo != nil && sp.EvolvesTo.Species == slug {
			return true
		}
	}
	return false
}

func maxNonCritHit(set *content.Set, atk content.Species, atkLevel int, move content.Move, def content.Species, defLevel int) int {
	a := game.NaturalStat(atk.BaseStats.Attack, atkLevel)
	if move.Category == "special" {
		a = game.NaturalStat(atk.BaseStats.SpAttack, atkLevel)
	}
	d := game.NaturalStat(def.BaseStats.Defense, defLevel)
	base := int(move.Power*float64(a)/float64(d)/float64(battle.DamageDivisor)) + 2
	dmg := float64(base)
	if move.Type == atk.Type {
		dmg *= battle.STABMultiplier
	}
	dmg *= set.Effectiveness(move.Type, def.Type)
	dmg *= battle.VarianceMax
	return max(battle.MinDamage, int(dmg))
}
