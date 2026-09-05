package sprite

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"termon.sh/internal/content"
)

func TestSheetKeepsLabelsOnShortSprites(t *testing.T) {
	cols := []Column{
		{Lines: []string{"████████"}, Label: "tallboy"},
		{Lines: []string{"██"}, Label: "shorty"},
		{Lines: []string{"██████"}, Label: "midling"},
	}
	out := Sheet(cols, 3)
	plain := stripANSI(out)
	for _, want := range []string{"tallboy", "shorty", "midling"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing label %q in sheet:\n%s", want, plain)
		}
	}
	lines := strings.Split(strings.TrimRight(plain, "\n"), "\n")
	last := lines[len(lines)-1]
	for _, want := range []string{"tallboy", "shorty", "midling"} {
		if !strings.Contains(last, want) {
			t.Fatalf("label %q not on shared baseline %q", want, last)
		}
	}
}

func TestTrimEvenHeight(t *testing.T) {
	g := Trim([]string{
		"....",
		".xx.",
		".xx.",
		".xx.",
		"....",
	})
	if len(g)%2 != 0 {
		t.Fatalf("trimmed height %d, want even", len(g))
	}
	if emptyRow(g[0]) {
		t.Fatal("leading transparent row survived Trim")
	}
}

func TestCompileIdleFramesDiffer(t *testing.T) {
	art := content.Art{
		Slug: "spark",
		Palette: map[string]string{
			"o": "#101010",
			"Y": "#ffd24a",
			"k": "#000000",
		},
		Grid: []string{
			"..ooo..",
			".oYYYo.",
			".oYkYo.",
			".oYYYo.",
			"..ooo..",
			"..o.o..",
		},
	}
	anim := Compile(art, "current")
	a, b := anim.Frames[PoseIdleA], anim.Frames[PoseIdleB]
	if len(a) == 0 || len(b) == 0 {
		t.Fatal("idle frames empty")
	}
	if strings.Join(a, "\n") == strings.Join(b, "\n") {
		t.Fatal("idle A and B are identical; bounce did not land")
	}
	for _, pose := range []string{PoseAtk1, PoseAtk2, PoseHurt, PoseFaint1, PoseFaint2, PoseFaint3} {
		if len(anim.Frames[pose]) == 0 {
			t.Fatalf("pose %s empty", pose)
		}
	}
}

func TestRenderOnPaintsTransparent(t *testing.T) {
	grid := []string{"..", ".."}
	plain := strings.Join(Render(grid, nil), "")
	backed := strings.Join(RenderOn(grid, nil, "#14121c"), "")
	if plain == backed {
		t.Fatal("backdrop should style transparent cells so they match the screen")
	}
}

func TestFlipGridMirrorsRows(t *testing.T) {
	got := FlipGrid([]string{"ab..", ".cd."})
	if got[0] != "..ba" || got[1] != ".dc." {
		t.Fatalf("flipped %v", got)
	}
}

func TestCompileFacingMirrorsPlayer(t *testing.T) {
	art := content.Art{
		Slug: "spark",
		Palette: map[string]string{
			"o": "#101010",
			"Y": "#ffd24a",
		},
		Grid: []string{
			"ooo....",
			"YYY....",
			"ooo....",
		},
	}
	left := strings.Join(CompileFacing(art, "current", false).Frames[PoseIdleA], "\n")
	right := strings.Join(CompileFacing(art, "current", true).Frames[PoseIdleA], "\n")
	if left == right {
		t.Fatal("player-facing compile should mirror the foe sprite")
	}
}

func TestAttackLungeKeepsSilhouette(t *testing.T) {
	art := content.Art{
		Slug: "spark",
		Palette: map[string]string{
			"o": "#101010",
			"Y": "#ffd24a",
			"k": "#000000",
		},
		Grid: []string{
			"ooo....",
			"YYYo...",
			"YkYo...",
			"YYYo...",
			"ooo....",
			"o.o....",
		},
	}
	anim := Compile(art, "current")
	idle := opaqueCells(anim.Frames[PoseIdleA])
	atk := opaqueCells(anim.Frames[PoseAtk2])
	if atk < idle {
		t.Fatalf("atk2 clipped the silhouette: idle %d cells, atk2 %d", idle, atk)
	}
}

func opaqueCells(lines []string) int {
	n := 0
	for _, line := range lines {
		for _, ch := range ansi.Strip(line) {
			if ch != ' ' {
				n++
			}
		}
	}
	return n
}

func TestDownsampleHalvesGrid(t *testing.T) {
	g := Downsample([]string{
		"aabb",
		"aabb",
		"cc..",
		"cc..",
	}, 2)
	if gridWidth(g) != 2 {
		t.Fatalf("width %d, want 2", gridWidth(g))
	}
	if len(g) < 2 {
		t.Fatalf("height %d, want at least 2", len(g))
	}
	if !strings.Contains(g[0], "a") && !strings.Contains(g[0], "c") {
		t.Fatalf("lost silhouette: %v", g)
	}
}

func TestSortedSlugsRosterThenExtra(t *testing.T) {
	arts := map[string]content.Art{
		"zaplet":  {Slug: "zaplet"},
		"rootkit": {Slug: "rootkit"},
		"zzz":     {Slug: "zzz"},
	}
	got := SortedSlugs(arts)
	want := []string{"rootkit", "zaplet", "zzz"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v want %v", got, want)
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for line := range strings.SplitSeq(s, "\n") {
		b.WriteString(ansi.Strip(line))
		b.WriteByte('\n')
	}
	return b.String()
}

func TestCompileOnJoinsEveryPose(t *testing.T) {
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	slug := "rootkit"
	art, ok := set.Arts[slug]
	if !ok {
		t.Skipf("no art for %s", slug)
	}
	anim := CompileOn(art, set.Species[slug].Type, false, "")
	if len(anim.Joined) != len(anim.Frames) {
		t.Fatalf("joined %d poses, frames has %d", len(anim.Joined), len(anim.Frames))
	}
	for pose, frame := range anim.Frames {
		if got := anim.Joined[pose]; got != strings.Join(frame, "\n") {
			t.Fatalf("pose %s joined string does not match its frames", pose)
		}
	}
}
