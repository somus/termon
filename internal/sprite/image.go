package sprite

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"

	"termon.sh/internal/content"
)

const paletteRunes = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// LoadPNGArt decodes a PNG file into the palette/grid format consumed by
// Compile.
func LoadPNGArt(slug, path string) (content.Art, error) {
	file, err := os.Open(path) //nolint:gosec // offline art pipeline tool, operator-supplied paths
	if err != nil {
		return content.Art{}, fmt.Errorf("open %s: %w", path, err)
	}
	img, decodeErr := png.Decode(file)
	closeErr := file.Close()
	if decodeErr != nil {
		return content.Art{}, fmt.Errorf("decode %s: %w", path, decodeErr)
	}
	if closeErr != nil {
		return content.Art{}, fmt.Errorf("close %s: %w", path, closeErr)
	}
	return ArtFromImage(slug, img)
}

// ArtFromImage converts an RGBA sprite into the palette/grid format consumed
// by Compile. Pixels below half opacity are transparent.
func ArtFromImage(slug string, img image.Image) (content.Art, error) {
	bounds := img.Bounds()
	if bounds.Dx() == 0 || bounds.Dy() == 0 || bounds.Dx() > 32 || bounds.Dy() > 30 {
		return content.Art{}, fmt.Errorf("sprite %s: dimensions %dx%d must be within 32x30", slug, bounds.Dx(), bounds.Dy())
	}

	keys := []rune(paletteRunes)
	colorKeys := make(map[color.NRGBA]rune)
	palette := make(map[string]string)
	grid := make([]string, 0, bounds.Dy())
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		row := make([]rune, 0, bounds.Dx())
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pixel := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if pixel.A < 128 {
				row = append(row, '.')
				continue
			}
			pixel.A = 255
			key, ok := colorKeys[pixel]
			if !ok {
				if len(colorKeys) == len(keys) {
					return content.Art{}, fmt.Errorf("sprite %s: more than %d colors", slug, len(keys))
				}
				key = keys[len(colorKeys)]
				colorKeys[pixel] = key
				palette[string(key)] = fmt.Sprintf("#%02x%02x%02x", pixel.R, pixel.G, pixel.B)
			}
			row = append(row, key)
		}
		grid = append(grid, string(row))
	}

	return content.Art{Slug: slug, Palette: palette, Grid: grid}, nil
}
