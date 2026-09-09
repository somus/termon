package dojo_test

import (
	"testing"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
)

func TestChooseReferenceActionRejectsUnknownPolicy(t *testing.T) {
	_, _, err := dojo.ChooseReferenceAction(testContentSet(t), masterView(400), "unknown", battle.Seeded(1))
	if err == nil {
		t.Fatal("unknown reference policy did not fail")
	}
}

func TestChooseReferenceActionSupportsPublishedPolicies(t *testing.T) {
	for _, name := range []string{dojo.ReferencePressure, dojo.ReferencePivot, dojo.ReferencePreservation} {
		t.Run(name, func(t *testing.T) {
			act, explanation, err := dojo.ChooseReferenceAction(testContentSet(t), masterView(400), name, battle.Seeded(1))
			if err != nil {
				t.Fatal(err)
			}
			if act.Kind == "" {
				t.Fatal("empty action")
			}
			if len(explanation.Considered) == 0 {
				t.Fatal("missing considered actions")
			}
			if name == dojo.ReferencePreservation && explanation.Considered[0].KOProbability == nil {
				t.Fatal("missing preservation tuple")
			}
		})
	}
}

func TestPreservationAttacksPredictedSwitch(t *testing.T) {
	set := testContentSet(t)
	set.Moves["probe_hit"] = content.Move{Slug: "probe_hit", Type: "organic", Category: "physical", Power: 100, Accuracy: 100}
	set.Moves["probe_risky"] = content.Move{Slug: "probe_risky", Type: "organic", Category: "physical", Power: 100, Accuracy: 50}
	view := battle.PolicyView{
		Self: []battle.PolicyMember{
			{ID: "active", Type: "organic", HP: 10, MaxHP: 10, Atk: 100, Def: 10, Spe: 10, Active: true, Loadout: []string{"probe_hit"}, PublicMovepool: []string{"probe_hit"}},
			{ID: "reserve", Type: "organic", HP: 1000, MaxHP: 1000, Atk: 10, Def: 1000, Spe: 1, Loadout: []string{"probe_hit"}, PublicMovepool: []string{"probe_hit"}},
		},
		FoeActive: battle.PolicyFoe{ID: "foe", Type: "organic", HP: 10, MaxHP: 10, Atk: 100, Def: 10, Spe: 20, Active: true, PublicMovepool: []string{"probe_risky"}},
	}
	view.FoeRoster = []battle.PolicyFoe{view.FoeActive, {ID: "foe-reserve", Type: "organic", MaxHP: 1000, Atk: 10, Def: 1000, Spe: 1, PublicMovepool: []string{"probe_risky"}}}
	action, explanation, err := dojo.ChooseReferenceAction(set, view, dojo.ReferencePreservation, battle.Seeded(1))
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != battle.ActionMove || action.Move != "probe_hit" {
		t.Fatalf("predicted safe opposing switch should permit pressure: %+v", action)
	}
	for _, score := range explanation.Considered {
		if score.Kind == battle.ActionMove && (score.KOProbability == nil || *score.KOProbability != 0 || score.ExpectedHPLoss == nil || *score.ExpectedHPLoss <= 0) {
			t.Fatalf("attack against switched target has wrong score: %+v", score)
		}
	}
}
