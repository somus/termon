// Command sheetextract segments a generated contact sheet into one transparent
// PNG per named species. It assigns detached effects to the nearest main sprite
// instead of relying on fixed cell boundaries.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"termon.sh/internal/sprite"
)

const (
	sheetColumns        = 6
	maxDetachedBelowGap = 30
)

type component struct {
	id         int
	area       int
	minX, minY int
	maxX, maxY int
	sumX       int64
}

func main() {
	columns := flag.Int("columns", sheetColumns, "number of sprite columns in the contact sheet")
	namesValue := flag.String("names", "", "comma-separated species slugs in row-major order; defaults to the full roster")
	flag.Parse()
	if flag.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/sheetextract [--columns N] [--names SLUGS] INPUT.png OUTPUT_DIR")
		os.Exit(2)
	}
	if *columns < 1 {
		panic("columns must be at least 1")
	}
	names := sprite.RosterOrder
	if *namesValue != "" {
		names = strings.Split(*namesValue, ",")
	}
	if len(names)%*columns != 0 {
		panic("sprite name count must be divisible by columns")
	}

	input, err := os.Open(flag.Arg(0)) //nolint:gosec // offline sheet tool, operator-supplied path is the point
	if err != nil {
		panic(err)
	}

	sheet, err := png.Decode(input)
	closeErr := input.Close()
	if err != nil {
		panic(err)
	}
	if closeErr != nil {
		panic(closeErr)
	}
	if err := os.MkdirAll(flag.Arg(1), 0o755); err != nil { //nolint:gosec // offline sheet tool
		panic(err)
	}

	rows := len(names) / *columns
	bounds := sheet.Bounds()
	for row := range rows {
		y0 := bounds.Min.Y + bounds.Dy()*row/rows
		y1 := bounds.Min.Y + bounds.Dy()*(row+1)/rows
		start := row * *columns
		extractRow(
			sheet,
			image.Rect(bounds.Min.X, y0, bounds.Max.X, y1),
			names[start:start+*columns],
			flag.Arg(1),
		)
	}
}

func extractRow(sheet image.Image, bounds image.Rectangle, names []string, outputDir string) {
	width, height := bounds.Dx(), bounds.Dy()
	labels := make([]int, width*height)
	for index := range labels {
		labels[index] = -1
	}

	components := connectedComponents(sheet, bounds, labels)
	byArea := append([]component(nil), components...)
	sort.Slice(byArea, func(i, j int) bool { return byArea[i].area > byArea[j].area })
	if len(byArea) < len(names) {
		panic(fmt.Sprintf("sheet row has %d sprite components; need %d", len(byArea), len(names)))
	}

	anchors := append([]component(nil), byArea[:len(names)]...)
	sort.Slice(anchors, func(i, j int) bool {
		return anchors[i].sumX*int64(anchors[j].area) < anchors[j].sumX*int64(anchors[i].area)
	})
	owners := componentOwners(components, anchors)

	for spriteIndex, name := range names {
		crop := image.Rectangle{Min: image.Pt(width, height), Max: image.Pt(0, 0)}
		for _, item := range components {
			if owners[item.id] != spriteIndex {
				continue
			}
			crop.Min.X = min(crop.Min.X, item.minX)
			crop.Min.Y = min(crop.Min.Y, item.minY)
			crop.Max.X = max(crop.Max.X, item.maxX+1)
			crop.Max.Y = max(crop.Max.Y, item.maxY+1)
		}
		crop = crop.Inset(-3).Intersect(image.Rect(0, 0, width, height))
		writeSprite(sheet, bounds, labels, owners, spriteIndex, crop, filepath.Join(outputDir, name+".png"))
	}
}

func connectedComponents(sheet image.Image, bounds image.Rectangle, labels []int) []component {
	width, height := bounds.Dx(), bounds.Dy()
	queue := make([]int, 0, width*height/4)
	var components []component

	for y := range height {
		for x := range width {
			index := y*width + x
			if labels[index] != -1 || !opaque(sheet.At(bounds.Min.X+x, bounds.Min.Y+y)) {
				continue
			}

			id := len(components)
			item := component{id: id, minX: x, minY: y, maxX: x, maxY: y}
			labels[index] = id
			queue = append(queue[:0], index)
			for len(queue) > 0 {
				current := queue[0]
				queue = queue[1:]
				cx, cy := current%width, current/width
				item.area++
				item.sumX += int64(cx)
				item.minX = min(item.minX, cx)
				item.maxX = max(item.maxX, cx)
				item.minY = min(item.minY, cy)
				item.maxY = max(item.maxY, cy)

				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						if dx == 0 && dy == 0 {
							continue
						}
						nx, ny := cx+dx, cy+dy
						if nx < 0 || nx >= width || ny < 0 || ny >= height {
							continue
						}
						next := ny*width + nx
						if labels[next] == -1 && opaque(sheet.At(bounds.Min.X+nx, bounds.Min.Y+ny)) {
							labels[next] = id
							queue = append(queue, next)
						}
					}
				}
			}
			components = append(components, item)
		}
	}

	return components
}

func componentOwners(components, anchors []component) []int {
	owners := make([]int, len(components))
	for index := range owners {
		owners[index] = -1
	}
	for index, anchor := range anchors {
		owners[anchor.id] = index
	}

	for _, item := range components {
		if owners[item.id] >= 0 || item.area < 12 {
			continue
		}
		owner, distance := -1, int(^uint(0)>>1)
		for index, anchor := range anchors {
			if item.minY-anchor.maxY-1 > maxDetachedBelowGap {
				continue
			}
			candidate := boxDistance(item, anchor)
			if candidate < distance {
				owner, distance = index, candidate
			}
		}
		if distance <= 55*55 || item.area >= 80 && distance <= 90*90 {
			owners[item.id] = owner
		}
	}

	return owners
}

func writeSprite(
	sheet image.Image,
	rowBounds image.Rectangle,
	labels, owners []int,
	spriteIndex int,
	crop image.Rectangle,
	path string,
) {
	output := image.NewNRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
	rowWidth := rowBounds.Dx()
	for y := crop.Min.Y; y < crop.Max.Y; y++ {
		for x := crop.Min.X; x < crop.Max.X; x++ {
			componentID := labels[y*rowWidth+x]
			if componentID >= 0 && owners[componentID] == spriteIndex {
				output.Set(x-crop.Min.X, y-crop.Min.Y, sheet.At(rowBounds.Min.X+x, rowBounds.Min.Y+y))
			}
		}
	}

	file, err := os.Create(path) //nolint:gosec // offline sheet tool output path
	if err != nil {
		panic(err)
	}
	encodeErr := png.Encode(file, output)
	closeErr := file.Close()
	if encodeErr != nil {
		panic(encodeErr)
	}
	if closeErr != nil {
		panic(closeErr)
	}
}

func opaque(value color.Color) bool {
	red, green, blue, alpha := value.RGBA()
	return alpha > 0x1000 && max(red, green, blue) > 0x0a0a
}

func boxDistance(a, b component) int {
	dx := max(0, max(a.minX-b.maxX-1, b.minX-a.maxX-1))
	dy := max(0, max(a.minY-b.maxY-1, b.minY-a.maxY-1))
	return dx*dx + dy*dy
}
