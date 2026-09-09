package balance

import (
	"path/filepath"
	"reflect"
	"testing"

	"termon.sh/internal/battle"
	"termon.sh/internal/capture"
	"termon.sh/internal/content"
)

func TestCaptureMatrixCaseIsDeterministic(t *testing.T) {
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	first := runCaptureCase(set, ReferenceTeams[0], NaturalCheckpoints[0], "rootkit", "min_variance")
	second := runCaptureCase(set, ReferenceTeams[0], NaturalCheckpoints[0], "rootkit", "min_variance")
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("capture case is not deterministic:\nfirst:  %+v\nsecond: %+v", first, second)
	}
}

func TestCaptureMatrixMissRequiresRecoveredCapture(t *testing.T) {
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	result := runCaptureCase(set, ReferenceTeams[0], NaturalCheckpoints[0], "rootkit", "miss")
	if result.Passed && (trainerMisses(result.Events) != 1 || result.Outcome != "captured") {
		t.Fatalf("miss case passed without exactly one recovered miss: %+v", result)
	}
}

func TestCaptureMatrixDoesNotInventWildFaint(t *testing.T) {
	session := capture.NewSession(nil)
	if got := session.OutcomeAfterTurn(false); got != "" {
		t.Fatalf("OutcomeAfterTurn(false) = %q, want no target outcome", got)
	}
}

func TestCaptureMatrixVarianceLabelsUseProvenQueue(t *testing.T) {
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	result := runCaptureCase(set, ReferenceTeams[0], NaturalCheckpoints[0], "rootkit", "min_variance")
	if result.Passed && hasCritical(result.Events) {
		t.Fatalf("variance case passed with a critical hit: %+v", result)
	}
}

func TestCaptureTurnRollsAccountsForSpeedTieAndMiss(t *testing.T) {
	move := battle.Action{Kind: battle.ActionMove, Move: "m"}
	rolls := captureTurnRolls(move, move, 10, 10, "miss", 1, 1)
	if len(rolls) != 5 || rolls[0] != 0 || rolls[1] < 0.99 {
		t.Fatalf("tie miss rolls = %v, want tie then trainer miss and wild hit rolls", rolls)
	}
}

func TestTrainerMissesCountsOnlyTrainer(t *testing.T) {
	events := []battle.Event{{Kind: battle.EventMissed, Actor: captureWild}, {Kind: battle.EventMissed, Actor: captureTrainer}}
	if got := trainerMisses(events); got != 1 {
		t.Fatalf("trainerMisses() = %d, want 1", got)
	}
}

func TestCaptureMatrixPrioritizesCaptureBeforeKO(t *testing.T) {
	session := capture.NewSession([]capture.Objective{{ID: capture.ReadTheMatchup, Award: 100}})
	session.AfterTurn(capture.TurnInput{TrainerMoveHit: true, TrainerSuperEff: true, TrainerDamage: 1})
	if outcome := session.OutcomeAfterTurn(true); outcome != "captured" {
		t.Fatalf("OutcomeAfterTurn(true) = %q, want captured when the Gauge fills on the KO turn", outcome)
	}
}

func TestPendingObjectiveRequiresKnownObjective(t *testing.T) {
	session := capture.NewSession([]capture.Objective{{ID: capture.ReadTheMatchup}})
	if pendingObjective(session, capture.SafeSwitch) {
		t.Fatal("absent objective is pending")
	}
	if !pendingObjective(session, capture.ReadTheMatchup) {
		t.Fatal("known objective is not pending")
	}
	session.Completed[capture.ReadTheMatchup] = true
	if pendingObjective(session, capture.ReadTheMatchup) {
		t.Fatal("completed objective remains pending")
	}
}

func TestCaptureMatrixMissHasOneMissOrExplainsFailure(t *testing.T) {
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	result := runCaptureCase(set, ReferenceTeams[0], NaturalCheckpoints[0], "rootkit", "miss")
	if trainerMisses(result.Events) == 1 {
		return
	}
	if result.Failure == "" {
		t.Fatalf("miss result has %d misses without explanation: %+v", trainerMisses(result.Events), result)
	}
}

func TestCapturePlannerSwitchesToCompletePublicMatchup(t *testing.T) {
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	snap := battle.Snapshot{
		YourParty: []battle.SnapshotMember{
			{ID: "root", Species: "rootkit", Active: true, HP: 80, MaxHP: 80, Loadout: []string{"root_pulse", "bark_bash", "sudo_surge", "branch_breach"}},
			{ID: "ember", Species: "emberbyte", HP: 80, MaxHP: 80, Loadout: []string{"burn_in"}},
		},
		FoeRoster: []battle.SnapshotFoe{{Species: "rootkit", Active: true, HP: 100, MaxHP: 100}},
	}
	session := capture.NewSession([]capture.Objective{{ID: capture.ReadTheMatchup, Award: 35}})
	action, _ := captureAction(set, snap, session, map[string]bool{}, "min_variance", 0, false)
	if action.Kind != battle.ActionSwitch || action.SwitchTo != "ember" {
		t.Fatalf("matchup planner action = %+v", action)
	}
}
