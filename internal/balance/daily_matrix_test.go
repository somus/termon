package balance

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
)

func TestDailyProofLineReplaysExactly(t *testing.T) {
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	fixture := dojo.DailyFixtures[0]
	root, err := replayDaily(set, fixture, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(root.pending) == 0 {
		t.Fatal("Daily root has no public player actions")
	}
	choices := []battle.Action{root.pending[0]}
	first, err := replayDaily(set, fixture, choices)
	if err != nil {
		t.Fatal(err)
	}
	second, err := replayDaily(set, fixture, choices)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.line, second.line) {
		t.Fatalf("replay changed trace:\nfirst %+v\nsecond %+v", first.line, second.line)
	}
}

func TestDailyProofReportsUnprovenImpossiblePar(t *testing.T) {
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	fixture := dojo.DailyFixtures[0]
	fixture.ID = "unproven-par"
	fixture.Par = 0
	if _, err := findDailyProof(set, fixture, true); err == nil {
		t.Fatal("impossible zero-turn par was reported as proven")
	}
}

func TestRunDailyMatrixCoversEveryFixture(t *testing.T) {
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	matrix, err := RunDailyMatrix(set)
	if err != nil || !matrix.Passed {
		t.Fatalf("Daily proof matrix failed: %v", err)
	}
	if len(matrix.Fixtures) != len(dojo.DailyFixtures) {
		t.Fatalf("fixtures = %d, want %d", len(matrix.Fixtures), len(dojo.DailyFixtures))
	}
	withinPar, overPar := 0, 0
	for _, fixture := range matrix.Fixtures {
		if fixture.WithinPar == nil || fixture.OverPar == nil || fixture.Failure != "" {
			t.Fatalf("%s lacks both required proofs: %s", fixture.ID, fixture.Failure)
		}
		if fixture.WithinPar != nil {
			withinPar++
		}
		if fixture.OverPar != nil {
			overPar++
		}
		if fixture.Failure != "" {
			t.Logf("%s: %s", fixture.ID, fixture.Failure)
		}
	}
	t.Logf("within-par proofs: %d/%d; over-par wins: %d/%d", withinPar, len(matrix.Fixtures), overPar, len(matrix.Fixtures))
}

func TestRecordedDailyProofsRemainValid(t *testing.T) {
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	var lines []recordedDailyLine
	if err := json.Unmarshal(dailyProofData, &lines); err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		found := false
		for _, fixture := range dojo.DailyFixtures {
			if fixture.ID != line.ID {
				continue
			}
			found = true
			replay, err := replayDaily(set, fixture, line.Choices)
			if err != nil || fixture.Seed != line.Seed || !dailyLineMatches(replay, line.WithinPar) {
				t.Fatalf("%s within=%t recorded proof failed: %v", line.ID, line.WithinPar, err)
			}
		}
		if !found {
			t.Fatalf("unknown recorded fixture %s", line.ID)
		}
	}
}

func TestDailyOverParProofRequiresObjectiveClear(t *testing.T) {
	replay := dailyReplay{terminal: true, playerWon: true, parMet: false}
	if dailyLineMatches(replay, false) {
		t.Fatal("battle win without objective cannot isolate a missed par")
	}
	replay.objectiveMet = true
	if !dailyLineMatches(replay, false) {
		t.Fatal("objective win over par should prove missed mastery")
	}
}

func TestDailyOverParSearchRanksObjectiveProgress(t *testing.T) {
	fixture := dojo.DailyFixture{Objective: "full_rotation", Par: 12}
	without := dailyReplay{tracker: &dojo.DailyTracker{MonTurn: map[string]bool{"first": true}}}
	with := dailyReplay{tracker: &dojo.DailyTracker{MonTurn: map[string]bool{"first": true, "second": true}}}
	for _, turns := range []int{5, 13} {
		without.line.Turns = turns
		with.line.Turns = turns
		if dailySearchScore(with, fixture, false) <= dailySearchScore(without, fixture, false) {
			t.Fatalf("turn %d: over-par search must prefer objective progress when other state is equal", turns)
		}
	}
}
