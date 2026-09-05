// Package balance runs reproducible Balance Runs against the authoritative engine.
package balance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"termon.sh/internal/content"
	"termon.sh/internal/dojo"
)

// Run-wide identity and scenario defaults recorded with every Balance Run
// snapshot, so reports are comparable across versions.
const (
	// RulesRevision identifies the balance scenario contract.
	RulesRevision = "party-battles-v1"
	// PackageIdentity is recorded in run snapshots.
	PackageIdentity = "termon.sh/internal/balance"
	// DefaultSeedBase is the recorded corpus identity base.
	DefaultSeedBase = 1
	// DefaultCorpusSize is the launch seed corpus size.
	DefaultCorpusSize = 1024
	// DefaultMaxTurns caps non-terminating battles.
	DefaultMaxTurns = 200
)

// ReferenceTeam is one three-Family anchor team from balance-methodology.md.
type ReferenceTeam struct {
	Name     string    `json:"name"`
	Families [3]string `json:"families"`
}

// ReferenceTeams are the eight launch anchor teams.
var ReferenceTeams = []ReferenceTeam{
	{Name: "starter-balance", Families: [3]string{"rootkit", "emberbyte", "aquabit"}},
	{Name: "alternate-balance", Families: [3]string{"zaplet", "spamlet", "chippunk"}},
	{Name: "bulky-control", Families: [3]string{"rootanami", "flowcell", "bloatware"}},
	{Name: "fast-pressure", Families: [3]string{"sproutware", "wickware", "mistcache"}},
	{Name: "physical-pressure", Families: [3]string{"thornpatch", "gushkit", "joulpup"}},
	{Name: "bruiser-core", Families: [3]string{"cindernode", "amperent", "coghound"}},
	{Name: "mixed-endurance", Families: [3]string{"mossmuff", "splashscreen", "surgetail"}},
	{Name: "specialist-pressure", Families: [3]string{"scorchip", "wormate", "servoboar"}},
}

// NaturalCheckpoints are progression levels for natural battles.
var NaturalCheckpoints = []int{1, 14, 24, 30, 40, 50}

// DefaultPolicy returns the Pivot-equivalent Dojo policy (Rival tier).
func DefaultPolicy() dojo.PolicyConfig {
	return dojo.TierConfig(dojo.TierRival)
}

// ContentRevisionFromDir hashes species, moves, and types JSON in sorted order.
func ContentRevisionFromDir(dir string) (string, error) {
	h := sha256.New()
	for _, sub := range []string{"species", "moves", "types"} {
		root := filepath.Join(dir, sub)
		var files []string
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".json") {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("balance: missing %s under %s", sub, dir)
			}
			return "", err
		}
		slices.Sort(files)
		for _, path := range files {
			raw, err := os.ReadFile(path) //nolint:gosec // operator-controlled content pack path
			if err != nil {
				return "", err
			}
			_, _ = h.Write([]byte(sub + "/"))
			_, _ = h.Write([]byte(filepath.Base(path)))
			_, _ = h.Write(raw)
		}
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:16]), nil
}

// ContentRevision hashes a loaded content pack directory discovered from set paths.
func ContentRevision(set *content.Set) string {
	if set == nil {
		return ""
	}
	// Loaded sets do not retain dir; callers should prefer ContentRevisionFromDir.
	_ = set
	return ""
}
