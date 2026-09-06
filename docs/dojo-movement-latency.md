# Dojo movement latency investigation — TERM-71

The single-Trainer burst tests did not reproduce growing input latency or reversed
positions on an unconstrained link. Camera redraws did produce repeatable spikes
when bandwidth was constrained. This is a measured contributor, not evidence that
a particular production player's connection is bandwidth-limited.

The server baseline includes `808f2f6` and the uncommitted onboarding invalidation
prototype. That prototype does not change Dojo movement or rendering. All servers,
SQLite fixtures, and credentials were disposable; no deployment was changed.

The follow-up fixes preserve the current visuals: input-ordered Hub movement,
capture-ordered snapshots, and a narrowly scoped renderer backport. They are
separate from the rejected camera and floor experiments below. The initial
movement probes and findings were committed as `84d0a07`.

## Burst and reversal measurements

The probe uses a 120×40 `xterm-256color` terminal emulator and a 225 ms RTT relay.
Each camera-crossing run sends 32 keys in eight four-step legs along the empty
south entrance corridor. Keys within a leg are sent without waiting for feedback.
The route crosses camera boundaries and returns to its initial world position.
A separate workload sends `ddddaaaa` at 33 ms intervals, reversing direction while
keys are still in flight. Each reversal run contains eight cycles, or 64 keys.

| Workload | Three repeated key-to-marker p95 measurements |
| --- | --- |
| Camera route, 100 ms key interval, unlimited bandwidth | 261 / 253 / 252 ms |
| Camera route, 33 ms key interval, unlimited bandwidth | 258 / 259 / 254 ms |
| Rapid reversal, 33 ms key interval, unlimited bandwidth | 252 / 254 / 255 ms |
| Rapid reversal, 33 ms key interval, 32 KiB/s | 267 / 274 / 315 ms |
| Camera route, 100 ms key interval, 32 KiB/s, initial series | 432 / 430 / 424 ms |

Every expected step was observed in these runs, every reversal trace matched the
expected sequence, and every final position was correct. On the constrained
camera route, the individual camera-crossing steps took approximately 406–537 ms;
ordinary steps were usually near 260 ms, with some following a redraw delayed too.

A later final reversal smoke run contained one partial cycle: seven of its eight
positions were observed, in order, and its final position was correct. The probe
excluded that entire ambiguous cycle, emitting 56 timings rather than inventing
64. Renderer coalescing or multiple updates in one SSH read can hide an intermediate
position; this observation alone does not establish a lost movement intent.

These are short repeatable workloads, not credible production p99 estimates.
Four-step legs pause and drain between bursts; the reversal workload removes that
pause at the direction change. Neither exercises indefinitely held keys, collisions
with other Trainers, or crowded-room contention. The probe does not prove that
asynchronous command/snapshot ordering is safe under those conditions.

## What is in the output

A captured camera-route run sent 1.7–1.8 kB per ordinary four-step leg, versus
6.5–10.2 kB per leg containing a camera shift. Approximately 65–71% of those bytes
were SGR styling sequences, predominantly switches between the two floor background
colors. The renderer already used indexed 256-color sequences, not verbose RGB
sequences; simply forcing 256 colors would not improve this measured case.

This explains why skipping duplicate model frame construction didn't fix these
spikes: the camera really changes the visible scene, and the renderer must send
new terminal cells and their styles. Real terminals which negotiate different
capabilities or true color can have different payloads and display timings.

## Experiments, not shipped changes

### Smaller camera jumps: rejected

An experimental camera advanced four tiles rather than recentering on horizontal
viewport exit. The same route and link settings produced:

| Implementation | Three p95 measurements | Payload over 32 keys, including settling |
| --- | --- | --- |
| Current camera | 433 / 449 / 469 ms | 39.8–39.9 kB |
| Four-tile camera | 426 / 438 / 438 ms | 44.4–44.5 kB |

The overlapping latency results and approximately 11% higher payload don't justify
changing camera behavior. This prototype was reverted. A one-tile-follow experiment
was also discarded: the marker stays at the same screen position during a pan,
which makes marker-only per-step latency attribution invalid. Its apparent latency
numbers were **not** used as evidence. The probe now rejects ambiguous per-leg
marker positions and excludes observations earlier than the injected RTT permits.

### Uniform floor: positive bandwidth result, visual trade-off

A second ablation rendered the tatami floor with its first background color only.
It retained the existing camera, Trainer models, landmarks, collision rules, and
movement processing. Against the same current-camera baseline above:

| Implementation | Three p95 measurements | Payload over 32 keys, including settling |
| --- | --- | --- |
| Current two-color floor | 433 / 449 / 469 ms | 39.8–39.9 kB |
| Uniform floor | 345 / 345 / 354 ms | 21.2–21.3 kB |

The uniform floor reduced payload by about 47% and the measured p95 by roughly
20–25% on this constrained link. Median latency remained approximately 263–273 ms:
it does not remove propagation delay. All 32 steps per run were observed in order,
with correct final positions.

This was an ablation, not an approved art change. The source change was reverted
after building its experimental binary. A reduced-detail option would need an
explicit presentation policy, correctly separated scenery-cache keys, matching
Trainer backgrounds, and user-facing configuration. No such option exists yet.

## Validated follow-up fixes

### Movement intent order and snapshot delivery order are separate

The crowded setup reproduced a stale population header: all 32 SSH sessions had
entered the Dojo, but the primary displayed 27. Hub outboxes capture under the lock
and deliver afterward, so an older capture can arrive last. Snapshots now carry a
monotonic, process-local capture sequence; the TUI rejects older snapshots before
updating position, camera, presence, or offers. A deterministic regression replays
a newer movement snapshot followed by an older one and verifies no rewind.

That alone cannot order the movement commands themselves. A second deterministic
regression starts at the west wall, inputs right then left, and executes the two
old Bubble Tea commands in reverse order. It ends on the wrong tile because the
left intent collides with the wall before the right intent executes. Movement now
uses `Hub.MoveAndSnapshot` synchronously in input order, with no persistence,
callbacks, or broadcast delivery in that operation. The Hub retains all collision
and state authority. The same input update paints the result rather than first
painting the old state and later handling a command result.

Six serial model benchmarks on the same M1 Pro, against `84d0a07`, measured:

| Per movement Update/View path | Before | Fixed |
| --- | --- | --- |
| Full frame builds | 2 | 1 |
| Time, six-run range | 4.07–4.59 ms | 2.00–2.21 ms |
| Allocated bytes | 1.825–1.829 MB | 0.915–0.917 MB |
| Allocations | 12,885–12,886 | 6,445 |

These are model-path costs, not total server CPU or end-to-end latency.

### Renderer correction without changing pixels

`TerminalRenderer.putRange` in the pinned Ultraviolet source computes `inline`,
the estimated cursor-movement cost, but compares the unchanged run against the
entire remaining interval instead. That branch cannot fire at an interior mismatch.
Changing the comparison to `same > inline` reuses the existing cursor-movement path
rather than re-emitting unchanged colored cells. The exact same cost comparison
exists in ncurses; see the [primary-source web research](ssh-tui-latency-web-research.md).

Upstream HEAD still contained the bug when checked. The temporary local
[backport](../third_party/ultraviolet/TERMON-PATCH.md) preserves the upstream version
and license and changes one production-source line. Its removal condition is an
upstream version containing the fix with passing regressions. No issue or PR has
been published from this checkout.

Final repeated camera-route measurements, using the corrected emulator below:

| 225 ms RTT, 32 KiB/s, 32 keys per run | Before | Fixed |
| --- | --- | --- |
| p95, three runs | 448 / 452 / 435 ms | 368 / 370 / 372 ms |
| Payload including settling | 39.7–39.8 kB | 31.9–32.0 kB |

All steps were observed in order with correct final positions. This is about a
20% payload reduction and a 14–18% reduction in these measured p95 values. Ordinary
median latency remains approximately 263–275 ms; no geography-independent speedup
is claimed. [Per-run results](dojo-movement-results.csv) retain sample counts.

### Sustained input with 31 real moving peers

`scripts/ssh-movement.py` seats the primary first, then 31 fixture peers in the same
Dojo. Peers validate their initial screen, then drain without running 31 extra
Python screen parsers. They move north/south at 10 Hz while the primary sends 606
keys at 33 ms intervals, without waiting between keys. Its final x=2 position is
unique to the tail, allowing an unambiguous final-key latency even when intermediate
positions were not individually observed.

The final implementation reached the correct final position in all three runs;
that unique final step appeared 239 / 251 / 239 ms after its key. Observed positions
were 600 / 594 / 606 of 606, and no session disconnected. The complete third trace
supports per-key timings; the first two deliberately emit no per-key percentiles.
Their gaps are not invented timings or proof of lost movement intents. Client
loop-delay p95 was 3.1 / 3.2 / 2.0 ms. Static-peer comparisons also exercised a
blocked move into an occupied entrance tile.

The intermediate sequence-only implementation still had a wrong final position
in one stressed run, which motivated the separate input-order regression and fix.
These tests do not establish a reliable crowded-room p95 comparison across all
runs, nor capacity on a separate Linux server/client pair.

### Emulator and terminal-state validation

Replay validation exposed two pyte 0.8.2 limitations: it does not dispatch CSI S/T
scrolling, and it leaves an out-of-bounds cursor after a last-column write with
autowrap disabled. `scripts/terminal_emulator.py` supplies these behaviors with
unit tests, preserving scroll regions, cursor position, and erase backgrounds.
The corrected sustained probes recorded zero scroll commands in their measured
primary streams, but both fixes were still included in the final reruns.

160 deterministic frames cover horizontal/vertical camera movement, resizing,
256-color and true-color output, and wide/combining labels. Incremental output
matches independent full repaints for visible cells, background colors, and visible
attributes. Renderer cursor tracking and terminal modes also agree. Foreground
color and bold intensity on plain, undecorated spaces are ignored because they
paint no ink; underline, reverse, strike-through, and backgrounds are not ignored.
Both the original and patched renderers pass this replay comparison. The targeted
colored-interior regression fails on the original renderer with 586 bytes of
redundant output and passes with the backport.

## Reproduce

Prepare a new returning-Trainer fixture with `TestPrepareLatencyFixture` and start
an isolated server as described in [load-baseline.md](load-baseline.md#in-session-latency-and-output-pressure).
Use one Trainer in an otherwise empty Dojo; the route assumes the initial entrance
position and fixed 120×40 viewport.

```sh
uv run scripts/ssh-latency.py --address 127.0.0.1:2235 \
  --fixture "$FIXTURE" --mode movement-burst --keys 8 --hold 1 \
  --key-interval-ms 100 --rtt-ms 225 --bytes-per-second 32768

uv run scripts/ssh-latency.py --address 127.0.0.1:2235 \
  --fixture "$FIXTURE" --mode movement-turn --keys 8 --hold 1 \
  --key-interval-ms 33 --rtt-ms 225
```

Repeat three times; omit the bandwidth limit for the unconstrained comparison.
`--keys` means four-step legs in `movement-burst` and eight-key cycles in
`movement-turn`. The experimental `--camera-mode step` matcher is only for a server
implementing the rejected four-tile camera; the current server uses `jump`.

For the sustained workload and independent renderer replay:

```sh
uv run scripts/ssh-movement.py --address 127.0.0.1:2235 \
  --fixture "$FIXTURE" --peers 31 --moving-peers --cycles 75 \
  --key-interval-ms 33 --rtt-ms 225

uv run scripts/test_terminal_emulator.py
TERMON_RENDER_TRACE="$NEW_TRACE_FILE" go test ./internal/tui \
  -run '^TestExportLobbyRendererTrace$' -count=1
uv run scripts/check-render-trace.py "$NEW_TRACE_FILE"
go test ./internal/tui -run '^$' -bench '^BenchmarkLobbyMovement$' -benchmem -count=6
```

`--refresh-population` is an explicit workaround for measuring the old server:
it makes one left/right move after concurrent joins so a fresh snapshot replaces
the stale header. The final fixed-server runs do not need it. Moving peers can
legitimately end away from the entrance, so the occupied-tile assertion runs only
with stationary peers.

Read the raw `movement_bursts` traces as well as percentiles. Intermediate positions
which are not observed must not be assigned invented timings or called dropped
movement intents. Rapid-reversal timings are emitted only for an exact complete
trace, since repeated positions make partial traces ambiguous. A rendering or
transport batch may legitimately hide an intermediate position.
