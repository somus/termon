package dojo

import (
	"errors"

	"termon.sh/internal/content"
)

// ValidateBoot runs Dojo content checks at server startup.
// Tables live in Go until content/dojo/*.json ships; see docs/design/dojo-policy.md.
func ValidateBoot(set *content.Set) error {
	if set == nil {
		return errors.New("dojo: nil content")
	}
	if err := ValidateSparringPool(set); err != nil {
		return err
	}
	if err := ValidateDailyFixtures(set); err != nil {
		return err
	}
	return nil
}
