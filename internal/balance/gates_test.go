package balance_test

import (
	"testing"

	"termon.sh/internal/balance"
	"termon.sh/internal/battle"
)

func TestEvaluateGatesNeutralKOPaceUsesHitsPerFaint(t *testing.T) {
	// Three-Monster fight: 4+4+4 hits, 12 landed total. Old gate failed on
	// the battle total; per-faint median is 4.
	ok := []*balance.BattleOutcome{{
		Reason: battle.EndKO,
		FaintPaces: []balance.FaintPace{
			{Hits: 4}, {Hits: 4}, {Hits: 5}, {Hits: 3}, {Hits: 4}, {Hits: 4},
		},
	}}
	if g := koPaceGate(ok); !g.Passed {
		t.Fatalf("median 4 hits/faint should pass: %+v", g)
	}
	if g := koPaceGate(ok); g.Value != 4 {
		t.Fatalf("value = %v, want median 4", g.Value)
	}
}

func TestEvaluateGatesNeutralKOPaceIgnoresSuperEffectiveAndCritOHKO(t *testing.T) {
	results := []*balance.BattleOutcome{{
		Reason: battle.EndKO,
		FaintPaces: []balance.FaintPace{
			{Hits: 4},
			{Hits: 4},
			{Hits: 4},
			{Hits: 1, SuperEffective: true},
			{Hits: 1, Critical: true},
		},
	}}
	if g := koPaceGate(results); !g.Passed {
		t.Fatalf("SE/crit one-shots must not fail the gate: %+v", g)
	}
}

func TestEvaluateGatesNeutralKOPaceFailsNonCritNeutralOHKO(t *testing.T) {
	results := []*balance.BattleOutcome{{
		Reason: battle.EndKO,
		FaintPaces: []balance.FaintPace{
			{Hits: 4}, {Hits: 4}, {Hits: 1},
		},
	}}
	g := koPaceGate(results)
	if g.Passed {
		t.Fatal("non-crit neutral 1-hit faint should fail")
	}
}

func TestEvaluateGatesNeutralKOPaceFailsMedianOutsideBand(t *testing.T) {
	var paces []balance.FaintPace
	for range 7 {
		paces = append(paces, balance.FaintPace{Hits: 2})
	}
	results := []*balance.BattleOutcome{{Reason: battle.EndKO, FaintPaces: paces}}
	g := koPaceGate(results)
	if g.Passed {
		t.Fatalf("median 2 should fail: %+v", g)
	}
}

func TestCollectFaintPacesCountsHitsUntilFaint(t *testing.T) {
	events := []battle.Event{
		{Kind: battle.EventMoveUsed},
		{Kind: battle.EventDamageDealt, TargetID: "a"},
		{Kind: battle.EventMoveUsed},
		{Kind: battle.EventCriticalHit},
		{Kind: battle.EventDamageDealt, TargetID: "a"},
		{Kind: battle.EventFainted, MonsterID: "a"},
		{Kind: battle.EventMoveUsed},
		{Kind: battle.EventSuperEffective},
		{Kind: battle.EventDamageDealt, TargetID: "b"},
		{Kind: battle.EventFainted, MonsterID: "b"},
		{Kind: battle.EventMoveUsed},
		{Kind: battle.EventDamageDealt, TargetID: "c"},
		{Kind: battle.EventMoveUsed},
		{Kind: battle.EventDamageDealt, TargetID: "c"},
		{Kind: battle.EventMoveUsed},
		{Kind: battle.EventDamageDealt, TargetID: "c"},
		{Kind: battle.EventFainted, MonsterID: "c"},
	}
	got := balance.CollectFaintPaces(events)
	want := []balance.FaintPace{
		{Hits: 2, Critical: true},
		{Hits: 1, SuperEffective: true},
		{Hits: 3},
	}
	if len(got) != len(want) {
		t.Fatalf("paces = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pace[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func koPaceGate(results []*balance.BattleOutcome) balance.GateResult {
	for _, g := range balance.EvaluateGates(results, nil) {
		if g.Name == balance.GateNeutralKOPace {
			return g
		}
	}
	return balance.GateResult{}
}
