package capture

import (
	"testing"

	"termon.sh/internal/battle"
)

func TestBuildTurnInputReadsMatchupFromEvents(t *testing.T) {
	events := []battle.Event{
		{Turn: 1, Actor: "you", Kind: battle.EventMoveUsed, Text: "Rootkit used Root Access!"},
		{Turn: 1, Actor: "you", Kind: battle.EventSuperEffective, Text: "It's super effective!"},
		{Turn: 1, Actor: "you", Kind: battle.EventDamageDealt, Damage: 28, Text: "Mistcache took 28 damage."},
		{Turn: 1, Actor: "wild", Kind: battle.EventMoveUsed, Text: "Mistcache used Datastream!"},
	}
	snap := battle.Snapshot{YourParty: []battle.SnapshotMember{{
		Active: true, HP: 45, MaxHP: 55,
	}}}
	in := BuildTurnInput(events, 1, "you", "wild", "root_access", snap, 120, 150)
	if !in.TrainerMoveHit || !in.TrainerSuperEff {
		t.Fatalf("hit=%v se=%v", in.TrainerMoveHit, in.TrainerSuperEff)
	}
	if in.TrainerDamage != 28 {
		t.Fatalf("damage = %d, want 28", in.TrainerDamage)
	}
	if !in.WildActed {
		t.Fatal("wild should have acted")
	}
}

func TestBuildTurnInputDamageSurvivesNicknamesWithSpaces(t *testing.T) {
	events := []battle.Event{
		{Turn: 1, Actor: "you", Kind: battle.EventMoveUsed, Text: "Rootkit used Root Access!"},
		{Turn: 1, Actor: "you", Kind: battle.EventDamageDealt, Damage: 28, Text: "Hot Dog took 28 damage."},
	}
	in := BuildTurnInput(events, 1, "you", "wild", "root_access", battle.Snapshot{}, 120, 150)
	if in.TrainerDamage != 28 {
		t.Fatalf("damage = %d, want 28 despite target nickname with spaces", in.TrainerDamage)
	}
}

func TestBuildTurnInputSwitchTargetIsTheMonsterSwitchedIn(t *testing.T) {
	events := []battle.Event{
		{Turn: 1, Actor: "you", Kind: battle.EventSwitched, Text: "you switched to Mistcache!"},
	}
	snap := battle.Snapshot{YourParty: []battle.SnapshotMember{
		{Species: "rootkit", Active: false, HP: 40, MaxHP: 55},
		{Species: "mistcache", Active: true, HP: 48, MaxHP: 48},
	}}
	in := BuildTurnInput(events, 1, "you", "wild", "", snap, 120, 150)
	if !in.VoluntarySwitch {
		t.Fatal("expected voluntary switch")
	}
	if in.SwitchTargetHP != 48 || in.SwitchTargetMaxHP != 48 {
		t.Fatalf("switch target hp=%d/%d, want active reserve 48/48", in.SwitchTargetHP, in.SwitchTargetMaxHP)
	}
}
