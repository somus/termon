package dojo_test

import (
	"testing"

	"termon.sh/internal/battle"
	"termon.sh/internal/dojo"
)

// masterView builds a two-side policy view with synthetic stats so the
// one-turn versus two-turn trade-off is easy to shape. Species stay real so
// movepools and effectiveness resolve against the content pack.
func masterView(foeHP int) battle.PolicyView {
	frail := battle.PolicyMember{
		ID: "frail", Species: "rootkit", Type: "organic", Level: 30,
		HP: 20, MaxHP: 20, Atk: 800, Def: 50, SpA: 420, Spe: 100,
		Active: true, Loadout: []string{"root_access"},
	}
	wall := battle.PolicyMember{
		ID: "wall", Species: "rootkit", Type: "organic", Level: 30,
		HP: 500, MaxHP: 500, Atk: 50, Def: 100, SpA: 50, Spe: 50,
		Loadout: []string{"root_access"},
	}
	foe := battle.PolicyMember{
		ID: "foe", Species: "mistcache", Type: "coolant", Level: 30,
		HP: foeHP, MaxHP: 400, Atk: 200, Def: 50, SpA: 200, Spe: 90,
		Active: true,
	}
	return battle.PolicyView{
		Viewer:    "bot",
		Self:      []battle.PolicyMember{frail, wall},
		FoeActive: foe,
	}
}

// A Master that models the foe's reply must not stay in on a Monster the
// foe's best reply knocks out: chipping the foe low buys a KO next turn,
// but the reply lands first and the switch to the healthy reserve is better.
func TestMasterPolicyAvoidsKOReplyLine(t *testing.T) {
	set := testContentSet(t)
	cfg := dojo.PolicyConfig{Tier: dojo.TierMaster, NearBestBand: 0}

	act, exp, err := dojo.ChoosePolicyAction(set, masterView(400), cfg, battle.Seeded(1))
	if err != nil {
		t.Fatal(err)
	}
	if act.Kind != battle.ActionSwitch || act.SwitchTo != "wall" {
		t.Fatalf("master chose %+v (%s), want switch to wall: it must weigh the foe's KO reply", act, exp.PrimaryReason)
	}
}
