package tui

import (
	"sync"
	"testing"

	"termon.sh/internal/content"
)

var (
	loadSetOnce sync.Once
	loadedSet   *content.Set
	loadSetErr  error
)

func loadSet(t *testing.T) *content.Set {
	t.Helper()
	loadSetOnce.Do(func() {
		loadedSet, loadSetErr = content.Load("../../content")
	})
	if loadSetErr != nil {
		t.Fatal(loadSetErr)
	}
	return loadedSet
}
