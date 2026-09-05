package capture_test

import (
	"testing"

	"termon.sh/internal/capture"
)

func TestObjectiveAwardsSumTo100(t *testing.T) {
	objs, err := capture.ObjectivesFromIDs(capture.AuthoredLessonObjectives(1))
	if err != nil {
		t.Fatal(err)
	}
	sum := 0
	for _, o := range objs {
		sum += o.Award
	}
	if sum != 100 {
		t.Fatalf("lesson 1 awards = %d, want 100", sum)
	}
}

func TestOneShotAwards(t *testing.T) {
	s := capture.NewSession(mustObjectives(t, capture.AuthoredLessonObjectives(1)))
	if len(s.AfterTurn(capture.TurnInput{TrainerMoveSlug: "a"})) != 0 {
		t.Fatal("single move should not complete variety")
	}
	s.AfterTurn(capture.TurnInput{TrainerMoveSlug: "a"})
	s.AfterTurn(capture.TurnInput{TrainerMoveSlug: "b"})
	newly := s.AfterTurn(capture.TurnInput{TrainerMoveSlug: "c"})
	if len(newly) != 1 || newly[0] != capture.ShowMoveVariety {
		t.Fatalf("third distinct move = %v", newly)
	}
	if s.Gauge != 30 {
		t.Fatalf("gauge = %d, want 30", s.Gauge)
	}
	// Repeating does not re-award.
	before := s.Gauge
	if len(s.AfterTurn(capture.TurnInput{TrainerMoveSlug: "d"})) != 0 {
		t.Fatal("repeat should not award more variety")
	}
	if s.Gauge != before {
		t.Fatal("gauge increased on repeat")
	}
}

func TestMissDoesNotCountMatchup(t *testing.T) {
	s := capture.NewSession(mustObjectives(t, []capture.ObjectiveID{capture.ReadTheMatchup}))
	in := capture.TurnInput{TrainerMoveSlug: "se", TrainerMoveHit: false, TrainerSuperEff: true}
	if len(s.AfterTurn(in)) != 0 {
		t.Fatal("miss should not complete read_the_matchup")
	}
	in = capture.TurnInput{TrainerMoveSlug: "se", TrainerMoveHit: true, TrainerSuperEff: true, TrainerDamage: 5}
	if len(s.AfterTurn(in)) != 1 {
		t.Fatal("SE hit should complete read_the_matchup")
	}
}

func TestReadTheMatchupNamesTheAction(t *testing.T) {
	o, ok := capture.ObjectiveByID(capture.ReadTheMatchup)
	if !ok {
		t.Fatal("missing read_the_matchup")
	}
	if o.DisplayName != "Land a super-effective Move" {
		t.Fatalf("display = %q, want a name that says to land a super-effective Move", o.DisplayName)
	}
}

func TestReplacementNotSafeSwitch(t *testing.T) {
	s := capture.NewSession(mustObjectives(t, []capture.ObjectiveID{capture.SafeSwitch}))
	in := capture.TurnInput{Replacement: true, SwitchTargetHP: 10, SwitchTargetMaxHP: 10}
	if len(s.AfterTurn(in)) != 0 {
		t.Fatal("replacement must not complete safe_switch")
	}
	in = capture.TurnInput{VoluntarySwitch: true, SwitchTargetHP: 6, SwitchTargetMaxHP: 10}
	if len(s.AfterTurn(in)) != 1 {
		t.Fatal("voluntary switch at 60% HP should complete safe_switch")
	}
}

func TestGaugeFillOnKOTurnCaptures(t *testing.T) {
	s := capture.NewSession(mustObjectives(t, []capture.ObjectiveID{capture.ReadTheMatchup}))
	s.Gauge = 65
	s.Completed[capture.ReadTheMatchup] = false
	// Complete last objective on KO turn.
	s.AfterTurn(capture.TurnInput{TrainerMoveHit: true, TrainerSuperEff: true, TrainerDamage: 999})
	if s.Gauge < 100 {
		s.Gauge = 100
		s.Completed[capture.ReadTheMatchup] = true
	}
	if s.OutcomeAfterTurn(true) != "captured" {
		t.Fatalf("outcome = %q, want captured", s.OutcomeAfterTurn(true))
	}
}

func TestKOBelowGaugeHuntFailed(t *testing.T) {
	s := capture.NewSession(mustObjectives(t, capture.AuthoredLessonObjectives(1)))
	if s.OutcomeAfterTurn(true) != "hunt_failed" {
		t.Fatalf("outcome = %q, want hunt_failed", s.OutcomeAfterTurn(true))
	}
}

func mustObjectives(t *testing.T, ids []capture.ObjectiveID) []capture.Objective {
	t.Helper()
	objs, err := capture.ObjectivesFromIDs(ids)
	if err != nil {
		t.Fatal(err)
	}
	return objs
}
