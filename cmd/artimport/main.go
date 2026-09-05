// Command artimport converts the accepted PNG roster into boot-validated
// content/art JSON files.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"termon.sh/internal/sprite"
)

func main() {
	pngDir := flag.String("png-dir", "docs/design/monster-sprites", "directory containing slug-named PNG sprites")
	outputDir := flag.String("output-dir", "content/art", "directory for generated art JSON files")
	flag.Parse()

	if err := os.MkdirAll(*outputDir, 0o755); err != nil { //nolint:gosec // local content import tool
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	for _, slug := range sprite.RosterOrder {
		art, err := sprite.LoadPNGArt(slug, filepath.Join(*pngDir, slug+".png"))
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		data, err := json.MarshalIndent(art, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		data = append(data, '\n')
		path := filepath.Join(*outputDir, slug+".json")
		if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec // generated art JSON, world-readable by design
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	}
	fmt.Printf("wrote %d art files to %s\n", len(sprite.RosterOrder), *outputDir)
}
