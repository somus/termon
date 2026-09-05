package balance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"termon.sh/internal/balance"
	"termon.sh/internal/content"
)

func loadContent(t *testing.T) *content.Set {
	t.Helper()
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestContentRevisionStableForSamePack(t *testing.T) {
	dir := filepath.Join("testdata", "content")
	rev1, err := balance.ContentRevisionFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	rev2, err := balance.ContentRevisionFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rev1 == "" {
		t.Fatal("empty revision")
	}
	if rev1 != rev2 {
		t.Fatalf("revision changed without edits: %q vs %q", rev1, rev2)
	}
}

func TestContentRevisionChangesWhenSpeciesEdited(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join("testdata", "content")
	for _, sub := range []string{"species", "moves", "types"} {
		if err := copyDir(filepath.Join(src, sub), filepath.Join(dir, sub)); err != nil {
			t.Fatal(err)
		}
	}
	before, err := balance.ContentRevisionFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(dir, "species", "spark.json")
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := balance.ContentRevisionFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatalf("revision unchanged after species edit: %q", before)
	}
}

func TestRecordedSeedReproducesWinner(t *testing.T) {
	set := loadContent(t)
	cfg := balance.Config{
		Set:       set,
		Seeds:     []uint64{balance.CorpusSeed(1, 1)},
		Policy:    balance.DefaultPolicy(),
		MaxTurns:  balance.DefaultMaxTurns,
		Rules:     balance.RulesRevision,
		ContentID: balance.ContentRevision(set),
	}
	teamA := balance.ReferenceTeams[0]
	teamB := balance.ReferenceTeams[1]
	sc := balance.NormalizedScenario("repro", teamA, teamB, 0)
	sc.Seed = balance.CorpusSeed(1, 1)

	r1, err := balance.RunScenario(cfg, sc)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := balance.RunScenario(cfg, sc)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Winner != r2.Winner {
		t.Fatalf("winner drift: %q vs %q", r1.Winner, r2.Winner)
	}
	if r1.Reason != r2.Reason {
		t.Fatalf("reason drift: %q vs %q", r1.Reason, r2.Reason)
	}
}

func TestNonMirrorRunsTwiceWithSideAndOrderSwap(t *testing.T) {
	set := loadContent(t)
	cfg := balance.Config{
		Set:      set,
		Seeds:    []uint64{99},
		Policy:   balance.DefaultPolicy(),
		MaxTurns: balance.DefaultMaxTurns,
	}
	teamA := balance.ReferenceTeams[0]
	teamB := balance.ReferenceTeams[1]
	pair := balance.PairedNormalizedRuns(cfg, teamA, teamB, 0, cfg.Seeds[0])
	if len(pair) != 2 {
		t.Fatalf("paired runs = %d, want 2", len(pair))
	}
	if pair[0].SideA.Trainer == pair[1].SideA.Trainer {
		t.Fatal("expected engine-side swap between paired runs")
	}
	if pair[0].PartyOrderSwapped == pair[1].PartyOrderSwapped {
		t.Fatal("expected party-order flag to differ between paired runs")
	}
}

func TestFailedGateReportIncludesScenarioSeedTeams(t *testing.T) {
	report := balance.FailedGateReport{
		Gate:     balance.GateMirrorWinRate,
		Scenario: "mirror/starter-balance",
		Seed:     42,
		TeamA:    balance.ReferenceTeams[0],
		TeamB:    balance.ReferenceTeams[0],
		Loadouts: map[string][]string{"a-lead": {"jab"}},
		ActionLog: []balance.ActionEntry{
			{Turn: 1, Trainer: "balance:a", Kind: "move", Move: "jab"},
		},
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded balance.FailedGateReport
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Scenario != report.Scenario || decoded.Seed != report.Seed {
		t.Fatalf("decoded metadata mismatch: %+v", decoded)
	}
	if decoded.TeamA.Name != report.TeamA.Name || decoded.TeamB.Name != report.TeamB.Name {
		t.Fatal("teams not preserved in report")
	}
}

func TestRunUsesSingleSeedAndPair(t *testing.T) {
	set := loadContent(t)
	rev, err := balance.ContentRevisionFromDir(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := balance.Run(balance.Config{
		Set:            set,
		Seeds:          []uint64{7},
		Policy:         balance.DefaultPolicy(),
		MaxTurns:       balance.DefaultMaxTurns,
		ContentID:      rev,
		NormalizedOnly: true,
		TeamLimit:      2,
		FailGates:      false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Snapshot.SeedCount != 1 {
		t.Fatalf("seed count = %d, want 1", out.Snapshot.SeedCount)
	}
	if out.BattlesRun < 1 {
		t.Fatal("expected at least one battle")
	}
	if out.Snapshot.ContentRevision == "" {
		t.Fatal("missing content revision in snapshot")
	}
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
