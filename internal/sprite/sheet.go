package sprite

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"termon.sh/internal/content"
)

// RosterOrder groups each three-stage family inside its Type.
var RosterOrder = []string{
	"rootkit", "barkdoor", "priviloak",
	"sproutware", "vinemount", "canopynet",
	"thornpatch", "briarwall", "fortiforest",
	"mossmuff", "lichenloop", "bogdaemon",
	"rootanami", "taprouter", "rhizoracle",
	"emberbyte", "cinderhash", "flarestack",
	"cindernode", "furnacehub", "calderdaemon",
	"scorchip", "burnboard", "infernalink",
	"wickware", "torchthread", "daemoflare",
	"aquabit", "bytefin", "datadeluge",
	"flowcell", "wavebank", "tidalarray",
	"gushkit", "pipelinx", "torrentiger",
	"mistcache", "fogbuffer", "cloudvault",
	"splashscreen", "cachelotl", "rebootide",
	"zaplet", "voltalon", "stormkernel",
	"joulpup", "voltweiler", "ampmastiff",
	"amperent", "coilobra", "gridaconda",
	"surgetail", "stormfin", "tempestray",
	"spamlet", "mailgnant", "phishmonger",
	"bloatware", "featurmoil", "heapocalypse",
	"wormate", "segmaggot", "hexhelminth",
	"chippunk", "solderat", "rackoon",
	"coghound", "trackbyte", "watchdaemon",
	"servoboar", "ramhog", "racktusk",
}

// SortedSlugs returns art slugs in roster order, then any extras alphabetically.
func SortedSlugs(arts map[string]content.Art) []string {
	seen := map[string]bool{}
	var out []string
	for _, slug := range RosterOrder {
		if _, ok := arts[slug]; ok {
			out = append(out, slug)
			seen[slug] = true
		}
	}
	var extra []string
	for slug := range arts {
		if !seen[slug] {
			extra = append(extra, slug)
		}
	}
	slices.Sort(extra)
	return append(out, extra...)
}

// Column is one labeled sprite in a contact sheet.
type Column struct {
	Lines []string
	Label string
}

// Sheet lays out columns left-to-right, wrapping every perRow. Sprites are
// bottom-aligned so every label shares a baseline. Labels are always the last
// row of each column — using the padded height, not the original sprite height
// (the old path dropped labels on shorter sprites in a row).
func Sheet(cols []Column, perRow int) string {
	if perRow < 1 {
		perRow = 1
	}
	labelStyle := lipgloss.NewStyle().Bold(true)
	var b strings.Builder
	for start := 0; start < len(cols); start += perRow {
		end := min(start+perRow, len(cols))
		b.WriteString(sheetRow(cols[start:end], labelStyle))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func sheetRow(cols []Column, labelStyle lipgloss.Style) string {
	cells := make([][]string, len(cols))
	for i, c := range cols {
		cells[i] = append(append([]string{}, c.Lines...), "", labelStyle.Render(c.Label))
	}
	widths := make([]int, len(cells))
	for i, c := range cells {
		w := 0
		for _, l := range c {
			if lw := ansi.StringWidth(l); lw > w {
				w = lw
			}
		}
		widths[i] = w + 2
	}
	maxH := 0
	for _, c := range cells {
		if len(c) > maxH {
			maxH = len(c)
		}
	}
	for j := range cells {
		for len(cells[j]) < maxH {
			cells[j] = append([]string{""}, cells[j]...)
		}
	}
	rowLines := make([]strings.Builder, maxH)
	for i := range rowLines {
		for j, cell := range cells {
			line := cell[i]
			pad := widths[j] - ansi.StringWidth(line)
			if pad < 1 {
				pad = 2
			}
			_, _ = rowLines[i].WriteString(line + strings.Repeat(" ", pad)) // strings.Builder writes never fail
		}
	}
	out := make([]string, maxH)
	for i := range rowLines {
		out[i] = strings.TrimRight(rowLines[i].String(), " ")
	}
	return strings.Join(out, "\n")
}

// TitleLine is the contact-sheet header.
func TitleLine(n int) string {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).
		Render(fmt.Sprintf("termon contact sheet — %d sprites", n))
}
