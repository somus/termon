package tui

import (
	"slices"
	"testing"

	"termon.sh/internal/battle"
	"termon.sh/internal/onboard"
	"termon.sh/internal/server"
)

// hpBattle builds a 3v2 battle with explicit Monster IDs and a model wired to
// it, pushed once so the send-outs are on the log.
func hpBattle(t *testing.T) (*battle.Battle, *Model) {
	t.Helper()
	set := loadSet(t)
	mk := func(id, species string) battle.PartyMember {
		mon, err := onboard.DefaultLoadout(set, species)
		if err != nil {
			t.Fatal(err)
		}
		mon.ID = id
		return battle.PartyMember{Monster: mon}
	}
	bt, err := battle.New(set,
		battle.Party{Trainer: "aaa", Members: []battle.PartyMember{
			mk("aaa-1", "aquabit"), mk("aaa-2", "wickware"), mk("aaa-3", "spamlet"),
		}},
		battle.Party{Trainer: "bbb", Members: []battle.PartyMember{
			mk("bbb-1", "zaplet"), mk("bbb-2", "thornpatch"),
		}},
		battle.Seeded(7),
	)
	if err != nil {
		t.Fatal(err)
	}
	m := &Model{
		width: 120, height: 40, hash: "aaa", set: set, screen: screenBattle,
		battle: battleScreenModel{session: battleSession{battle: bt, you: "aaa", foe: "bot", foeHash: "bbb"}},
	}
	m.applyBattle(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bot", FoeHash: "bbb"})
	m.wipeHold = 0
	m.battle.introHold = 0
	return bt, m
}

// push locks both actions and streams the resolved turn into the model
// without advancing playback, so tests can assert mid-replay frames.
func push(t *testing.T, m *Model, bt *battle.Battle, youAct, foeAct battle.Action) {
	t.Helper()
	if bt.State() == battle.StateRevealing {
		if err := bt.AdvanceReveal(); err != nil {
			t.Fatal(err)
		}
	}
	if err := bt.Select("aaa", youAct); err != nil {
		t.Fatal(err)
	}
	if err := bt.Select("bbb", foeAct); err != nil {
		t.Fatal(err)
	}
	m.applyBattle(server.BattleMsg{Battle: bt, You: "aaa", Foe: "bot", FoeHash: "bbb"})
}

// leadMove locks the fielded Monster's first Move for a Trainer.
func leadMove(t *testing.T, bt *battle.Battle, trainer string) battle.Action {
	t.Helper()
	f, ok := bt.Fighter(trainer)
	if !ok {
		t.Fatalf("no fighter for %q", trainer)
	}
	return battle.Action{Kind: battle.ActionMove, Move: f.Moves[0]}
}

// seekBeat parks the playhead on beat idx with a fresh hold, as the frame the
// beat opens with.
func seekBeat(m *Model, idx int) {
	m.battle.playing = true
	m.battle.playPause = false
	m.battle.playAt = idx
	m.applyBeat(m.battleEvents()[idx])
}

// beatPlate asserts the damage beat targeted a fielded Monster and returns
// that plate's HP.
func beatPlate(t *testing.T, you, foe battle.Fighter, target string) int {
	t.Helper()
	switch target {
	case you.ID:
		return you.HP
	case foe.ID:
		return foe.HP
	default:
		t.Fatalf("damage target %q is neither fielded Monster (%q, %q)", target, you.ID, foe.ID)
		return 0
	}
}

// platePost returns the projected post-hit HP of the Monster a damage beat
// targeted, taken from the one-locked projection the plates render from.
func platePost(t *testing.T, bt *battle.Battle, e battle.Event, n int) (post int) {
	t.Helper()
	you, foe := bt.FieldedHPAfter("aaa", n)
	switch e.TargetID {
	case you.MonsterID:
		return you.HP
	case foe.MonsterID:
		return foe.HP
	default:
		t.Fatalf("damage target %q is neither fielded Monster (%q, %q)", e.TargetID, you.MonsterID, foe.MonsterID)
		return 0
	}
}

// A damage beat animates once: it opens on the exact pre-hit HP, drains
// across its hold, and settles on the exact post-hit HP without re-applying
// the hit.
func TestDamageBeatAnimatesExactHP(t *testing.T) {
	bt, m := hpBattle(t)

	var dmgIdx int
	for range 8 {
		push(t, m, bt, leadMove(t, bt, "aaa"), leadMove(t, bt, "bbb"))
		dmgIdx = slices.IndexFunc(m.battleEvents(), func(e battle.Event) bool {
			return e.Kind == battle.EventDamageDealt && e.Damage >= 2
		})
		if dmgIdx >= 0 {
			break
		}
		if bt.State() != battle.StateAwaitingActions && bt.State() != battle.StateRevealing {
			t.Fatalf("state %s before any damage", bt.State())
		}
	}
	if dmgIdx < 0 {
		t.Fatal("no damage event within 8 turns")
	}
	e := m.battleEvents()[dmgIdx]
	post := platePost(t, bt, e, dmgIdx+1)
	pre := post + e.Damage

	seekBeat(m, dmgIdx)
	you, foe := m.arenaFighters()
	if got := beatPlate(t, you, foe, e.TargetID); got != pre {
		t.Fatalf("beat opens at %d, want exact pre-hit %d", got, pre)
	}

	// Fixed 20-tick hold sampled at fixed points: the bar drains monotonically
	// from the exact pre-hit HP to the exact post-hit HP, and the halfway
	// frame sits within one HP of the midpoint — asserted independently of
	// the production rounding.
	m.battle.playHoldTotal = 20
	plateAt := func(hold int) int {
		t.Helper()
		m.battle.playHold = hold
		you, foe := m.arenaFighters()
		return beatPlate(t, you, foe, e.TargetID)
	}
	quarter := plateAt(15)
	half := plateAt(10)
	if quarter > pre || quarter < half || half < post {
		t.Fatalf("drain not monotonic: pre %d, quarter %d, half %d, post %d", pre, quarter, half, post)
	}
	if mid := (pre + post) / 2; half < mid-1 || half > mid+1 {
		t.Fatalf("halfway plate %d not within 1 of midpoint %d", half, mid)
	}
	if got := plateAt(0); got != post {
		t.Fatalf("beat settles at %d, want exact post-hit %d", got, post)
	}
	m.battle.playPause = true
	you, foe = m.arenaFighters()
	if got := beatPlate(t, you, foe, e.TargetID); got != post {
		t.Fatalf("paused plate %d, want exact post-hit %d", got, post)
	}

	// Once the beat has played, the plate holds at post-hit — no snap back.
	if dmgIdx+1 < len(m.battleEvents()) {
		seekBeat(m, dmgIdx+1)
		you, foe = m.arenaFighters()
		if got := beatPlate(t, you, foe, e.TargetID); got != post {
			t.Fatalf("plate after the beat %d, want post-hit %d", got, post)
		}
	}
}

// Plates keep ID, name, HP, and MaxHP on the same Monster at every playhead
// position: a damaged lead keeps its damage while benched, and its re-entry
// shows the damaged HP instead of resetting to full.
func TestPlateTracksMonsterAcrossSwitchAndReentry(t *testing.T) {
	bt, m := hpBattle(t)

	// Take a hit on the lead.
	for range 8 {
		push(t, m, bt, leadMove(t, bt, "aaa"), leadMove(t, bt, "bbb"))
		snap := bt.Snapshot("aaa")
		if snap.YourParty[0].HP < snap.YourParty[0].MaxHP {
			break
		}
		if bt.State() != battle.StateAwaitingActions && bt.State() != battle.StateRevealing {
			t.Fatalf("state %s before the lead was damaged", bt.State())
		}
	}
	startSnap := bt.Snapshot("aaa")
	lead := startSnap.YourParty[0]
	if lead.HP >= lead.MaxHP {
		t.Fatal("lead never took damage within 8 turns")
	}
	if bt.State() != battle.StateAwaitingActions && bt.State() != battle.StateRevealing {
		t.Fatalf("state %s; switch scenario needs a healthy lead", bt.State())
	}

	// Switch to the reserve. Before the switch beat plays, the plate must
	// still name the damaged lead at its own damaged HP and MaxHP.
	push(t, m, bt, battle.Action{Kind: battle.ActionSwitch, SwitchTo: "aaa-2"}, leadMove(t, bt, "bbb"))
	events := m.battleEvents()
	sw := slices.IndexFunc(events, func(e battle.Event) bool {
		return e.Kind == battle.EventSwitched && e.MonsterID == "aaa-2"
	})
	if sw < 1 {
		t.Fatalf("no switched-to-aaa-2 event in %d events", len(events))
	}
	seekBeat(m, sw-1)
	you, _ := m.arenaFighters()
	if you.ID != "aaa-1" || you.Name != lead.Name {
		t.Fatalf("pre-switch plate names %s (%q), want aaa-1 (%q)", you.ID, you.Name, lead.Name)
	}
	if you.HP != lead.HP || you.MaxHP != lead.MaxHP {
		t.Fatalf("pre-switch plate %d/%d, want damaged lead %d/%d", you.HP, you.MaxHP, lead.HP, lead.MaxHP)
	}

	// The switch beat names the incoming reserve with its own full bar.
	reserve := startSnap.YourParty[1]
	seekBeat(m, sw)
	you, _ = m.arenaFighters()
	if you.ID != "aaa-2" || you.Name != reserve.Name {
		t.Fatalf("switch plate names %s (%q), want aaa-2 (%q)", you.ID, you.Name, reserve.Name)
	}
	if you.HP != reserve.MaxHP || you.MaxHP != reserve.MaxHP {
		t.Fatalf("switch plate %d/%d, want full reserve %d/%d", you.HP, you.MaxHP, reserve.MaxHP, reserve.MaxHP)
	}

	// Switch the damaged lead back in: re-entry keeps the damage.
	push(t, m, bt, battle.Action{Kind: battle.ActionSwitch, SwitchTo: "aaa-1"}, leadMove(t, bt, "bbb"))
	events = m.battleEvents()
	re := slices.IndexFunc(events, func(e battle.Event) bool {
		return e.Kind == battle.EventSwitched && e.MonsterID == "aaa-1"
	})
	if re < sw {
		t.Fatalf("no re-entry event after %d in %d events", sw, len(events))
	}
	seekBeat(m, re)
	you, _ = m.arenaFighters()
	wantHP := lead.MaxHP - damageTo(events[:re+1], "aaa-1")
	if wantHP >= lead.MaxHP {
		t.Fatal("re-entry arithmetic expects a damaged lead")
	}
	if you.ID != "aaa-1" || you.Name != lead.Name {
		t.Fatalf("re-entry plate names %s (%q), want aaa-1 (%q)", you.ID, you.Name, lead.Name)
	}
	if you.HP != wantHP || you.MaxHP != lead.MaxHP {
		t.Fatalf("re-entry plate %d/%d, want damaged %d/%d", you.HP, you.MaxHP, wantHP, lead.MaxHP)
	}

	// Every playhead position — including the initial send-outs — names one
	// Monster per plate with that Monster's own values. Your own plate is
	// checked against snapshot truth plus the damage ledger of the unplayed
	// log suffix; the foe plate against the engine projection.
	liveSnap := bt.Snapshot("aaa")
	maxHPs := map[string]int{}
	for i := range events {
		m.battle.playing = true
		m.battle.playPause = true // settled frame: pure projection, no beat animation
		m.battle.playAt = i
		you, foe := m.arenaFighters()
		assertYourPlate(t, liveSnap, events, i, you)
		assertFoePlate(t, bt, liveSnap, i, foe)
		for _, f := range []battle.Fighter{you, foe} {
			if prev, ok := maxHPs[f.ID]; ok && prev != f.MaxHP {
				t.Fatalf("playhead %d: %s MaxHP %d, was %d earlier", i, f.ID, f.MaxHP, prev)
			}
			maxHPs[f.ID] = f.MaxHP
		}
	}
}

// While the foe's voluntary switch is still at the playhead, its plate must
// carry the outgoing damaged Monster's own values; at the switch beat it must
// name the incoming Monster with its own full bar.
func TestPlateTracksFoeVoluntarySwitch(t *testing.T) {
	bt, m := hpBattle(t)

	// Take a hit on the foe lead.
	var beforeSnap battle.Snapshot
	for range 8 {
		push(t, m, bt, leadMove(t, bt, "aaa"), leadMove(t, bt, "bbb"))
		beforeSnap = bt.Snapshot("aaa")
		if beforeSnap.FoeRoster[0].HP < beforeSnap.FoeRoster[0].MaxHP {
			break
		}
		if bt.State() != battle.StateAwaitingActions && bt.State() != battle.StateRevealing {
			t.Fatalf("state %s before the foe lead was damaged", bt.State())
		}
	}
	outgoing := beforeSnap.FoeRoster[0]
	if outgoing.HP >= outgoing.MaxHP {
		t.Fatal("foe lead never took damage within 8 turns")
	}
	if bt.State() != battle.StateAwaitingActions && bt.State() != battle.StateRevealing {
		t.Fatalf("state %s; switch scenario needs a healthy foe lead", bt.State())
	}

	// The foe voluntarily switches while your Monster attacks.
	push(t, m, bt, leadMove(t, bt, "aaa"), battle.Action{Kind: battle.ActionSwitch, SwitchTo: "bbb-2"})
	events := m.battleEvents()
	sw := slices.IndexFunc(events, func(e battle.Event) bool {
		return e.Kind == battle.EventSwitched && e.MonsterID == "bbb-2"
	})
	if sw < 1 {
		t.Fatalf("no switched-to-bbb-2 event in %d events", len(events))
	}
	incoming := bt.Snapshot("aaa").FoeRoster[1]

	seekBeat(m, sw-1)
	_, foe := m.arenaFighters()
	if foe.ID != "bbb-1" || foe.Name != outgoing.Name {
		t.Fatalf("pre-switch foe plate names %s (%q), want bbb-1 (%q)", foe.ID, foe.Name, outgoing.Name)
	}
	if foe.HP != outgoing.HP || foe.MaxHP != outgoing.MaxHP {
		t.Fatalf("pre-switch foe plate %d/%d, want damaged outgoing %d/%d",
			foe.HP, foe.MaxHP, outgoing.HP, outgoing.MaxHP)
	}

	seekBeat(m, sw)
	you, foe := m.arenaFighters()
	if you.ID != "aaa-1" {
		t.Fatalf("your plate moved to %s during the foe switch", you.ID)
	}
	if foe.ID != "bbb-2" || foe.Name != incoming.Name {
		t.Fatalf("switch foe plate names %s (%q), want bbb-2 (%q)", foe.ID, foe.Name, incoming.Name)
	}
	if foe.HP != incoming.MaxHP || foe.MaxHP != incoming.MaxHP {
		t.Fatalf("switch foe plate %d/%d, want full incoming %d/%d",
			foe.HP, foe.MaxHP, incoming.MaxHP, incoming.MaxHP)
	}
}

// damageTo sums a Monster's recorded damage over a log prefix.
func damageTo(events []battle.Event, id string) int {
	sum := 0
	for _, e := range events {
		if e.Kind == battle.EventDamageDealt && e.TargetID == id {
			sum += e.Damage
		}
	}
	return sum
}

// assertYourPlate checks your plate at playhead i against snapshot truth plus
// the damage ledger of the unplayed log suffix — an oracle independent of the
// projection under test.
func assertYourPlate(t *testing.T, snap battle.Snapshot, events []battle.Event, i int, f battle.Fighter) {
	t.Helper()
	for _, mem := range snap.YourParty {
		if mem.ID != f.ID {
			continue
		}
		wantHP := mem.HP + damageTo(events[i+1:], mem.ID)
		if f.Name != mem.Name {
			t.Fatalf("playhead %d: your plate %q names %s", i, f.Name, mem.ID)
		}
		if f.HP != wantHP {
			t.Fatalf("playhead %d: your plate HP %d, ledger says %d", i, f.HP, wantHP)
		}
		if f.MaxHP != mem.MaxHP {
			t.Fatalf("playhead %d: your plate MaxHP %d, snapshot %d", i, f.MaxHP, mem.MaxHP)
		}
		return
	}
	t.Fatalf("playhead %d: your plate names unknown Monster %q", i, f.ID)
}

// assertFoePlate checks the foe plate at playhead i against the engine
// projection and the public roster identity.
func assertFoePlate(t *testing.T, bt *battle.Battle, snap battle.Snapshot, i int, f battle.Fighter) {
	t.Helper()
	_, want := bt.FieldedHPAfter("aaa", i+1)
	if f.ID != want.MonsterID || f.HP != want.HP || f.MaxHP != want.MaxHP {
		t.Fatalf("playhead %d: foe plate %s %d/%d, projection %s %d/%d",
			i, f.ID, f.HP, f.MaxHP, want.MonsterID, want.HP, want.MaxHP)
	}
	if f.MaxHP < 1 || f.HP < 0 || f.HP > f.MaxHP {
		t.Fatalf("playhead %d: foe plate shows impossible %d/%d", i, f.HP, f.MaxHP)
	}
	for _, r := range snap.FoeRoster {
		if r.ID == f.ID && f.Name != r.Name {
			t.Fatalf("playhead %d: foe plate %q names %s", i, f.Name, r.ID)
		}
	}
}
