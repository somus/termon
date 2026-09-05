package tui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"termon.sh/internal/battle"
	"termon.sh/internal/capture"
	"termon.sh/internal/content"
	"termon.sh/internal/game"
	"termon.sh/internal/lobby"
	"termon.sh/internal/server"
	"termon.sh/internal/store"
)

func (p *player) waitTypewriter() {
	p.t.Helper()
	for range 120 {
		if p.talkReady() {
			return
		}
		p.tick()
	}
	p.t.Fatal(p.dump("typewriter never finished"))
}

func (p *player) talkReady() bool {
	o := p.m.onboard
	switch o.stage {
	case stageWelcome:
		return o.lineReady(titleTag)
	case stageTalk:
		return o.lineReady(talkPages[o.page])
	case stageHandleOK:
		return o.lineReady(o.handleOKText())
	case stageJoined:
		return o.lineReady(o.joinedText(p.set))
	case stageLesson:
		return o.lineReady(lessonPages[o.page])
	default:
		return true
	}
}

func (p *player) finishOnboard() {
	p.t.Helper()
	for range 40 {
		if p.m.save != nil {
			p.flush()
			for range 50 {
				if p.m.screen == screenBattle {
					return
				}
				p.tick()
			}
			return
		}
		if p.m.screen != screenOnboard {
			p.t.Fatal(p.dump("left onboard without a save"))
		}
		p.waitTypewriter()
		p.press("enter")
	}
	p.t.Fatal(p.dump("onboard did not persist a save"))
}

func (p *player) walkAdjacentTo(tx, ty int) {
	p.t.Helper()
	geom := lobby.NewDojo()
	start := point{p.m.snap.You.X, p.m.snap.You.Y}
	if absInt(start.x-tx)+absInt(start.y-ty) == 1 {
		return
	}
	occ := map[point]bool{}
	for _, other := range p.m.snap.Others {
		occ[point{other.X, other.Y}] = true
	}
	path := walkPath(geom, occ, start, tx, ty)
	if path == nil {
		p.t.Fatalf("no walk to (%d,%d) from (%d,%d)", tx, ty, start.x, start.y)
	}
	keys := []string{"up", "down", "left", "right"}
	deltas := []point{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}
	for i := 1; i < len(path); i++ {
		step := point{path[i].x - path[i-1].x, path[i].y - path[i-1].y}
		key := ""
		for d, delta := range deltas {
			if delta == step {
				key = keys[d]
				break
			}
		}
		if key == "" {
			p.t.Fatalf("bad path step %+v", step)
		}
		before := point{p.m.snap.You.X, p.m.snap.You.Y}
		p.press(key)
		after := point{p.m.snap.You.X, p.m.snap.You.Y}
		if after == before {
			p.t.Fatal(p.dump("move did not change tile"))
		}
	}
	if absInt(p.m.snap.You.X-tx)+absInt(p.m.snap.You.Y-ty) != 1 {
		p.t.Fatal(p.dump(fmt.Sprintf("not adjacent to (%d,%d)", tx, ty)))
	}
}

type point struct{ x, y int }

func walkPath(geom *lobby.Room, occ map[point]bool, start point, tx, ty int) []point {
	type node struct {
		at     point
		parent int
	}
	goal := func(at point) bool {
		return absInt(at.x-tx)+absInt(at.y-ty) == 1 && !geom.Blocked(at.x, at.y)
	}
	if goal(start) {
		return []point{start}
	}
	seen := map[point]bool{start: true}
	queue := []node{{at: start, parent: -1}}
	deltas := []point{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}
	for i := 0; i < len(queue); i++ {
		cur := queue[i]
		for _, d := range deltas {
			next := point{cur.at.x + d.x, cur.at.y + d.y}
			if seen[next] || geom.Blocked(next.x, next.y) || occ[next] {
				continue
			}
			seen[next] = true
			queue = append(queue, node{at: next, parent: i})
			if goal(next) {
				var path []point
				for idx := len(queue) - 1; idx >= 0; idx = queue[idx].parent {
					path = append([]point{queue[idx].at}, path...)
					if queue[idx].parent < 0 {
						break
					}
				}
				return path
			}
		}
	}
	return nil
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (p *player) settleBattle() {
	p.t.Helper()
	for range 400 {
		p.flush()
		if p.m.screen != screenBattle {
			return
		}
		if p.m.wipeHold > 0 || p.m.battle.introHold > 0 {
			p.tick()
			continue
		}
		if p.m.battle.battleIntro {
			p.press("enter")
			continue
		}
		if p.m.battle.playing {
			// Path tests cover routing through complete Battles, while focused
			// playback tests cover animation timing. Consume every event and
			// group transition directly so -race does not instrument hundreds
			// of redundant animation ticks per scenario.
			for p.m.battle.playing {
				p.m.battle.playGrace = 0
				if p.m.battle.playPause {
					p.m.battle.continuePlayback()
					continue
				}
				p.m.battle.nextPlayEvent()
			}
			p.tick()
			continue
		}
		if p.m.battle.session.battle != nil && p.m.battle.session.battle.State() == battle.StateRevealing {
			if p.m.captureLanded() {
				return
			}
			p.skipReveal()
			if p.m.screen != screenBattle {
				return
			}
			if p.m.battle.session.battle != nil && p.m.battle.session.battle.State() == battle.StateRevealing &&
				p.m.battle.hasPendingProgression {
				p.m.openPendingProgression()
				return
			}
			continue
		}
		if p.m.battle.resultHold > 0 {
			p.tick()
			continue
		}
		return
	}
	p.t.Fatal(p.dump("battle never settled") + "\n" + p.battleDebug())
}

func (p *player) skipReveal() {
	if p.m.battle.session.battle == nil {
		return
	}
	if p.m.battle.session.battle.State() != battle.StateRevealing {
		return
	}
	p.m.battle.playing = false
	p.m.battle.playPause = false
	p.m.battle.playHold = 0
	p.m.battle.playGrace = 0
	p.m.battle.revealPending = false
	p.m.battle.playSeen = p.m.battle.session.battle.EventCount()
	if err := p.hub.AdvanceReveal(p.m.hash); err != nil {
		p.flush()
		return
	}
	for len(p.inbox) > 0 {
		msg := p.inbox[0]
		p.inbox = p.inbox[1:]
		p.apply(msg)
	}
	p.m.battle.playing = false
	if p.m.battle.session.battle != nil {
		p.m.battle.playSeen = p.m.battle.session.battle.EventCount()
	}
}

func (p *player) stepPath() {
	p.flush()
	switch p.m.screen {
	case screenProgression, screenExpedition:
		p.press("enter")
	case screenQueue:
		p.tick()
	case screenBattle:
		p.actBattle()
	default:
		p.tick()
	}
}

func (p *player) playUntil(done func() bool) {
	p.t.Helper()
	p.usedMove = map[string]struct{}{}
	p.switched = false
	for range 4000 {
		p.flush()
		if done() {
			return
		}
		p.stepPath()
	}
	p.t.Fatal(p.dump("path battle did not finish") + "\n" + p.battleDebug())
}

func (p *player) battleDebug() string {
	if p.m.battle.session.battle == nil {
		return "battle=nil"
	}
	snap := p.m.battleSnap()
	return fmt.Sprintf("phase=%s turn=%d locked=%v fightRoot=%v switchRoot=%v playing=%v intro=%v captureOn=%v gauge=%d events=%d reserves=%d objs=%v used=%d switched=%v",
		snap.Phase, snap.Turn, p.m.battle.session.battle.Locked(p.m.battle.session.you),
		p.m.battle.fightRoot, p.m.battle.switchRoot, p.m.battle.playing, p.m.battle.battleIntro,
		p.m.battle.captureOn, p.m.battle.capture.Gauge, p.m.battle.session.battle.EventCount(),
		len(snap.HealthyReserves()), p.m.battle.capture.Objectives, len(p.usedMove), p.switched)
}

func playPair(a, b *player, done func() bool) {
	a.t.Helper()
	a.usedMove = map[string]struct{}{}
	b.usedMove = map[string]struct{}{}
	a.switched = false
	b.switched = false
	for range 4000 {
		a.flush()
		b.flush()
		if done() {
			return
		}
		a.stepPath()
		b.stepPath()
	}
	a.t.Fatal(a.dump("paired path battle did not finish") + "\n" + a.battleDebug() + "\n" + b.dump("other") + "\n" + b.battleDebug())
}

func (p *player) actBattle() {
	p.t.Helper()
	p.settleBattle()
	if p.m.screen != screenBattle || p.m.battle.session.battle == nil {
		return
	}
	p.skipReveal()
	if p.m.screen != screenBattle || p.m.battle.session.battle == nil {
		return
	}
	if p.m.battle.session.battle.Turn() == 0 && p.m.battle.session.battle.EventCount() <= 2 {
		p.usedMove = map[string]struct{}{}
		p.switched = false
	}
	snap := p.m.battleSnap()
	if snap.Phase == battle.StateOver {
		if p.m.battle.hasPendingProgression || p.m.battle.hasPendingBattle {
			p.tick()
			return
		}
		p.press("enter")
		// Snapshot holds the Over screen until dismissed; if the hub already
		// released the match, force the lobby view so the driver can continue.
		if p.m.screen == screenBattle && !p.m.snap.You.InBattle {
			p.m.battle.session = battleSession{}
			p.m.battle.playing = false
			p.apply(p.hub.Snapshot(p.m.hash))
		}
		return
	}
	if p.m.battle.captureOn && p.m.battle.capture.Gauge >= 100 {
		p.flush()
		p.press("enter")
		p.tick()
		return
	}
	if p.m.battle.session.battle.Locked(p.m.battle.session.you) {
		p.tick()
		return
	}
	if snap.ReplacementRequired {
		p.press("1")
		return
	}
	if p.m.battle.session.battle.State() == battle.StateAwaitingReplacement {
		p.tick()
		return
	}
	if p.wantSwitch(snap) {
		if p.m.battle.switchRoot {
			p.switched = true
			p.press("1")
			return
		}
		if p.m.battle.fightRoot {
			p.press("s")
			return
		}
		p.press("esc")
		return
	}
	if p.m.battle.fightRoot {
		p.press("enter")
		return
	}
	if p.m.battle.switchRoot {
		p.press("esc")
		return
	}
	idx := p.pickMove(snap)
	p.press(strconv.Itoa(idx + 1))
}

func (p *player) wantSwitch(snap battle.Snapshot) bool {
	if p.switched || len(snap.HealthyReserves()) == 0 {
		return false
	}
	if p.m.battle.expeditionPhase == "prep1" || p.m.battle.expeditionPhase == "prep2" {
		return false
	}
	if p.m.battle.captureOn && p.hasObjective(capture.SafeSwitch) {
		if p.objectiveDone(capture.SafeSwitch) {
			return false
		}
		// Two distinct chips first so the post-switch SE hit can finish variety.
		if !p.objectiveDone(capture.ShowMoveVariety) && len(p.usedMove) < 2 {
			return false
		}
		return true
	}
	return len(snap.YourParty) >= 2 && len(snap.FoeRoster) == 1
}

func (p *player) hasObjective(id capture.ObjectiveID) bool {
	for _, o := range p.m.battle.capture.Objectives {
		if o.ID == id {
			return true
		}
	}
	return false
}

func (p *player) objectiveDone(id capture.ObjectiveID) bool {
	if len(p.m.battle.capture.Objectives) == 0 {
		return false
	}
	for _, o := range p.m.battle.capture.Objectives {
		if o.ID == id {
			return o.Done
		}
	}
	return true
}

func (p *player) pickMove(snap battle.Snapshot) int {
	moves := snap.YourPartyActiveLoadout()
	if len(moves) == 0 {
		return 0
	}
	foeType := ""
	wildHP, wildMax := 0, 0
	for _, f := range snap.FoeRoster {
		if !f.Active {
			continue
		}
		wildHP, wildMax = f.HP, f.MaxHP
		if spec, ok := p.set.Species[f.Species]; ok {
			foeType = spec.Type
		}
		break
	}
	capturing := p.m.battle.captureOn && p.m.battle.capture.Gauge < 100
	needVariety := capturing && !p.objectiveDone(capture.ShowMoveVariety)
	needSE := capturing && !p.objectiveDone(capture.ReadTheMatchup)
	best, bestScore := 0, -1_000_000
	for i, slug := range moves {
		score := 0
		mv, ok := p.set.Moves[slug]
		if !ok {
			continue
		}
		if needVariety {
			if _, used := p.usedMove[slug]; !used {
				score += 50
			} else {
				score -= 20
			}
		}
		if needSE && p.set.Effectiveness(mv.Type, foeType) >= battle.SuperEffectiveAt {
			score += 80
		}
		if capturing {
			score -= int(mv.Power)
			if mv.Accuracy < 100 {
				if needVariety {
					score -= 15
				} else {
					score -= 200
				}
			}
			if wildMax > 0 && wildHP*2 <= wildMax {
				score -= int(mv.Power)
			}
		} else {
			score += int(mv.Power)
		}
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	p.usedMove[moves[best]] = struct{}{}
	return best
}

func (p *player) completeLesson(n int) {
	p.t.Helper()
	before := partySize(p.save())
	p.playLessonUntilProgression(n, before)
	if p.m.screen == screenProgression {
		p.press("enter")
	}
	if partySize(p.save()) > before {
		return
	}
	p.t.Fatal(p.dump("lesson did not fill a party slot"))
}

func (p *player) playLessonUntilProgression(n, before int) {
	p.t.Helper()
	for range 12 {
		if p.m.screen == screenProgression {
			return
		}
		if p.m.screen != screenBattle {
			p.walkAdjacentTo(lobby.MasterX, lobby.MasterY)
			p.press("enter")
			if p.m.screen != screenDojoMenu {
				p.t.Fatal(p.dump("expected Dojo Master menu"))
			}
			for range n - 1 {
				p.press("down")
			}
			p.press("enter")
			if p.m.screen != screenBattle {
				p.t.Fatal(p.dump("lesson did not start"))
			}
		}
		sawBattle := true
		p.playUntil(func() bool {
			if p.m.screen == screenBattle {
				sawBattle = true
				return false
			}
			if p.m.screen == screenProgression {
				return true
			}
			if partySize(p.save()) > before {
				return p.m.screen == screenLobby && !p.m.snap.You.InBattle
			}
			return sawBattle && p.m.screen == screenLobby && !p.m.snap.You.InBattle
		})
		if p.m.screen == screenProgression {
			return
		}
		if partySize(p.save()) > before {
			return
		}
	}
	p.t.Fatal(p.dump("lesson did not fill a party slot"))
}

func partySize(sv *game.Save) int {
	if sv == nil {
		return 0
	}
	n := 0
	for _, id := range sv.Party {
		if id != "" {
			n++
		}
	}
	return n
}

func (p *player) openWorkbenchRoundtrip() {
	p.t.Helper()
	p.press("p")
	if p.m.screen != screenWorkbench {
		p.t.Fatal(p.dump("expected Workbench"))
	}
	if !strings.Contains(p.view(), "Collection") {
		p.t.Fatal(p.dump("Workbench missing Collection"))
	}
	p.press("esc")
	if p.m.screen != screenLobby {
		p.t.Fatal(p.dump("esc should return to the Dojo"))
	}
}

func joinReady(t *testing.T, hub *server.Hub, set *content.Set, s *store.MemoryStore, cred string) *player {
	t.Helper()
	trainer, err := hub.Authenticate(cred, "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.CompleteOnboard(trainer.ID, cred, "rootkit"); err != nil {
		t.Fatal(err)
	}
	fillTUIParty(t, s, trainer.ID)
	sv, err := hub.Load(trainer.ID)
	if err != nil {
		t.Fatal(err)
	}
	p := &player{t: t, hub: hub, set: set, m: New(trainer.ID, sv, set, hub)}
	p.attachPlayer(playWidth, playHeight)
	return p
}

func joinOnboarded(t *testing.T, hub *server.Hub, set *content.Set, cred string) *player {
	t.Helper()
	trainer, err := hub.Authenticate(cred, "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	sv, err := hub.CompleteOnboard(trainer.ID, cred, "rootkit")
	if err != nil {
		t.Fatal(err)
	}
	p := &player{t: t, hub: hub, set: set, m: New(trainer.ID, sv, set, hub)}
	p.attachPlayer(playWidth, playHeight)
	return p
}

func (p *player) declineOffer() {
	p.flush()
	if p.m.snap.Offer != nil {
		p.press("n")
	}
}

func (p *player) enterQueue() {
	p.t.Helper()
	p.declineOffer()
	p.press("f")
	if p.m.screen != screenQueueEditor {
		p.t.Fatal(p.dump("expected Battle Party editor"))
	}
	p.press("enter")
}

func (p *player) walkAdjacentToTrainer(other *player) {
	p.t.Helper()
	other.flush()
	p.walkAdjacentTo(other.m.snap.You.X, other.m.snap.You.Y)
	other.flush()
}

func pvpFinished(a, b *player) bool {
	if a.m.screen != screenLobby || b.m.screen != screenLobby {
		return false
	}
	return a.save().Wins+a.save().Losses+b.save().Wins+b.save().Losses > 0
}

func battleLoadouts(sv *game.Save) [][]string {
	out := make([][]string, len(sv.Collection))
	for i, m := range sv.Collection {
		out[i] = append([]string(nil), m.BattleLoadout...)
	}
	return out
}

func loadoutsUnchanged(t *testing.T, before, after [][]string) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("collection size %d -> %d", len(before), len(after))
	}
	for i := range before {
		if len(before[i]) != len(after[i]) {
			t.Fatalf("loadout %d len %d -> %d", i, len(before[i]), len(after[i]))
		}
		for j := range before[i] {
			if before[i][j] != after[i][j] {
				t.Fatalf("loadout %d slot %d %q -> %q", i, j, before[i][j], after[i][j])
			}
		}
	}
}
