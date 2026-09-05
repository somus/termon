package sprite

import (
	"fmt"
	"strings"

	"termon.sh/internal/content"
)

// Pose names compiled from a base grid. Idle is a 2-frame breathe; the rest
// play once in combat.
const (
	PoseIdleA  = "idleA"
	PoseIdleB  = "idleB"
	PoseAtk1   = "atk1"
	PoseAtk2   = "atk2"
	PoseHurt   = "hurt"
	PoseFaint1 = "faint1"
	PoseFaint2 = "faint2"
	PoseFaint3 = "faint3"
)

// Anim is one species' pre-rendered poses.
type Anim struct {
	Slug   string
	Type   string
	Frames map[string][]string
	// Joined caches each pose's frames pre-joined with newlines so render
	// paths never re-join per frame.
	Joined map[string]string
}

// FlipGrid mirrors a pixel grid horizontally.
func FlipGrid(grid []string) []string {
	w := gridWidth(grid)
	out := make([]string, len(grid))
	for i, row := range grid {
		rr := []rune(row)
		for len(rr) < w {
			rr = append(rr, '.')
		}
		for l, r := 0, len(rr)-1; l < r; l, r = l+1, r-1 {
			rr[l], rr[r] = rr[r], rr[l]
		}
		out[i] = string(rr)
	}
	return out
}

// Compile builds idle/attack/hurt/faint frames from a single Art grid.
// Room is added on every side so a 1px bounce and a 4px lunge stay in frame.
func Compile(art content.Art, typeSlug string) Anim {
	return CompileFacing(art, typeSlug, false)
}

// CompileFacing is Compile, optionally mirrored so a player sprite faces right.
func CompileFacing(art content.Art, typeSlug string, faceRight bool) Anim {
	return CompileOn(art, typeSlug, faceRight, "")
}

// CompileOn is CompileFacing with a backdrop hex under transparent pixels.
func CompileOn(art content.Art, typeSlug string, faceRight bool, backdrop string) Anim {
	pal := art.RunePalette()
	grid := art.Grid
	if faceRight {
		grid = FlipGrid(grid)
	}
	base := padRoom(grid, 2, 2, 4)
	dir := -1
	if faceRight {
		dir = 1
	}
	bright := func(amt float64) map[rune]string { return brightenPal(pal, amt) }
	draw := func(g []string, p map[rune]string) []string { return RenderOn(g, p, backdrop) }
	frames := map[string][]string{
		PoseIdleA:  draw(base, pal),
		PoseIdleB:  draw(shift(base, 0, -1), pal),
		PoseAtk1:   draw(shift(base, dir*2, -1), bright(0.25)),
		PoseAtk2:   draw(shift(base, dir*4, 0), bright(0.55)),
		PoseHurt:   draw(shift(base, dir*-2, -1), tintPal(pal, "#ff4030", 0.45)),
		PoseFaint1: draw(squash(base, 0.75), bright(0.15)),
		PoseFaint2: draw(squash(base, 0.50), tintPal(pal, "#403a35", 0.35)),
		PoseFaint3: draw(squash(base, 0.35), tintPal(pal, "#26221f", 0.60)),
	}
	joined := make(map[string]string, len(frames))
	for pose, f := range frames {
		joined[pose] = strings.Join(f, "\n")
	}
	return Anim{Slug: art.Slug, Type: typeSlug, Frames: frames, Joined: joined}
}

func padRoom(grid []string, top, bottom, horiz int) []string {
	w := gridWidth(grid)
	innerW := w + horiz*2
	blank := strings.Repeat(".", innerW)
	side := strings.Repeat(".", horiz)
	out := make([]string, 0, len(grid)+top+bottom)
	for range top {
		out = append(out, blank)
	}
	for _, r := range grid {
		rr := []rune(r)
		for len(rr) < w {
			rr = append(rr, '.')
		}
		out = append(out, side+string(rr)+side)
	}
	for range bottom {
		out = append(out, blank)
	}
	return out
}

func dims(g []string) (int, int) {
	return gridWidth(g), len(g)
}

func blankLike(g []string) []string {
	w, h := dims(g)
	out := make([]string, h)
	for i := range out {
		out[i] = strings.Repeat(".", w)
	}
	return out
}

func paste(dst, src []string, dx, dy int) {
	w, _ := dims(dst)
	for y, row := range src {
		yy := y + dy
		if yy < 0 || yy >= len(dst) {
			continue
		}
		dr := []rune(dst[yy])
		for x, ch := range row {
			if transparent(ch) {
				continue
			}
			xx := x + dx
			if xx < 0 || xx >= w {
				continue
			}
			if xx >= len(dr) {
				dr = append(dr, []rune(strings.Repeat(".", xx-len(dr)+1))...)
			}
			dr[xx] = ch
		}
		dst[yy] = string(dr)
	}
}

func shift(g []string, dx, dy int) []string {
	out := blankLike(g)
	paste(out, g, dx, dy)
	return out
}

func squash(g []string, frac float64) []string {
	_, h := dims(g)
	keep := max(4, int(float64(h)*frac))
	start := h - keep
	out := blankLike(g[:start])
	return append(out, g[start:]...)
}

func hexRGB(s string) (uint8, uint8, uint8) {
	var ri, gi, bi int
	_, _ = fmt.Sscanf(s, "#%02x%02x%02x", &ri, &gi, &bi)
	return uint8(ri), uint8(gi), uint8(bi) //nolint:gosec // %02x caps each channel at 255
}

func brightenPal(pal map[rune]string, amt float64) map[rune]string {
	np := make(map[rune]string, len(pal))
	for r, hex := range pal {
		r0, g0, b0 := hexRGB(hex)
		up := func(v uint8) uint8 { return uint8(float64(v) + (255-float64(v))*amt) }
		np[r] = fmt.Sprintf("#%02x%02x%02x", up(r0), up(g0), up(b0))
	}
	return np
}

func tintPal(pal map[rune]string, target string, pct float64) map[rune]string {
	tr, tg, tb := hexRGB(target)
	np := make(map[rune]string, len(pal))
	for r, hex := range pal {
		r0, g0, b0 := hexRGB(hex)
		mix := func(v, tv uint8) uint8 { return uint8(float64(v)*(1-pct) + float64(tv)*pct) }
		np[r] = fmt.Sprintf("#%02x%02x%02x", mix(r0, tr), mix(g0, tg), mix(b0, tb))
	}
	return np
}
