# Render caching: component-level caches for busy frames

Follow-up finding from the sustained idle-render profile (see
`load-baseline.md`). Status: **not scheduled** — build when busy-frame
measurements justify it.

## Problem space

Frame memoization (`internal/tui/model.go`, dirty-flag invalidation) removed
idle-render cost: settled screens rebuild nothing between messages. What
remains is **busy-frame** cost — frames that legitimately change every tick:

- a lobby where several Trainers walk at once,
- Battle playback (typing/shake/faint animation, idle-pose alternation).

On those frames `buildFrame` still recomposes and re-measures the entire
frame each tick even though most of its content is identical to last tick.
The post-memoization profile attributes roughly half the remaining CPU to
exactly this: `tui.Model.Update`/`buildFrame` (~35% cum),
`x/ansi.stringWidth` (~22% cum), `lipgloss.Style.Render` (~11% cum).

## Design sketch

Cache rendered pieces, not whole frames. Every expensive sub-component
produces `(ansiString, width)` pairs keyed by a cheap input tuple:

| Component | Cache key | Width source |
|---|---|---|
| Sprite block | species slug × facing × pose frame | fixed art grid — known at load time, never measure |
| Lobby Trainer block | handle + species + emote + walk-frame | measured once per key |
| Floor layer | already cached (static) | cached alongside |
| Chrome wrapper | width × height | arithmetic, not measurement |

Composition then concatenates cached strings and **sums cached widths**;
`stringWidth` runs only on pieces that actually changed. The existing
event-log cache (`evCache`) is the precedent to follow.

Invalidation stays explicit like `frameDirty`: walkers carry a walk-frame
counter; battle animation state already derives from `battleAge`; nothing
keys on wall-clock time.

## Measurement gate — build only if triggered

Re-run the sustained-profile procedure while driving movement (harness sends
no keystrokes today, so a walking-lobby scenario needs either harness keys or
a scripted multi-client driver). Build this only if a busy lobby at realistic
population still shows `buildFrame`+width-measurement above ~100% of one core
at 512 sessions. Idle cost is already solved and must not regress: the
memoized fast path must stay allocation-free (assert via the frame tests'
`frameBuilds` counter plus an allocations check).

## Out of scope when built

Upstream renderer changes (bubbletea/ultraviolet diffing is not the
bottleneck), sprite pipeline changes, protocol changes.
