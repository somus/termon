package tui

import (
	"termon.sh/internal/battle"
	"termon.sh/internal/server"
)

// These adapters keep the characterization tests focused on behavior while
// their fixture state now lives behind Model.battle.
func (m *Model) battleEvents() []battle.Event {
	battleScreen := m.battle.withContext(m.battleContext())
	events := battleScreen.battleEvents()
	m.battle = battleScreen.withoutContext()
	return events
}

func (m *Model) applyBeat(event battle.Event) {
	battleScreen := m.battle.withContext(m.battleContext())
	battleScreen.applyBeat(event)
	m.battle = battleScreen.withoutContext()
}

func (m *Model) continuePlayback() {
	battleScreen := m.battle.withContext(m.battleContext())
	battleScreen.continuePlayback()
	m.battle = battleScreen.withoutContext()
}

func (m *Model) finishPlayback() {
	battleScreen := m.battle.withContext(m.battleContext())
	battleScreen.finishPlayback()
	m.storeBattleScreen(battleScreen)
}

func (m *Model) nextPlayEvent() {
	battleScreen := m.battle.withContext(m.battleContext())
	battleScreen.nextPlayEvent()
	m.storeBattleScreen(battleScreen)
}

func (m *Model) storeBattleScreen(battleScreen battleScreenModel) {
	outcome := battleScreen.outcome
	battleScreen.outcome = battleOutcome{}
	m.applyBattleUpdate(battleUpdateResult{
		model:         battleScreen.withoutContext(),
		battleOutcome: outcome,
	})
}

func (m Model) battleSnap() battle.Snapshot {
	return m.battle.withContext(m.battleContext()).battleSnap()
}

func (m *Model) applyBattle(msg server.BattleMsg) {
	m.applyBattleUpdate(m.battle.update(
		battleStartInput{session: battleSessionFrom(msg)},
		m.battleContext(),
	))
}

func (m *Model) syncBattleAnims() {
	battleScreen := m.battle.withContext(m.battleContext())
	battleScreen.syncBattleAnims()
	m.battle = battleScreen.withoutContext()
}

func (m *Model) openPendingProgression() {
	battleScreen := m.battle.withContext(m.battleContext())
	battleScreen.openPendingProgression()
	m.storeBattleScreen(battleScreen)
}

func (m Model) arenaFighters() (battle.Fighter, battle.Fighter) {
	return m.battle.withContext(m.battleContext()).arenaFighters()
}

func (m Model) battleFooter() string {
	return m.battle.withContext(m.battleContext()).footer()
}

func (m Model) battleLogMaxTop() int {
	battleScreen := m.battle.withContext(m.battleContext())
	return battleScreen.battleLogMaxTop()
}

func (m Model) battlePoses() (string, string) {
	return m.battle.withContext(m.battleContext()).battlePoses()
}

func (m Model) captureLanded() bool {
	return m.battle.withContext(m.battleContext()).captureLanded()
}

func (m Model) commandRow(cursor int) string {
	return m.battle.withContext(m.battleContext()).commandRow(cursor)
}

func (m Model) introSlide() (float64, float64) {
	return m.battle.withContext(m.battleContext()).introSlide()
}

func (m Model) placeSprites(height int) (spritePlace, spritePlace) { //nolint:unparam // Fixtures use the canonical arena height.
	return m.battle.withContext(m.battleContext()).placeSprites(height)
}

func (m Model) playEvent() (battle.Event, bool) {
	battleScreen := m.battle.withContext(m.battleContext())
	return battleScreen.playEvent()
}

func (m Model) playheadEnd() int {
	battleScreen := m.battle.withContext(m.battleContext())
	return battleScreen.playheadEnd()
}

func (m Model) renderArena(height int) string {
	return m.battle.withContext(m.battleContext()).renderArena(height)
}

func (m Model) renderBattle() string {
	return m.battle.view(m.battleContext())
}

func (m Model) renderBattleMsg() string {
	return m.battle.withContext(m.battleContext()).renderBattleMsg()
}

func (m Model) renderWipe() string {
	return renderWipe(m.width, m.height, m.wipeHold)
}
