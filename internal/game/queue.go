package game

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"termon.sh/internal/content"
)

// Queue normalization constants (docs/design/matchmaking.md).
const (
	// QueueLevel is the normalized level for ranked PvP copies.
	QueueLevel = 30
	// QueueStatBudget is the total stat points after queue normalization.
	QueueStatBudget = 320
	// unlockLevelUnknown stands in for a Move with no Family movepool entry;
	// it sorts last and never passes the QueueLevel filter.
	unlockLevelUnknown = 99
)

// ErrPartialParty means fewer than three battle-ready Party slots are filled.
var ErrPartialParty = errors.New("game: partial party")

// ErrInvalidQueueLoadout means a Queue Move Set is not legal.
var ErrInvalidQueueLoadout = errors.New("game: invalid queue move set")

// FullParty reports whether all three Party slots are occupied with loadouts.
func FullParty(save *Save) bool {
	if save == nil {
		return false
	}
	for _, id := range save.Party {
		if id == "" {
			return false
		}
		m, ok := MonsterByID(save, id)
		if !ok || len(m.BattleLoadout) < 1 {
			return false
		}
	}
	return true
}

// RequireFullParty returns ErrPartialParty when Queue or Challenge is unavailable.
func RequireFullParty(save *Save) error {
	if !FullParty(save) {
		return ErrPartialParty
	}
	return nil
}

// QueueMovePool returns eligible move slugs for normalized PvP editing.
func QueueMovePool(set *content.Set, m Monster) ([]string, error) {
	if set == nil {
		return nil, errors.New("game: nil content")
	}
	sp, ok := set.Species[m.Species]
	if !ok {
		return nil, fmt.Errorf("game: unknown species %q", m.Species)
	}
	seen := map[string]int{}
	for _, entry := range sp.Movepool {
		if entry.Level <= QueueLevel {
			seen[entry.Move] = entry.Level
		}
	}
	for _, slug := range m.MoveLibrary {
		if _, ok := seen[slug]; ok {
			continue
		}
		lv := moveUnlockLevel(set, m.Species, slug)
		if lv <= QueueLevel {
			seen[slug] = lv
		}
	}
	out := make([]string, 0, len(seen))
	for slug := range seen {
		out = append(out, slug)
	}
	orderMovesByUnlock(out, seen)
	return out, nil
}

func moveUnlockLevel(set *content.Set, species, slug string) int {
	sp, ok := set.Species[species]
	if !ok {
		return unlockLevelUnknown
	}
	for _, e := range sp.Movepool {
		if e.Move == slug {
			return e.Level
		}
	}
	return unlockLevelUnknown
}

// orderMovesByUnlock orders Move slugs by unlock level ascending, ties by
// slug, so a normalized loadout is reproducible for the same input.
func orderMovesByUnlock(out []string, unlockLevel map[string]int) {
	slices.SortFunc(out, func(a, b string) int {
		if unlockLevel[a] != unlockLevel[b] {
			return unlockLevel[a] - unlockLevel[b]
		}
		return strings.Compare(a, b)
	})
}

// DefaultQueueMoveSet picks the first four eligible pool moves.
func DefaultQueueMoveSet(set *content.Set, m Monster) ([]string, error) {
	pool, err := QueueMovePool(set, m)
	if err != nil {
		return nil, err
	}
	if len(pool) < 1 {
		return nil, ErrInvalidQueueLoadout
	}
	n := min(4, len(pool))
	return append([]string(nil), pool[:n]...), nil
}

// ValidateQueueMoveSet checks up to four moves from the pool.
func ValidateQueueMoveSet(set *content.Set, m Monster, moves []string) error {
	if len(moves) < 1 || len(moves) > 4 {
		return ErrInvalidQueueLoadout
	}
	pool, err := QueueMovePool(set, m)
	if err != nil {
		return err
	}
	allowed := make(map[string]struct{}, len(pool))
	for _, slug := range pool {
		allowed[slug] = struct{}{}
	}
	seen := map[string]struct{}{}
	for _, slug := range moves {
		if _, ok := allowed[slug]; !ok {
			return ErrInvalidQueueLoadout
		}
		if _, dup := seen[slug]; dup {
			return ErrInvalidQueueLoadout
		}
		seen[slug] = struct{}{}
	}
	return nil
}

// QueueStats rescales natural Level-30 stats to sum exactly QueueStatBudget.
func QueueStats(spec content.Species) [5]int {
	nat := [5]int{
		NaturalStat(spec.BaseStats.HP, QueueLevel),
		NaturalStat(spec.BaseStats.Attack, QueueLevel),
		NaturalStat(spec.BaseStats.Defense, QueueLevel),
		NaturalStat(spec.BaseStats.SpAttack, QueueLevel),
		NaturalStat(spec.BaseStats.Speed, QueueLevel),
	}
	return rescaleBudget(nat, QueueStatBudget)
}

func rescaleBudget(nat [5]int, budget int) [5]int {
	sum := 0
	for _, v := range nat {
		sum += v
	}
	if sum < 1 {
		return [5]int{1, 1, 1, 1, 1}
	}
	var out [5]int
	remainders := make([]float64, 5)
	allocated := 0
	for i, v := range nat {
		exact := float64(budget) * float64(v) / float64(sum)
		floor := max(1, int(exact))
		out[i] = floor
		allocated += floor
		remainders[i] = exact - float64(floor)
	}
	for allocated > budget {
		drop := -1
		dropVal := 0
		for i, v := range out {
			if v > 1 && v > dropVal {
				drop = i
				dropVal = v
			}
		}
		if drop < 0 {
			break
		}
		out[drop]-- //nolint:gosec // drop is always a valid index from the loop above.
		allocated--
	}
	for allocated < budget {
		best := 0
		for i := 1; i < 5; i++ {
			if remainders[i] > remainders[best] {
				best = i
			}
		}
		out[best]++
		allocated++
		remainders[best] = -1
	}
	return out
}

// NormalizedMonster builds a battle-only copy at QueueLevel with queue moves.
func NormalizedMonster(set *content.Set, m Monster, queueMoves []string) (Monster, error) {
	if err := ValidateQueueMoveSet(set, m, queueMoves); err != nil {
		return Monster{}, err
	}
	copyMon := m
	copyMon.Level = QueueLevel
	copyMon.BattleLoadout = append([]string(nil), queueMoves...)
	return copyMon, nil
}
