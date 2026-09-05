// Balance Run operator harness (TERM-62 slice 6).
//
// Run from repo root:
//
//	go run ./cmd/balancerun -content ./content
//
// Flags:
//
//	-content     content pack directory (default: discover ./content)
//	-seeds       corpus size (default 1024)
//	-seed-base   recorded corpus identity base (default 1)
//	-rules       rules revision (default party-battles-v1)
//	-report      write machine-readable JSON results
//	-fail-gates  exit 1 when a gate fails (default false for baseline recording)
//	-capture     run capture generator eligibility smoke checks
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"termon.sh/internal/balance"
	"termon.sh/internal/content"
)

func main() {
	contentDir := flag.String("content", "", "content pack directory")
	seedCount := flag.Int("seeds", balance.DefaultCorpusSize, "corpus seed count")
	seedBase := flag.Uint64("seed-base", balance.DefaultSeedBase, "corpus seed base")
	rules := flag.String("rules", balance.RulesRevision, "rules revision")
	reportPath := flag.String("report", "", "JSON report output path")
	failGates := flag.Bool("fail-gates", false, "exit 1 on gate failure")
	captureSmoke := flag.Bool("capture", false, "run capture generator smoke checks")
	flag.Parse()

	dir := *contentDir
	if dir == "" {
		dir = findContent()
	}
	set, err := content.Load(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "balancerun: load content: %v\n", err)
		os.Exit(1)
	}
	rev, err := balance.ContentRevisionFromDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "balancerun: content revision: %v\n", err)
		os.Exit(1)
	}
	if *rules != balance.RulesRevision {
		fmt.Fprintf(os.Stderr, "balancerun: unsupported rules revision %q (want %q)\n", *rules, balance.RulesRevision)
		os.Exit(1)
	}

	out, err := balance.Run(balance.Config{
		Set:          set,
		Seeds:        balance.CorpusSeeds(*seedBase, *seedCount),
		SeedBase:     *seedBase,
		Policy:       balance.DefaultPolicy(),
		MaxTurns:     balance.DefaultMaxTurns,
		Rules:        *rules,
		ContentID:    rev,
		CaptureSmoke: *captureSmoke,
		FailGates:    *failGates,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	fmt.Println(balance.FormatSummary(out))
	if len(out.FailedGates) > 0 {
		for _, rep := range out.FailedGates {
			raw, _ := json.MarshalIndent(rep, "", "  ")
			fmt.Println(string(raw))
		}
	}
	if *reportPath != "" {
		raw, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "balancerun: report encode: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*reportPath, raw, 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "balancerun: write report: %v\n", err)
			os.Exit(1)
		}
	}
	if *failGates && !out.Passed {
		os.Exit(1)
	}
}

func findContent() string {
	dir, err := os.Getwd()
	if err != nil {
		return "content"
	}
	for range 8 {
		cand := filepath.Join(dir, "content")
		if st, err := os.Stat(filepath.Join(cand, "species")); err == nil && st.IsDir() {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "content"
}
