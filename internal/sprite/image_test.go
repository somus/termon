package sprite

import (
	"image"
	"image/color"
	"slices"
	"testing"
)

func TestArtFromImageConvertsColorsAndTransparency(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	red := color.NRGBA{R: 255, A: 255}
	blue := color.NRGBA{B: 255, A: 255}
	img.SetNRGBA(0, 0, red)
	img.SetNRGBA(1, 0, color.NRGBA{R: 255, A: 127})
	img.SetNRGBA(2, 0, blue)
	img.SetNRGBA(0, 1, red)

	art, err := ArtFromImage("sample", img)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := art.Grid, []string{"0.1", "0.."}; !slices.Equal(got, want) {
		t.Fatalf("grid = %q, want %q", got, want)
	}
	if art.Palette["0"] != "#ff0000" || art.Palette["1"] != "#0000ff" {
		t.Fatalf("palette = %#v", art.Palette)
	}
}

func TestArtFromImageRejectsOversizedSprites(t *testing.T) {
	_, err := ArtFromImage("wide", image.NewNRGBA(image.Rect(0, 0, 33, 30)))
	if err == nil {
		t.Fatal("expected oversized sprite error")
	}
}
