package battle

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"termon.sh/internal/content"
	"termon.sh/internal/game"
)

// scriptRand returns predetermined Float64 values. Resolution consumes them
// in this order: speed-tie (only when Speeds match), then per move an
// accuracy roll, and on a hit a crit roll plus a variance roll.
type scriptRand struct {
	vals []float64
	i    int
}

func (s *scriptRand) Float64() float64 {
	if s.i >= len(s.vals) {
		return 0.5
	}
	v := s.vals[s.i]
	s.i++
	return v
}

func hitNoCritMinVar() []float64 { return []float64{0.0, 0.5, 0.0} }
func hitNoCritMaxVar() []float64 { return []float64{0.0, 0.5, 1.0} }
func hitCritMinVar() []float64   { return []float64{0.0, 0.0, 0.0} }
func missRoll() []float64        { return []float64{0.90} }

func concat(parts ...[]float64) []float64 {
	var out []float64
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func testSet() *content.Set {
	return &content.Set{
		Types: map[string]content.TypeDef{
			"thermal": {Slug: "thermal", Matchup: map[string]float64{"organic": 2.0}},
			"organic": {Slug: "organic", Matchup: map[string]float64{"coolant": 2.0}},
			"coolant": {Slug: "coolant", Matchup: map[string]float64{"thermal": 2.0}},
			"dampen":  {Slug: "dampen", Matchup: map[string]float64{"thermal": 0.5}},
		},
		Moves: map[string]content.Move{
			"jab":    {Slug: "jab", Name: "Jab", Type: "thermal", Category: "physical", Power: 40, Accuracy: 100},
			"bolt":   {Slug: "bolt", Name: "Bolt", Type: "thermal", Category: "special", Power: 40, Accuracy: 100},
			"poke":   {Slug: "poke", Name: "Poke", Type: "coolant", Category: "physical", Power: 40, Accuracy: 100},
			"whiff":  {Slug: "whiff", Name: "Whiff", Type: "thermal", Category: "physical", Power: 40, Accuracy: 80},
			"crush":  {Slug: "crush", Name: "Crush", Type: "thermal", Category: "physical", Power: 90, Accuracy: 100},
			"leaf":   {Slug: "leaf", Name: "Leaf", Type: "organic", Category: "physical", Power: 40, Accuracy: 100},
			"drip":   {Slug: "drip", Name: "Drip", Type: "dampen", Category: "special", Power: 40, Accuracy: 100},
			"splash": {Slug: "splash", Name: "Splash", Type: "coolant", Category: "special", Power: 40, Accuracy: 100},
		},
		Species: map[string]content.Species{
			"spark": {
				Slug: "spark", Name: "Spark", Type: "thermal",
				BaseStats: content.Stats{HP: 50, Attack: 50, Defense: 50, SpAttack: 80, Speed: 80},
				Movepool:  pool("jab", "bolt", "poke", "whiff", "crush", "drip", "splash"),
			},
			"moss": {
				Slug: "moss", Name: "Moss", Type: "organic",
				BaseStats: content.Stats{HP: 50, Attack: 50, Defense: 50, SpAttack: 50, Speed: 20},
				Movepool:  pool("leaf", "jab"),
			},
			"twin": {
				Slug: "twin", Name: "Twin", Type: "thermal",
				BaseStats: content.Stats{HP: 50, Attack: 50, Defense: 50, SpAttack: 50, Speed: 40},
				Movepool:  pool("jab", "poke"),
			},
			"frail": {
				Slug: "frail", Name: "Frail", Type: "organic",
				BaseStats: content.Stats{HP: 8, Attack: 20, Defense: 50, SpAttack: 20, Speed: 10},
				Movepool:  pool("leaf"),
			},
			"tank": {
				Slug: "tank", Name: "Tank", Type: "organic",
				BaseStats: content.Stats{HP: 60, Attack: 40, Defense: 60, SpAttack: 40, Speed: 30},
				Movepool:  pool("leaf", "jab"),
			},
		},
	}
}

func pool(moves ...string) []content.MovepoolEntry {
	out := make([]content.MovepoolEntry, len(moves))
	for i, m := range moves {
		out[i] = content.MovepoolEntry{Move: m, Level: 1}
	}
	return out
}

func mon(id, species string, moves ...string) game.Monster {
	return game.Monster{ID: id, Species: species, Level: 1, MoveLibrary: moves, BattleLoadout: moves}
}

func partyOf(trainer string, monsters ...game.Monster) Party {
	members := make([]PartyMember, len(monsters))
	for i, m := range monsters {
		members[i] = PartyMember{Monster: m}
	}
	return Party{Trainer: trainer, Members: members}
}

func newBattle(t *testing.T, set *content.Set, a, b Party, rng Rand) *Battle {
	t.Helper()
	bt, err := New(set, a, b, rng)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return bt
}

func soloBattle(t *testing.T, set *content.Set, aMon, bMon game.Monster, rng Rand) *Battle {
	return newBattle(t, set,
		partyOf("a", aMon),
		partyOf("b", bMon),
		rng,
	)
}

func moveAct(slug string) Action { return Action{Kind: ActionMove, Move: slug} }
func switchAct(id string) Action { return Action{Kind: ActionSwitch, SwitchTo: id} }

func advanceIfRevealing(t *testing.T, bt *Battle) {
	t.Helper()
	if bt.State() == StateRevealing {
		if err := bt.AdvanceReveal(); err != nil {
			t.Fatalf("AdvanceReveal: %v", err)
		}
	}
}

func resolveTurnTrainers(t *testing.T, bt *Battle, aTrainer, bTrainer string, a, b Action) {
	t.Helper()
	if err := bt.Select(aTrainer, a); err != nil {
		t.Fatalf("Select %s: %v", aTrainer, err)
	}
	if err := bt.Select(bTrainer, b); err != nil {
		t.Fatalf("Select %s: %v", bTrainer, err)
	}
	advanceIfRevealing(t, bt)
}

func resolveTurn(t *testing.T, bt *Battle, a, b Action) {
	t.Helper()
	if err := bt.Select("a", a); err != nil {
		t.Fatalf("Select a: %v", err)
	}
	if err := bt.Select("b", b); err != nil {
		t.Fatalf("Select b: %v", err)
	}
	advanceIfRevealing(t, bt)
}

func kinds(events []Event) []EventKind {
	out := make([]EventKind, len(events))
	for i, e := range events {
		out[i] = e.Kind
	}
	return out
}

func TestSelectStaysHiddenUntilBothLockIn(t *testing.T) {
	set := testSet()
	bt := soloBattle(t, set, mon("a1", "spark", "jab"), mon("b1", "moss", "leaf"),
		&scriptRand{vals: concat(hitNoCritMinVar(), hitNoCritMinVar())},
	)
	if bt.State() != StateAwaitingActions {
		t.Fatalf("start state = %s, want %s", bt.State(), StateAwaitingActions)
	}
	if err := bt.Select("a", moveAct("jab")); err != nil {
		t.Fatalf("first Select: %v", err)
	}
	if bt.State() != StateAwaitingActions {
		t.Fatalf("after one lock state = %s, want still awaiting", bt.State())
	}
	for _, e := range bt.Events() {
		if e.Kind == EventTurnStarted || e.Kind == EventMoveUsed {
			t.Fatalf("hidden selection leaked turn event: %v", e.Kind)
		}
	}
	if !bt.Locked("a") || bt.Locked("b") {
		t.Fatalf("lock flags a=%v b=%v", bt.Locked("a"), bt.Locked("b"))
	}
}

func TestBothSelectionsResolveOneTurn(t *testing.T) {
	set := testSet()
	bt := soloBattle(t, set, mon("a1", "spark", "jab"), mon("b1", "moss", "leaf"),
		&scriptRand{vals: concat(hitNoCritMinVar(), hitNoCritMinVar())},
	)
	resolveTurn(t, bt, moveAct("jab"), moveAct("leaf"))
	advanceIfRevealing(t, bt)
	if bt.State() != StateAwaitingActions && bt.State() != StateOver {
		t.Fatalf("state after turn = %s, want awaiting or over", bt.State())
	}
	got := kinds(bt.Events())
	wantPrefix := []EventKind{EventSendOut, EventSendOut, EventTurnStarted, EventMoveUsed}
	if len(got) < 4 || got[2] != wantPrefix[2] || got[3] != wantPrefix[3] {
		t.Fatalf("events = %v, want turn_started then move_used", got)
	}
	if !slices.Contains(got, EventDamageDealt) {
		t.Fatalf("events = %v, want a damage_dealt", got)
	}
}

func TestPhysicalDamageFormulaBoundary(t *testing.T) {
	set := testSet()
	bt := soloBattle(t, set, mon("a1", "spark", "poke"), mon("b1", "moss", "leaf"),
		&scriptRand{vals: concat(hitNoCritMinVar(), hitNoCritMinVar())},
	)
	resolveTurn(t, bt, moveAct("poke"), moveAct("leaf"))
	hp, maxHP := bt.HP("b")
	if maxHP != 50 {
		t.Fatalf("moss max HP = %d, want 50", maxHP)
	}
	if hp != 42 {
		t.Fatalf("moss HP = %d, want 42 after 8 damage", hp)
	}
}

func TestSpecialUsesSpAttack(t *testing.T) {
	set := testSet()
	bt := soloBattle(t, set, mon("a1", "spark", "bolt"), mon("b1", "moss", "leaf"),
		&scriptRand{vals: concat(hitNoCritMinVar(), hitNoCritMinVar())},
	)
	resolveTurn(t, bt, moveAct("bolt"), moveAct("leaf"))
	hp, _ := bt.HP("b")
	if hp != 15 {
		t.Fatalf("moss HP = %d, want 15 after 35 special damage", hp)
	}
}

func TestSTABAndSuperEffective(t *testing.T) {
	set := testSet()
	bt := soloBattle(t, set, mon("a1", "spark", "jab"), mon("b1", "moss", "leaf"),
		&scriptRand{vals: concat(hitNoCritMinVar(), hitNoCritMinVar())},
	)
	resolveTurn(t, bt, moveAct("jab"), moveAct("leaf"))
	hp, _ := bt.HP("b")
	if hp != 25 {
		t.Fatalf("moss HP = %d, want 25 after 25 damage", hp)
	}
	if !slices.Contains(kinds(bt.Events()), EventSuperEffective) {
		t.Fatalf("events = %v, want super_effective", kinds(bt.Events()))
	}
}

func TestNotVeryEffectiveAndCrit(t *testing.T) {
	set := testSet()
	bt := soloBattle(t, set, mon("a1", "spark", "drip"), mon("b1", "twin", "jab"),
		&scriptRand{vals: concat(hitCritMinVar(), hitNoCritMinVar())},
	)
	resolveTurn(t, bt, moveAct("drip"), moveAct("jab"))
	hp, _ := bt.HP("b")
	if hp != 42 {
		t.Fatalf("twin HP = %d, want 42 after 8 damage", hp)
	}
	got := kinds(bt.Events())
	if !slices.Contains(got, EventCriticalHit) {
		t.Fatalf("events = %v, want critical_hit", got)
	}
	if !slices.Contains(got, EventNotVeryEffective) {
		t.Fatalf("events = %v, want not_very_effective", got)
	}
}

func TestMissConsumesTurnWithoutDamage(t *testing.T) {
	set := testSet()
	bt := soloBattle(t, set, mon("a1", "spark", "whiff"), mon("b1", "moss", "leaf"),
		&scriptRand{vals: concat(missRoll(), hitNoCritMinVar())},
	)
	resolveTurn(t, bt, moveAct("whiff"), moveAct("leaf"))
	hp, maxHP := bt.HP("b")
	if hp != maxHP {
		t.Fatalf("moss HP = %d/%d, want full after miss", hp, maxHP)
	}
	if !slices.Contains(kinds(bt.Events()), EventMissed) {
		t.Fatalf("events = %v, want missed", kinds(bt.Events()))
	}
}

func TestFasterMonsterActsFirst(t *testing.T) {
	set := testSet()
	bt := newBattle(t, set,
		partyOf("slow", mon("s1", "moss", "leaf")),
		partyOf("fast", mon("f1", "spark", "jab")),
		&scriptRand{vals: concat(hitNoCritMinVar(), hitNoCritMinVar())},
	)
	resolveTurnTrainers(t, bt, "slow", "fast", moveAct("leaf"), moveAct("jab"))
	var firstMove Event
	for _, e := range bt.Events() {
		if e.Kind == EventMoveUsed {
			firstMove = e
			break
		}
	}
	if firstMove.Actor != "fast" {
		t.Fatalf("first move actor = %q, want faster trainer", firstMove.Actor)
	}
	if firstMove.MoveSlug != "jab" {
		t.Fatalf("first move slug = %q, want jab", firstMove.MoveSlug)
	}
}

func TestSpeedTieUsesInjectedCoinFlip(t *testing.T) {
	set := testSet()
	aFirst := soloBattle(t, set, mon("a1", "twin", "jab"), mon("b1", "twin", "poke"),
		&scriptRand{vals: concat([]float64{0.1}, hitNoCritMinVar(), hitNoCritMinVar())},
	)
	resolveTurn(t, aFirst, moveAct("jab"), moveAct("poke"))
	bFirst := soloBattle(t, set, mon("a1", "twin", "jab"), mon("b1", "twin", "poke"),
		&scriptRand{vals: concat([]float64{0.9}, hitNoCritMinVar(), hitNoCritMinVar())},
	)
	resolveTurn(t, bFirst, moveAct("jab"), moveAct("poke"))

	firstActor := func(bt *Battle) string {
		for _, e := range bt.Events() {
			if e.Kind == EventMoveUsed {
				return e.Actor
			}
		}
		return ""
	}
	if got := firstActor(aFirst); got != "a" {
		t.Fatalf("tie roll 0.1 first actor = %q, want a", got)
	}
	if got := firstActor(bFirst); got != "b" {
		t.Fatalf("tie roll 0.9 first actor = %q, want b", got)
	}
}

func TestKOShortCircuitsSecondMove(t *testing.T) {
	set := testSet()
	bt := soloBattle(t, set, mon("a1", "spark", "jab"), mon("b1", "frail", "leaf"),
		&scriptRand{vals: hitNoCritMinVar()},
	)
	resolveTurn(t, bt, moveAct("jab"), moveAct("leaf"))
	if bt.State() != StateOver {
		t.Fatalf("state = %s, want battle_over", bt.State())
	}
	if bt.Reason() != EndKO {
		t.Fatalf("reason = %s, want ko", bt.Reason())
	}
	if bt.Winner() != "a" {
		t.Fatalf("winner = %q, want a", bt.Winner())
	}
	got := kinds(bt.Events())
	moveUsed := 0
	for _, k := range got {
		if k == EventMoveUsed {
			moveUsed++
		}
	}
	if moveUsed != 1 {
		t.Fatalf("move_used count = %d, want 1 (second move short-circuited)", moveUsed)
	}
}

func TestForfeitEndsBattle(t *testing.T) {
	set := testSet()
	bt := soloBattle(t, set, mon("a1", "spark", "jab"), mon("b1", "moss", "leaf"), &scriptRand{})
	if err := bt.Forfeit("a"); err != nil {
		t.Fatal(err)
	}
	if bt.State() != StateOver || bt.Reason() != EndForfeit || bt.Winner() != "b" {
		t.Fatalf("state=%s reason=%s winner=%s", bt.State(), bt.Reason(), bt.Winner())
	}
}

func TestDisconnectTimeoutEndsBattle(t *testing.T) {
	set := testSet()
	bt := soloBattle(t, set, mon("a1", "spark", "jab"), mon("b1", "moss", "leaf"), &scriptRand{})
	if err := bt.DisconnectTimeout("b"); err != nil {
		t.Fatal(err)
	}
	if bt.State() != StateOver || bt.Reason() != EndDisconnectTimeout || bt.Winner() != "a" {
		t.Fatalf("state=%s reason=%s winner=%s", bt.State(), bt.Reason(), bt.Winner())
	}
}

func TestExpireDecisionEndsBattleWithHealthyReserves(t *testing.T) {
	set := testSet()
	bt := newBattle(t, set,
		partyOf("a", mon("a1", "spark", "jab"), mon("a2", "tank", "leaf")),
		partyOf("b", mon("b1", "moss", "leaf"), mon("b2", "tank", "leaf")),
		&scriptRand{},
	)
	if err := bt.ExpireDecision("a"); err != nil {
		t.Fatal(err)
	}
	if bt.State() != StateOver || bt.Reason() != EndDecisionTimeout || bt.Winner() != "b" {
		t.Fatalf("state=%s reason=%s winner=%s", bt.State(), bt.Reason(), bt.Winner())
	}
}

func TestSeededBattlesAreDeterministic(t *testing.T) {
	set := testSet()
	run := func() []Event {
		bt := soloBattle(t, set, mon("a1", "spark", "jab"), mon("b1", "moss", "leaf"), Seeded(42))
		resolveTurn(t, bt, moveAct("jab"), moveAct("leaf"))
		return bt.Events()
	}
	first, second := run(), run()
	if !slices.Equal(kinds(first), kinds(second)) {
		t.Fatalf("seeded event kinds differ:\n%v\n%v", kinds(first), kinds(second))
	}
}

func TestFighterSnapshot(t *testing.T) {
	set := testSet()
	bt := soloBattle(t, set, mon("a1", "spark", "jab", "bolt"), mon("b1", "moss", "leaf"), &scriptRand{})
	f, ok := bt.Fighter("a")
	if !ok {
		t.Fatal("missing fighter a")
	}
	if f.Name != "Spark" || f.Species != "spark" || f.Type != "thermal" {
		t.Fatalf("fighter = %+v", f)
	}
	if f.HP != 50 || f.MaxHP != 50 {
		t.Fatalf("hp %d/%d", f.HP, f.MaxHP)
	}
	if !slices.Equal(f.Moves, []string{"jab", "bolt"}) {
		t.Fatalf("moves %v", f.Moves)
	}
}

func TestBattleAllowsConcurrentInspect(t *testing.T) {
	set := testSet()
	bt := soloBattle(t, set, mon("a1", "spark", "jab"), mon("b1", "moss", "leaf"), Seeded(1))
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 200 {
			_ = bt.State()
			_ = bt.Events()
			_, _ = bt.Fighter("a")
			_ = bt.Snapshot("a")
			_, _ = bt.HP("b")
			_ = bt.Locked("a")
			_ = bt.Winner()
			_ = bt.Reason()
		}
	}()
	resolveTurn(t, bt, moveAct("jab"), moveAct("leaf"))
	<-done
}

func TestMoveVsMove(t *testing.T) {
	set := testSet()
	bt := soloBattle(t, set, mon("a1", "spark", "jab"), mon("b1", "moss", "leaf"),
		&scriptRand{vals: concat(hitNoCritMinVar(), hitNoCritMinVar())},
	)
	resolveTurn(t, bt, moveAct("jab"), moveAct("leaf"))
	if !slices.Contains(kinds(bt.Events()), EventMoveUsed) {
		t.Fatal("expected move resolution")
	}
}

func TestMoveVsSwitch(t *testing.T) {
	set := testSet()
	bt := newBattle(t, set,
		partyOf("a", mon("a1", "spark", "jab")),
		partyOf("b", mon("b1", "moss", "leaf"), mon("b2", "tank", "leaf", "jab")),
		&scriptRand{vals: concat(hitNoCritMinVar())},
	)
	resolveTurn(t, bt, moveAct("jab"), switchAct("b2"))
	if !slices.Contains(kinds(bt.Events()), EventSwitched) {
		t.Fatal("expected switch event")
	}
	foe, _ := bt.Fighter("b")
	if foe.Species != "tank" {
		t.Fatalf("foe active = %s, want tank after switch", foe.Species)
	}
	if foe.HP >= 60 {
		t.Fatalf("move should hit switched-in tank, hp=%d", foe.HP)
	}
}

func TestSwitchVsSwitch(t *testing.T) {
	set := testSet()
	bt := newBattle(t, set,
		partyOf("a", mon("a1", "spark", "jab"), mon("a2", "twin", "jab", "poke")),
		partyOf("b", mon("b1", "moss", "leaf"), mon("b2", "tank", "leaf")),
		&scriptRand{},
	)
	resolveTurn(t, bt, switchAct("a2"), switchAct("b2"))
	got := kinds(bt.Events())
	if countKind(got, EventSwitched) != 2 {
		t.Fatalf("events = %v, want two switched", got)
	}
	if countKind(got, EventMoveUsed) != 0 {
		t.Fatal("switch-switch should not resolve moves")
	}
}

func TestInvalidAndDuplicateSelections(t *testing.T) {
	set := testSet()
	bt := soloBattle(t, set, mon("a1", "spark", "jab"), mon("b1", "moss", "leaf"), &scriptRand{})
	if err := bt.Select("a", moveAct("nope")); err == nil {
		t.Fatal("unknown move should fail")
	}
	if err := bt.Select("a", moveAct("jab")); err != nil {
		t.Fatal(err)
	}
	if err := bt.Select("a", moveAct("jab")); !errors.Is(err, ErrAlreadySelected) {
		t.Fatalf("duplicate = %v, want ErrAlreadySelected", err)
	}
	if err := bt.Select("b", switchAct("b1")); err == nil {
		t.Fatal("switch to active should fail")
	}
}

func TestFirstMoveFaintCancelsSecondMove(t *testing.T) {
	set := testSet()
	bt := soloBattle(t, set, mon("a1", "spark", "jab"), mon("b1", "frail", "leaf"),
		&scriptRand{vals: concat(hitNoCritMinVar(), hitNoCritMinVar())},
	)
	resolveTurn(t, bt, moveAct("jab"), moveAct("leaf"))
	if countKind(kinds(bt.Events()), EventMoveUsed) != 1 {
		t.Fatal("fainted monster move should be cancelled")
	}
}

func TestMandatoryReplacement(t *testing.T) {
	set := testSet()
	bt := newBattle(t, set,
		partyOf("a", mon("a1", "spark", "jab"), mon("a2", "twin", "jab")),
		partyOf("b", mon("b1", "frail", "leaf"), mon("b2", "tank", "leaf")),
		&scriptRand{vals: hitNoCritMinVar()},
	)
	resolveTurn(t, bt, moveAct("jab"), moveAct("leaf"))
	if bt.State() != StateAwaitingReplacement {
		t.Fatalf("state = %s, want awaiting_replacement", bt.State())
	}
	if err := bt.Select("a", moveAct("jab")); !errors.Is(err, ErrNotAwaitingActions) {
		t.Fatalf("survivor select = %v", err)
	}
	if err := bt.Replace("b", "b2"); err != nil {
		t.Fatal(err)
	}
	if bt.State() != StateRevealing {
		t.Fatalf("after replace state = %s, want revealing", bt.State())
	}
	if err := bt.AdvanceReveal(); err != nil {
		t.Fatal(err)
	}
	if bt.State() != StateAwaitingActions {
		t.Fatalf("state = %s, want awaiting_actions", bt.State())
	}
}

func TestDamagePreservedAcrossSwitches(t *testing.T) {
	set := testSet()
	bt := newBattle(t, set,
		partyOf("a", mon("a1", "spark", "jab"), mon("a2", "twin", "jab")),
		partyOf("b", mon("b1", "moss", "leaf"), mon("b2", "tank", "leaf")),
		&scriptRand{vals: concat(hitNoCritMinVar(), hitNoCritMinVar())},
	)
	resolveTurn(t, bt, moveAct("jab"), moveAct("leaf"))
	damaged := bt.Snapshot("a").YourParty[0].HP
	if damaged >= 50 {
		t.Fatalf("expected damage on lead, hp=%d", damaged)
	}
	resolveTurn(t, bt, switchAct("a2"), moveAct("leaf"))
	snap := bt.Snapshot("a")
	if snap.YourParty[0].HP != damaged {
		t.Fatalf("switched-out HP changed: %d -> %d", damaged, snap.YourParty[0].HP)
	}
}

func TestVictoryAfterThirdFaint(t *testing.T) {
	set := testSet()
	makeTeam := func(prefix string) []game.Monster {
		return []game.Monster{
			mon(prefix+"1", "frail", "leaf"),
			mon(prefix+"2", "frail", "leaf"),
			mon(prefix+"3", "frail", "leaf"),
		}
	}
	bt := newBattle(t, set,
		partyOf("a", mon("a0", "spark", "jab")),
		partyOf("b", makeTeam("b")...),
		&scriptRand{vals: concat(hitNoCritMinVar(), hitNoCritMinVar(), hitNoCritMinVar())},
	)
	for i := 0; i < 3 && bt.State() != StateOver; i++ {
		if bt.State() == StateAwaitingReplacement {
			if err := bt.Replace("b", bt.Snapshot("b").YourParty[1].ID); err != nil {
				// pick next healthy
				for _, m := range bt.Snapshot("b").YourParty {
					if !m.Fainted && !m.Active {
						_ = bt.Replace("b", m.ID)
						break
					}
				}
			}
			advanceIfRevealing(t, bt)
		}
		if bt.State() == StateAwaitingActions {
			resolveTurn(t, bt, moveAct("jab"), moveAct("leaf"))
		}
	}
	if bt.State() != StateOver || bt.Winner() != "a" {
		t.Fatalf("state=%s winner=%q", bt.State(), bt.Winner())
	}
}

func TestSnapshotHidesFoeLoadoutAndLockKind(t *testing.T) {
	set := testSet()
	bt := soloBattle(t, set, mon("a1", "spark", "jab"), mon("b1", "moss", "leaf"), &scriptRand{})
	_ = bt.Select("a", moveAct("jab"))
	snapA := bt.Snapshot("a")
	if len(snapA.YourParty[0].Loadout) == 0 {
		t.Fatal("viewer should see own loadout")
	}
	for _, f := range snapA.FoeRoster {
		if f.HP != 0 && !f.Active {
			t.Fatal("foe reserve HP should be hidden")
		}
	}
	if !snapA.YouLocked || snapA.YouLockKind != ActionMove {
		t.Fatal("viewer should see own lock kind when locked")
	}
	if snapA.FoeLocked {
		t.Fatal("foe should not appear locked before they choose")
	}
	snapB := bt.Snapshot("b")
	if snapB.YouLocked {
		t.Fatal("foe viewer should not be locked before choosing")
	}
	if !snapB.FoeLocked {
		t.Fatal("foe viewer should see opponent locked")
	}
	foeFighter, _ := bt.Fighter("b")
	_ = foeFighter.Moves // convenience API still has moves; Snapshot is the privacy seam
}

func TestEventIdentityAcrossSwitch(t *testing.T) {
	set := testSet()
	bt := newBattle(t, set,
		partyOf("a", mon("a1", "spark", "jab"), mon("a2", "twin", "jab")),
		partyOf("b", mon("b1", "moss", "leaf")),
		&scriptRand{vals: concat(hitNoCritMinVar())},
	)
	resolveTurn(t, bt, switchAct("a2"), moveAct("leaf"))
	var sw Event
	for _, e := range bt.Events() {
		if e.Kind == EventSwitched {
			sw = e
			break
		}
	}
	if sw.MonsterID != "a2" || sw.Slot != 1 {
		t.Fatalf("switch event = %+v, want monster a2 slot 1", sw)
	}
}

func TestSendOutAndSwitchNameTheMonster(t *testing.T) {
	set := testSet()
	youID, foeID := "deadbeefdeadbeef", "foe-id-ffffffffffff"
	bt := newBattle(t, set,
		partyOf(youID, mon("a1", "spark", "jab"), mon("a2", "twin", "jab")),
		partyOf(foeID, mon("b1", "moss", "leaf")),
		&scriptRand{vals: hitNoCritMinVar()},
	)
	for _, e := range bt.Events() {
		if e.Kind != EventSendOut {
			continue
		}
		if strings.Contains(e.Text, youID) || strings.Contains(e.Text, foeID) {
			t.Fatalf("send-out leaked trainer id: %q", e.Text)
		}
		if !strings.Contains(e.Text, "Spark") && !strings.Contains(e.Text, "Moss") {
			t.Fatalf("send-out should name the Monster: %q", e.Text)
		}
	}
	resolveTurnTrainers(t, bt, youID, foeID, switchAct("a2"), moveAct("leaf"))
	var switched string
	for _, e := range bt.Events() {
		if e.Kind == EventSwitched {
			switched = e.Text
		}
	}
	if switched == "" {
		t.Fatal("expected a switch event")
	}
	if strings.Contains(switched, youID) || strings.Contains(switched, foeID) {
		t.Fatalf("switch leaked trainer id: %q", switched)
	}
	if !strings.Contains(switched, "Twin") {
		t.Fatalf("switch should name the incoming Monster: %q", switched)
	}
}

func TestClampedWildHitVariesWithVariance(t *testing.T) {
	set := testSet()
	dealt := func(rng Rand) int {
		t.Helper()
		wild := partyOf("b", mon("b1", "spark", "jab"))
		wild.ClampOutgoingDamage = true
		bt := newBattle(t, set, partyOf("a", mon("a1", "moss", "leaf")), wild, rng)
		resolveTurn(t, bt, moveAct("leaf"), moveAct("jab"))
		_, maxHP := bt.HP("a")
		hp, _ := bt.HP("a")
		return maxHP - hp
	}
	lo := dealt(&scriptRand{vals: concat(hitNoCritMinVar(), hitNoCritMinVar())})
	hi := dealt(&scriptRand{vals: concat(hitNoCritMaxVar(), hitNoCritMaxVar())})
	if lo == hi {
		t.Fatalf("clamped hits were both %d; variance should still show", lo)
	}
	moss := partyOf("a", mon("a1", "moss", "leaf"))
	bt := newBattle(t, set, moss, partyOf("b", mon("b1", "spark", "jab")), &scriptRand{})
	_, maxHP := bt.HP("a")
	limit := max(1, (maxHP-1)/5)
	if lo < 1 || hi > limit {
		t.Fatalf("clamped hits %d and %d, want 1..%d", lo, hi, limit)
	}
}

func countKind(kinds []EventKind, k EventKind) int {
	n := 0
	for _, kind := range kinds {
		if kind == k {
			n++
		}
	}
	return n
}
