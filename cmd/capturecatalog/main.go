// ANALYSIS — throwaway (TERM-54). Prints Capture Objective eligibility
// and PvE-band gate results. Do not ship this in termond.
//
// Run from repo root: go run ./cmd/capturecatalog
package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"termon.sh/internal/battle"
	"termon.sh/internal/capture"
	"termon.sh/internal/content"
	"termon.sh/internal/game"
)

const (
	varianceMin = battle.VarianceMin
	varianceMax = battle.VarianceMax
	superEffAt  = battle.SuperEffectiveAt
)

type objectiveID string

const (
	showMoveVariety  objectiveID = "show_move_variety"
	readTheMatchup   objectiveID = "read_the_matchup"
	safeSwitch       objectiveID = "safe_switch"
	measuredPressure objectiveID = "measured_pressure"
	holdTheLine      objectiveID = "hold_the_line"
)

var familyOrder = []string{
	"rootkit", "sproutware", "thornpatch", "mossmuff", "rootanami",
	"emberbyte", "cindernode", "scorchip", "wickware",
	"aquabit", "flowcell", "gushkit", "mistcache", "splashscreen",
	"zaplet", "joulpup", "amperent", "surgetail",
	"spamlet", "bloatware", "wormate",
	"chippunk", "coghound", "servoboar",
}

var identity = map[string]objectiveID{
	"rootkit":      holdTheLine,
	"sproutware":   measuredPressure,
	"thornpatch":   holdTheLine,
	"mossmuff":     holdTheLine,
	"rootanami":    holdTheLine,
	"emberbyte":    measuredPressure,
	"cindernode":   holdTheLine,
	"scorchip":     measuredPressure,
	"wickware":     measuredPressure,
	"aquabit":      measuredPressure,
	"flowcell":     holdTheLine,
	"gushkit":      measuredPressure,
	"mistcache":    measuredPressure,
	"splashscreen": holdTheLine,
	"zaplet":       measuredPressure,
	"joulpup":      measuredPressure,
	"amperent":     holdTheLine,
	"surgetail":    holdTheLine,
	"spamlet":      measuredPressure,
	"bloatware":    holdTheLine,
	"wormate":      holdTheLine,
	"chippunk":     measuredPressure,
	"coghound":     measuredPressure,
	"servoboar":    holdTheLine,
}

var referenceTeams = [][]string{
	{"rootkit", "emberbyte", "aquabit"},
	{"zaplet", "spamlet", "chippunk"},
	{"rootanami", "flowcell", "bloatware"},
	{"sproutware", "wickware", "mistcache"},
	{"thornpatch", "gushkit", "joulpup"},
	{"cindernode", "amperent", "coghound"},
	{"mossmuff", "splashscreen", "surgetail"},
	{"scorchip", "wormate", "servoboar"},
}

var checkpoints = []int{1, 14, 24, 30, 40, 50}

type family struct {
	base, middle, final string
	midLevel, finLevel  int
}

type fighter struct {
	family string
	spec   content.Species
	level  int
	load   []content.Move
}

type band struct {
	offensePct int
}

func main() {
	set, err := content.Load(findContent())
	if err != nil {
		fmt.Fprintf(os.Stderr, "capturecatalog: %v\n", err)
		os.Exit(1)
	}
	families := buildFamilies(set)
	reportCoverage(set)

	chosen := band{offensePct: 100}
	failures := runGates(set, families, chosen)
	if len(failures) > 0 {
		fmt.Printf("full-offense + HP/5 clamp failed %d cases; searching offense...\n", len(failures))
		chosen, failures = searchBand(set, families)
	}
	if len(failures) > 0 {
		fmt.Printf("NO passing band (best offense=%d, %d failures). sample:\n", chosen.offensePct, len(failures))
		for i, f := range failures {
			if i == 16 {
				break
			}
			fmt.Println(" -", f)
		}
		os.Exit(1)
	}
	fmt.Println("\nPassing PvE band: wild_level = max party Level; Capture HP is party-relative; wild Target damage is clamped to floor((defenderMaxHP-1)/5).")
	fmt.Println("Gate corpus: 24 Families × 6 checkpoints × 40 Party fixtures (24 solo + 8 duo + 8 trio) = 5760 cases.")
	printProfiles(set, families, chosen)
	printFocusedSims(set, families, chosen)
}

func buildFamilies(set *content.Set) map[string]family {
	out := make(map[string]family, len(familyOrder))
	for _, base := range familyOrder {
		sp := set.Species[base]
		mid := sp.EvolvesTo.Species
		fin := set.Species[mid].EvolvesTo.Species
		out[base] = family{
			base: base, middle: mid, final: fin,
			midLevel: sp.EvolvesTo.Level,
			finLevel: set.Species[mid].EvolvesTo.Level,
		}
	}
	return out
}

func stageAt(fam family, set *content.Set, level int) content.Species {
	if level >= fam.finLevel {
		return set.Species[fam.final]
	}
	if level >= fam.midLevel {
		return set.Species[fam.middle]
	}
	return set.Species[fam.base]
}

func reportCoverage(set *content.Set) {
	fmt.Println("Default loadout Type coverage (first four Movepool entries)")
	offType := 0
	for _, slug := range familyOrder {
		sp := set.Species[slug]
		types := make([]string, 0, 4)
		for i := range 4 {
			mv := set.Moves[sp.Movepool[i].Move]
			types = append(types, mv.Type)
			if mv.Type != sp.Type {
				offType++
			}
		}
		se := superEffectiveDefenders(set, types)
		fmt.Printf("  %-13s %-8s moves=%s  SE_vs=%s\n", slug, sp.Type, strings.Join(types, ","), strings.Join(se, ","))
	}
	fmt.Printf("Off-Type Moves in any base default loadout: %d\n", offType)
}

func superEffectiveDefenders(set *content.Set, moveTypes []string) []string {
	seen := map[string]bool{}
	var out []string
	for def := range set.Types {
		for _, mt := range moveTypes {
			if set.Effectiveness(mt, def) >= superEffAt {
				if !seen[def] {
					seen[def] = true
					out = append(out, def)
				}
			}
		}
	}
	slices.Sort(out)
	return out
}

func searchBand(set *content.Set, families map[string]family) (band, []string) {
	var best band
	var bestFail []string
	bestN := math.MaxInt
	for _, off := range []int{20, 15, 12, 10, 9, 8, 7, 6} {
		b := band{offensePct: off}
		fails := runGates(set, families, b)
		if len(fails) < bestN {
			bestN = len(fails)
			best = b
			bestFail = fails
		}
		if len(fails) == 0 {
			return b, nil
		}
	}
	return best, bestFail
}

func runGates(set *content.Set, families map[string]family, b band) []string {
	var fails []string
	for _, target := range familyOrder {
		for _, lv := range checkpoints {
			for _, team := range hunterParties(lv, set, families) {
				if msg := checkCase(set, families, b, target, lv, team); msg != "" {
					fails = append(fails, msg)
				}
			}
		}
	}
	return fails
}

func hunterParties(level int, set *content.Set, families map[string]family) [][]fighter {
	var out [][]fighter
	for _, base := range familyOrder {
		out = append(out, []fighter{makeHunter(base, level, set, families)})
	}
	for _, team := range referenceTeams {
		duo := []fighter{makeHunter(team[0], level, set, families), makeHunter(team[1], level, set, families)}
		trio := append([]fighter{}, duo...)
		trio = append(trio, makeHunter(team[2], level, set, families))
		out = append(out, duo, trio)
	}
	return out
}

func makeHunter(famSlug string, level int, set *content.Set, families map[string]family) fighter {
	sp := stageAt(families[famSlug], set, level)
	return fighter{family: famSlug, spec: sp, level: level, load: referenceLoadout(set, sp, level)}
}

func makeWild(set *content.Set, families map[string]family, target string, level int) fighter {
	sp := set.Species[families[target].base]
	return fighter{family: target, spec: sp, level: level, load: defaultLoadout(set, sp)}
}

func defaultLoadout(set *content.Set, sp content.Species) []content.Move {
	out := make([]content.Move, 4)
	for i := range 4 {
		out[i] = set.Moves[sp.Movepool[i].Move]
	}
	return out
}

func referenceLoadout(set *content.Set, sp content.Species, level int) []content.Move {
	var eligible []content.Move
	for _, e := range sp.Movepool {
		if e.Level <= level {
			eligible = append(eligible, set.Moves[e.Move])
		}
	}
	if len(eligible) < 4 {
		return defaultLoadout(set, sp)
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
	add := func(m *content.Move, dest *[]content.Move) {
		if m == nil || used[m.Slug] {
			return
		}
		used[m.Slug] = true
		*dest = append(*dest, *m)
	}
	var out []content.Move
	add(pick(func(m content.Move) bool { return m.Category == "physical" }, func(a, b content.Move) bool { return a.Power > b.Power }), &out)
	add(pick(func(m content.Move) bool { return m.Category == "special" }, func(a, b content.Move) bool { return a.Power > b.Power }), &out)
	add(pick(func(content.Move) bool { return true }, func(a, b content.Move) bool {
		if a.Accuracy != b.Accuracy {
			return a.Accuracy > b.Accuracy
		}
		return a.Slug < b.Slug
	}), &out)
	add(pick(func(content.Move) bool { return true }, func(a, b content.Move) bool {
		la, lb := moveUnlock(sp, a.Slug), moveUnlock(sp, b.Slug)
		if la != lb {
			return la < lb
		}
		return a.Slug < b.Slug
	}), &out)
	for _, m := range eligible {
		if len(out) == 4 {
			break
		}
		add(&m, &out)
	}
	return out
}

func moveUnlock(sp content.Species, slug string) int {
	for _, e := range sp.Movepool {
		if e.Move == slug {
			return e.Level
		}
	}
	return 99
}

func checkCase(set *content.Set, families map[string]family, b band, target string, level int, party []fighter) string {
	wild := makeWild(set, families, target, level)
	objs := generate(set, identity[target], party, wild, b)
	if len(objs) != 3 {
		return fmt.Sprintf("%s L%d party=%s: generated %d objectives %v", target, level, partyLabel(party), len(objs), objs)
	}
	if dup(objs) {
		return fmt.Sprintf("%s L%d party=%s: duplicate objectives %v", target, level, partyLabel(party), objs)
	}
	if !containsID(objs, showMoveVariety) {
		return fmt.Sprintf("%s L%d: missing variety", target, level)
	}
	sum := 0
	for _, id := range objs {
		sum += award(id)
	}
	if sum != 100 {
		return fmt.Sprintf("%s L%d: awards %d", target, level, sum)
	}
	for _, id := range objs {
		if !eligible(set, id, party, wild, b) {
			return fmt.Sprintf("%s L%d party=%s: ineligible %s in %v", target, level, partyLabel(party), id, objs)
		}
	}
	if msg := simScripted(set, objs, party, wild, b); msg != "" {
		return fmt.Sprintf("%s L%d party=%s: %s objs=%v", target, level, partyLabel(party), msg, objs)
	}
	if msg := simOveragg(set, objs, party, wild, b); msg != "" {
		return fmt.Sprintf("%s L%d party=%s: %s objs=%v", target, level, partyLabel(party), msg, objs)
	}
	return ""
}

func generate(set *content.Set, ident objectiveID, party []fighter, wild fighter, b band) []objectiveID {
	out := []objectiveID{showMoveVariety}
	order := []objectiveID{ident, readTheMatchup, safeSwitch, complement(ident)}
	for _, id := range order {
		if len(out) == 3 {
			break
		}
		if containsID(out, id) {
			continue
		}
		if eligible(set, id, party, wild, b) {
			out = append(out, id)
		}
	}
	return out
}

func complement(id objectiveID) objectiveID {
	if id == measuredPressure {
		return holdTheLine
	}
	return measuredPressure
}

func award(id objectiveID) int {
	if id == showMoveVariety {
		return 30
	}
	return 35
}

func eligible(set *content.Set, id objectiveID, party []fighter, wild fighter, b band) bool {
	switch id {
	case showMoveVariety:
		for _, p := range party {
			slugs := map[string]bool{}
			for _, m := range p.load {
				if m.Power > 0 {
					slugs[m.Slug] = true
				}
			}
			if len(slugs) < 3 {
				return false
			}
		}
		return true
	case readTheMatchup:
		for _, p := range party {
			for _, m := range p.load {
				if set.Effectiveness(m.Type, wild.spec.Type) >= superEffAt {
					return true
				}
			}
		}
		return false
	case safeSwitch:
		return len(party) >= 2
	case measuredPressure:
		weakMin, _, _ := partyHits(set, party, wild, b)
		hp := captureHP(set, party, wild, b)
		if weakMin < 1 {
			return false
		}
		return 2*weakMin < (hp*3)/4
	case holdTheLine:
		wmove := wildStrike(wild)
		for _, p := range party {
			dmg := hit(set, wild, wmove, p, b, true, varianceMax)
			if dmg*2 < naturalStat(p.spec.BaseStats.HP, p.level) {
				return true
			}
		}
		return false
	}
	return false
}

func simScripted(set *content.Set, objs []objectiveID, party []fighter, wild fighter, b band) string {
	whp := captureHP(set, party, wild, b)
	php := make([]int, len(party))
	pmax := make([]int, len(party))
	for i, p := range party {
		pmax[i] = naturalStat(p.spec.BaseStats.HP, p.level)
		php[i] = pmax[i]
	}
	active := 0
	used := map[string]bool{}
	distinct := 0
	pressureHits := 0
	done := map[objectiveID]bool{}
	gauge := 0
	complete := func(id objectiveID) {
		if done[id] || !containsID(objs, id) {
			return
		}
		done[id] = true
		gauge += award(id)
	}
	wmove := wildStrike(wild)
	plan := scriptedActions(set, objs, party, wild, b)
	missedOnce := false

	resolveWild := func() string {
		if whp <= 0 {
			return ""
		}
		php[active] -= hit(set, wild, wmove, party[active], b, true, varianceMax)
		if php[active] <= 0 {
			return "player fainted on scripted line"
		}
		if php[active]*2 > pmax[active] {
			complete(holdTheLine)
		}
		return ""
	}
	resolvePlayer := func(mv content.Move, count bool) {
		dmg := hit(set, party[active], mv, wild, b, false, varianceMin)
		if count {
			whp -= dmg
			if dmg > 0 {
				pressureHits++
				if pressureHits >= 2 && whp*4 > captureHP(set, party, wild, b) {
					complete(measuredPressure)
				}
			}
			if set.Effectiveness(mv.Type, wild.spec.Type) >= superEffAt {
				complete(readTheMatchup)
			}
			if !used[mv.Slug] {
				used[mv.Slug] = true
				distinct++
				if distinct >= 3 {
					complete(showMoveVariety)
				}
			}
		}
	}

	for _, act := range plan {
		pFirst := naturalStat(party[active].spec.BaseStats.Speed, party[active].level) >= naturalStat(wild.spec.BaseStats.Speed, wild.level)
		switch act.kind {
		case "switch":
			if act.target < 0 || act.target >= len(party) || act.target == active || php[act.target] <= 0 {
				return "illegal switch in scripted line"
			}
			active = act.target
			if php[active]*2 >= pmax[active] {
				complete(safeSwitch)
			}
			if msg := resolveWild(); msg != "" {
				return msg
			}
		case "move":
			land := true
			if act.missable && !missedOnce {
				missedOnce = true
				land = false
			}
			if pFirst {
				if land {
					resolvePlayer(act.move, true)
					if gauge >= 100 {
						return ""
					}
					if whp <= 0 {
						return "KO before gauge on min-damage scripted line"
					}
				}
				if msg := resolveWild(); msg != "" {
					return msg
				}
			} else {
				if msg := resolveWild(); msg != "" {
					return msg
				}
				if land {
					resolvePlayer(act.move, true)
					if gauge >= 100 {
						return ""
					}
					if whp <= 0 {
						return "KO before gauge on min-damage scripted line"
					}
				}
			}
		}
		if gauge >= 100 {
			return ""
		}
	}
	return fmt.Sprintf("scripted line ended at gauge %d wildHP %d distinct %d objs done %d", gauge, whp, distinct, len(done))
}

type action struct {
	kind     string
	move     content.Move
	target   int
	missable bool
}

func scriptedActions(set *content.Set, objs []objectiveID, party []fighter, wild fighter, b band) []action {
	lead := pickActive(set, objs, party, wild, b)
	var acts []action
	if lead != 0 {
		acts = append(acts, action{kind: "switch", target: lead})
	}
	moves := orderedMoves(set, party[lead], wild, containsID(objs, readTheMatchup))
	if len(moves) == 0 {
		return acts
	}
	acts = append(acts, action{kind: "move", move: moves[0], missable: true})
	acts = append(acts, action{kind: "move", move: moves[0]})
	for i := 1; i < 3 && i < len(moves); i++ {
		acts = append(acts, action{kind: "move", move: moves[i]})
	}
	return acts
}

func pickActive(set *content.Set, objs []objectiveID, party []fighter, wild fighter, b band) int {
	needSwitch := containsID(objs, safeSwitch) && len(party) >= 2
	needMatch := containsID(objs, readTheMatchup)
	needHold := containsID(objs, holdTheLine)

	if needMatch {
		for i, p := range party {
			if firstSE(set, p, wild) == nil {
				continue
			}
			if needSwitch && i == 0 {
				continue
			}
			return i
		}
		for i, p := range party {
			if firstSE(set, p, wild) != nil {
				return i
			}
		}
	}
	if needSwitch {
		if i := sturdiestIndex(set, party, wild, b); i > 0 {
			return i
		}
		return 1
	}
	if needHold {
		if i := sturdiestIndex(set, party, wild, b); i >= 0 {
			return i
		}
	}
	return 0
}

func orderedMoves(set *content.Set, active, wild fighter, wantSE bool) []content.Move {
	moves := distinctDamaging(active.load)
	slices.SortFunc(moves, func(a, b content.Move) int {
		if a.Power != b.Power {
			if a.Power < b.Power {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Slug, b.Slug)
	})
	if !wantSE || len(moves) < 3 {
		return moves
	}
	se := firstSE(set, active, wild)
	if se == nil {
		return moves
	}
	out := make([]content.Move, 0, len(moves))
	for _, m := range moves {
		if m.Slug != se.Slug {
			out = append(out, m)
		}
	}
	if len(out) >= 2 {
		out = append(out[:2], append([]content.Move{*se}, out[2:]...)...)
	} else {
		out = append(out, *se)
	}
	return out
}

func distinctDamaging(load []content.Move) []content.Move {
	seen := map[string]bool{}
	var out []content.Move
	for _, m := range load {
		if m.Power <= 0 || seen[m.Slug] {
			continue
		}
		seen[m.Slug] = true
		out = append(out, m)
	}
	return out
}

func firstSE(set *content.Set, active, wild fighter) *content.Move {
	for _, m := range active.load {
		if set.Effectiveness(m.Type, wild.spec.Type) >= superEffAt {
			cp := m
			return &cp
		}
	}
	return nil
}

func simOveragg(set *content.Set, _ []objectiveID, party []fighter, wild fighter, b band) string {
	best := 0
	for _, p := range party {
		for _, m := range p.load {
			d := hit(set, p, m, wild, b, false, varianceMax)
			if d > best {
				best = d
			}
		}
	}
	hp := captureHP(set, party, wild, b)
	if best >= hp {
		return fmt.Sprintf("over-agg one-shot (%d dmg vs %d hp)", best, hp)
	}
	hits := (hp + best - 1) / best
	if hits > 8 {
		return "over-agg could not KO within 8 max-damage hits"
	}
	return ""
}

func wildStrike(wild fighter) content.Move {
	var best content.Move
	bestScore := -1.0
	for _, m := range wild.load {
		if m.Power <= 0 {
			continue
		}
		score := m.Accuracy*1000 + m.Power
		if score > bestScore {
			bestScore = score
			best = m
		}
	}
	return best
}

func hit(set *content.Set, atk fighter, move content.Move, def fighter, _ band, wildAttacker bool, variance float64) int {
	a := game.NaturalStat(atk.spec.BaseStats.Attack, atk.level)
	if move.Category == "special" {
		a = game.NaturalStat(atk.spec.BaseStats.SpAttack, atk.level)
	}
	d := game.NaturalStat(def.spec.BaseStats.Defense, def.level)
	dmg := battle.DamageBase(move.Power, a, d, move.Type, atk.spec.Type, set.Effectiveness(move.Type, def.spec.Type))
	dealt := max(battle.MinDamage, int(dmg*variance))
	if wildAttacker {
		dealt = capture.ClampWildDamage(game.NaturalStat(def.spec.BaseStats.HP, def.level), dealt)
	}
	return dealt
}

func partyHits(set *content.Set, party []fighter, wild fighter, b band) (weakMin, strongMax, _ int) {
	weakMin = math.MaxInt
	for _, p := range party {
		for _, m := range p.load {
			if m.Power <= 0 {
				continue
			}
			mn := hit(set, p, m, wild, b, false, varianceMin)
			mx := hit(set, p, m, wild, b, false, varianceMax)
			if mn < weakMin {
				weakMin = mn
			}
			if mx > strongMax {
				strongMax = mx
			}
		}
	}
	if weakMin == math.MaxInt {
		weakMin = 1
	}
	if strongMax < 1 {
		strongMax = 1
	}
	return weakMin, strongMax, 0
}

func captureHP(set *content.Set, party []fighter, wild fighter, b band) int {
	globalMax := 0
	low := 1
	for _, p := range party {
		var mins []int
		mx := 0
		for _, m := range p.load {
			if m.Power <= 0 {
				continue
			}
			a := hit(set, p, m, wild, b, false, varianceMin)
			c := hit(set, p, m, wild, b, false, varianceMax)
			mins = append(mins, a)
			if c > mx {
				mx = c
			}
		}
		if mx < 1 {
			mx = 1
		}
		if mx > globalMax {
			globalMax = mx
		}
		slices.Sort(mins)
		line := 0
		for i, v := range mins {
			if i >= 3 {
				break
			}
			line += v
		}
		if se := firstSE(set, p, wild); se != nil && len(mins) >= 3 {
			line = mins[0] + mins[1] + hit(set, p, *se, wild, b, false, varianceMin)
		}
		low = max(low, mx+1, line+1)
	}
	high := max(low, 8*globalMax)
	return min(high, low+globalMax)
}

func sturdiestIndex(set *content.Set, party []fighter, wild fighter, b band) int {
	wmove := wildStrike(wild)
	best := -1
	bestRemain := -1
	for i, p := range party {
		maxHP := naturalStat(p.spec.BaseStats.HP, p.level)
		dmg := hit(set, wild, wmove, p, b, true, varianceMax)
		remain := maxHP - dmg
		if remain > bestRemain {
			bestRemain = remain
			best = i
		}
	}
	return best
}

// naturalStat forwards to game.NaturalStat, the one stat formula.
func naturalStat(base, level int) int { return game.NaturalStat(base, level) }

func partyLabel(party []fighter) string {
	parts := make([]string, len(party))
	for i, p := range party {
		parts[i] = p.spec.Slug
	}
	return strings.Join(parts, "+")
}

func containsID(ids []objectiveID, id objectiveID) bool {
	return slices.Contains(ids, id)
}

func dup(ids []objectiveID) bool {
	seen := map[objectiveID]bool{}
	for _, id := range ids {
		if seen[id] {
			return true
		}
		seen[id] = true
	}
	return false
}

func printProfiles(set *content.Set, families map[string]family, b band) {
	fmt.Println("\nFamily profiles (L1 solo same-Family, L1 solo counter, L1 trio starter)")
	starter := []fighter{
		makeHunter("rootkit", 1, set, families),
		makeHunter("emberbyte", 1, set, families),
		makeHunter("aquabit", 1, set, families),
	}
	fmt.Printf("%-13s %-8s %-18s  solo-self              solo-counter           trio-starter\n", "family", "type", "identity")
	for _, slug := range familyOrder {
		ident := identity[slug]
		self := generate(set, ident, []fighter{makeHunter(slug, 1, set, families)}, makeWild(set, families, slug, 1), b)
		counterFam := counterOf(set.Species[slug].Type)
		var counter []objectiveID
		if counterFam != "" {
			counter = generate(set, ident, []fighter{makeHunter(counterFam, 1, set, families)}, makeWild(set, families, slug, 1), b)
		}
		trio := generate(set, ident, starter, makeWild(set, families, slug, 1), b)
		fmt.Printf("%-13s %-8s %-18s  %-22s %-22s %s\n", slug, set.Species[slug].Type, ident, joinIDs(self), joinIDs(counter), joinIDs(trio))
	}
}

func counterOf(defType string) string {
	switch defType {
	case "organic":
		return "emberbyte"
	case "thermal":
		return "aquabit"
	case "coolant":
		return "rootkit"
	case "silicon":
		return "zaplet"
	case "virus":
		return "chippunk"
	case "current":
		return "spamlet"
	}
	return ""
}

func joinIDs(ids []objectiveID) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = string(id)
	}
	return strings.Join(parts, ",")
}

func printFocusedSims(set *content.Set, families map[string]family, b band) {
	fmt.Println("\nFocused simulations")
	cases := []struct {
		name   string
		target string
		level  int
		party  []string
	}{
		{"gauge fill L1 starter vs emberbyte", "emberbyte", 1, []string{"rootkit"}},
		{"1-mon no matchup (thermal vs thermal)", "emberbyte", 1, []string{"scorchip"}},
		{"switch trio vs bloatware", "bloatware", 1, []string{"rootkit", "emberbyte", "aquabit"}},
		{"L50 final vs base servoboar", "servoboar", 50, []string{"scorchip"}},
		{"L50 trio vs mossmuff", "mossmuff", 50, []string{"rootkit", "emberbyte", "aquabit"}},
	}
	for _, c := range cases {
		var party []fighter
		for _, f := range c.party {
			party = append(party, makeHunter(f, c.level, set, families))
		}
		wild := makeWild(set, families, c.target, c.level)
		objs := generate(set, identity[c.target], party, wild, b)
		whp := captureHP(set, party, wild, b)
		maxHit := 0
		minHit := math.MaxInt
		for _, p := range party {
			for _, m := range p.load {
				mx := hit(set, p, m, wild, b, false, varianceMax)
				mn := hit(set, p, m, wild, b, false, varianceMin)
				if mx > maxHit {
					maxHit = mx
				}
				if mn < minHit {
					minHit = mn
				}
			}
		}
		wdmg := hit(set, wild, wildStrike(wild), party[0], b, true, varianceMax)
		fmt.Printf("  %s\n    objs=%s wildHP=%d maxHit=%d minHit=%d wildMaxVsLead=%d leadHP=%d\n",
			c.name, joinIDs(objs), whp, maxHit, minHit, wdmg, naturalStat(party[0].spec.BaseStats.HP, party[0].level))
		if msg := simScripted(set, objs, party, wild, b); msg != "" {
			fmt.Println("    SCRIPTED FAIL:", msg)
		} else {
			fmt.Println("    scripted: gauge fills before KO (min dmg, one injected miss)")
		}
		if msg := simOveragg(set, objs, party, wild, b); msg != "" {
			fmt.Println("    OVERAGG FAIL:", msg)
		} else {
			fmt.Println("    over-agg: same-move max dmg KOs without variety")
		}
	}
}

func findContent() string {
	dir, err := os.Getwd()
	if err != nil {
		return "content"
	}
	for range 8 {
		cand := filepath.Join(dir, "content")
		if st, err := os.Stat(filepath.Join(cand, "species")); err == nil && st.IsDir() {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "content"
}
