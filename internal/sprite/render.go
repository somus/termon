// Package sprite renders species pixel grids as half-block terminal art and
// derives idle/attack/hurt/faint poses from a single base grid.
package sprite

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Render maps a pixel grid to half-block lines (▀ / ▄). Each source row-pair
// becomes one terminal row: fg = top pixel, bg = bottom pixel. '.' and ' ' are
// transparent.
func Render(grid []string, pal map[rune]string) []string {
	return RenderOn(grid, pal, "")
}

// RenderOn is Render with a backdrop hex for transparent cells, so sprites
// sit on a fixed screen color instead of the client's terminal theme.
func RenderOn(grid []string, pal map[rune]string, backdrop string) []string {
	w, rows := padGrid(grid)
	if len(rows)%2 == 1 {
		rows = append(rows, []rune(strings.Repeat(".", w)))
	}
	style := func(fg, bg string) lipgloss.Style {
		s := lipgloss.NewStyle()
		if fg != "" {
			s = s.Foreground(lipgloss.Color(fg))
		}
		if bg != "" {
			s = s.Background(lipgloss.Color(bg))
		}
		return s
	}
	var lines []string
	for y := 0; y < len(rows); y += 2 {
		var b strings.Builder
		for x := range w {
			top, bot := rows[y][x], rows[y+1][x]
			switch {
			case transparent(top) && transparent(bot):
				if backdrop != "" {
					b.WriteString(style("", backdrop).Render(" "))
				} else {
					b.WriteByte(' ')
				}
			case transparent(bot):
				b.WriteString(style(pal[top], backdrop).Render("▀"))
			case transparent(top):
				b.WriteString(style(pal[bot], backdrop).Render("▄"))
			default:
				b.WriteString(style(pal[top], pal[bot]).Render("▀"))
			}
		}
		lines = append(lines, b.String())
	}
	return lines
}

func transparent(ch rune) bool {
	return ch == '.' || ch == ' '
}

func emptyRow(row string) bool {
	for _, ch := range row {
		if !transparent(ch) {
			return false
		}
	}
	return true
}

// Trim drops fully transparent rows from the top and bottom of a grid, then
// pads to even height so half-block pairing stays aligned.
func Trim(grid []string) []string {
	start, end := 0, len(grid)-1
	for start <= end && emptyRow(grid[start]) {
		start++
	}
	for end >= start && emptyRow(grid[end]) {
		end--
	}
	if start > end {
		return []string{"."}
	}
	out := append([]string{}, grid[start:end+1]...)
	w := gridWidth(out)
	if len(out)%2 == 1 {
		out = append(out, strings.Repeat(".", w))
	}
	return out
}

func gridWidth(grid []string) int {
	w := 0
	for _, r := range grid {
		if n := len([]rune(r)); n > w {
			w = n
		}
	}
	return w
}

// Downsample shrinks a pixel grid by factor, keeping the first opaque
// pixel in each block so silhouettes stay readable at card size.
func Downsample(grid []string, factor int) []string {
	if factor < 2 || len(grid) == 0 {
		return grid
	}
	w, rows := padGrid(grid)
	outH := (len(rows) + factor - 1) / factor
	outW := (w + factor - 1) / factor
	out := make([]string, 0, outH)
	for y := range outH {
		var b strings.Builder
		for x := range outW {
			ch := rune('.')
			for dy := 0; dy < factor && transparent(ch); dy++ {
				yy := y*factor + dy
				if yy >= len(rows) {
					break
				}
				for dx := range factor {
					xx := x*factor + dx
					if xx >= w {
						break
					}
					if p := rows[yy][xx]; !transparent(p) {
						ch = p
						break
					}
				}
			}
			b.WriteRune(ch)
		}
		out = append(out, b.String())
	}
	return Trim(out)
}

func padGrid(grid []string) (int, [][]rune) {
	w := gridWidth(grid)
	rows := make([][]rune, len(grid))
	for i, r := range grid {
		rr := []rune(r)
		for len(rr) < w {
			rr = append(rr, '.')
		}
		rows[i] = rr
	}
	return w, rows
}
