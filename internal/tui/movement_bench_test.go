package tui

import (
	"testing"

	"termon.sh/internal/content"
	"termon.sh/internal/gametest"
	"termon.sh/internal/server"
	"termon.sh/internal/store"
)

func BenchmarkLobbyMovement(b *testing.B) {
	set, err := content.Load("../../content")
	if err != nil {
		b.Fatal(err)
	}
	saves := store.NewMemoryStore()
	saves.UseContent(set)
	hub := server.NewHub(set, saves, server.Admission{OpenAccess: true, RegistrationsPerIP: -1})
	trainer, err := hub.Authenticate("probe", "127.0.0.1")
	if err != nil {
		b.Fatal(err)
	}
	if _, err := hub.CompleteOnboard(trainer.ID, "probe", "rootkit"); err != nil {
		b.Fatal(err)
	}
	save := gametest.FillParty(b, saves, trainer.ID)
	m := New(trainer.ID, save, set, hub)
	m.width, m.height = 120, 40
	m = drive(m, hub.Snapshot(trainer.ID))
	before := m.frameBuilds
	left := true
	b.ReportAllocs()
	for b.Loop() {
		key := "d"
		if left {
			key = "a"
		}
		left = !left
		next, cmd := m.Update(press(key))
		m = next.(Model)
		// Also measures the old asynchronous API: its command result caused
		// another frame build after the key had already repainted old state.
		if cmd != nil {
			m = drive(m, cmd())
		}
		_ = m.View()
	}
	b.ReportMetric(float64(m.frameBuilds-before)/float64(b.N), "frames/move")
}
