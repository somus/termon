package dojo_test

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
	"termon.sh/internal/game"
)

func testContentSet(t *testing.T) *content.Set {
	t.Helper()
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestLessonTargetTable(t *testing.T) {
	cases := []struct {
		starter string
		lesson  int
		want    string
	}{
		{"rootkit", 1, "mistcache"},
		{"rootkit", 2, "chippunk"},
		{"emberbyte", 1, "sproutware"},
		{"emberbyte", 2, "zaplet"},
		{"aquabit", 1, "wickware"},
		{"aquabit", 2, "spamlet"},
	}
	for _, tc := range cases {
		got, err := dojo.LessonTarget(tc.starter, tc.lesson)
		if err != nil {
			t.Fatalf("%s/%d: %v", tc.starter, tc.lesson, err)
		}
		if got != tc.want {
			t.Fatalf("%s lesson %d = %q, want %q", tc.starter, tc.lesson, got, tc.want)
		}
	}
}

func TestSparringRosterRotatesDeterministically(t *testing.T) {
	set := testContentSet(t)
	party := []game.Monster{
		{Species: "rootkit", Level: 1},
		{Species: "emberbyte", Level: 1},
		{Species: "aquabit", Level: 1},
	}
	first, err := dojo.BuildSparringRoster(set, party, 100)
	if err != nil {
		t.Fatal(err)
	}
	again, err := dojo.BuildSparringRoster(set, party, 100)
	if err != nil {
		t.Fatal(err)
	}
	next, err := dojo.BuildSparringRoster(set, party, 101)
	if err != nil {
		t.Fatal(err)
	}
	roles := []string{"favorable", "neutral", "unfavorable"}
	playerTypes := []string{"organic", "thermal", "coolant"}
	var firstFamilies, nextFamilies []string
	for i := range first {
		firstFamilies = append(firstFamilies, first[i].Family)
		nextFamilies = append(nextFamilies, next[i].Family)
		if first[i].Family != again[i].Family || first[i].Species != again[i].Species ||
			first[i].Level != again[i].Level || !slices.Equal(first[i].Loadout, again[i].Loadout) {
			t.Fatalf("slot %d changed within one cycle: %+v vs %+v", i+1, first[i], again[i])
		}
		if first[i].Role != roles[i] || first[i].Level != 1 {
			t.Fatalf("slot %d = %+v, want role %s at level 1", i+1, first[i], roles[i])
		}
		if !slices.Contains(dojo.SparringPool[first[i].Type], first[i].Family) {
			t.Fatalf("slot %d family %q is not in the %s pool", i+1, first[i].Family, first[i].Type)
		}
		favorable := set.Effectiveness(first[i].Type, playerTypes[i]) >= battle.SuperEffectiveAt
		unfavorable := set.Effectiveness(playerTypes[i], first[i].Type) >= battle.SuperEffectiveAt
		if i == 0 && !favorable || i == 1 && (favorable || unfavorable) || i == 2 && !unfavorable {
			t.Fatalf("slot %d role %s has opponent type %s against %s", i+1, roles[i], first[i].Type, playerTypes[i])
		}
	}
	if slices.Equal(firstFamilies, nextFamilies) {
		t.Fatalf("consecutive cycles did not rotate roster: %v", firstFamilies)
	}
}

func TestValidateSparringPool(t *testing.T) {
	set := testContentSet(t)
	if err := dojo.ValidateSparringPool(set); err != nil {
		t.Fatal(err)
	}
}

func TestDailyIndexDeterministic(t *testing.T) {
	day := time.Date(2026, 8, 28, 15, 0, 0, 0, time.UTC)
	a := dojo.FixtureForDay(day)
	b := dojo.FixtureForDay(day)
	if a.ID != b.ID || a.Seed != b.Seed {
		t.Fatalf("fixture mismatch: %+v vs %+v", a, b)
	}
}

func TestDailyPartiesSameSeedTeams(t *testing.T) {
	set := testContentSet(t)
	fx := dojo.FixtureForDay(time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC))
	p1, o1, err := dojo.DailyParties(set, fx)
	if err != nil {
		t.Fatal(err)
	}
	p2, o2, err := dojo.DailyParties(set, fx)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if p1.Members[i].Monster.Species != p2.Members[i].Monster.Species {
			t.Fatalf("player slot %d species differ", i)
		}
		if o1.Members[i].Monster.Species != o2.Members[i].Monster.Species {
			t.Fatalf("opp slot %d species differ", i)
		}
	}
}

func TestValidateDailyFixtures(t *testing.T) {
	set := testContentSet(t)
	if err := dojo.ValidateDailyFixtures(set); err != nil {
		t.Fatal(err)
	}
}
