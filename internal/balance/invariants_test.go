package balance_test

import (
	"path/filepath"
	"testing"

	"termon.sh/internal/balance"
	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
	"termon.sh/internal/game"
)

func TestCaptureSmokeAllReferenceTeams(t *testing.T) {
	set := loadContent(t)
	smoke, err := balance.RunCaptureSmoke(set)
	if err != nil {
		t.Fatal(err)
	}
	if !smoke.Passed || smoke.Failed != 0 {
		t.Fatalf("capture smoke = %+v, want every Family profile eligible", smoke)
	}
	if smoke.Eligible != smoke.Total {
		t.Fatalf("eligible %d / total %d", smoke.Eligible, smoke.Total)
	}
}

func TestQueueNeutralMatchupsAreNotOHKO(t *testing.T) {
	set := loadContent(t)
	pred := map[string]bool{}
	for _, sp := range set.Species {
		if sp.EvolvesTo != nil {
			pred[sp.EvolvesTo.Species] = true
		}
	}
	var roots []string
	for slug := range set.Species {
		if !pred[slug] {
			roots = append(roots, slug)
		}
	}

	type fighter struct {
		spec    content.Species
		stats   [5]int
		loadout []string
	}
	roster := make([]fighter, 0, len(roots))
	for _, root := range roots {
		slug, err := balance.SpeciesAtLevel(set, root, game.QueueLevel)
		if err != nil {
			t.Fatal(err)
		}
		sp := set.Species[slug]
		mon := game.Monster{Species: slug, Level: game.QueueLevel}
		loadout, err := game.DefaultQueueMoveSet(set, mon)
		if err != nil {
			t.Fatal(err)
		}
		roster = append(roster, fighter{spec: sp, stats: game.QueueStats(sp), loadout: loadout})
	}

	for _, atk := range roster {
		for _, def := range roster {
			best, bestEff := 0, 1.0
			bestMove := ""
			for _, slug := range atk.loadout {
				mv := set.Moves[slug]
				d := queueMaxNonCrit(set, atk.spec, atk.stats, mv, def.spec, def.stats)
				eff := set.Effectiveness(mv.Type, def.spec.Type)
				if d > best {
					best, bestEff, bestMove = d, eff, mv.Name
				}
			}
			if bestEff >= battle.SuperEffectiveAt {
				continue
			}
			if best >= def.stats[0] {
				t.Errorf("Queue non-crit OHKO: %s %s deals %d to %s HP %d",
					atk.spec.Slug, bestMove, best, def.spec.Slug, def.stats[0])
			}
		}
	}
}

func TestNormalizedCorpusKeepsPassingGates(t *testing.T) {
	set := loadContent(t)
	rev, err := balance.ContentRevisionFromDir(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := balance.Run(balance.Config{
		Set:            set,
		Seeds:          balance.CorpusSeeds(1, 8),
		SeedBase:       1,
		Policy:         balance.DefaultPolicy(),
		MaxTurns:       balance.DefaultMaxTurns,
		Rules:          balance.RulesRevision,
		ContentID:      rev,
		NormalizedOnly: true,
		CaptureSmoke:   true,
		FailGates:      false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.BattlesRun < 1 {
		t.Fatal("expected battles")
	}
	for _, g := range out.Gates {
		switch g.Name {
		case balance.GateMirrorWinRate, balance.GateEngineSideAdvantage:
			// These need the 1,024-seed corpus; 8 seeds is too noisy for 47–53%.
			continue
		default:
			if !g.Passed {
				t.Errorf("gate %s failed: value=%v threshold=%s %s", g.Name, g.Value, g.Threshold, g.Detail)
			}
		}
	}
}

func TestReferenceLoadoutAtThirtyDiffersFromQueueDefault(t *testing.T) {
	set := loadContent(t)
	sp := set.Species["rootkit"]
	ref, err := dojo.ReferenceLoadout(set, sp.Slug, game.QueueLevel)
	if err != nil {
		t.Fatal(err)
	}
	queue, err := game.DefaultQueueMoveSet(set, game.Monster{Species: sp.Slug, Level: game.QueueLevel})
	if err != nil {
		t.Fatal(err)
	}
	refMax, queueMax := 0.0, 0.0
	for _, slug := range ref {
		if p := set.Moves[slug].Power; p > refMax {
			refMax = p
		}
	}
	for _, slug := range queue {
		if p := set.Moves[slug].Power; p > queueMax {
			queueMax = p
		}
	}
	if queueMax > 75 {
		t.Errorf("Queue default max power %v, want the Level-1 cap of 75", queueMax)
	}
	if refMax < 90 {
		t.Errorf("Reference Loadout max power %v, want the Evolution rung", refMax)
	}
}

func queueMaxNonCrit(set *content.Set, atk content.Species, atkStats [5]int, move content.Move, def content.Species, defStats [5]int) int {
	a := atkStats[1]
	if move.Category == "special" {
		a = atkStats[3]
	}
	d := defStats[2]
	base := int(move.Power*float64(a)/float64(d)/float64(battle.DamageDivisor)) + 2
	dmg := float64(base)
	if move.Type == atk.Type {
		dmg *= battle.STABMultiplier
	}
	dmg *= set.Effectiveness(move.Type, def.Type)
	dmg *= battle.VarianceMax
	return max(battle.MinDamage, int(dmg))
}
