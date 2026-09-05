package onboard

import (
	"testing"

	"termon.sh/internal/content"
)

func TestNewSaveStarterLoadouts(t *testing.T) {
	set, err := content.Load("../../content")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"rootkit":   {"root_access", "chmod", "sudo", "setuid"},
		"emberbyte": {"burn_in", "xor_fold", "crc32", "salted_hash"},
		"aquabit":   {"ping_flood", "hop_count", "checksum", "jumbo_frame"},
	}
	for slug, moves := range want {
		save, err := NewSave(set, "swift-otter-12", slug)
		if err != nil {
			t.Fatalf("%s: %v", slug, err)
		}
		if save.Handle != "swift-otter-12" {
			t.Fatalf("%s identity/handle not persisted", slug)
		}
		if len(save.Collection) != 1 {
			t.Fatalf("%s collection length %d", slug, len(save.Collection))
		}
		got := save.Collection[0].BattleLoadout
		if len(got) != 4 {
			t.Fatalf("%s loadout %v, want 4 moves", slug, got)
		}
		for i, m := range moves {
			if got[i] != m {
				t.Fatalf("%s loadout[%d] = %s, want %s", slug, i, got[i], m)
			}
		}
	}
}

func TestValidHandle(t *testing.T) {
	if !ValidHandle("su") || !ValidHandle("swift-otter-12") {
		t.Fatal("expected simple handles to pass")
	}
	if ValidHandle("") || ValidHandle("has space") || ValidHandle("way-too-long-handle-name") {
		t.Fatal("expected invalid handles to fail")
	}
}
