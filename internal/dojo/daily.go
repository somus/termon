package dojo

import (
	"fmt"
	"time"

	"termon.sh/internal/battle"
	"termon.sh/internal/content"
	"termon.sh/internal/game"
)

const dailyLevel = 20

// DailyFixture is one authored Daily Challenge puzzle.
type DailyFixture struct {
	ID           string
	PlayerLead   []string // family bases in lead order
	OpponentLead []string
	Objective    string
	Par          int
	PolicyTier   string
	Seed         uint64
}

// DailyFixtures is the seven-day cycle from dojo-policy.md.
var DailyFixtures = []DailyFixture{
	{ID: "type_read", PlayerLead: []string{"emberbyte", "rootkit", "aquabit"}, OpponentLead: []string{"mossmuff", "bloatware", "servoboar"}, Objective: "type_read", Par: 10, PolicyTier: TierRival, Seed: 55001},
	{ID: "safe_switch", PlayerLead: []string{"rootkit", "aquabit", "emberbyte"}, OpponentLead: []string{"emberbyte", "cindernode", "scorchip"}, Objective: "safe_switch", Par: 10, PolicyTier: TierRival, Seed: 55002},
	{ID: "full_rotation", PlayerLead: []string{"thornpatch", "gushkit", "joulpup"}, OpponentLead: []string{"flowcell", "amperent", "bloatware"}, Objective: "full_rotation", Par: 12, PolicyTier: TierRival, Seed: 55003},
	{ID: "tempo", PlayerLead: []string{"scorchip", "wickware", "zaplet"}, OpponentLead: []string{"mossmuff", "bloatware", "servoboar"}, Objective: "tempo", Par: 8, PolicyTier: TierRival, Seed: 55004},
	{ID: "preservation", PlayerLead: []string{"rootanami", "flowcell", "thornpatch"}, OpponentLead: []string{"gushkit", "joulpup", "sproutware"}, Objective: "preservation", Par: 10, PolicyTier: TierRival, Seed: 55005},
	{ID: "limited_toolkit", PlayerLead: []string{"chippunk", "spamlet", "mistcache"}, OpponentLead: []string{"wormate", "cindernode", "rootkit"}, Objective: "limited_toolkit", Par: 12, PolicyTier: TierRival, Seed: 55006},
	{ID: "master_trial", PlayerLead: []string{"emberbyte", "aquabit", "rootkit"}, OpponentLead: []string{"thornpatch", "scorchip", "flowcell"}, Objective: "master_trial", Par: 14, PolicyTier: TierMaster, Seed: 55007},
}

// FixtureForDay returns the fixture for a Server Day UTC date.
func FixtureForDay(t time.Time) DailyFixture {
	return DailyForIndex(DailyIndex(t))
}

// DailyForIndex returns the fixture for cycle index 0-6.
func DailyForIndex(index int) DailyFixture {
	if index < 0 {
		index = -index
	}
	return DailyFixtures[index%len(DailyFixtures)]
}

// ValidateDailyFixtures ensures authored species exist.
func ValidateDailyFixtures(set *content.Set) error {
	seen := map[string]struct{}{}
	for _, fx := range DailyFixtures {
		if _, dup := seen[fx.ID]; dup {
			return fmt.Errorf("dojo: duplicate daily id %q", fx.ID)
		}
		seen[fx.ID] = struct{}{}
		for _, slug := range append(append([]string{}, fx.PlayerLead...), fx.OpponentLead...) {
			species, err := DailySpeciesAtLevel(set, slug, dailyLevel)
			if err != nil {
				return err
			}
			if _, ok := set.Species[species]; !ok {
				return fmt.Errorf("dojo: daily %s unknown species %q", fx.ID, species)
			}
		}
	}
	return nil
}

// DailyParties builds loaned player and opponent battle parties for a fixture.
func DailyParties(set *content.Set, fx DailyFixture) (player, opponent battle.Party, err error) {
	player, err = dailySide(set, "daily:player", fx.PlayerLead, fx.Objective == "limited_toolkit")
	if err != nil {
		return battle.Party{}, battle.Party{}, err
	}
	opponent, err = dailySide(set, BotTrainer, fx.OpponentLead, false)
	return player, opponent, err
}

func dailySide(set *content.Set, trainer string, families []string, limitedToolkit bool) (battle.Party, error) {
	p := battle.Party{Trainer: trainer}
	for i, fam := range families {
		species, err := DailySpeciesAtLevel(set, fam, dailyLevel)
		if err != nil {
			return battle.Party{}, err
		}
		loadout, err := ReferenceLoadout(set, species, dailyLevel)
		if err != nil {
			return battle.Party{}, err
		}
		if limitedToolkit {
			loadout, err = FilterLoadoutMaxPower(set, loadout, 65)
			if err != nil {
				return battle.Party{}, err
			}
		}
		id := fmt.Sprintf("%s-%d", trainer, i+1)
		p.Members = append(p.Members, battle.PartyMember{Monster: game.Monster{
			ID: id, Species: species, Level: dailyLevel, BattleLoadout: loadout,
		}})
	}
	return p, nil
}

// DailyTracker records objective progress during a Daily attempt.
type DailyTracker struct {
	Fixture    DailyFixture
	SEMove     bool
	SafeSwitch bool
	MonTurn    map[string]bool
	UsedOver65 bool
}

// NewDailyTracker starts tracking for fixture fx.
func NewDailyTracker(fx DailyFixture) *DailyTracker {
	return &DailyTracker{Fixture: fx, MonTurn: map[string]bool{}}
}

// ObserveTurn updates tracker state from resolved turn events.
func (d *DailyTracker) ObserveTurn(set *content.Set, events []battle.Event, turn int, trainer, trainerMove string, snap battle.Snapshot) {
	if d == nil {
		return
	}
	for _, ev := range events {
		if ev.Turn != turn {
			continue
		}
		switch ev.Kind {
		case battle.EventMoveUsed:
			if ev.Actor != trainer {
				continue
			}
			d.MonTurn[ev.MonsterID] = true
			if trainerMove != "" {
				if mv, ok := set.Moves[trainerMove]; ok && set.Effectiveness(mv.Type, foeType(set, snap)) >= battle.SuperEffectiveAt {
					d.SEMove = true
				}
				if mv, ok := set.Moves[trainerMove]; ok && mv.Power > 65 && d.Fixture.Objective == "limited_toolkit" {
					d.UsedOver65 = true
				}
			}
		case battle.EventSwitched:
			if ev.Actor != trainer {
				continue
			}
			d.MonTurn[ev.MonsterID] = true
		}
	}
}

func foeType(set *content.Set, snap battle.Snapshot) string {
	for _, f := range snap.FoeRoster {
		if f.Active {
			if sp, ok := set.Species[f.Species]; ok {
				return sp.Type
			}
		}
	}
	return ""
}

// ObjectiveMet reports whether the Daily objective is satisfied on a win.
func (d *DailyTracker) ObjectiveMet(_ *content.Set, won bool, _ int, snap battle.Snapshot) bool {
	if !won || d == nil || d.UsedOver65 {
		return false
	}
	switch d.Fixture.Objective {
	case "tempo":
		return true
	case "type_read":
		return d.SEMove
	case "safe_switch":
		return d.SafeSwitch
	case "full_rotation":
		for _, m := range snap.YourParty {
			if !d.MonTurn[m.ID] {
				return false
			}
		}
		return true
	case "preservation":
		healthy := 0
		for _, m := range snap.YourParty {
			if !m.Fainted && m.HP > 0 {
				healthy++
			}
		}
		return healthy >= 2
	case "limited_toolkit":
		return !d.UsedOver65
	case "master_trial":
		if !d.SEMove || !d.SafeSwitch {
			return false
		}
		for _, m := range snap.YourParty {
			if !d.MonTurn[m.ID] {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// ParMet reports whether turn par was achieved.
func (d *DailyTracker) ParMet(won bool, turns int) bool {
	if !won || d == nil {
		return false
	}
	return turns <= d.Fixture.Par
}

// RecordSafeSwitch marks a voluntary switch from disadvantage to safe reserve.
func (d *DailyTracker) RecordSafeSwitch(set *content.Set, fromType, toType, foeType string) {
	if d == nil {
		return
	}
	if set.Effectiveness(foeType, fromType) >= battle.SuperEffectiveAt &&
		set.Effectiveness(foeType, toType) < battle.SuperEffectiveAt {
		d.SafeSwitch = true
	}
}
