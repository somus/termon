package battle

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"termon.sh/internal/content"
	"termon.sh/internal/game"
)

// Battle is the authoritative state for one duel. Mutations resolve while
// holding the lock; inspection methods are safe for concurrent TUI renders.
type Battle struct {
	mu sync.RWMutex

	set   *content.Set
	rng   Rand
	sides [2]sideState

	state         State
	turn          int
	actions       [2]Action
	locked        [2]bool
	lockKind      [2]ActionKind
	events        []Event
	eventsVersion uint64
	winner        string
	reason        EndReason

	// replaceSide is the party index (0 or 1) that must send out a replacement,
	// set while revealing after a faint and consumed by AdvanceReveal.
	replaceSide int
}

type memberState struct {
	id       string
	species  string
	nickname string
	level    int
	loadout  []string
	spec     content.Species
	hp       int
	maxHP    int
	atk      int
	def      int
	spa      int
	spe      int
	fainted  bool
}

type sideState struct {
	trainer             string
	members             []memberState
	active              int
	clampOutgoingDamage bool
}

// ErrBattleOver means the Battle has ended and can accept no further actions.
var ErrBattleOver = errors.New("battle: battle is over")

// ErrAlreadySelected means this Trainer's choice is locked in for the current
// phase and cannot be changed.
var ErrAlreadySelected = errors.New("battle: trainer already selected")

// ErrNotAwaitingActions means Select is only valid during awaiting_actions.
var ErrNotAwaitingActions = errors.New("battle: not awaiting actions")

// ErrNotAwaitingReplacement means Replace is only valid during awaiting_replacement.
var ErrNotAwaitingReplacement = errors.New("battle: not awaiting replacement")

// ErrNotRevealing means AdvanceReveal is a no-op outside revealing.
var ErrNotRevealing = errors.New("battle: not revealing")

// New starts a Battle with every Monster at full natural HP.
func New(set *content.Set, a, b Party, rng Rand) (*Battle, error) {
	if set == nil || rng == nil {
		return nil, errors.New("battle: nil argument")
	}
	if a.Trainer == "" || b.Trainer == "" || a.Trainer == b.Trainer {
		return nil, errors.New("battle: trainers must be distinct and non-empty")
	}
	bt := &Battle{set: set, rng: rng, replaceSide: -1}
	if err := bt.initSide(0, a); err != nil {
		return nil, err
	}
	if err := bt.initSide(1, b); err != nil {
		return nil, err
	}
	bt.emitStartSendOuts()
	bt.state = StateAwaitingActions
	return bt, nil
}

func (b *Battle) initSide(i int, p Party) error {
	if len(p.Members) < 1 || len(p.Members) > 3 {
		return fmt.Errorf("battle: trainer %q party size %d, want 1–3", p.Trainer, len(p.Members))
	}
	seen := make(map[string]struct{})
	s := sideState{trainer: p.Trainer}
	for slot, pm := range p.Members {
		m := pm.Monster
		if m.ID == "" {
			return fmt.Errorf("battle: trainer %q slot %d missing monster id", p.Trainer, slot)
		}
		if _, dup := seen[m.ID]; dup {
			return fmt.Errorf("battle: trainer %q duplicate monster id %q", p.Trainer, m.ID)
		}
		seen[m.ID] = struct{}{}
		if m.Species == "" {
			return fmt.Errorf("battle: trainer %q monster %q missing species", p.Trainer, m.ID)
		}
		spec, ok := b.set.Species[m.Species]
		if !ok {
			return fmt.Errorf("battle: unknown species %q", m.Species)
		}
		level := max(m.Level, 1)
		if len(m.BattleLoadout) < 1 || len(m.BattleLoadout) > 4 {
			return fmt.Errorf("battle: trainer %q monster %q loadout size %d, want 1–4", p.Trainer, m.ID, len(m.BattleLoadout))
		}
		for _, slug := range m.BattleLoadout {
			if _, ok := b.set.Moves[slug]; !ok {
				return fmt.Errorf("battle: unknown move %q", slug)
			}
		}
		maxHP := game.NaturalStat(spec.BaseStats.HP, level)
		atk := game.NaturalStat(spec.BaseStats.Attack, level)
		def := game.NaturalStat(spec.BaseStats.Defense, level)
		spa := game.NaturalStat(spec.BaseStats.SpAttack, level)
		spe := game.NaturalStat(spec.BaseStats.Speed, level)
		if pm.MaxHP > 0 {
			maxHP = pm.MaxHP
		}
		if pm.Stats != nil {
			maxHP, atk, def, spa, spe = pm.Stats[0], pm.Stats[1], pm.Stats[2], pm.Stats[3], pm.Stats[4]
		}
		if maxHP < 1 || def < 1 {
			return fmt.Errorf("battle: species %q has invalid battle stats", spec.Slug)
		}
		s.members = append(s.members, memberState{
			id: m.ID, species: m.Species, nickname: m.Nickname, level: level,
			loadout: append([]string(nil), m.BattleLoadout...),
			spec:    spec,
			hp:      maxHP, maxHP: maxHP,
			atk: atk, def: def, spa: spa, spe: spe,
		})
	}
	s.active = 0
	s.clampOutgoingDamage = p.ClampOutgoingDamage
	b.sides[i] = s
	return nil
}

func (b *Battle) emitStartSendOuts() {
	for i := range b.sides {
		m := b.sides[i].activeMember()
		b.appendEvents(Event{
			Actor: b.sides[i].trainer, MonsterID: m.id, Slot: b.sides[i].active,
			Kind: EventSendOut,
			Text: fmt.Sprintf("Go! %s!", memberName(m)),
		})
	}
}

// Turn reports the current turn number (0 before the first resolved turn).
func (b *Battle) Turn() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.turn
}

// State reports the Battle's current phase.
func (b *Battle) State() State {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}

// Events returns a snapshot of the accumulated event log.
func (b *Battle) Events() []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]Event(nil), b.events...)
}

// EventsVersion increments whenever the event log changes.
func (b *Battle) EventsVersion() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.eventsVersion
}

// EventCount reports how many events have accumulated without copying them.
func (b *Battle) EventCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.events)
}

func (b *Battle) appendEvents(events ...Event) {
	b.events = append(b.events, events...)
	b.eventsVersion++
}

// Fighter returns the active Monster for a Trainer. Prefer Snapshot for
// opponent loadouts; this helper always includes the requested Trainer's loadout.
func (b *Battle) Fighter(trainer string) (Fighter, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	i, ok := b.sideIndex(trainer)
	if !ok {
		return Fighter{}, false
	}
	m := b.sides[i].activeMember()
	return Fighter{
		Trainer: trainer,
		ID:      m.id,
		Name:    memberName(m),
		Species: m.spec.Slug,
		Type:    m.spec.Type,
		HP:      m.hp,
		MaxHP:   m.maxHP,
		Moves:   append([]string(nil), m.loadout...),
	}, true
}

// HP returns a Trainer's active Monster current and maximum HP.
func (b *Battle) HP(trainer string) (int, int) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	i, ok := b.sideIndex(trainer)
	if !ok {
		return 0, 0
	}
	m := b.sides[i].activeMember()
	return m.hp, m.maxHP
}

// Locked reports whether a Trainer has locked an action for the current phase.
func (b *Battle) Locked(trainer string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	i, ok := b.sideIndex(trainer)
	return ok && b.locked[i]
}

// Winner returns the winning Trainer fingerprint, or empty while active.
func (b *Battle) Winner() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.winner
}

// Reason returns the end reason, or empty while active.
func (b *Battle) Reason() EndReason {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.reason
}

// Snapshot returns a viewer-specific Battle view.
func (b *Battle) Snapshot(viewer string) Snapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()
	snap := Snapshot{
		Phase:  b.state,
		Turn:   b.turn,
		Winner: b.winner,
		Reason: b.reason,
	}
	you, ok := b.sideIndex(viewer)
	if !ok {
		return snap
	}
	foe := 1 - you
	for slot, m := range b.sides[you].members {
		sm := SnapshotMember{
			Slot: slot + 1, ID: m.id, Species: m.spec.Slug, Name: memberName(&m),
			HP: m.hp, MaxHP: m.maxHP, Fainted: m.fainted,
			Active:  slot == b.sides[you].active,
			Loadout: append([]string(nil), m.loadout...),
		}
		snap.YourParty = append(snap.YourParty, sm)
	}
	for _, m := range b.sides[foe].members {
		sf := SnapshotFoe{
			Species: m.spec.Slug, Name: memberName(&m), ID: m.id,
			Fainted: m.fainted, Active: m.id == b.sides[foe].activeMember().id,
		}
		if sf.Active {
			sf.HP, sf.MaxHP = m.hp, m.maxHP
		}
		snap.FoeRoster = append(snap.FoeRoster, sf)
	}
	snap.YouLocked = b.locked[you]
	if snap.YouLocked {
		snap.YouLockKind = b.lockKind[you]
	}
	snap.FoeLocked = b.locked[foe]
	snap.ReplacementRequired = b.state == StateAwaitingReplacement && b.replaceSide == you
	return snap
}

// FieldedHPAfter projects, for viewer, each side's fielded Monster as of the
// log position after the first n events: the Monster the viewer had fielded
// and the opponent Monster active at that position. That is the only HP a
// viewer was ever shown, so the projection is structurally unable to expose a
// benched Monster's HP or MaxHP. Before a side's send-out beat has played,
// its opening lead is the fielded Monster.
func (b *Battle) FieldedHPAfter(viewer string, n int) (you, foe FieldedHP) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	view, ok := b.sideIndex(viewer)
	if !ok {
		return FieldedHP{}, FieldedHP{}
	}
	opp := 1 - view
	n = min(max(n, 0), len(b.events))
	youID, foeID := b.fieldedAt(view, n), b.fieldedAt(opp, n)
	you, foe = b.plate(view, youID), b.plate(opp, foeID)
	for i := len(b.events) - 1; i >= n; i-- {
		e := b.events[i]
		if e.Kind != EventDamageDealt || e.Damage <= 0 {
			continue
		}
		switch e.TargetID {
		case youID:
			you.HP += e.Damage
		case foeID:
			foe.HP += e.Damage
		}
	}
	return you, foe
}

// fieldedAt names the Monster a side had fielded as of the log position after
// the first n events: its last fielding event at or before that position, or
// the opening lead before any fielding event.
func (b *Battle) fieldedAt(side int, n int) string {
	trainer := b.sides[side].trainer
	for i := n - 1; i >= 0; i-- {
		e := b.events[i]
		if e.Actor == trainer && fieldingKind(e.Kind) {
			return e.MonsterID
		}
	}
	return b.sides[side].members[0].id
}

func (b *Battle) plate(side int, id string) FieldedHP {
	idx, ok := b.memberIndex(side, id)
	if !ok {
		return FieldedHP{}
	}
	m := &b.sides[side].members[idx]
	return FieldedHP{MonsterID: m.id, HP: m.hp, MaxHP: m.maxHP}
}

func fieldingKind(k EventKind) bool {
	switch k {
	case EventSendOut, EventSwitched, EventReplacement:
		return true
	default:
		return false
	}
}

// Select locks a Trainer's hidden Battle Action. The second lock resolves the
// turn synchronously.
func (b *Battle) Select(trainer string, action Action) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == StateOver {
		return ErrBattleOver
	}
	if b.state != StateAwaitingActions {
		return ErrNotAwaitingActions
	}
	i, ok := b.sideIndex(trainer)
	if !ok {
		return fmt.Errorf("battle: unknown trainer %q", trainer)
	}
	if b.locked[i] {
		return ErrAlreadySelected
	}
	switch action.Kind {
	case ActionMove:
		if !b.knowsMove(i, action.Move) {
			return fmt.Errorf("battle: trainer %q does not know move %q", trainer, action.Move)
		}
	case ActionSwitch:
		if !b.validSwitch(i, action.SwitchTo) {
			return fmt.Errorf("battle: trainer %q cannot switch to %q", trainer, action.SwitchTo)
		}
	default:
		return fmt.Errorf("battle: unknown action kind %q", action.Kind)
	}
	b.actions[i] = action
	b.lockKind[i] = action.Kind
	b.locked[i] = true
	if b.locked[0] && b.locked[1] {
		b.resolveTurn()
	}
	return nil
}

// Replace sends out a healthy reserve during awaiting_replacement.
func (b *Battle) Replace(trainer, monsterID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == StateOver {
		return ErrBattleOver
	}
	if b.state != StateAwaitingReplacement {
		return ErrNotAwaitingReplacement
	}
	i, ok := b.sideIndex(trainer)
	if !ok {
		return fmt.Errorf("battle: unknown trainer %q", trainer)
	}
	if b.replaceSide != i {
		return fmt.Errorf("battle: trainer %q is not awaiting replacement", trainer)
	}
	idx, ok := b.memberIndex(i, monsterID)
	if !ok || idx == b.sides[i].active || b.sides[i].members[idx].fainted || b.sides[i].members[idx].hp <= 0 {
		return fmt.Errorf("battle: invalid replacement %q", monsterID)
	}
	b.sides[i].active = idx
	m := b.sides[i].activeMember()
	b.appendEvents(Event{
		Turn: b.turn, Actor: trainer, MonsterID: m.id, Slot: idx,
		Kind: EventReplacement,
		Text: fmt.Sprintf("Go! %s!", memberName(m)),
	})
	b.replaceSide = -1
	b.state = StateRevealing
	return nil
}

// AdvanceReveal leaves the revealing playback window for the next phase.
func (b *Battle) AdvanceReveal() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state != StateRevealing {
		return ErrNotRevealing
	}
	if b.replaceSide >= 0 {
		b.state = StateAwaitingReplacement
		return nil
	}
	b.state = StateAwaitingActions
	return nil
}

// Forfeit ends the Battle with the surrendering Trainer as the loser.
func (b *Battle) Forfeit(trainer string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.endByLoss(trainer, EndForfeit, EventForfeit, "forfeited")
}

// DisconnectTimeout ends the Battle after reconnect grace expires.
func (b *Battle) DisconnectTimeout(trainer string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.endByLoss(trainer, EndDisconnectTimeout, EventForfeit, "lost connection")
}

// ExpireDecision ends the Battle when a Decision Clock expires.
func (b *Battle) ExpireDecision(trainer string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.endByLoss(trainer, EndDecisionTimeout, EventDecisionTimeout, "ran out of time")
}

func (b *Battle) resolveTurn() {
	b.state = StateResolvingTurn
	b.turn++
	b.appendEvents(Event{
		Turn: b.turn, Kind: EventTurnStarted,
		Text: fmt.Sprintf("Turn %d", b.turn),
	})

	cancelled := [2]bool{}
	b.executeSwitches()

	moves := b.collectMoves()
	order := b.moveOrder(moves)
	for _, side := range order {
		if cancelled[side] || b.actions[side].Kind != ActionMove {
			continue
		}
		def := 1 - side
		if b.executeMove(side, def) && b.sides[def].activeMember().fainted {
			cancelled[def] = true
		}
		if b.allFainted(def) {
			b.endWin(side, EndKO)
			b.clearTurnLocks()
			return
		}
	}

	b.clearTurnLocks()
	if b.state == StateOver {
		return
	}
	if b.checkReplacement() {
		b.state = StateRevealing
		return
	}
	b.state = StateRevealing
}

func (b *Battle) executeSwitches() {
	for _, side := range [2]int{0, 1} {
		if b.actions[side].Kind != ActionSwitch {
			continue
		}
		idx, ok := b.memberIndex(side, b.actions[side].SwitchTo)
		if !ok {
			continue
		}
		b.sides[side].active = idx
		m := b.sides[side].activeMember()
		b.appendEvents(Event{
			Turn: b.turn, Actor: b.sides[side].trainer, MonsterID: m.id, Slot: idx,
			Kind: EventSwitched,
			Text: fmt.Sprintf("Switched to %s!", memberName(m)),
		})
	}
}

func (b *Battle) collectMoves() []int {
	var out []int
	for side := range 2 {
		if b.actions[side].Kind == ActionMove {
			out = append(out, side)
		}
	}
	return out
}

func (b *Battle) moveOrder(moves []int) []int {
	if len(moves) == 0 {
		return nil
	}
	if len(moves) == 1 {
		return []int{moves[0]}
	}
	a, c := moves[0], moves[1]
	speedA := b.sides[a].activeMember().spe
	speedC := b.sides[c].activeMember().spe
	first, second := a, c
	if speedC > speedA || (speedC == speedA && b.rng.Float64() >= 0.5) {
		first, second = c, a
	}
	return []int{first, second}
}

func (b *Battle) executeMove(attacker, defender int) (fainted bool) {
	atk := b.sides[attacker].activeMember()
	def := b.sides[defender].activeMember()
	move := b.set.Moves[b.actions[attacker].Move]

	b.appendEvents(Event{
		Turn: b.turn, Actor: b.sides[attacker].trainer, MonsterID: atk.id, Slot: b.sides[attacker].active,
		TargetID: def.id, MoveSlug: move.Slug, Kind: EventMoveUsed,
		Text: fmt.Sprintf("%s used %s!", memberName(atk), move.Name),
	})
	if b.rng.Float64()*100 >= move.Accuracy {
		b.appendEvents(Event{
			Turn: b.turn, Actor: b.sides[attacker].trainer, Kind: EventMissed,
			Text: "...it missed!",
		})
		return false
	}

	attack := atk.atk
	if move.Category == "special" {
		attack = atk.spa
	}
	effectiveness := b.set.Effectiveness(move.Type, def.spec.Type)
	damage := DamageBase(move.Power, attack, def.def, move.Type, atk.spec.Type, effectiveness)
	critical := b.rng.Float64() < 1.0/CritChance
	if critical {
		damage *= CritMultiplier
		b.appendEvents(Event{
			Turn: b.turn, Actor: b.sides[attacker].trainer, Kind: EventCriticalHit,
			Text: "A critical hit!",
		})
	}
	switch {
	case effectiveness >= SuperEffectiveAt:
		b.appendEvents(Event{
			Turn: b.turn, Actor: b.sides[attacker].trainer, Kind: EventSuperEffective,
			Text: "It's super effective!",
		})
	case effectiveness > 0 && effectiveness < ResistedBelow:
		b.appendEvents(Event{
			Turn: b.turn, Actor: b.sides[attacker].trainer, Kind: EventNotVeryEffective,
			Text: "It's not very effective...",
		})
	}
	// One clamp suffices: variance never exceeds 1.0, so clamping after the
	// roll bounds the dealt value exactly as a pre-roll float clamp would.
	if b.sides[attacker].clampOutgoingDamage {
		limit := float64(max(1, (def.maxHP-1)/5))
		if damage > limit {
			damage = limit
		}
	}
	damage *= VarianceMin + (VarianceMax-VarianceMin)*b.rng.Float64()
	dealt := max(MinDamage, int(damage))
	// The pre-variance clamp above already bounds dealt below the limit
	// (variance never exceeds 1.0), so no post-roll clamp is needed.
	dealt = min(dealt, def.hp)
	def.hp -= dealt
	b.appendEvents(Event{
		Turn: b.turn, Actor: b.sides[attacker].trainer, TargetID: def.id,
		Kind: EventDamageDealt, Damage: dealt,
		Text: fmt.Sprintf("%s took %d damage.", memberName(def), dealt),
	})
	if def.hp > 0 {
		return false
	}
	def.fainted = true
	b.appendEvents(Event{
		Turn: b.turn, Actor: b.sides[defender].trainer, MonsterID: def.id, Slot: b.sides[defender].active,
		Kind: EventFainted,
		Text: memberName(def) + " fainted!",
	})
	return true
}

func (b *Battle) checkReplacement() bool {
	for side := range 2 {
		m := b.sides[side].activeMember()
		if !m.fainted {
			continue
		}
		if b.hasHealthyReserve(side) {
			b.replaceSide = side
			return true
		}
		other := 1 - side
		b.endWin(other, EndKO)
		return false
	}
	return false
}

func (b *Battle) endWin(winner int, reason EndReason) {
	if b.state == StateOver {
		return
	}
	b.state = StateOver
	b.reason = reason
	b.winner = b.sides[winner].trainer
	w := b.sides[winner].activeMember()
	b.appendEvents(Event{
		Turn: b.turn, Actor: b.winner, Kind: EventBattleOver,
		Text: memberName(w) + " wins!",
	})
}

func (b *Battle) endByLoss(loser string, reason EndReason, kind EventKind, action string) error {
	if b.state == StateOver {
		return ErrBattleOver
	}
	loserIndex, ok := b.sideIndex(loser)
	if !ok {
		return fmt.Errorf("battle: unknown trainer %q", loser)
	}
	winnerIndex := 1 - loserIndex
	b.state = StateOver
	b.reason = reason
	b.winner = b.sides[winnerIndex].trainer
	b.replaceSide = -1
	b.actions = [2]Action{}
	b.locked = [2]bool{}
	b.appendEvents(
		Event{
			Turn: b.turn, Actor: loser, Kind: kind,
			Text: fmt.Sprintf("%s %s.", memberName(b.sides[loserIndex].activeMember()), action),
		},
		Event{
			Turn: b.turn, Actor: b.winner, Kind: EventBattleOver,
			Text: memberName(b.sides[winnerIndex].activeMember()) + " wins!",
		},
	)
	return nil
}

func (b *Battle) clearTurnLocks() {
	b.actions = [2]Action{}
	b.locked = [2]bool{}
	b.lockKind = [2]ActionKind{}
}

func (b *Battle) sideIndex(trainer string) (int, bool) {
	for i := range b.sides {
		if b.sides[i].trainer == trainer {
			return i, true
		}
	}
	return 0, false
}

func (b *Battle) memberIndex(side int, id string) (int, bool) {
	for i, m := range b.sides[side].members {
		if m.id == id {
			return i, true
		}
	}
	return 0, false
}

func (b *Battle) knowsMove(side int, slug string) bool {
	m := b.sides[side].activeMember()
	if slices.Contains(m.loadout, slug) {
		_, ok := b.set.Moves[slug]
		return ok
	}
	return false
}

func (b *Battle) validSwitch(side int, monsterID string) bool {
	idx, ok := b.memberIndex(side, monsterID)
	if !ok || idx == b.sides[side].active {
		return false
	}
	m := b.sides[side].members[idx]
	return !m.fainted && m.hp > 0
}

func (b *Battle) hasHealthyReserve(side int) bool {
	for i, m := range b.sides[side].members {
		if i != b.sides[side].active && !m.fainted && m.hp > 0 {
			return true
		}
	}
	return false
}

func (b *Battle) allFainted(side int) bool {
	for _, m := range b.sides[side].members {
		if !m.fainted {
			return false
		}
	}
	return true
}

func (s *sideState) activeMember() *memberState {
	return &s.members[s.active]
}

func memberName(m *memberState) string {
	if m.nickname != "" {
		return m.nickname
	}
	return m.spec.Name
}
