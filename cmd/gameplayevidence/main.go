// Command gameplayevidence runs supporting-gameplay proof matrices separately
// from the competitive Balance Run. Failed or unproven cases exit nonzero.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"termon.sh/internal/balance"
	"termon.sh/internal/content"
)

type report struct {
	Suite           string `json:"suite"`
	ContentRevision string `json:"content_revision"`
	SeedBase        uint64 `json:"seed_base"`
	SeedCount       int    `json:"seed_count"`
	Passed          bool   `json:"passed"`
	Error           string `json:"error,omitempty"`
	Evidence        any    `json:"evidence"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	suite := flag.String("suite", "capture", "evidence suite: capture, dojo, daily, or counterplay")
	dir := flag.String("content", "./content", "content pack directory")
	output := flag.String("report", "", "required JSON report path")
	count := flag.Int("seeds", balance.DefaultCorpusSize, "Dojo seed count; capture uses explicit roll trajectories")
	base := flag.Uint64("seed-base", balance.DefaultSeedBase, "Dojo fixed corpus base")
	flag.Parse()
	if *output == "" {
		return errors.New("gameplayevidence: -report is required")
	}
	if *suite != "capture" && *suite != "dojo" && *suite != "daily" && *suite != "counterplay" {
		return fmt.Errorf("gameplayevidence: unknown suite %q", *suite)
	}
	if *count < 1 {
		return errors.New("gameplayevidence: seeds must be positive")
	}
	set, err := content.Load(*dir)
	if err != nil {
		return err
	}
	revision, err := balance.ContentRevisionFromDir(*dir)
	if err != nil {
		return err
	}
	result := report{Suite: *suite, ContentRevision: revision}
	switch *suite {
	case "capture":
		evidence := balance.RunCaptureMatrix(set)
		result.Evidence, result.Passed = evidence, evidence.Passed
	case "dojo":
		result.SeedBase, result.SeedCount = *base, *count
		evidence, runErr := balance.RunDojoMatrix(set, balance.CorpusSeeds(*base, *count))
		result.Evidence, result.Passed = evidence, evidence.Passed && runErr == nil
		if runErr != nil {
			result.Error = runErr.Error()
		}
	case "daily":
		evidence, runErr := balance.RunDailyMatrix(set)
		result.Evidence, result.Passed = evidence, evidence.Passed && runErr == nil
		if runErr != nil {
			result.Error = runErr.Error()
		}
	case "counterplay":
		result.SeedBase, result.SeedCount = *base, *count
		evidence, runErr := balance.RunCounterplay(set, balance.CorpusSeeds(*base, *count))
		result.Evidence, result.Passed = evidence, evidence.Passed && runErr == nil
		if runErr != nil {
			result.Error = runErr.Error()
		}
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(*output, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if !result.Passed {
		return fmt.Errorf("gameplayevidence: %s contains failed or unproven cases; see %s", *suite, *output)
	}
	return nil
}
