package tui

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"termon.sh/internal/game"
	"termon.sh/internal/gametest"
	"termon.sh/internal/lobby"
	"termon.sh/internal/server"
)

func fullPartyTestSave() *game.Save {
	moves := []string{"root_access", "chmod", "sudo", "setuid"}
	lead := gametest.Starter("test-rootkit", "rootkit", moves)
	second := gametest.Starter("slot-2", "mistcache", moves)
	third := gametest.Starter("slot-3", "wickware", moves)
	return &game.Save{
		Handle:     "alpha",
		Collection: []game.Monster{lead, second, third},
		Party:      [3]string{lead.ID, second.ID, third.ID},
	}
}

func TestLobbyChromeHasHeaderAndFooter(t *testing.T) {
	m := New("aaa", fullPartyTestSave(), nil, nil)
	m.save.Wins = 2
	m.save.Losses = 1
	m.save.Handle = "cosmic-raven-42"
	m.width, m.height = 100, 24
	m.screen = screenLobby
	m.snap = server.SnapshotMsg{
		You: lobby.Presence{Handle: "cosmic-raven-42", Species: "rootkit", X: 2, Y: 2},
		Others: []lobby.Presence{
			{Handle: "bravo", Species: "emberbyte", X: 4, Y: 3},
		},
	}
	v := ansi.Strip(m.View().Content)
	for _, want := range []string{"termon", "Dojo", "inside", "cosmic-raven-42", "ROOTKIT", "2–1", "walk", "challenge", "party"} {
		if !strings.Contains(v, want) {
			t.Fatalf("missing %q", want)
		}
	}
	lines := strings.Split(strings.TrimRight(v, "\n"), "\n")
	if !strings.HasPrefix(lines[0], "╭") || !strings.HasPrefix(lines[len(lines)-1], "╰") {
		t.Fatal("dojo should sit inside a rounded page frame")
	}
	if !strings.Contains(lines[0], "termon") {
		t.Fatal("brand belongs on the top border")
	}
	if !strings.Contains(lines[len(lines)-1], "leave") {
		t.Fatal("shortcuts belong on the bottom border")
	}
}

func TestFullDojoFitsEightyByTwentyFour(t *testing.T) {
	m := New("trainer-01", gametest.SaveWithStarter("test-rootkit", "rootkit", []string{"root_access", "chmod", "sudo", "setuid"}), nil, nil)
	m.save.Handle = "trainer-01"
	m.width, m.height = 80, 24
	m.screen = screenLobby
	m.snap = server.SnapshotMsg{
		You:  lobby.Presence{Hash: "trainer-01", Handle: "trainer-01", X: 16, Y: 16},
		Dojo: 1,
	}
	for i := 2; i <= lobby.Capacity; i++ {
		m.snap.Others = append(m.snap.Others, lobby.Presence{
			Hash: fmt.Sprintf("trainer-%02d", i), Handle: fmt.Sprintf("trainer-%02d", i),
			X: 15 + i, Y: 16,
		})
	}

	view := ansi.Strip(m.View().Content)
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) != 24 {
		t.Fatalf("Lobby height = %d, want 24", len(lines))
	}
	for row, line := range lines {
		if width := ansi.StringWidth(line); width != 80 {
			t.Fatalf("row %d width = %d, want 80", row, width)
		}
	}
	if strings.Contains(view, "trainer-32") {
		t.Fatal("full Dojo rendered an unbounded all-Trainer name list")
	}
	if !strings.Contains(view, "Dojo 1 · 32 inside") {
		t.Fatal("header does not identify the Dojo and bounded population")
	}
}

func TestCameraLobbyRendersTrainerCardsMasterAndBubble(t *testing.T) {
	m := New("aaa", gametest.SaveWithStarter("test-rootkit", "rootkit", []string{"root_access", "chmod", "sudo", "setuid"}), nil, nil)
	m.width, m.height = 80, 24
	m.screen = screenLobby
	m.snap = server.SnapshotMsg{
		You: lobby.Presence{
			Hash: "aaa", Handle: "alpha", Species: "rootkit",
			X: lobby.MasterX - 1, Y: lobby.MasterY,
		},
		Others: []lobby.Presence{{
			Hash: "bbb", Handle: "bravo", Species: "aquabit",
			X: lobby.MasterX + 2, Y: lobby.MasterY + 1, Emote: "gl hf",
		}},
		Dojo:    1,
		Context: "Master Sable waits.",
	}

	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"MASTER", "alpha", "bravo", "gl hf", "Master Sable waits"} {
		if !strings.Contains(view, want) {
			t.Fatalf("camera Lobby missing %q\n%s", want, view)
		}
	}
	if strings.Contains(view, "nearby:") {
		t.Fatal("camera Lobby should attach names to Trainer models")
	}
}

func overlayLobbySnap() server.SnapshotMsg {
	return server.SnapshotMsg{
		You: lobby.Presence{
			Hash: "aaa", Handle: "alpha", Species: "rootkit", X: 5, Y: 1,
		},
		Dojo:    1,
		Context: "Master Sable waits.",
	}
}

func assertOverlayShowsLobby(t *testing.T, view string, card ...string) {
	t.Helper()
	for _, want := range card {
		if !strings.Contains(view, want) {
			t.Fatalf("overlay missing card %q\n%s", want, view)
		}
	}
	for _, want := range []string{"Master Sable waits"} {
		if !strings.Contains(view, want) {
			t.Fatalf("overlay missing previous screen %q\n%s", want, view)
		}
	}
}

func TestDojoMenuOverlaysLobby(t *testing.T) {
	m := New("aaa", gametest.SaveWithStarter("test-rootkit", "rootkit", []string{"root_access", "chmod", "sudo", "setuid"}), nil, nil)
	m.width, m.height = 80, 24
	m.screen = screenDojoMenu
	m.dojo = dojoMenuModel{menu: server.DojoMenuMsg{ServerDay: "2026-08-31", Daily: server.DailyMenuInfo{ID: "preservation"}}}
	m.snap = overlayLobbySnap()
	view := ansi.Strip(m.View().Content)
	assertOverlayShowsLobby(t, view, "DOJO MASTER", "Lesson 1", "preservation")
	if !strings.Contains(view, "▓━━━━━━━▓") && !strings.Contains(view, "◆") {
		t.Fatalf("dojo menu replaced the floor instead of overlaying it\n%s", view)
	}
}

func TestDojoMenuHidesLessonsAfterOnboarding(t *testing.T) {
	m := New("aaa", fullPartyTestSave(), nil, nil)
	m.width, m.height = 80, 24
	m.screen = screenDojoMenu
	m.dojo = dojoMenuModel{menu: server.DojoMenuMsg{
		ServerDay:   "2026-09-03",
		Lesson1Done: true,
		Lesson2Done: true,
		Daily:       server.DailyMenuInfo{ID: "type_read"},
	}}
	m.snap = overlayLobbySnap()
	view := ansi.Strip(m.View().Content)
	assertOverlayShowsLobby(t, view, "DOJO MASTER", "Sparring", "type_read")
	if strings.Contains(view, "Lesson 1") || strings.Contains(view, "Lesson 2") {
		t.Fatalf("completed Capture Lessons should leave the Dojo menu\n%s", view)
	}

	next, _ := m.Update(press("enter"))
	m = next.(Model)
	if m.dojo.view != dojoViewSparringTiers {
		t.Fatalf("first menu item should open Sparring, view=%v", m.dojo.view)
	}
}

func TestSparringPreviewExplainsDailyRosterAndRemix(t *testing.T) {
	preview := server.SparringPreviewMsg{
		Tier: "apprentice", ServerDay: "2026-09-04", FirstClear: true, XP: 65,
		PolicySummary: "weighted Type effectiveness",
		Slots:         []server.SparringPreviewSlot{{Slot: 1, Species: "scorchip", Level: 1, Type: "thermal", Role: "favorable"}},
	}
	first := ansi.Strip(renderDojoSparringPreview(preview))
	if !strings.Contains(first, "daily roster") || !strings.Contains(first, "shared across tiers") {
		t.Fatalf("first-clear preview does not explain the daily roster:\n%s", first)
	}
	if strings.Contains(first, "R remix") {
		t.Fatalf("first-clear preview offered a no-reward remix:\n%s", first)
	}

	preview.FirstClear = false
	preview.Remix = 2
	replay := ansi.Strip(renderDojoSparringPreview(preview))
	if !strings.Contains(replay, "practice remix 2") || !strings.Contains(replay, "R remix") ||
		!strings.Contains(replay, "practice only") {
		t.Fatalf("cleared preview does not offer a practice remix:\n%s", replay)
	}
}

func TestQueueAndProgressionOverlayLobby(t *testing.T) {
	base := func(scr screen) Model {
		m := New("aaa", gametest.SaveWithStarter("test-rootkit", "rootkit", []string{"root_access", "chmod", "sudo", "setuid"}), nil, nil)
		m.width, m.height = 80, 24
		m.screen = scr
		m.snap = overlayLobbySnap()
		return m
	}

	t.Run("queue editor", func(t *testing.T) {
		m := base(screenQueueEditor)
		assertOverlayShowsLobby(t, ansi.Strip(m.View().Content), "FIND BATTLE")
	})
	t.Run("queue wait", func(t *testing.T) {
		m := base(screenQueue)
		m.queue.Position, m.queue.Waiting = 1, 2
		assertOverlayShowsLobby(t, ansi.Strip(m.View().Content), "waiting for another trainer")
	})
	t.Run("progression", func(t *testing.T) {
		m := base(screenProgression)
		assertOverlayShowsLobby(t, ansi.Strip(m.View().Content), "PROGRESSION SUMMARY")
	})
}

func TestDojoMenuStaysOpenOnSnapshot(t *testing.T) {
	m := New("aaa", gametest.SaveWithStarter("test-rootkit", "rootkit", []string{"root_access", "chmod", "sudo", "setuid"}), nil, nil)
	m.width, m.height = 80, 24
	m.screen = screenDojoMenu
	m.dojo = dojoMenuModel{menu: server.DojoMenuMsg{ServerDay: "2026-08-31"}}
	m.snap = overlayLobbySnap()
	next := drive(m, server.SnapshotMsg{
		You: overlayLobbySnap().You,
		Others: []lobby.Presence{{
			Hash: "bbb", Handle: "bravo", Species: "aquabit", X: 8, Y: 2,
		}},
		Dojo:    1,
		Context: "Master Sable waits.",
	})
	if next.screen != screenDojoMenu {
		t.Fatalf("screen = %v, want dojo menu", next.screen)
	}
	view := ansi.Strip(next.View().Content)
	assertOverlayShowsLobby(t, view, "DOJO MASTER")
}

func TestTrainerLayerOverridesPassableDojoObject(t *testing.T) {
	view := ansi.Strip(renderLobby(server.SnapshotMsg{
		You: lobby.Presence{
			Hash: "aaa", Handle: "alpha", Species: "rootkit", X: 18, Y: 11,
		},
		Context: "The old scroll reads: patience wins turns that speed cannot.",
	}, 78, 22))
	if !strings.Contains(view, "alpha") || !strings.Contains(view, "The old scroll reads") {
		t.Fatalf("Trainer or discovery missing\n%s", view)
	}
	if strings.Contains(view, "≋≋≋") {
		t.Fatal("passable scroll rendered through the Trainer layer")
	}
}

func TestFurnishedDojoLandmarksRender(t *testing.T) {
	layout := lobby.SharedLayout()
	wants := []struct {
		name string
		x, y int
		text string
	}{
		{"banner", 5, 1, "◆"},
		{"wall scroll", 11, 1, "≋≋≋"},
		{"lantern", 18, 1, "◇"},
		{"crest", 24, 1, "╲◆╱"},
		{"trophy cabinet", 3, 2, "♜"},
		{"badge display", 7, 2, "◆ ◇ ◆"},
		{"plant", 3, 5, "♣♣♣"},
		{"bench", 17, 2, "┬┬┬┬"},
		{"cubbies", 7, 10, "□ □ □"},
		{"gear rack", 6, 9, "╱╱╱╱╱"},
		{"practice pads", 5, 4, "▣ ▣"},
		{"water urn", 9, 4, "≋"},
		{"record terminal", 4, 7, "01>"},
		{"notice board", 9, 8, "• ─ •"},
		{"first aid", 43, 4, "+++"},
		{"towel station", 39, 4, "≋≋ ≋≋"},
	}
	for _, want := range wants {
		t.Run(want.name, func(t *testing.T) {
			art := ansi.Strip(renderDojoTile(layout, want.x, want.y))
			if !strings.Contains(art, want.text) {
				t.Fatalf("landmark at (%d,%d) missing %q\n%s", want.x, want.y, want.text, art)
			}
			lines := strings.Split(art, "\n")
			if len(lines) != lobbyTileH {
				t.Fatalf("landmark height = %d, want %d", len(lines), lobbyTileH)
			}
			for row, line := range lines {
				if width := ansi.StringWidth(line); width != lobbyTileW {
					t.Fatalf("landmark row %d width = %d, want %d", row, width, lobbyTileW)
				}
			}
		})
	}
}

func TestDojoPillarHasStraightSides(t *testing.T) {
	art := ansi.Strip(renderDojoTile(lobby.SharedLayout(), 12, 4))
	want := []string{
		"   ╥═╥   ",
		"   ║▓║   ",
		"   ║█║   ",
		"   ╨═╨   ",
	}
	if got := strings.Split(art, "\n"); !slices.Equal(got, want) {
		t.Fatalf("pillar rows = %q, want aligned rows %q", got, want)
	}
}

func TestDojoArchitectureAndCourtRender(t *testing.T) {
	layout := lobby.SharedLayout()
	tests := []struct {
		name string
		x, y int
		text string
	}{
		{"north roof", 2, 0, "▓━━━━━━━▓"},
		{"north wall face", 2, 1, "╱╲╱╲╱╲╱"},
		{"west wall", 0, 6, "        ┃"},
		{"east wall", lobby.Width - 1, 6, "┃"},
		{"south wall", 2, lobby.Height - 1, "━━━━━━━━━"},
		{"court top border", 15, lobby.CourtMinY, "━━━━━━━━━"},
		{"court west border", lobby.CourtMinX, lobby.CourtCenterY, "┃"},
		{"court east border", lobby.CourtMaxX, lobby.CourtCenterY, "┃"},
		{"court start", 20, lobby.CourtCenterY, "╽"},
		{"court crest", lobby.CourtCenterX, lobby.CourtCenterY, "◇"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			art := renderDojoTile(layout, tt.x, tt.y)
			if !strings.Contains(ansi.Strip(art), tt.text) {
				t.Fatalf("tile at (%d,%d) missing %q\n%s", tt.x, tt.y, tt.text, ansi.Strip(art))
			}
			if !strings.Contains(art, "\x1b[") {
				t.Fatalf("tile at (%d,%d) has no terminal color", tt.x, tt.y)
			}
		})
	}

	northBorder := ansi.Strip(renderDojoTile(layout, lobby.CourtCenterX, lobby.CourtMinY))
	if !strings.Contains(northBorder, "━━━━━━━━━") {
		t.Fatalf("passable north court border should remain visible\n%s", northBorder)
	}

	tatami := ansi.Strip(renderDojoTile(layout, 2, 6))
	if strings.ContainsAny(tatami, "┌┐└┘│─") {
		t.Fatalf("tatami should use color-only checks without box outlines\n%s", tatami)
	}

	borderTests := []struct {
		name string
		x, y int
		want []string
	}{
		{
			name: "north-west corner",
			x:    lobby.CourtMinX,
			y:    lobby.CourtMinY,
			want: []string{"┏━━━━━━━━", "┃        ", "┃        ", "┃        "},
		},
		{
			name: "west column",
			x:    lobby.CourtMinX,
			y:    lobby.CourtCenterY,
			want: []string{"┃        ", "┃        ", "┃        ", "┃        "},
		},
		{
			name: "south-west corner",
			x:    lobby.CourtMinX,
			y:    lobby.CourtMaxY,
			want: []string{"┃        ", "┃        ", "┃        ", "┗━━━━━━━━"},
		},
		{
			name: "north-east corner",
			x:    lobby.CourtMaxX,
			y:    lobby.CourtMinY,
			want: []string{"━━━━━━━━┓", "        ┃", "        ┃", "        ┃"},
		},
		{
			name: "east column",
			x:    lobby.CourtMaxX,
			y:    lobby.CourtCenterY,
			want: []string{"        ┃", "        ┃", "        ┃", "        ┃"},
		},
		{
			name: "south-east corner",
			x:    lobby.CourtMaxX,
			y:    lobby.CourtMaxY,
			want: []string{"        ┃", "        ┃", "        ┃", "━━━━━━━━┛"},
		},
	}
	for _, tt := range borderTests {
		t.Run(tt.name+" connects", func(t *testing.T) {
			got := strings.Split(ansi.Strip(renderDojoTile(layout, tt.x, tt.y)), "\n")
			if !slices.Equal(got, tt.want) {
				t.Fatalf("border tile at (%d,%d) = %q, want %q", tt.x, tt.y, got, tt.want)
			}
		})
	}

	floorLines := strings.Split(ansi.Strip(renderDojoFloor(
		layout,
		0,
		0,
		lobby.Width,
		lobby.Height,
	)), "\n")
	westColumn := lobby.CourtMinX * lobbyTileW
	eastColumn := (lobby.CourtMaxX+1)*lobbyTileW - 1
	topRow := lobby.CourtMinY * lobbyTileH
	bottomRow := (lobby.CourtMaxY+1)*lobbyTileH - 1
	for row := topRow; row <= bottomRow; row++ {
		west, east := "┃", "┃"
		if row == topRow {
			west, east = "┏", "┓"
		}
		if row == bottomRow {
			west, east = "┗", "┛"
		}
		if got := ansi.Cut(floorLines[row], westColumn, westColumn+1); got != west {
			t.Fatalf("west court column row %d = %q, want %q", row, got, west)
		}
		if got := ansi.Cut(floorLines[row], eastColumn, eastColumn+1); got != east {
			t.Fatalf("east court column row %d = %q, want %q", row, got, east)
		}
	}

	for _, tt := range []struct {
		name string
		x, y int
	}{
		{"tatami", 2, 6},
		{"court", lobby.CourtCenterX - 2, lobby.CourtCenterY},
		{"court start", 20, lobby.CourtCenterY},
		{"court crest", lobby.CourtCenterX, lobby.CourtCenterY},
	} {
		t.Run(tt.name+" has no dotted fill", func(t *testing.T) {
			art := ansi.Strip(renderDojoTile(layout, tt.x, tt.y))
			if strings.ContainsAny(art, "·░▒") {
				t.Fatalf("floor tile at (%d,%d) contains dotted fill\n%s", tt.x, tt.y, art)
			}
		})
	}

	checkerTests := []struct {
		name  string
		art   string
		style lipgloss.Style
	}{
		{"bare floor", renderDojoTile(layout, 2, 6), dojoTatamiA},
		{"floor behind plant", renderDojoTile(layout, 3, 5), dojoPlantInk},
	}
	for _, tt := range checkerTests {
		t.Run(tt.name+" uses half-tile checks", func(t *testing.T) {
			backgroundA := stylePrefix(t, tt.style.Background(dojoTatamiA.GetBackground()).Render(" "))
			backgroundB := stylePrefix(t, tt.style.Background(dojoTatamiB.GetBackground()).Render(" "))
			if strings.Count(tt.art, backgroundA) != lobbyTileH || strings.Count(tt.art, backgroundB) != lobbyTileH {
				t.Fatalf("tile should contain both checker colors on every row\n%s", tt.art)
			}
		})
	}
}

func stylePrefix(t *testing.T, rendered string) string {
	t.Helper()
	i := strings.IndexByte(rendered, ' ')
	if i < 0 {
		t.Fatalf("styled space has no visible cell: %q", rendered)
	}
	return rendered[:i]
}

func TestTrainerWalkCycleOnlyRunsAfterPositionChanges(t *testing.T) {
	p := lobby.Presence{Hash: "aaa", Handle: "alpha", X: 2, Y: 2}
	idle := ansi.Strip(renderTrainer(p, true, false, 0, lobby.SurfaceTatami))
	if !strings.Contains(idle, "╱ ╲") {
		t.Fatalf("idle Trainer missing standing legs\n%s", idle)
	}
	if strings.Contains(idle, "╱ ┘") || strings.Contains(idle, "└ ╲") {
		t.Fatalf("idle Trainer rendered a walking frame\n%s", idle)
	}

	m := New("aaa", &game.Save{Handle: "alpha"}, nil, nil)
	m.snap = server.SnapshotMsg{You: p}
	m.markLobbyMovement(server.SnapshotMsg{You: lobby.Presence{
		Hash: "aaa", Handle: "alpha", X: 3, Y: 2,
	}})
	if m.lobbyWalk["aaa"].remaining != lobbyWalkTicks {
		t.Fatalf("walk ticks = %d, want %d", m.lobbyWalk["aaa"].remaining, lobbyWalkTicks)
	}
	walking := ansi.Strip(renderTrainer(p, true, true, 0, lobby.SurfaceTatami))
	if !strings.Contains(walking, "╱ ┘") {
		t.Fatalf("moving Trainer missing gait frame\n%s", walking)
	}
	for range lobbyWalkTicks {
		next, _ := m.Update(tickMsg{})
		m = next.(Model)
	}
	if _, walking := m.lobbyWalk["aaa"]; walking {
		t.Fatalf("walk animation did not return to idle: %v", m.lobbyWalk)
	}
}

func TestTrainerGaitFramesHaveStableCellWidth(t *testing.T) {
	p := lobby.Presence{Hash: "aaa", Handle: "alpha"}
	standing := ansi.Strip(renderTrainer(p, true, false, 0, lobby.SurfaceTatami))
	first := ansi.Strip(renderTrainer(p, true, true, 0, lobby.SurfaceTatami))
	second := ansi.Strip(renderTrainer(p, true, true, 1, lobby.SurfaceTatami))
	standingLines := strings.Split(standing, "\n")
	firstLines := strings.Split(first, "\n")
	secondLines := strings.Split(second, "\n")
	for row := range standingLines {
		want := ansi.StringWidth(standingLines[row])
		if got := ansi.StringWidth(firstLines[row]); got != want {
			t.Fatalf("first gait frame row %d width = %d, want %d", row, got, want)
		}
		if got := ansi.StringWidth(secondLines[row]); got != want {
			t.Fatalf("second gait frame row %d width = %d, want %d", row, got, want)
		}
	}
}

func TestTrainerFramesPreserveFloorBackground(t *testing.T) {
	p := lobby.Presence{Hash: "aaa", Handle: "alpha"}
	backgroundA := stylePrefix(t, selStyle.Background(dojoTatamiA.GetBackground()).Render(" "))
	backgroundB := stylePrefix(t, selStyle.Background(dojoTatamiB.GetBackground()).Render(" "))
	for _, tt := range []struct {
		name    string
		walking bool
	}{
		{name: "idle"},
		{name: "walking", walking: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			art := renderTrainer(p, true, tt.walking, 0, lobby.SurfaceTatami)
			if strings.Count(art, backgroundA) != lobbyTileH || strings.Count(art, backgroundB) != lobbyTileH {
				t.Fatalf("Trainer frame should retain both checker colors on every row\n%s", art)
			}
		})
	}

	courtBackground := stylePrefix(t, selStyle.Background(dojoCourtInk.GetBackground()).Render(" "))
	court := renderTrainer(p, true, true, 0, lobby.SurfaceCourt)
	if !strings.Contains(court, courtBackground) {
		t.Fatalf("Trainer frame should retain the court background\n%s", court)
	}
}

func TestTrainerWalkCadenceTracksMovementSpeed(t *testing.T) {
	p := lobby.Presence{Hash: "aaa", Handle: "alpha", X: 2, Y: 2}
	m := New("aaa", &game.Save{Handle: "alpha"}, nil, nil)
	m.snap = server.SnapshotMsg{You: p}
	m.lobbyLastMove = map[string]int{"aaa": 9}
	m.lobbyAge = 10
	m.markLobbyMovement(server.SnapshotMsg{You: lobby.Presence{Hash: "aaa", X: 3, Y: 2}})
	fast := m.lobbyWalk["aaa"].period

	m.snap.You.X = 3
	m.lobbyLastMove["aaa"] = 2
	m.lobbyAge = 10
	m.markLobbyMovement(server.SnapshotMsg{You: lobby.Presence{Hash: "aaa", X: 4, Y: 2}})
	slow := m.lobbyWalk["aaa"].period

	if fast != 1 || slow != 4 {
		t.Fatalf("gait periods fast=%d slow=%d, want 1 and 4", fast, slow)
	}
}

func TestEmoteBubbleHasPaddedBodyAndDownwardTail(t *testing.T) {
	bubble := ansi.Strip(renderEmoteBubble("well fought"))
	want := "╭─────────────╮\n│ well fought │\n╰──────┬──────╯"
	if bubble != want {
		t.Fatalf("emote bubble:\n%s\nwant:\n%s", bubble, want)
	}
	lines := strings.Split(bubble, "\n")
	for i, line := range lines {
		if ansi.StringWidth(line) != ansi.StringWidth(lines[0]) {
			t.Fatalf("bubble row %d width = %d, want %d", i, ansi.StringWidth(line), ansi.StringWidth(lines[0]))
		}
	}

	canvas := lipgloss.NewCanvas(lobbyTileW, lobbyTileH)
	canvas.Compose(lipgloss.NewLayer(renderTatamiTile()))
	shortBubble := renderEmoteBubble("ok")
	backgrounds := make([][4]uint32, 0, lipgloss.Width(shortBubble)*lipgloss.Height(shortBubble))
	for y := range lipgloss.Height(shortBubble) {
		for x := 1; x < 1+lipgloss.Width(shortBubble); x++ {
			background := canvas.CellAt(x, y).Style.Bg
			backgrounds = append(backgrounds, rgba(background))
		}
	}
	overlayEmote(canvas, shortBubble, 1, 0)
	i := 0
	for y := range lipgloss.Height(shortBubble) {
		for x := 1; x < 1+lipgloss.Width(shortBubble); x++ {
			if got := rgba(canvas.CellAt(x, y).Style.Bg); got != backgrounds[i] {
				t.Fatalf("emote changed floor background at (%d,%d): %v, want %v", x, y, got, backgrounds[i])
			}
			i++
		}
	}
}

func rgba(c color.Color) [4]uint32 {
	if c == nil {
		return [4]uint32{}
	}
	r, g, b, a := c.RGBA()
	return [4]uint32{r, g, b, a}
}

func TestLobbyCameraStaysStillWhileTrainerMovesInsideViewport(t *testing.T) {
	m := New("aaa", &game.Save{Handle: "alpha"}, nil, nil)
	m.width, m.height = 80, 24
	m.syncLobbyCamera(lobby.Presence{Hash: "aaa", X: 24, Y: 7}, false)
	x, y := m.lobbyCameraX, m.lobbyCameraY

	m.syncLobbyCamera(lobby.Presence{Hash: "aaa", X: 25, Y: 7}, false)
	if m.lobbyCameraX != x || m.lobbyCameraY != y {
		t.Fatalf("camera moved inside viewport: (%d,%d) -> (%d,%d)", x, y, m.lobbyCameraX, m.lobbyCameraY)
	}

	m.syncLobbyCamera(lobby.Presence{Hash: "aaa", X: x - 1, Y: 7}, false)
	if m.lobbyCameraX == x {
		t.Fatal("camera did not recenter after Trainer left viewport")
	}
}

func TestSaveMsgRefreshesLobbyRecord(t *testing.T) {
	base := gametest.SaveWithStarter("test-rootkit", "rootkit", []string{"root_access", "chmod", "sudo", "setuid"})
	m := New("aaa", base, nil, nil)
	updated := *base
	updated.Wins = 3
	updated.Losses = 2
	next, _ := m.Update(server.SaveMsg{Save: &updated})
	m = next.(Model)
	if m.save.Wins != 3 || m.save.Losses != 2 {
		t.Fatalf("record = %d-%d, want 3-2", m.save.Wins, m.save.Losses)
	}
}

func TestSuccessfulMoveClearsBlockedStatus(t *testing.T) {
	hub, set, s := testHub(t)
	onboardTrainer(t, hub, s, "aaa", "alpha", "rootkit")
	m := New("aaa", gametest.LoadSave(t, s, "aaa"), set, hub)
	m.screen = screenLobby
	m.status = "lobby: blocked"
	m.statusHold = holdStatus

	next, _ := m.Update(press("w"))
	m = next.(Model)
	if m.status != "" {
		t.Fatalf("status after successful move = %q, want cleared", m.status)
	}
	if m.statusHold != 0 {
		t.Fatalf("status timer after successful move = %d, want cleared", m.statusHold)
	}
}

func TestLobbyStatusesExpireAndReplacementRestartsTimer(t *testing.T) {
	messages := []string{
		"lobby: blocked",
		"challenge sent to alpha",
		"challenge declined",
		"challenge expired",
	}
	for _, text := range messages {
		t.Run(text, func(t *testing.T) {
			m := New("aaa", &game.Save{Handle: "alpha"}, nil, nil)
			next, _ := m.Update(server.ErrorMsg{Text: text})
			m = next.(Model)
			if m.status != text || m.statusHold != holdStatus {
				t.Fatalf("status = %q hold=%d, want %q hold=%d", m.status, m.statusHold, text, holdStatus)
			}
			for range holdStatus - 1 {
				next, _ = m.Update(tickMsg{})
				m = next.(Model)
			}
			if m.status != text {
				t.Fatalf("status cleared before timeout: %q", m.status)
			}
			next, _ = m.Update(tickMsg{})
			m = next.(Model)
			if m.status != "" || m.statusHold != 0 {
				t.Fatalf("status after timeout = %q hold=%d, want cleared", m.status, m.statusHold)
			}
		})
	}

	m := New("aaa", &game.Save{Handle: "alpha"}, nil, nil)
	next, _ := m.Update(server.ErrorMsg{Text: "challenge sent to bravo"})
	m = next.(Model)
	for range holdStatus / 2 {
		next, _ = m.Update(tickMsg{})
		m = next.(Model)
	}
	next, _ = m.Update(server.ErrorMsg{Text: "challenge declined"})
	m = next.(Model)
	if m.status != "challenge declined" || m.statusHold != holdStatus {
		t.Fatalf("replacement status = %q hold=%d, want a fresh timer", m.status, m.statusHold)
	}
}

func TestLobbyFooterShowsChallenge(t *testing.T) {
	m := New("aaa", &game.Save{Handle: "alpha"}, nil, nil)
	m.width, m.height = 100, 24
	m.screen = screenLobby
	m.snap = server.SnapshotMsg{
		You:   lobby.Presence{Handle: "alpha", Species: "rootkit", X: 2, Y: 2},
		Offer: &server.ChallengeOffer{FromHandle: "bravo"},
	}
	v := ansi.Strip(m.View().Content)
	if !strings.Contains(v, "challenged you") || !strings.Contains(v, "accept") {
		t.Fatal("offer should replace the shortcut footer")
	}
}

func TestTitleScreenHasNoAppChrome(t *testing.T) {
	m := New("aaa", nil, loadOnboardSet(t), nil)
	m.width, m.height = 100, 32
	m.onboard.lineAge = 40
	v := ansi.Strip(m.View().Content)
	if !strings.Contains(v, "press any key") {
		t.Fatal("expected title prompt")
	}
	if !strings.Contains(v, titleTag) {
		t.Fatal("expected the title subtitle")
	}
	if strings.Contains(v, "ROOTKIT") || strings.Contains(v, "EMBERBYTE") {
		t.Fatal("title should not list starters")
	}
	if strings.Contains(v, "walk") || strings.Contains(v, "First run") {
		t.Fatal("title should not use the app header/footer")
	}
	lines := strings.Split(strings.TrimRight(v, "\n"), "\n")
	if len(lines) < 3 || !strings.HasPrefix(strings.TrimSpace(lines[1]), "╭") || !strings.HasPrefix(strings.TrimSpace(lines[len(lines)-2]), "╰") {
		t.Fatal("title should sit inside a rounded page frame")
	}
	if !strings.Contains(lines[len(lines)-2], "quit") {
		t.Fatal("quit belongs on the bottom border")
	}
}

func TestOnboardTalkUsesAppChrome(t *testing.T) {
	m := New("aaa", nil, loadOnboardSet(t), nil)
	m.width, m.height = 100, 32
	m.onboard.stage = stageTalk
	m.onboard.lineAge = 40
	v := ansi.Strip(m.View().Content)
	if !strings.Contains(v, "termon") || !strings.Contains(v, "First run") {
		t.Fatal("talk should sit in the app chrome")
	}
	if !strings.Contains(v, "continue") {
		t.Fatal("expected continue in the footer")
	}
	lines := strings.Split(strings.TrimRight(v, "\n"), "\n")
	if !strings.HasPrefix(lines[0], "╭") || !strings.HasPrefix(lines[len(lines)-1], "╰") {
		t.Fatal("talk should sit inside a rounded page frame")
	}
}
