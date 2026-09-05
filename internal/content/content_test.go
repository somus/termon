package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFrozenRoster(t *testing.T) {
	set, err := Load("../../content")
	if err != nil {
		t.Fatalf("frozen roster failed validation: %v", err)
	}

	t.Run("entity counts", func(t *testing.T) {
		if len(set.Types) != 6 {
			t.Errorf("got %d types, want 6", len(set.Types))
		}
		if len(set.Moves) != 144 {
			t.Errorf("got %d moves, want 144", len(set.Moves))
		}
		if len(set.Species) != 72 {
			t.Errorf("got %d species, want 72", len(set.Species))
		}
		if len(set.Arts) != 72 {
			t.Errorf("got %d arts, want 72", len(set.Arts))
		}
	})

	t.Run("type chart", func(t *testing.T) {
		super := [][2]string{
			{"thermal", "organic"},
			{"thermal", "virus"},
			{"coolant", "thermal"},
			{"coolant", "silicon"},
			{"organic", "coolant"},
			{"organic", "current"},
			{"current", "coolant"},
			{"current", "silicon"},
			{"virus", "organic"},
			{"virus", "current"},
			{"silicon", "thermal"},
			{"silicon", "virus"},
		}
		for _, pair := range super {
			if got := set.Effectiveness(pair[0], pair[1]); got != 2.0 {
				t.Errorf("%s vs %s = %v, want 2.0", pair[0], pair[1], got)
			}
		}
		if got := set.Effectiveness("thermal", "coolant"); got != 1.0 {
			t.Errorf("thermal vs coolant = %v, want neutral 1.0", got)
		}
		for slug, typ := range set.Types {
			if len(typ.Matchup) != 2 {
				t.Errorf("type %s has %d strengths, want 2", slug, len(typ.Matchup))
			}
		}
	})

	t.Run("every species has art", func(t *testing.T) {
		for slug := range set.Species {
			if _, ok := set.Arts[slug]; !ok {
				t.Errorf("species %s missing sprite art", slug)
			}
		}
	})

	t.Run("starter loadouts", func(t *testing.T) {
		want := map[string][]string{
			"rootkit":   {"root_access", "chmod", "sudo", "setuid"},
			"emberbyte": {"burn_in", "xor_fold", "crc32", "salted_hash"},
			"aquabit":   {"ping_flood", "hop_count", "checksum", "jumbo_frame"},
		}
		for slug, moves := range want {
			sp, ok := set.Species[slug]
			if !ok {
				t.Errorf("starter %s missing", slug)
				continue
			}
			if len(sp.Movepool) < 4 {
				t.Errorf("starter %s movepool length %d, want at least 4", slug, len(sp.Movepool))
				continue
			}
			for i, move := range moves {
				if sp.Movepool[i].Move != move {
					t.Errorf("starter %s loadout[%d] = %s, want %s", slug, i, sp.Movepool[i].Move, move)
				}
			}
		}
	})

	t.Run("family identity movepools", func(t *testing.T) {
		wantPower := []float64{40, 55, 65, 75, 90, 100}
		wantAcc := []float64{100, 100, 95, 90, 85, 80}

		pred := map[string]string{}
		for slug, sp := range set.Species {
			if sp.EvolvesTo != nil {
				pred[sp.EvolvesTo.Species] = slug
			}
		}
		moveOwner := map[string]string{}
		families := 0
		for slug, sp := range set.Species {
			if pred[slug] != "" {
				continue
			}
			families++
			chain := []Species{sp}
			cur := sp
			for cur.EvolvesTo != nil {
				nxt, ok := set.Species[cur.EvolvesTo.Species]
				if !ok {
					t.Errorf("family %s missing successor %s", slug, cur.EvolvesTo.Species)
					break
				}
				chain = append(chain, nxt)
				cur = nxt
			}
			if len(chain) != 3 {
				t.Errorf("family %s has %d stages, want 3", slug, len(chain))
				continue
			}
			pool := chain[0].Movepool
			if len(pool) != 6 {
				t.Errorf("%s movepool length %d, want 6", slug, len(pool))
				continue
			}
			wantLvl := []int{1, 1, 1, 1, chain[0].EvolvesTo.Level, chain[1].EvolvesTo.Level}
			for i, e := range pool {
				if e.Level != wantLvl[i] {
					t.Errorf("%s movepool[%d] level %d, want %d", slug, i, e.Level, wantLvl[i])
				}
				mv, ok := set.Moves[e.Move]
				if !ok {
					t.Errorf("%s movepool references unknown move %s", slug, e.Move)
					continue
				}
				if mv.Power != wantPower[i] {
					t.Errorf("%s %s power %v, want %v", slug, e.Move, mv.Power, wantPower[i])
				}
				if mv.Accuracy != wantAcc[i] {
					t.Errorf("%s %s accuracy %v, want %v", slug, e.Move, mv.Accuracy, wantAcc[i])
				}
				if owner, ok := moveOwner[e.Move]; ok {
					t.Errorf("move %s shared by families %s and %s", e.Move, owner, slug)
				}
				moveOwner[e.Move] = slug
			}
			for _, member := range chain[1:] {
				if len(member.Movepool) != len(pool) {
					t.Errorf("%s movepool length %d, want family %s length %d", member.Slug, len(member.Movepool), slug, len(pool))
					continue
				}
				for i, e := range member.Movepool {
					if e != pool[i] {
						t.Errorf("%s movepool[%d] = %+v, want family %s %+v", member.Slug, i, e, slug, pool[i])
					}
				}
			}
		}
		if families != 24 {
			t.Errorf("got %d families, want 24", families)
		}
		if len(moveOwner) != 144 {
			t.Errorf("got %d unique family moves, want 144", len(moveOwner))
		}
	})

	t.Run("evolution families", func(t *testing.T) {
		type expectedFamily struct {
			middle      string
			final       string
			middleLevel int
			finalLevel  int
		}
		want := map[string]expectedFamily{
			"rootkit":      {"barkdoor", "priviloak", 16, 32},
			"sproutware":   {"vinemount", "canopynet", 18, 34},
			"thornpatch":   {"briarwall", "fortiforest", 20, 36},
			"mossmuff":     {"lichenloop", "bogdaemon", 22, 38},
			"rootanami":    {"taprouter", "rhizoracle", 24, 40},
			"emberbyte":    {"cinderhash", "flarestack", 15, 31},
			"cindernode":   {"furnacehub", "calderdaemon", 19, 35},
			"scorchip":     {"burnboard", "infernalink", 17, 33},
			"wickware":     {"torchthread", "daemoflare", 21, 37},
			"aquabit":      {"bytefin", "datadeluge", 14, 30},
			"flowcell":     {"wavebank", "tidalarray", 20, 36},
			"gushkit":      {"pipelinx", "torrentiger", 16, 32},
			"mistcache":    {"fogbuffer", "cloudvault", 18, 34},
			"splashscreen": {"cachelotl", "rebootide", 17, 33},
			"zaplet":       {"voltalon", "stormkernel", 15, 31},
			"joulpup":      {"voltweiler", "ampmastiff", 18, 34},
			"amperent":     {"coilobra", "gridaconda", 20, 36},
			"surgetail":    {"stormfin", "tempestray", 22, 38},
			"spamlet":      {"mailgnant", "phishmonger", 16, 32},
			"bloatware":    {"featurmoil", "heapocalypse", 22, 38},
			"wormate":      {"segmaggot", "hexhelminth", 19, 35},
			"chippunk":     {"solderat", "rackoon", 17, 33},
			"coghound":     {"trackbyte", "watchdaemon", 20, 36},
			"servoboar":    {"ramhog", "racktusk", 24, 40},
		}

		total := func(species Species) int {
			stats := species.BaseStats
			return stats.HP + stats.Attack + stats.Defense + stats.SpAttack + stats.Speed
		}
		seen := make(map[string]bool, len(set.Species))
		for baseSlug, expected := range want {
			base := set.Species[baseSlug]
			middle := set.Species[expected.middle]
			final := set.Species[expected.final]
			for _, species := range []Species{base, middle, final} {
				if species.Slug == "" {
					t.Errorf("family %s has a missing species", baseSlug)
					continue
				}
				if seen[species.Slug] {
					t.Errorf("species %s appears in multiple families", species.Slug)
				}
				seen[species.Slug] = true
				if species.Type != base.Type {
					t.Errorf("species %s type = %s, want family type %s", species.Slug, species.Type, base.Type)
				}
			}

			wantMiddle := Evolution{Species: expected.middle, Level: expected.middleLevel}
			if base.EvolvesTo == nil || *base.EvolvesTo != wantMiddle {
				t.Errorf("species %s evolution = %+v, want %+v", baseSlug, base.EvolvesTo, wantMiddle)
			}
			wantFinal := Evolution{Species: expected.final, Level: expected.finalLevel}
			if middle.EvolvesTo == nil || *middle.EvolvesTo != wantFinal {
				t.Errorf("species %s evolution = %+v, want %+v", expected.middle, middle.EvolvesTo, wantFinal)
			}
			if final.EvolvesTo != nil {
				t.Errorf("final species %s unexpectedly evolves to %+v", expected.final, final.EvolvesTo)
			}
			if middleTotal := total(middle); middleTotal != 320 {
				t.Errorf("middle species %s stat total = %d, want 320", middle.Slug, middleTotal)
			}
			if finalTotal := total(final); finalTotal != 400 {
				t.Errorf("final species %s stat total = %d, want 400", final.Slug, finalTotal)
			}
			if total(base) >= total(middle) {
				t.Errorf("base species %s stat total must be below middle species %s", base.Slug, middle.Slug)
			}
		}
		if len(want) != 24 || len(seen) != 72 {
			t.Errorf("got %d complete families covering %d species, want 24 families covering 72 species", len(want), len(seen))
		}
	})

	t.Run("level one and queue eligibility", func(t *testing.T) {
		const queueLevel = 30
		for slug, sp := range set.Species {
			atOne, atQueue := 0, 0
			var queueMax float64
			for _, e := range sp.Movepool {
				mv, ok := set.Moves[e.Move]
				if !ok {
					t.Errorf("%s movepool references unknown move %s", slug, e.Move)
					continue
				}
				if e.Level <= 1 {
					atOne++
				}
				if e.Level <= queueLevel {
					atQueue++
					if mv.Power > queueMax {
						queueMax = mv.Power
					}
				}
			}
			if atOne < 4 {
				t.Errorf("%s has %d Level-1 Moves, want at least 4", slug, atOne)
			}
			if atQueue < 4 {
				t.Errorf("%s has %d Moves eligible at Level %d, want at least 4", slug, atQueue, queueLevel)
			}
			if queueMax < 90 {
				t.Errorf("%s Queue-eligible max power %v, want the Evolution-rung 90", slug, queueMax)
			}
		}
	})
}

func TestValidateEvolutions(t *testing.T) {
	tests := []struct {
		name    string
		species map[string]Species
		wantErr string
	}{
		{
			name: "valid three-stage family",
			species: map[string]Species{
				"one":   {Slug: "one", EvolvesTo: &Evolution{Species: "two", Level: 10}},
				"two":   {Slug: "two", EvolvesTo: &Evolution{Species: "three", Level: 20}},
				"three": {Slug: "three"},
			},
		},
		{
			name: "unknown target",
			species: map[string]Species{
				"one": {Slug: "one", EvolvesTo: &Evolution{Species: "missing", Level: 10}},
			},
			wantErr: "unknown species",
		},
		{
			name: "level below two",
			species: map[string]Species{
				"one": {Slug: "one", EvolvesTo: &Evolution{Species: "two", Level: 1}},
				"two": {Slug: "two"},
			},
			wantErr: "at least 2",
		},
		{
			name: "multiple predecessors",
			species: map[string]Species{
				"one":   {Slug: "one", EvolvesTo: &Evolution{Species: "three", Level: 10}},
				"two":   {Slug: "two", EvolvesTo: &Evolution{Species: "three", Level: 12}},
				"three": {Slug: "three"},
			},
			wantErr: "multiple predecessors",
		},
		{
			name: "cycle",
			species: map[string]Species{
				"one": {Slug: "one", EvolvesTo: &Evolution{Species: "two", Level: 10}},
				"two": {Slug: "two", EvolvesTo: &Evolution{Species: "one", Level: 20}},
			},
			wantErr: "cycle",
		},
		{
			name: "more than three stages",
			species: map[string]Species{
				"one":   {Slug: "one", EvolvesTo: &Evolution{Species: "two", Level: 10}},
				"two":   {Slug: "two", EvolvesTo: &Evolution{Species: "three", Level: 20}},
				"three": {Slug: "three", EvolvesTo: &Evolution{Species: "four", Level: 30}},
				"four":  {Slug: "four"},
			},
			wantErr: "exceeds 3 stages",
		},
		{
			name: "non-increasing thresholds",
			species: map[string]Species{
				"one":   {Slug: "one", EvolvesTo: &Evolution{Species: "two", Level: 20}},
				"two":   {Slug: "two", EvolvesTo: &Evolution{Species: "three", Level: 10}},
				"three": {Slug: "three"},
			},
			wantErr: "must be greater",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEvolutions(tt.species)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateEvolutions() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateEvolutions() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadRejectsBadMovepool(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"types", "moves", "species"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(rel, body string) {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("types/organic.json", `{"slug":"organic","name":"Organic"}`)
	write("types/thermal.json", `{"slug":"thermal","name":"Thermal","matchup":{"organic":2.0}}`)
	write("types/coolant.json", `{"slug":"coolant","name":"Coolant"}`)
	write("moves/root_access.json", `{"slug":"root_access","name":"Root Access","type":"organic","category":"physical","power":45,"accuracy":100}`)
	write("species/rootkit.json", `{"slug":"rootkit","name":"Rootkit","type":"organic","base_stats":{"hp":55,"attack":45,"defense":60,"sp_attack":48,"speed":42},"movepool":[{"move":"root_access","level":1}]}`)

	if _, err := Load(dir); err == nil {
		t.Fatal("expected validation failure for movepool with fewer than 4 moves")
	}
}

func TestLoadRejectsBadArt(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"types", "moves", "species", "art"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(rel, body string) {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("types/a.json", `{"slug":"a","name":"A"}`)
	write("types/b.json", `{"slug":"b","name":"B"}`)
	write("types/c.json", `{"slug":"c","name":"C"}`)
	write("moves/m1.json", `{"slug":"m1","name":"M1","type":"a","category":"physical","power":10,"accuracy":100}`)
	write("moves/m2.json", `{"slug":"m2","name":"M2","type":"b","category":"physical","power":10,"accuracy":100}`)
	write("moves/m3.json", `{"slug":"m3","name":"M3","type":"c","category":"physical","power":10,"accuracy":100}`)
	write("moves/m4.json", `{"slug":"m4","name":"M4","type":"a","category":"special","power":10,"accuracy":100}`)
	write("species/s1.json", `{"slug":"s1","name":"S1","type":"a","base_stats":{"hp":50,"attack":50,"defense":50,"sp_attack":50,"speed":50},"movepool":[{"move":"m1","level":1},{"move":"m2","level":1},{"move":"m3","level":1},{"move":"m4","level":1}]}`)
	write("art/s1.json", `{"slug":"s1","palette":{"o":"#101010"},"grid":["okk"]}`)

	if _, err := Load(dir); err == nil {
		t.Fatal("expected validation failure: grid rune not in palette (need >=3 colors too)")
	}
}

func TestLoadRejectsMissingArt(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"types", "moves", "species", "art"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(rel, body string) {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("types/a.json", `{"slug":"a","name":"A"}`)
	write("types/b.json", `{"slug":"b","name":"B"}`)
	write("types/c.json", `{"slug":"c","name":"C"}`)
	write("moves/m1.json", `{"slug":"m1","name":"M1","type":"a","category":"physical","power":10,"accuracy":100}`)
	write("moves/m2.json", `{"slug":"m2","name":"M2","type":"a","category":"physical","power":10,"accuracy":100}`)
	write("moves/m3.json", `{"slug":"m3","name":"M3","type":"a","category":"physical","power":10,"accuracy":100}`)
	write("moves/m4.json", `{"slug":"m4","name":"M4","type":"a","category":"special","power":10,"accuracy":100}`)
	write("species/s1.json", `{"slug":"s1","name":"S1","type":"a","base_stats":{"hp":50,"attack":50,"defense":50,"sp_attack":50,"speed":50},"movepool":[{"move":"m1","level":1},{"move":"m2","level":1},{"move":"m3","level":1},{"move":"m4","level":1}]}`)
	write("art/unrelated.json", `{"slug":"unrelated","palette":{"o":"#101010","k":"#202020","w":"#303030"},"grid":["okk"]}`)

	if _, err := Load(dir); err == nil {
		t.Fatal("expected validation failure for species without sprite art")
	}
}

func TestLoadRejectsMalformedPacks(t *testing.T) {
	// buildPack writes a minimal otherwise-valid pack into a fresh temp dir
	// and returns its path. Each subtest then corrupts one thing.
	buildPack := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		for _, d := range []string{"types", "moves", "species"} {
			if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		write := func(rel, body string) {
			if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		write("types/organic.json", `{"slug":"organic","name":"Organic"}`)
		write("types/thermal.json", `{"slug":"thermal","name":"Thermal","matchup":{"organic":2.0}}`)
		write("types/coolant.json", `{"slug":"coolant","name":"Coolant"}`)
		for i, name := range []string{"m1", "m2", "m3", "m4"} {
			cat := "physical"
			if i == 3 {
				cat = "special"
			}
			write("moves/"+name+".json",
				`{"slug":"`+name+`","name":"`+name+`","type":"organic","category":"`+cat+`","power":40,"accuracy":100}`)
		}
		write("species/rootkit.json",
			`{"slug":"rootkit","name":"Rootkit","type":"organic","base_stats":{"hp":55,"attack":45,"defense":60,"sp_attack":48,"speed":42},"movepool":[{"move":"m1","level":1},{"move":"m2","level":1},{"move":"m3","level":1},{"move":"m4","level":1}]}`)
		return dir
	}

	t.Run("matchup references unknown type", func(t *testing.T) {
		dir := buildPack(t)
		if err := os.WriteFile(filepath.Join(dir, "types", "thermal.json"),
			[]byte(`{"slug":"thermal","name":"Thermal","matchup":{"themal":2.0}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(dir)
		if err == nil || !strings.Contains(err.Error(), "themal") {
			t.Fatalf("err = %v, want unknown matchup type themal", err)
		}
	})

	t.Run("zero base stat", func(t *testing.T) {
		dir := buildPack(t)
		if err := os.WriteFile(filepath.Join(dir, "species", "rootkit.json"),
			[]byte(`{"slug":"rootkit","name":"Rootkit","type":"organic","base_stats":{"hp":55,"attack":45,"defense":0,"sp_attack":48,"speed":42},"movepool":[{"move":"m1","level":1},{"move":"m2","level":1},{"move":"m3","level":1},{"move":"m4","level":1}]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "base stats") {
			t.Fatalf("err = %v, want base stats refusal", err)
		}
	})

	t.Run("unknown field is rejected", func(t *testing.T) {
		dir := buildPack(t)
		if err := os.WriteFile(filepath.Join(dir, "species", "rootkit.json"),
			[]byte(`{"slug":"rootkit","name":"Rootkit","type":"organic","basestats":{"hp":55},"movepool":[{"move":"m1","level":1}]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("err = %v, want unknown field rejection", err)
		}
	})
}
