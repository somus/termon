package battle

import (
	"math"
	"slices"

	"termon.sh/internal/content"
)

// Forecast is a mean-damage projection, not a sample of future random rolls.
// Equal Speed produces two equally weighted orders. Parties contain only data
// supplied by the policy: its own team and publicly modeled opponent choices.
type Forecast struct {
	Parties [2][]PolicyMember
	Weight  float64
}

// ForecastTurn projects two legal simultaneous actions through switching,
// Speed order and faint cancellation. It leaves forced Replacement to the
// caller's policy and never mutates the supplied parties.
func ForecastTurn(set *content.Set, parties [2][]PolicyMember, actions [2]Action) []Forecast {
	position := Forecast{Parties: [2][]PolicyMember{slices.Clone(parties[0]), slices.Clone(parties[1])}, Weight: 1}
	for side, action := range actions {
		if action.Kind != ActionSwitch {
			continue
		}
		for i := range position.Parties[side] {
			m := &position.Parties[side][i]
			m.Active = m.ID == action.SwitchTo
		}
	}
	a, b := forecastActive(position.Parties[0]), forecastActive(position.Parties[1])
	if a < 0 || b < 0 {
		return []Forecast{position}
	}
	firstSpeed, secondSpeed := position.Parties[0][a].Spe, position.Parties[1][b].Spe
	bothMove := actions[0].Kind == ActionMove && actions[1].Kind == ActionMove
	if bothMove && firstSpeed == secondSpeed {
		first := forecastOrder(set, position, actions, [2]int{0, 1})
		second := forecastOrder(set, position, actions, [2]int{1, 0})
		first.Weight, second.Weight = 0.5, 0.5
		return []Forecast{first, second}
	}
	order := [2]int{0, 1}
	if secondSpeed > firstSpeed {
		order = [2]int{1, 0}
	}
	return []Forecast{forecastOrder(set, position, actions, order)}
}

func forecastOrder(set *content.Set, start Forecast, actions [2]Action, order [2]int) Forecast {
	out := Forecast{Parties: [2][]PolicyMember{slices.Clone(start.Parties[0]), slices.Clone(start.Parties[1])}, Weight: start.Weight}
	for _, side := range order {
		if actions[side].Kind != ActionMove {
			continue
		}
		self, foe := forecastActive(out.Parties[side]), forecastActive(out.Parties[1-side])
		if self < 0 || foe < 0 {
			continue
		}
		atk, def := &out.Parties[side][self], &out.Parties[1-side][foe]
		move := set.Moves[actions[side].Move]
		attack := atk.Atk
		if move.Category == "special" {
			attack = atk.SpA
		}
		base := DamageBase(move.Power, attack, def.Def, move.Type, atk.Type, set.Effectiveness(move.Type, def.Type))
		damage := int(math.Round(ExpectedDamage(base, move.Accuracy)))
		def.HP = max(0, def.HP-damage)
		def.Fainted = def.HP == 0
	}
	return out
}

func forecastActive(party []PolicyMember) int {
	for i, member := range party {
		if member.Active && !member.Fainted && member.HP > 0 {
			return i
		}
	}
	return -1
}
