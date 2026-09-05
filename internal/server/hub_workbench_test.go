package server

import (
	"testing"

	"termon.sh/internal/game"
)

func TestHubSetNicknamePersistsThroughLoad(t *testing.T) {
	h := testHub(t)
	onboardTrainer(t, h, "wb-nick", "rootkit")
	var got SaveMsg
	h.Attach("wb-nick", func(msg any) {
		if m, ok := msg.(SaveMsg); ok {
			got = m
		}
	}, func() {})
	id := mustWorkbenchLoad(t, h, "wb-nick").Collection[0].ID
	if err := h.SetNickname("wb-nick", id, "Moss"); err != nil {
		t.Fatal(err)
	}
	if got.Save == nil || got.Save.Collection[0].Nickname != "Moss" {
		t.Fatalf("live SaveMsg = %+v", got.Save)
	}
	loaded := mustWorkbenchLoad(t, h, "wb-nick")
	if loaded.Collection[0].Nickname != "Moss" {
		t.Fatalf("loaded nickname = %q", loaded.Collection[0].Nickname)
	}
}

func TestHubSetBattleLoadoutPersistsThroughLoad(t *testing.T) {
	h := testHub(t)
	onboardTrainer(t, h, "wb-load", "rootkit")
	h.Attach("wb-load", func(any) {}, func() {})
	sv := mustWorkbenchLoad(t, h, "wb-load")
	id := sv.Collection[0].ID
	moves := sv.Collection[0].MoveLibrary[:1]
	if err := h.SetBattleLoadout("wb-load", id, moves, nil); err != nil {
		t.Fatal(err)
	}
	loaded := mustWorkbenchLoad(t, h, "wb-load")
	if len(loaded.Collection[0].BattleLoadout) != 1 || loaded.Collection[0].BattleLoadout[0] != moves[0] {
		t.Fatalf("loadout = %v, want %v", loaded.Collection[0].BattleLoadout, moves)
	}
}

func mustWorkbenchLoad(t *testing.T, h *Hub, id string) *game.Save {
	t.Helper()
	sv, err := h.Load(id)
	if err != nil || sv == nil {
		t.Fatalf("load %s: %v %v", id, sv, err)
	}
	return sv
}
