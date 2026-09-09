package battle

import (
	"reflect"
	"testing"
)

func forecastFixture() [2][]PolicyMember {
	return [2][]PolicyMember{
		{{ID: "a", Species: "spark", Type: "thermal", HP: 10, MaxHP: 10, Atk: 500, SpA: 500, Def: 50, Spe: 10, Active: true}},
		{{ID: "b", Species: "spark", Type: "thermal", HP: 10, MaxHP: 10, Atk: 500, SpA: 500, Def: 50, Spe: 100, Active: true}},
	}
}

func TestForecastTurnCancelsSlowerKOAttack(t *testing.T) {
	parties := forecastFixture()
	got := ForecastTurn(testSet(), parties, [2]Action{moveAct("jab"), moveAct("jab")})
	if len(got) != 1 || got[0].Parties[0][0].HP != 0 || got[0].Parties[1][0].HP != 10 {
		t.Fatalf("slower lethal attack must be cancelled: %+v", got)
	}
	if !reflect.DeepEqual(parties, forecastFixture()) {
		t.Fatal("projection mutated input")
	}
}

func TestForecastTurnAveragesSpeedTieOrders(t *testing.T) {
	parties := forecastFixture()
	parties[0][0].Spe = 100
	got := ForecastTurn(testSet(), parties, [2]Action{moveAct("jab"), moveAct("jab")})
	if len(got) != 2 || got[0].Weight != 0.5 || got[1].Weight != 0.5 {
		t.Fatalf("tie must retain both orders: %+v", got)
	}
	if got[0].Parties[1][0].HP != 0 || got[1].Parties[0][0].HP != 0 {
		t.Fatalf("tie winners must differ: %+v", got)
	}
}

func TestForecastTurnSwitchesBeforeAttack(t *testing.T) {
	parties := forecastFixture()
	parties[0] = append(parties[0], PolicyMember{
		ID: "reserve", Species: "tank", Type: "organic", HP: 500, MaxHP: 500,
		Atk: 50, Def: 500, SpA: 50, Spe: 1,
	})
	got := ForecastTurn(testSet(), parties, [2]Action{switchAct("reserve"), moveAct("jab")})[0]
	if got.Parties[0][0].HP != 10 || got.Parties[0][0].Active || !got.Parties[0][1].Active {
		t.Fatalf("wrong switch projection: %+v", got.Parties[0])
	}
	if got.Parties[0][1].HP >= 500 || got.Parties[0][1].HP <= 0 {
		t.Fatalf("incoming reserve should take the hit: %+v", got.Parties[0][1])
	}
}
