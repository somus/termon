// PROTOTYPE — throwaway. Prints every species sprite from content/ as
// half-block art, 3 per row, for review. Do not ship this.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"termon.sh/internal/content"
	"termon.sh/internal/sprite"
)

func main() {
	pngDir := flag.String("png-dir", "", "directory containing slug-named PNG sprites (up to 32x30)")
	flag.Parse()

	set, err := content.Load("content")
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	slugs := sprite.SortedSlugs(set.Arts)
	if *pngDir != "" {
		slugs = sprite.RosterOrder
	}
	cols := make([]sprite.Column, 0, len(slugs))
	for _, slug := range slugs {
		art := set.Arts[slug]
		if *pngDir != "" {
			art, err = sprite.LoadPNGArt(slug, filepath.Join(*pngDir, slug+".png"))
			if err != nil {
				fmt.Println("error:", err)
				os.Exit(1)
			}
		}
		cols = append(cols, sprite.Column{
			Lines: sprite.Render(sprite.Trim(art.Grid), art.RunePalette()),
			Label: slug,
		})
	}
	fmt.Println(sprite.TitleLine(len(cols)))
	fmt.Print(sprite.Sheet(cols, 3))
}
