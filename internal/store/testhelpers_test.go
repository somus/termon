package store_test

import (
	"path/filepath"
	"testing"

	"termon.sh/internal/content"
	"termon.sh/internal/game"
	"termon.sh/internal/store"
)

func testContent(t *testing.T) *content.Set {
	t.Helper()
	set, err := content.Load(filepath.Join("..", "..", "content"))
	if err != nil {
		t.Fatal(err)
	}
	return set
}

// testStoreWithContent returns a content-backed SQLite store in a temp dir.
func testStoreWithContent(t *testing.T) *store.SQLiteStore {
	t.Helper()
	set := testContent(t)
	s, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "termon.db"))
	if err != nil {
		t.Fatal(err)
	}
	s.UseContent(set)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func onboardStarterSave(handle, species string) *game.Save {
	return &game.Save{
		Handle:     handle,
		Collection: []game.Monster{{Species: species}},
	}
}
