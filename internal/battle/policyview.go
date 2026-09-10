package battle

import (
	"slices"

	"termon.sh/internal/content"
)

// PolicyMember exposes one side's Monster stats for Dojo policy scoring.
type PolicyMember struct {
	ID, Species, Type  string
	Level              int
	HP, MaxHP          int
	Atk, Def, SpA, Spe int
	Loadout            []string
	PublicMovepool     []string
	Active, Fainted    bool
}

// PolicyView is the public snapshot plus the acting side's private party data.
type PolicyView struct {
	Viewer    string
	Turn      int
	Self      []PolicyMember
	FoeActive PolicyFoe
	FoeRoster []PolicyFoe
}

// PolicyFoe is public roster data for policy incoming-damage modeling.
type PolicyFoe struct {
	ID, Species, Type  string
	Level              int
	HP, MaxHP          int // HP is zero for reserves, never their private current HP
	Atk, Def, SpA, Spe int
	Active, Fainted    bool
	RevealedMoves      []string
	PublicMovepool     []string
}

// PolicyViewFor returns policy inputs for trainer without reading hidden pending actions.
func (b *Battle) PolicyViewFor(trainer string) (PolicyView, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	i, ok := b.sideIndex(trainer)
	if !ok {
		return PolicyView{}, false
	}
	foe := 1 - i
	view := PolicyView{Viewer: trainer, Turn: b.turn}
	for slot, m := range b.sides[i].members {
		view.Self = append(view.Self, policyMemberFrom(m, slot == b.sides[i].active))
	}
	for _, m := range b.sides[foe].members {
		pf := PolicyFoe{
			ID: m.id, Species: m.spec.Slug, Type: m.spec.Type, Level: m.level,
			MaxHP: m.maxHP, Atk: m.atk, Def: m.def, SpA: m.spa, Spe: m.spe,
			Fainted:        m.fainted,
			Active:         m.id == b.sides[foe].activeMember().id,
			PublicMovepool: slices.Clone(m.publicMovepool),
		}
		if pf.Active {
			pf.HP = m.hp
		}
		for _, event := range b.events {
			if event.Kind == EventMoveUsed && event.MonsterID == m.id && !slices.Contains(pf.RevealedMoves, event.MoveSlug) {
				pf.RevealedMoves = append(pf.RevealedMoves, event.MoveSlug)
			}
		}
		view.FoeRoster = append(view.FoeRoster, pf)
		if pf.Active {
			view.FoeActive = pf
		}
	}
	return view, true
}

func policyMemberFrom(m memberState, active bool) PolicyMember {
	return PolicyMember{
		ID: m.id, Species: m.spec.Slug, Type: m.spec.Type, Level: m.level,
		HP: m.hp, MaxHP: m.maxHP, Atk: m.atk, Def: m.def, SpA: m.spa, Spe: m.spe,
		Loadout: append([]string(nil), m.loadout...), Active: active, Fainted: m.fainted,
		PublicMovepool: slices.Clone(m.publicMovepool),
	}
}

// PolicyMovepool returns the mode's public candidates, independently of the
// foe's equipped loadout. Synthetic views without a pool use natural rules.
func PolicyMovepool(set *content.Set, foe PolicyFoe) []string {
	if foe.PublicMovepool != nil {
		return slices.Clone(foe.PublicMovepool)
	}
	return LevelLegalMovepool(set, foe.Species, foe.Level)
}

// LevelLegalMovepool returns level-eligible move slugs for a Species (unknown loadout modeling).
func LevelLegalMovepool(set *content.Set, species string, level int) []string {
	sp, ok := set.Species[species]
	if !ok {
		return nil
	}
	var out []string
	for _, e := range sp.Movepool {
		if e.Level <= level {
			out = append(out, e.Move)
		}
	}
	return out
}
