package battle

import "termon.sh/internal/content"

// PolicyMember exposes one side's Monster stats for Dojo policy scoring.
type PolicyMember struct {
	ID, Species, Type  string
	Level              int
	HP, MaxHP          int
	Atk, Def, SpA, Spe int
	Loadout            []string
	Active, Fainted    bool
}

// PolicyView is the public snapshot plus the acting side's private party data.
type PolicyView struct {
	Viewer    string
	Turn      int
	Self      []PolicyMember
	FoeActive PolicyMember
	FoeRoster []PolicyFoe
}

// PolicyFoe is public roster data for policy incoming-damage modeling.
type PolicyFoe struct {
	Species string
	Type    string
	Level   int
	HP      int
	MaxHP   int
	Active  bool
	Fainted bool
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
			Species: m.spec.Slug, Type: m.spec.Type, Level: m.level,
			HP: m.hp, MaxHP: m.maxHP, Fainted: m.fainted,
			Active: m.id == b.sides[foe].activeMember().id,
		}
		view.FoeRoster = append(view.FoeRoster, pf)
		if pf.Active {
			view.FoeActive = policyMemberFrom(m, true)
		}
	}
	return view, true
}

func policyMemberFrom(m memberState, active bool) PolicyMember {
	return PolicyMember{
		ID: m.id, Species: m.spec.Slug, Type: m.spec.Type, Level: m.level,
		HP: m.hp, MaxHP: m.maxHP, Atk: m.atk, Def: m.def, SpA: m.spa, Spe: m.spe,
		Loadout: append([]string(nil), m.loadout...), Active: active, Fainted: m.fainted,
	}
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
