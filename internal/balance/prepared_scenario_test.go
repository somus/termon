package balance

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"termon.sh/internal/battle"

	"termon.sh/internal/content"
)

func TestPreparedScenarioMatchesRunScenario(t *testing.T) {
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Set: set, Policy: DefaultPolicy(), MaxTurns: DefaultMaxTurns}
	cases := []Scenario{
		NormalizedScenario("normalized", ReferenceTeams[0], ReferenceTeams[1], 2),
		NaturalScenario("natural", ReferenceTeams[0], ReferenceTeams[1], 2, 0, 30),
	}
	for _, sc := range cases {
		sc.EngineSideA, sc.PartyOrderSwapped = false, true
		prepared, err := prepareScenario(cfg, sc)
		if err != nil {
			t.Fatal(err)
		}
		before, _ := json.Marshal([]battle.Party{prepared.partyA, prepared.partyB})
		for _, seed := range []uint64{1, 99} {
			sc.Seed = seed
			want, wantErr := RunScenario(cfg, sc)
			got, gotErr := runPreparedScenario(cfg, sc, prepared)
			if fmt.Sprint(wantErr) != fmt.Sprint(gotErr) {
				t.Fatalf("%s seed %d errors: %v %v", sc.Kind, seed, wantErr, gotErr)
			}
			wantJSON, _ := json.Marshal(want)
			gotJSON, _ := json.Marshal(got)
			if string(wantJSON) != string(gotJSON) {
				t.Fatalf("%s seed %d outcome drift", sc.Kind, seed)
			}
		}
		after, _ := json.Marshal([]battle.Party{prepared.partyA, prepared.partyB})
		if string(before) != string(after) {
			t.Fatalf("%s prepared party mutated", sc.Kind)
		}
	}
}

func TestPreparedScenarioMatchesTurnCapFailure(t *testing.T) {
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Set: set, Policy: DefaultPolicy(), MaxTurns: 1}
	sc := NormalizedScenario("cap", ReferenceTeams[0], ReferenceTeams[1], 0)
	sc.Seed = 7
	prepared, err := prepareScenario(cfg, sc)
	if err != nil {
		t.Fatal(err)
	}
	want, wantErr := RunScenario(cfg, sc)
	got, gotErr := runPreparedScenario(cfg, sc, prepared)
	if wantErr == nil || gotErr == nil || fmt.Sprint(wantErr) != fmt.Sprint(gotErr) {
		t.Fatal("expected turn-cap errors")
	}
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(got)
	if string(wantJSON) != string(gotJSON) {
		t.Fatal("turn-cap replay drift")
	}
}
