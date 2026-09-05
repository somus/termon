package sshload

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"termon.sh/internal/content"
	"termon.sh/internal/gametest"
	"termon.sh/internal/identity"
	"termon.sh/internal/server"
	"termon.sh/internal/store"
)

// TestPrepareLatencyFixture is an opt-in operator fixture, not a server bypass.
// It creates a NEW directory and SQLite database with 32 returning Trainers.
// Never point the server or this test at production data.
func TestPrepareLatencyFixture(t *testing.T) {
	dir := os.Getenv("TERMON_LATENCY_FIXTURE")
	if dir == "" {
		t.Skip("set TERMON_LATENCY_FIXTURE to a new directory to prepare SSH probes")
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	saves, err := store.OpenSQLiteStore(filepath.Join(dir, "termon.db"), store.SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := saves.Close(); err != nil {
			t.Error(err)
		}
	}()
	saves.UseContent(set)
	hub := server.NewHub(set, saves, server.Admission{OpenAccess: true, RegistrationsPerIP: -1})
	for i := range 32 {
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		signer, err := gossh.NewSignerFromKey(key)
		if err != nil {
			t.Fatal(err)
		}
		block, err := gossh.MarshalPrivateKey(key, "ephemeral latency fixture")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("key-%02d", i)), pem.EncodeToMemory(block), 0o600); err != nil {
			t.Fatal(err)
		}
		trainer, err := hub.Authenticate(identity.Hash(signer.PublicKey().Marshal()), "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := hub.CompleteOnboard(trainer.ID, fmt.Sprintf("probe-%02d", i), "rootkit"); err != nil {
			t.Fatal(err)
		}
		gametest.FillParty(t, saves, trainer.ID)
	}
}
