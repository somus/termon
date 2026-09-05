package capture

import (
	"termon.sh/internal/battle"
)

// TurnInput carries resolved-turn facts for objective evaluation.
type TurnInput struct {
	Turn              int
	TrainerMoveSlug   string
	TrainerMoveHit    bool
	TrainerSuperEff   bool
	TrainerDamage     int
	VoluntarySwitch   bool
	SwitchTargetMaxHP int
	SwitchTargetHP    int
	Replacement       bool
	WildActed         bool
	TrainerActiveHP   int
	TrainerActiveMax  int
	WildHP            int
	WildMaxHP         int
}

// Session tracks Capture Gauge progress for one encounter.
type Session struct {
	Objectives []Objective
	Gauge      int
	Completed  map[ObjectiveID]bool

	distinctMoves map[string]struct{}
	pressureTurns int
}

// NewSession starts gauge evaluation for one encounter.
func NewSession(objectives []Objective) *Session {
	completed := make(map[ObjectiveID]bool, len(objectives))
	for _, o := range objectives {
		completed[o.ID] = false
	}
	return &Session{
		Objectives:    objectives,
		Completed:     completed,
		distinctMoves: map[string]struct{}{},
	}
}

// AfterTurn evaluates incomplete objectives from one resolved turn.
func (s *Session) AfterTurn(in TurnInput) []ObjectiveID {
	var newly []ObjectiveID
	for _, obj := range s.Objectives {
		if s.Completed[obj.ID] {
			continue
		}
		if s.tryComplete(obj.ID, in) {
			s.Completed[obj.ID] = true
			s.Gauge = min(100, s.Gauge+obj.Award)
			newly = append(newly, obj.ID)
		}
	}
	return newly
}

func (s *Session) tryComplete(id ObjectiveID, in TurnInput) bool {
	switch id {
	case ShowMoveVariety:
		if in.TrainerMoveSlug != "" {
			s.distinctMoves[in.TrainerMoveSlug] = struct{}{}
		}
		return len(s.distinctMoves) >= 3
	case ReadTheMatchup:
		return in.TrainerMoveHit && in.TrainerSuperEff && in.TrainerDamage > 0
	case SafeSwitch:
		if in.Replacement || !in.VoluntarySwitch {
			return false
		}
		if in.SwitchTargetMaxHP < 1 {
			return false
		}
		return in.SwitchTargetHP*2 >= in.SwitchTargetMaxHP
	case MeasuredPressure:
		if in.TrainerDamage > 0 && in.WildHP*4 > in.WildMaxHP {
			s.pressureTurns++
		}
		return s.pressureTurns >= 2
	case HoldTheLine:
		return in.WildActed && in.TrainerActiveHP*2 > in.TrainerActiveMax
	default:
		return false
	}
}

// Full reports whether gauge reached 100.
func (s *Session) Full() bool {
	return s.Gauge >= 100
}

// OutcomeAfterTurn decides capture vs hunt_failed when the wild may have fainted.
func (s *Session) OutcomeAfterTurn(wildFainted bool) string {
	if s.Full() {
		return "captured"
	}
	if wildFainted {
		return "hunt_failed"
	}
	return ""
}

// BuildTurnInput constructs evaluation input from battle events and snapshot.
func BuildTurnInput(events []battle.Event, turn int, trainer, wildTrainer, trainerMove string, snap battle.Snapshot, wildHP, wildMax int) TurnInput {
	in := TurnInput{Turn: turn, TrainerMoveSlug: trainerMove, WildHP: wildHP, WildMaxHP: wildMax}
	for _, ev := range events {
		if ev.Turn != turn {
			continue
		}
		switch ev.Kind {
		case battle.EventMissed:
			if ev.Actor == trainer {
				in.TrainerMoveHit = false
			}
		case battle.EventSuperEffective:
			if ev.Actor == trainer {
				in.TrainerSuperEff = true
			}
		case battle.EventDamageDealt:
			if ev.Actor == trainer {
				in.TrainerMoveHit = true
				in.TrainerDamage += ev.Damage
			}
		case battle.EventSwitched:
			if ev.Actor == trainer {
				in.VoluntarySwitch = true
			}
		case battle.EventReplacement:
			if ev.Actor == trainer {
				in.Replacement = true
			}
		case battle.EventMoveUsed:
			if ev.Actor == wildTrainer {
				in.WildActed = true
			}
		}
	}
	for _, m := range snap.YourParty {
		if m.Active {
			in.TrainerActiveHP = m.HP
			in.TrainerActiveMax = m.MaxHP
		}
	}
	if in.VoluntarySwitch {
		for _, m := range snap.YourParty {
			if m.Active {
				in.SwitchTargetHP = m.HP
				in.SwitchTargetMaxHP = m.MaxHP
				break
			}
		}
	}
	return in
}
