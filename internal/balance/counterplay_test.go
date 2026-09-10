package balance

import (
	"path/filepath"
	"reflect"
	"testing"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
)

func TestCounterplayCaseIdentityCoversEveryAnchorFamily(t *testing.T) {
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	cases, err := buildCounterplayCases(set)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 24 {
		t.Fatalf("cases = %d, want 24", len(cases))
	}
	seen := map[string]bool{}
	for _, fixture := range cases {
		if seen[fixture.family] {
			t.Fatalf("duplicate family %q", fixture.family)
		}
		seen[fixture.family] = true
		if len(fixture.player.Members) != 3 || len(fixture.foe.Members) != 3 {
			t.Fatalf("%s did not retain three-Monster actual parties", fixture.family)
		}
		lead := fixture.player.Members[0]
		foe := fixture.foe.Members[0]
		if set.Effectiveness(set.Species[foe.Monster.Species].Type, set.Species[lead.Monster.Species].Type) < battle.SuperEffectiveAt {
			t.Fatalf("%s lead is not Type-disadvantaged against %s", fixture.family, foe.Monster.Species)
		}
		for _, reserve := range fixture.player.Members[1:] {
			if set.Effectiveness(set.Species[foe.Monster.Species].Type, set.Species[reserve.Monster.Species].Type) >= battle.SuperEffectiveAt {
				t.Fatalf("%s reserve %s remains Type-disadvantaged", fixture.family, reserve.Monster.Species)
			}
		}
		firstReserve := fixture.player.Members[1]
		if set.Effectiveness(set.Species[firstReserve.Monster.Species].Type, set.Species[foe.Monster.Species].Type) < battle.SuperEffectiveAt {
			t.Fatalf("%s first reserve %s is not favorable against %s", fixture.family, firstReserve.Monster.Species, foe.Monster.Species)
		}
	}
}

func TestCounterplayReducedCorpusIsDeterministic(t *testing.T) {
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := RunCounterplay(set, []uint64{31})
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunCounterplay(set, []uint64{31})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("reduced counterplay corpus is not deterministic")
	}
	if len(first.Cases) != 24 {
		t.Fatalf("matrix cases = %d, want 24", len(first.Cases))
	}
}

func TestCounterplayOpeningSwitchTakesIncomingHit(t *testing.T) {
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	fixtures, err := buildCounterplayCases(set)
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		switchTo := fixture.player.Members[1].Monster.ID
		trace, _, err := runCounterplayBattle(set, fixture, dojo.ReferencePressure, 31, true, battle.Action{Kind: battle.ActionSwitch, SwitchTo: switchTo})
		if err != nil {
			continue
		}
		for _, event := range trace.Events {
			if event.Turn == 1 && event.Kind == battle.EventDamageDealt && event.TargetID == switchTo {
				return
			}
		}
	}
	t.Fatal("no authored counterplay fixture dealt first-turn damage to the switched-in reserve")
}
