# Dojo movement latency investigation — TERM-71

The single-Trainer burst tests did not reproduce growing input latency or reversed
positions on an unconstrained link. Camera redraws did produce repeatable spikes
when bandwidth was constrained. This is a measured contributor, not evidence that
a particular production player's connection is bandwidth-limited.

The server baseline includes `808f2f6` and the uncommitted onboarding invalidation
prototype. That prototype does not change Dojo movement or rendering. All servers,
SQLite fixtures, and credentials were disposable; no deployment was changed.

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

Read the raw `movement_bursts` traces as well as percentiles. Intermediate positions
which are not observed must not be assigned invented timings or called dropped
movement intents. Rapid-reversal timings are emitted only for an exact complete
trace, since repeated positions make partial traces ambiguous. A rendering or
transport batch may legitimately hide an intermediate position.
