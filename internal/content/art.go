package content

import (
	"fmt"
	"strings"
)

// Art is a species' half-block sprite: a pixel grid where each rune maps to
// one palette color ('.' or ' ' = transparent). Format per
// docs/design/data-model.md; style per TERM-16.
type Art struct {
	Slug    string            `json:"slug"`
	Palette map[string]string `json:"palette"` // rune-as-string -> "#rrggbb"
	Grid    []string          `json:"grid"`
}

// RunePalette converts string keys to runes for the renderer.
func (a Art) RunePalette() map[rune]string {
	out := make(map[rune]string, len(a.Palette))
	for k, v := range a.Palette {
		r := []rune(k)
		if len(r) != 1 {
			continue
		}
		out[r[0]] = v
	}
	return out
}

func validHex(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	isHex := func(c byte) bool {
		return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
	}
	for i := 1; i < 7; i++ {
		if !isHex(s[i]) {
			return false
		}
	}
	return true
}

func validateArt(slug string, a Art) error {
	if a.Slug != slug {
		return fmt.Errorf("art slug %q does not match filename", a.Slug)
	}
	if len(a.Palette) < 3 {
		return fmt.Errorf("art %s: need at least 3 palette colors", slug)
	}
	for k, v := range a.Palette {
		if len([]rune(k)) != 1 {
			return fmt.Errorf("art %s: palette key %q is not a single rune", slug, k)
		}
		if !validHex(v) {
			return fmt.Errorf("art %s: palette %q has invalid hex %q", slug, k, v)
		}
	}
	if len(a.Grid) == 0 || len(a.Grid) > 30 {
		return fmt.Errorf("art %s: grid height must be 1-30", slug)
	}
	for y, row := range a.Grid {
		for _, ch := range row {
			if ch == '.' || ch == ' ' {
				continue
			}
			if _, ok := a.Palette[string(ch)]; !ok {
				return fmt.Errorf("art %s: row %d uses rune %q not in palette", slug, y, ch)
			}
		}
	}
	w := 0
	for _, row := range a.Grid {
		if lw := len([]rune(row)); lw > w {
			w = lw
		}
	}
	if w > 32 {
		return fmt.Errorf("art %s: grid width %d exceeds 32", slug, w)
	}
	return nil
}

// artCoverage checks every Species has an Art entry.
func artCoverage(arts map[string]Art, species map[string]Species) error {
	var missing []string
	for slug := range species {
		if _, ok := arts[slug]; !ok {
			missing = append(missing, slug)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("species missing sprite art: %s", strings.Join(missing, ", "))
	}
	return nil
}
