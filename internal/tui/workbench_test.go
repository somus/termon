package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"termon.sh/internal/game"
	"termon.sh/internal/gametest"
	"termon.sh/internal/server"
	"termon.sh/internal/store"
)

func TestLobbyStatsModalLoadsAuthoritativeStats(t *testing.T) {
	m, _, _, trainerID := newWorkbenchTestModel(t)
	next, cmd := m.Update(press("S"))
	m = next.(Model)
	if m.modal != modalStats || cmd == nil {
		t.Fatalf("S modal = %d, cmd nil = %v", m.modal, cmd == nil)
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	view := ansi.Strip(m.innerView())
	for _, want := range []string{"TRAINER STATS", groupedSupportID(trainerID), "Collection     1 Monsters"} {
		if !strings.Contains(view, want) {
			t.Fatalf("Stats modal missing %q:\n%s", want, view)
		}
	}
	next, _ = m.Update(press("esc"))
	if next.(Model).modal != modalNone {
		t.Fatal("escape did not close Stats modal")
	}
}

func TestLobbyHelpModalShowsSupportInstructions(t *testing.T) {
	m, _, _, trainerID := newWorkbenchTestModel(t)
	next, cmd := m.Update(press("?"))
	m = next.(Model)
	if m.modal != modalHelp || cmd != nil {
		t.Fatalf("Help modal = %d, cmd nil = %v", m.modal, cmd == nil)
	}
	view := ansi.Strip(m.innerView())
	for _, want := range []string{
		"HELP & SUPPORT", supportEmail, supportIssuesURL, groupedSupportID(trainerID),
		"error reference", "UTC time",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("Help modal missing %q:\n%s", want, view)
		}
	}
}

func TestWorkbenchTrainerStatsTab(t *testing.T) {
	m, _, _, trainerID := newWorkbenchTestModel(t)
	m.openWorkbench()
	m, _ = m.workbenchBrowseKey("tab")
	m, cmd := m.workbenchBrowseKey("tab")
	if cmd == nil {
		t.Fatal("stats tab did not request statistics")
	}
	msg := cmd()
	next, _ := m.Update(msg)
	m = next.(Model)
	view := ansi.Strip(m.renderWorkbench())
	for _, want := range []string{"Trainer Stats", "SUPPORT ID", groupedSupportID(trainerID), "Registered Trainers"} {
		if !strings.Contains(view, want) {
			t.Fatalf("stats view missing %q:\n%s", want, view)
		}
	}
}

func newWorkbenchTestModel(t *testing.T) (Model, *server.Hub, *store.MemoryStore, string) {
	t.Helper()
	set := loadSet(t)
	saves := newTestStore(t)
	hub := server.NewHub(set, saves, server.Admission{OpenAccess: true, RegistrationsPerIP: -1})
	cred := "wb-trainer-" + t.Name()
	trainer, err := hub.Authenticate(cred, "10.9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	sv, err := hub.CompleteOnboard(trainer.ID, "alpha-wb", "rootkit")
	if err != nil {
		t.Fatal(err)
	}
	m := New(trainer.ID, sv, set, hub)
	m.screen = screenLobby
	m.width, m.height = 100, 32
	return m, hub, saves, trainer.ID
}

func driveCmd(m Model, msg tea.Msg) Model {
	next, cmd := m.Update(msg)
	m = next.(Model)
	if cmd != nil {
		out, _ := m.Update(cmd())
		m = out.(Model)
	}
	return m
}

func driveCmdMust(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, cmd := m.Update(msg)
	m = next.(Model)
	if cmd == nil {
		t.Fatalf("expected hub cmd for %#v (layer=%d screen=%d)", msg, m.wb.layer, m.screen)
	}
	out := cmd()
	if errMsg, ok := out.(server.ErrorMsg); ok {
		t.Fatalf("hub error: %s", errMsg.Text)
	}
	next, _ = m.Update(out)
	return next.(Model)
}

func captureSpecies(t *testing.T, saves *store.MemoryStore, trainerID, species string) *game.Save {
	t.Helper()
	tr, err := saves.LoadTrainer(trainerID)
	if err != nil {
		t.Fatal(err)
	}
	sv, err := saves.RecordActivityResult(store.ActivityRecord{
		Kind: "expedition", NaturalKey: fmt.Sprintf("%s:%s:%d", trainerID, species, time.Now().UnixNano()),
		TrainerID: trainerID, ActiveIDs: []string{tr.Save.Party[0]}, Outcome: "captured",
		Capture:     &store.CaptureSpec{Species: species, FillParty: false},
		CompletedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return sv
}

func fillPartyForTest(t *testing.T, saves *store.MemoryStore, trainerID string) *game.Save {
	t.Helper()
	return gametest.FillParty(t, saves, trainerID)
}

func TestWorkbenchOpensFromLobby(t *testing.T) {
	m, _, _, _ := newWorkbenchTestModel(t)
	next := drive(m, press("p"))
	if next.screen != screenWorkbench {
		t.Fatalf("screen = %d, want workbench", next.screen)
	}
	view := ansi.Strip(next.View().Content)
	if !strings.Contains(view, "Collection") || !strings.Contains(view, "Party") || !strings.Contains(view, "LEAD") {
		t.Fatalf("workbench layout missing:\n%s", view)
	}
}

func TestWorkbenchTooSmallTerminal(t *testing.T) {
	m, _, _, _ := newWorkbenchTestModel(t)
	m.width, m.height = 80, 24
	next := drive(m, press("p"))
	view := ansi.Strip(next.View().Content)
	if !strings.Contains(view, "terminal too small") {
		t.Fatalf("expected size warning:\n%s", view)
	}
}

func TestBattleDoesNotOpenWorkbench(t *testing.T) {
	m := battleModel(t, nil, 100, 32)
	next, cmd := m.Update(press("p"))
	if cmd != nil {
		t.Fatal("p during battle should not invoke hub")
	}
	if next.(Model).screen != screenBattle {
		t.Fatalf("screen = %d, want battle", next.(Model).screen)
	}
}

func TestWorkbenchEscReturnsToLobbyWhenPartyIsFull(t *testing.T) {
	m, _, saves, id := newWorkbenchTestModel(t)
	m.save = fillPartyForTest(t, saves, id)
	m = drive(m, press("p"))
	m = drive(m, press("esc"))
	if m.screen != screenLobby {
		t.Fatalf("screen = %d, want lobby", m.screen)
	}
}

func TestWorkbenchEscWithPartialPartyStartsLesson(t *testing.T) {
	m, _, _, _ := newWorkbenchTestModel(t)
	m = drive(m, press("p"))
	next, cmd := m.Update(press("esc"))
	m = next.(Model)
	if m.screen == screenLobby {
		t.Fatal("partial Party must not return to Dojo roam")
	}
	if cmd == nil {
		t.Fatal("esc should start the required Lesson")
	}
}

func TestDuplicateSpeciesSeparateRows(t *testing.T) {
	m, _, saves, id := newWorkbenchTestModel(t)
	sv := captureSpecies(t, saves, id, "rootkit")
	m.save = sv
	m = drive(m, press("p"))
	view := ansi.Strip(m.View().Content)
	if strings.Count(view, "Rootkit")+strings.Count(view, "ROOTKIT") < 2 {
		t.Fatalf("expected two rootkit rows:\n%s", view)
	}
}

func TestWorkbenchCursorSurvivesSearchFilterSort(t *testing.T) {
	m, _, saves, id := newWorkbenchTestModel(t)
	sv := captureSpecies(t, saves, id, "emberbyte")
	target := sv.Collection[len(sv.Collection)-1].ID
	m.save = sv
	m = drive(m, press("p"))
	m.wb.selectedID = target
	m = drive(m, press("f"))
	m = drive(m, press("s"))
	m = drive(m, press("/"))
	for _, ch := range "ember" {
		m = drive(m, press(string(ch)))
	}
	m = drive(m, press("enter"))
	if m.wb.selectedID != target {
		t.Fatalf("cursor id = %q, want %q", m.wb.selectedID, target)
	}
}

func TestPartySlotLayerOpens(t *testing.T) {
	m, _, saves, id := newWorkbenchTestModel(t)
	sv := captureSpecies(t, saves, id, "emberbyte")
	benchID := sv.Collection[len(sv.Collection)-1].ID
	m.save = sv
	m = drive(m, press("p"))
	m.wb.selectedID = benchID
	m = drive(m, press("p"))
	if m.wb.layer != wbLayerPartySlot {
		t.Fatalf("layer=%d selected=%q tab=%d", m.wb.layer, m.wb.selectedID, m.wb.tab)
	}
}

func TestPartyAssignEmptySlot(t *testing.T) {
	m, _, saves, id := newWorkbenchTestModel(t)
	sv := captureSpecies(t, saves, id, "emberbyte")
	benchID := sv.Collection[len(sv.Collection)-1].ID
	m.save = sv
	m = drive(m, press("p"))
	m.wb.selectedID = benchID
	m = drive(m, press("p"))
	m = drive(m, press("down"))
	m = driveCmdMust(t, m, press("enter"))
	if m.save.Party[1] != benchID {
		t.Fatalf("party slot 2 = %q, want %q", m.save.Party[1], benchID)
	}
}

func TestPartyReplaceConfirm(t *testing.T) {
	m, _, saves, id := newWorkbenchTestModel(t)
	sv := captureSpecies(t, saves, id, "emberbyte")
	benchID := sv.Collection[len(sv.Collection)-1].ID
	m.save = sv
	m = drive(m, press("p"))
	m.wb.selectedID = benchID
	m = drive(m, press("p"))
	m = drive(m, press("1"))
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Replace") {
		t.Fatalf("expected replace confirm:\n%s", view)
	}
	m = drive(m, press("n"))
	if m.save.Party[0] == benchID {
		t.Fatal("cancel must not replace lead")
	}
	m = drive(m, press("1"))
	m = driveCmd(m, press("y"))
	if m.save.Party[0] != benchID {
		t.Fatalf("party lead = %q, want %q", m.save.Party[0], benchID)
	}
}

func TestPartySwapOccupiedSlots(t *testing.T) {
	m, hub, saves, id := newWorkbenchTestModel(t)
	sv := captureSpecies(t, saves, id, "emberbyte")
	benchID := sv.Collection[len(sv.Collection)-1].ID
	lead := sv.Party[0]
	next := sv.Party
	next[1] = benchID
	if err := hub.SetParty(id, next); err != nil {
		t.Fatal(err)
	}
	sv, _ = hub.Load(id)
	m.save = sv
	m = drive(m, press("p"))
	m.wb.selectedID = benchID
	m = drive(m, press("p"))
	m = driveCmdMust(t, m, press("1"))
	loaded, _ := hub.Load(id)
	if loaded.Party[0] != benchID || loaded.Party[1] != lead {
		t.Fatalf("swap failed: %+v", loaded.Party)
	}
}

func TestPartyRemove(t *testing.T) {
	m, _, _, _ := newWorkbenchTestModel(t)
	m = drive(m, press("p"))
	m = drive(m, press("enter"))
	m = driveCmd(m, press("enter")) // Remove from Party
	if m.save.Party[0] != "" {
		t.Fatalf("lead still occupied: %+v", m.save.Party)
	}
}

func openWBAction(m Model, name string) Model {
	m = drive(m, press("enter"))
	for range 8 {
		actions := m.wbActionList()
		for i, a := range actions {
			if a == name {
				for range i {
					m = drive(m, press("down"))
				}
				return drive(m, press("enter"))
			}
		}
		m = drive(m, press("down"))
	}
	return m
}

func TestLoadoutFillAndReplace(t *testing.T) {
	m, hub, _, id := newWorkbenchTestModel(t)
	monID := m.save.Collection[0].ID
	lib := m.save.Collection[0].MoveLibrary
	if err := hub.SetBattleLoadout(id, monID, lib[:2], nil); err != nil {
		t.Fatal(err)
	}
	sv, _ := hub.Load(id)
	m.save = sv
	m = drive(m, press("p"))
	m = openWBAction(m, "Moves")
	m = drive(m, press("right"))
	for range 2 {
		m = drive(m, press("down"))
	}
	m = drive(m, press("enter"))
	m = driveCmd(m, press("1"))
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "->") {
		t.Fatalf("expected replace preview:\n%s", view)
	}
	m = driveCmd(m, press("y"))
}

func TestLoadoutDuplicateRejected(t *testing.T) {
	m, _, _, _ := newWorkbenchTestModel(t)
	m = drive(m, press("p"))
	m = openWBAction(m, "Moves")
	equipped := m.save.Collection[0].BattleLoadout[0]
	m = drive(m, press("right"))
	for range 4 {
		m = drive(m, press("down"))
	}
	m = drive(m, press("enter"))
	m = drive(m, press("2"))
	m = drive(m, press("enter"))
	if m.status != "move already equipped" {
		t.Fatalf("status = %q, want duplicate rejection", m.status)
	}
	if got := m.save.Collection[0].BattleLoadout[0]; got != equipped {
		t.Fatalf("equipped[0] = %q, want unchanged %q", got, equipped)
	}
}

func TestLoadoutRefuseRemoveFinalMove(t *testing.T) {
	m, hub, _, id := newWorkbenchTestModel(t)
	monID := m.save.Collection[0].ID
	one := m.save.Collection[0].MoveLibrary[:1]
	if err := hub.SetBattleLoadout(id, monID, one, nil); err != nil {
		t.Fatal(err)
	}
	sv, _ := hub.Load(id)
	m.save = sv
	m = drive(m, press("p"))
	m = openWBAction(m, "Moves")
	m = drive(m, press("1"))
	m = driveCmd(m, press("d"))
	if !strings.Contains(m.status, "at least one") {
		t.Fatalf("status = %q", m.status)
	}
}

func TestNicknameSetClearInvalid(t *testing.T) {
	m, _, _, id := newWorkbenchTestModel(t)
	m = drive(m, press("p"))
	m = openWBAction(m, "Nickname")
	for _, ch := range "Mossy" {
		m = drive(m, press(string(ch)))
	}
	m = driveCmd(m, press("enter"))
	loaded, _ := m.hub.Load(id)
	if loaded.Collection[0].Nickname != "Mossy" {
		t.Fatalf("nickname = %q", loaded.Collection[0].Nickname)
	}
	m = openWBAction(m, "Nickname")
	for range 5 {
		m = drive(m, press("backspace"))
	}
	m = driveCmd(m, press("enter"))
	loaded, _ = m.hub.Load(id)
	if loaded.Collection[0].Nickname != "" {
		t.Fatalf("nickname = %q, want cleared", loaded.Collection[0].Nickname)
	}
	m = openWBAction(m, "Nickname")
	m = drive(m, press("!"))
	m = drive(m, press("enter"))
	if !strings.Contains(m.status, "invalid") {
		t.Fatalf("status = %q", m.status)
	}
}

func TestWorkbenchReconnectPersistence(t *testing.T) {
	m, hub, _, id := newWorkbenchTestModel(t)
	monID := m.save.Collection[0].ID
	if err := hub.SetNickname(id, monID, "Persist"); err != nil {
		t.Fatal(err)
	}
	loadout := m.save.Collection[0].MoveLibrary[:2]
	if err := hub.SetBattleLoadout(id, monID, loadout, nil); err != nil {
		t.Fatal(err)
	}
	sv, err := hub.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	m2 := New(id, sv, m.set, hub)
	m2.width, m2.height = 100, 32
	m2 = drive(m2, press("p"))
	view := ansi.Strip(m2.View().Content)
	if !strings.Contains(view, "Persist") && !strings.Contains(m2.save.Collection[0].Nickname, "Persist") {
		t.Fatalf("nickname missing after reload:\n%s", view)
	}
	mon, _ := game.MonsterByID(m2.save, monID)
	if len(mon.BattleLoadout) != 2 {
		t.Fatalf("loadout = %v", mon.BattleLoadout)
	}
}

func TestKeepLoadoutAcksNotice(t *testing.T) {
	m, hub, saves, id := newWorkbenchTestModel(t)
	injectMoveUnlockNotice(t, saves, id)
	sv, _ := hub.Load(id)
	m.save = sv
	m = drive(m, press("p"))
	m = openWBAction(m, "Review move unlock")
	m = drive(m, press("down"))
	m = driveCmd(m, press("enter"))
	reloaded, _ := hub.Load(id)
	if len(reloaded.Notices) != 0 {
		t.Fatalf("notices = %+v", reloaded.Notices)
	}
}

func TestEditLoadoutAcksNotice(t *testing.T) {
	m, hub, saves, id := newWorkbenchTestModel(t)
	injectMoveUnlockNotice(t, saves, id)
	sv, _ := hub.Load(id)
	monID := sv.Party[0]
	lib := sv.Collection[0].MoveLibrary
	if err := hub.SetBattleLoadout(id, monID, lib[:3], nil); err != nil {
		t.Fatal(err)
	}
	sv, _ = hub.Load(id)
	m.save = sv
	m = drive(m, press("p"))
	m = openWBAction(m, "Review move unlock")
	m = driveCmd(m, press("enter"))
	m = driveCmd(m, press("4"))
	reloaded, _ := hub.Load(id)
	if len(reloaded.Notices) != 0 {
		t.Fatalf("notices = %+v", reloaded.Notices)
	}
}

func TestDeferEvolutionKeepsPending(t *testing.T) {
	m, hub, saves, id := newWorkbenchTestModel(t)
	pendingEvolution(t, saves, id)
	sv, _ := hub.Load(id)
	m.save = sv
	m = drive(m, press("p"))
	m = openWBAction(m, "Review evolution")
	m = drive(m, press("down"))
	m = drive(m, press("enter"))
	loaded, _ := hub.Load(id)
	if !loaded.Collection[0].EvolutionPending {
		t.Fatal("evolution should stay pending")
	}
}

func TestAcceptEvolutionChangesSpeciesKeepsIdentity(t *testing.T) {
	m, hub, saves, id := newWorkbenchTestModel(t)
	pendingEvolution(t, saves, id)
	sv, _ := hub.Load(id)
	monID := sv.Collection[0].ID
	party := sv.Party
	xp := sv.Collection[0].XP
	m.save = sv
	m = drive(m, press("p"))
	m = openWBAction(m, "Review evolution")
	m = driveCmd(m, press("enter"))
	m = driveCmd(m, press("y"))
	loaded, _ := hub.Load(id)
	if loaded.Collection[0].ID != monID || loaded.Collection[0].XP != xp || loaded.Party != party {
		t.Fatalf("identity changed: id=%s xp=%d party=%v", loaded.Collection[0].ID, loaded.Collection[0].XP, loaded.Party)
	}
	if loaded.Collection[0].Species == "rootkit" {
		t.Fatal("species did not evolve")
	}
}

func TestEscDoesNotRollbackConfirmedPartyWrite(t *testing.T) {
	m, _, saves, id := newWorkbenchTestModel(t)
	sv := captureSpecies(t, saves, id, "emberbyte")
	benchID := sv.Collection[len(sv.Collection)-1].ID
	m.save = sv
	m = drive(m, press("p"))
	m.wb.selectedID = benchID
	m = driveCmd(m, press("p"))
	m = driveCmd(m, press("2"))
	if m.save.Party[1] != benchID {
		t.Fatal("party write did not apply")
	}
	m = drive(m, press("esc"))
	if m.save.Party[1] != benchID {
		t.Fatal("esc rolled back confirmed party write")
	}
}

func TestLobbyFooterNoPractice(t *testing.T) {
	m, _, _, _ := newWorkbenchTestModel(t)
	view := ansi.Strip(m.View().Content)
	if strings.Contains(view, "practice") {
		t.Fatalf("footer still advertises practice:\n%s", view)
	}
	if strings.Contains(view, "walk") {
		t.Fatalf("partial Party footer still advertises walking:\n%s", view)
	}
	if !strings.Contains(view, "party") {
		t.Fatal("footer should mention party")
	}
}

func TestOnboardNoPracticePromise(t *testing.T) {
	all := strings.Join(lessonPages, "\n")
	if strings.Contains(all, "practice") {
		t.Fatal("onboard lesson still promises practice")
	}
	if !strings.Contains(all, "Press p") || !strings.Contains(all, "Capture Lesson") {
		t.Fatal("lesson should mention workbench entry and Capture Lessons")
	}
}

func injectMoveUnlockNotice(t *testing.T, saves *store.MemoryStore, trainerID string) {
	t.Helper()
	tr, err := saves.LoadTrainer(trainerID)
	if err != nil {
		t.Fatal(err)
	}
	id := tr.Save.Party[0]
	for i := range 40 {
		sv, err := saves.RecordActivityResult(store.ActivityRecord{
			Kind: "lesson", NaturalKey: fmt.Sprintf("%s:unlock:%d", trainerID, i), TrainerID: trainerID,
			ActiveIDs: []string{id}, Outcome: "cleared",
			CompletedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range sv.Notices {
			if n.MonsterID == id && n.Kind == "move_unlock" {
				return
			}
		}
	}
	t.Fatal("move unlock notice never appeared")
}

func pendingEvolution(t *testing.T, saves *store.MemoryStore, trainerID string) {
	t.Helper()
	tr, err := saves.LoadTrainer(trainerID)
	if err != nil {
		t.Fatal(err)
	}
	id := tr.Save.Party[0]
	for i := range 25 {
		_, err := saves.RecordActivityResult(store.ActivityRecord{
			Kind: "lesson", NaturalKey: fmt.Sprintf("%s:evo:%d", trainerID, i), TrainerID: trainerID,
			ActiveIDs: []string{id}, Outcome: "cleared",
			CompletedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatal(err)
		}
		loaded, _ := saves.LoadTrainer(trainerID)
		if loaded.Save.Collection[0].EvolutionPending {
			return
		}
	}
	t.Fatal("evolution never became pending")
}
