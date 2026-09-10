package tui

import (
	"strings"
	"testing"

	"termon.sh/internal/server"
)

func TestSignalBoardListBrowsesAllFamiliesInStableOrder(t *testing.T) {
	families := make([]server.ExpeditionFamilyCard, 24)
	for i := range families {
		families[i] = server.ExpeditionFamilyCard{
			Slug: "family", Name: "Family " + string(rune('A'+i)), Type: "Data", Theme: "support", Index: i,
			Featured: i == 2 || i == 9 || i == 18,
		}
	}
	m := Model{signalBoard: signalBoardModel{board: server.SignalBoardMsg{Families: families}, card: 23}}
	view := m.signalBoardCards(100, 26)
	if !strings.Contains(view, "24   FAMILY X") || !strings.Contains(view, "10 ★ FAMILY J") || !strings.Contains(view, "19 ★ FAMILY S") {
		t.Fatalf("list did not retain stable catalog rows:\n%s", view)
	}
	if !strings.Contains(view, "9–24 of 24") {
		t.Fatalf("list did not scroll to focused row:\n%s", view)
	}
	if got := signalBoardNext(23, len(families)); got != 0 {
		t.Fatalf("next = %d, want 0", got)
	}
	if got := signalBoardPrevious(0, len(families)); got != 23 {
		t.Fatalf("previous = %d, want 23", got)
	}
}
