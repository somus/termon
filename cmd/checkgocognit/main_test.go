package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFindSuppressions(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		wantCount   int
		wantComment string
	}{
		{
			name: "native FuncDecl doc directive",
			source: `package fixture

//gocognit:ignore
func target() {}
`,
			wantCount:   1,
			wantComment: "//gocognit:ignore",
		},
		{
			name: "bare nolint",
			source: `package fixture

//nolint
func target() {}
`,
			wantCount:   1,
			wantComment: "//nolint",
		},
		{
			name: "bare nolint with reason",
			source: `package fixture

func target() {} // nolint // reason
`,
			wantCount:   1,
			wantComment: "// nolint // reason",
		},
		{
			name: "mixed-case all",
			source: `package fixture

//nolint:AlL
func target() {}
`,
			wantCount:   1,
			wantComment: "//nolint:AlL",
		},
		{
			name: "lowercase all prefix matches pinned parser",
			source: `package fixture

//nolint:alligator
func target() {}
`,
			wantCount:   1,
			wantComment: "//nolint:alligator",
		},
		{
			name: "mixed-case all prefix does not match pinned parser",
			source: `package fixture

//nolint:Alligator
func target() {}
`,
		},
		{
			name: "spaced mixed-case gocognit",
			source: `package fixture

//nolint:  revive, GoCoGnIt // reason
func target() {}
`,
			wantCount:   1,
			wantComment: "//nolint:  revive, GoCoGnIt // reason",
		},
		{
			name: "unrelated scoped nolint",
			source: `package fixture

//nolint:gosec, revive
func target() {}
`,
		},
		{
			name: "directive text in string",
			source: `package fixture

const message = "//nolint:gocognit"
`,
		},
		{
			name: "directive text in prose comment",
			source: `package fixture

// This prose mentions //nolint:gocognit without being a directive.
func target() {}
`,
		},
		{
			name: "detached native comment",
			source: `package fixture

//gocognit:ignore

func target() {}
`,
		},
		{
			name: "non-native spaced comment",
			source: `package fixture

// gocognit:ignore
func target() {}
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := findSuppressions("fixture.go", []byte(test.source))
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != test.wantCount {
				t.Fatalf("findSuppressions() returned %d findings, want %d: %#v", len(got), test.wantCount, got)
			}
			if test.wantCount != 0 && got[0].directive != test.wantComment {
				t.Fatalf("directive = %q, want %q", got[0].directive, test.wantComment)
			}
		})
	}
}

func TestRunReadsIndexedContent(t *testing.T) {
	t.Run("worktree suppressions absent from index", func(t *testing.T) {
		repoDir, trackedPath := newFixtureRepo(t, "package fixture\n")
		writeFile(t, trackedPath, "package fixture\n//nolint:GOCognit\nfunc tracked() {}\n")
		writeFile(t, filepath.Join(repoDir, "scratch.go"), "package fixture\n//nolint:all\nfunc scratch() {}\n")

		var output bytes.Buffer
		if err := run(repoDir, &output); err != nil {
			t.Fatalf("run() rejected worktree-only suppressions: %v\n%s", err, output.String())
		}
	})

	t.Run("indexed suppression absent from worktree", func(t *testing.T) {
		repoDir, trackedPath := newFixtureRepo(t, "package fixture\n//nolint:GOCognit\nfunc tracked() {}\n")
		writeFile(t, trackedPath, "package fixture\n")

		var output bytes.Buffer
		err := run(repoDir, &output)
		if !errors.Is(err, errSuppressionFound) {
			t.Fatalf("run() error = %v, want %v", err, errSuppressionFound)
		}
		if !bytes.Contains(output.Bytes(), []byte("tracked.go:2:1")) {
			t.Fatalf("run() output missing indexed finding: %q", output.String())
		}
	})
}

func newFixtureRepo(t *testing.T, indexedSource string) (string, string) {
	t.Helper()
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-q")
	trackedPath := filepath.Join(repoDir, "tracked.go")
	writeFile(t, trackedPath, indexedSource)
	runGit(t, repoDir, "add", "tracked.go")

	return repoDir, trackedPath
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
