package content

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxEvolutionStages = 3

// Set is the fully loaded and validated content pack.
type Set struct {
	Species map[string]Species
	Moves   map[string]Move
	Types   map[string]TypeDef
	Arts    map[string]Art
}

// decodeStrict unmarshals content JSON, refusing unknown fields so a typo'd
// key fails at boot instead of silently producing zero values.
func decodeStrict(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// Load reads and validates every content file under dir. Any malformed or
// inconsistent data aborts startup.
func Load(dir string) (*Set, error) {
	set := &Set{
		Species: map[string]Species{},
		Moves:   map[string]Move{},
		Types:   map[string]TypeDef{},
		Arts:    map[string]Art{},
	}

	if err := loadDir(filepath.Join(dir, "types"), func(slug string, raw []byte) error {
		var t TypeDef
		if err := decodeStrict(raw, &t); err != nil {
			return err
		}
		if t.Slug != slug {
			return fmt.Errorf("type slug %q does not match filename", t.Slug)
		}
		if t.Name == "" {
			return fmt.Errorf("type %s: missing name", slug)
		}
		for defender, mult := range t.Matchup {
			if mult != 2.0 && mult != 0.5 {
				return fmt.Errorf("type %s: matchup %s has multiplier %v, want 2.0 or 0.5", slug, defender, mult)
			}
		}
		set.Types[slug] = t
		return nil
	}); err != nil {
		return nil, err
	}
	if len(set.Types) < 3 {
		return nil, fmt.Errorf("content: need at least 3 types, got %d", len(set.Types))
	}
	// Second pass: matchup defender slugs may reference any type file, so
	// resolve them only after every type is loaded.
	for slug, t := range set.Types {
		for defender := range t.Matchup {
			if _, ok := set.Types[defender]; !ok {
				return nil, fmt.Errorf("type %s: matchup references unknown type %q", slug, defender)
			}
		}
	}

	if err := loadDir(filepath.Join(dir, "moves"), func(slug string, raw []byte) error {
		var m Move
		if err := decodeStrict(raw, &m); err != nil {
			return err
		}
		if m.Slug != slug {
			return fmt.Errorf("move slug %q does not match filename", m.Slug)
		}
		if _, ok := set.Types[m.Type]; !ok {
			return fmt.Errorf("move %s: unknown type %q", slug, m.Type)
		}
		if m.Category != "physical" && m.Category != "special" {
			return fmt.Errorf("move %s: category must be physical or special", slug)
		}
		if m.Power < 1 || m.Power > 150 {
			return fmt.Errorf("move %s: power %v out of range", slug, m.Power)
		}
		if m.Accuracy < 1 || m.Accuracy > 100 {
			return fmt.Errorf("move %s: accuracy %v out of range", slug, m.Accuracy)
		}
		set.Moves[slug] = m
		return nil
	}); err != nil {
		return nil, err
	}

	if err := loadDir(filepath.Join(dir, "species"), func(slug string, raw []byte) error {
		var s Species
		if err := decodeStrict(raw, &s); err != nil {
			return err
		}
		if s.Slug != slug {
			return fmt.Errorf("species slug %q does not match filename", s.Slug)
		}
		if _, ok := set.Types[s.Type]; !ok {
			return fmt.Errorf("species %s: unknown type %q", slug, s.Type)
		}
		bs := s.BaseStats
		if bs.HP < 1 || bs.Attack < 1 || bs.Defense < 1 || bs.SpAttack < 1 || bs.Speed < 1 {
			return fmt.Errorf("species %s: base stats must all be at least 1", slug)
		}
		if len(s.Movepool) < 4 {
			return fmt.Errorf("species %s: movepool needs at least 4 moves", slug)
		}
		seen := map[string]bool{}
		for _, e := range s.Movepool {
			if seen[e.Move] {
				return fmt.Errorf("species %s: duplicate movepool entry %q", slug, e.Move)
			}
			seen[e.Move] = true
			m, ok := set.Moves[e.Move]
			if !ok {
				return fmt.Errorf("species %s: movepool references unknown move %q", slug, e.Move)
			}
			if m.Type != s.Type {
				return fmt.Errorf("species %s: movepool move %s is type %s, want %s", slug, e.Move, m.Type, s.Type)
			}
		}
		set.Species[slug] = s
		return nil
	}); err != nil {
		return nil, err
	}
	if err := validateEvolutions(set.Species); err != nil {
		return nil, err
	}

	if err := loadDir(filepath.Join(dir, "art"), func(slug string, raw []byte) error {
		var a Art
		if err := decodeStrict(raw, &a); err != nil {
			return err
		}
		if err := validateArt(slug, a); err != nil {
			return err
		}
		set.Arts[slug] = a
		return nil
	}); err != nil {
		return nil, err
	}
	if len(set.Arts) == 0 {
		return nil, errors.New("content: no sprite art found")
	}
	if err := artCoverage(set.Arts, set.Species); err != nil {
		return nil, err
	}

	return set, nil
}

func validateEvolutions(species map[string]Species) error {
	predecessors := make(map[string]string)
	for slug, source := range species {
		evolution := source.EvolvesTo
		if evolution == nil {
			continue
		}
		if evolution.Level < 2 {
			return fmt.Errorf("species %s: evolution level must be at least 2", slug)
		}
		if evolution.Species == slug {
			return fmt.Errorf("species %s: cannot evolve into itself", slug)
		}
		if _, ok := species[evolution.Species]; !ok {
			return fmt.Errorf("species %s: evolution references unknown species %q", slug, evolution.Species)
		}
		if predecessor, ok := predecessors[evolution.Species]; ok {
			return fmt.Errorf(
				"species %s: evolution has multiple predecessors %q and %q",
				evolution.Species,
				predecessor,
				slug,
			)
		}
		predecessors[evolution.Species] = slug
	}

	for slug := range species {
		seen := make(map[string]bool)
		current := slug
		for stage := 1; ; stage++ {
			if seen[current] {
				return fmt.Errorf("species %s: evolution family contains a cycle at %q", slug, current)
			}
			if stage > maxEvolutionStages {
				return fmt.Errorf("species %s: evolution family exceeds %d stages", slug, maxEvolutionStages)
			}
			seen[current] = true
			evolution := species[current].EvolvesTo
			if evolution == nil {
				break
			}
			current = evolution.Species
		}
	}
	for _, source := range species {
		if source.EvolvesTo == nil {
			continue
		}
		target := species[source.EvolvesTo.Species]
		if target.EvolvesTo != nil && target.EvolvesTo.Level <= source.EvolvesTo.Level {
			return fmt.Errorf(
				"species %s: evolution level %d must be greater than predecessor level %d",
				target.Slug,
				target.EvolvesTo.Level,
				source.EvolvesTo.Level,
			)
		}
	}

	return nil
}

// Effectiveness returns the attacker-side multiplier for a move of the given
// type against a defender type. Unknown pairs are neutral (1.0).
func (s *Set) Effectiveness(attackType, defenderType string) float64 {
	t, ok := s.Types[attackType]
	if !ok {
		return 1.0
	}
	if mult, ok := t.Matchup[defenderType]; ok {
		return mult
	}
	return 1.0
}

func loadDir(dir string, fn func(slug string, raw []byte) error) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("content: read %s: %w", dir, err)
	}
	for _, e := range entries {
		const ext = ".json"
		if e.IsDir() || !strings.HasSuffix(e.Name(), ext) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // operator-supplied content pack root, validated at boot
		if err != nil {
			return err
		}
		slug := strings.TrimSuffix(e.Name(), ext)
		if err := fn(slug, raw); err != nil {
			return fmt.Errorf("%s: %w", filepath.Join(dir, e.Name()), err)
		}
	}
	return nil
}
