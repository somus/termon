# Ordinary rendering latency: follow-up to 808f2f6

The output-isolation fix is committed as `808f2f6`. This follow-up investigates
ordinary rendering independently of slow-client safety. The first prototype
eliminates redundant onboarding frame construction; it does not establish an
improvement in healthy key-to-visible latency.

## What late.sh actually does

The pinned `late-ssh/src/ssh.rs` at
[`bec3d9e1b3042a2286d206002ae63f645ec1f111`](https://github.com/mpiorowski/late-sh/blob/bec3d9e1b3042a2286d206002ae63f645ec1f111/late-ssh/src/ssh.rs#L1485-L1536)
separates input-triggered rendering from its world tick, uses a 15 ms minimum
render gap, and checks a dirty signal before rendering. Termon's input already
bypasses its 100 ms model tick; Bubble Tea's 60 FPS flush interval is 16.7 ms.
That 1.7 ms difference does not explain a substantial difference in perceived
responsiveness. Avoiding unnecessary painting is the stronger next hypothesis.

## Repeated redundancy measurement

`TestIdleFramesMatchFullRepaint` advances 120 ticks and compares each memoized
frame against a full repaint on a copy. Three baseline repeats agreed exactly:

| Settled screen | Full builds | Frames with changed pixels |
| --- | ---: | ---: |
| Lobby | 0 | 0 |
| Battle menu | 120 | 20 |
| Welcome | 120 | 24 |
| Dialogue | 120 | 0 |
| Handle selection | 120 | 0 |
| Handle input | 120 | 30 |
| Starter selection | 120 | 20 |
| Starter confirmation | 120 | 20 |

Bubble Tea suppresses identical output later, but Termon still allocates and
constructs these duplicate frames. This consumes server capacity without sending
useful pixels. It is distinct from animation bytes that genuinely change the
terminal and can delay input feedback on a bandwidth-constrained connection.

## First prototype: exact onboarding invalidation

`onboardModel.tickTouchesFrame` checks the existing pose and blink boundaries.
Finished dialogue and handle-selection screens become static. Typewriter text
still rebuilds until complete; the logical 100 ms clocks continue advancing.
Direct input and Hub messages continue to invalidate immediately.

The reference-frame test now observes exactly as many builds as changed frames
for the measured onboarding screens. A second regression covers every onboarding
stage from initial typewriter progress through 120 ticks, and another verifies
that selecting a starter bypasses the idle-tick gate.

Battle remains unchanged in this prototype: its clocks, playback, HP transitions,
and capture presentation need their own invalidation audit. A simple every-sixth-
tick gate could delay a Decision Clock display or a playback transition.

### Six-repeat model benchmark

Measured serially on the same Apple M1 Pro, darwin/arm64, against an isolated
worktree at `808f2f6`:

| Animated Update/View path | Before | Prototype |
| --- | ---: | ---: |
| Time per tick, range | 1.073–1.106 ms | 0.186–0.223 ms |
| Allocated bytes per tick | 408–409 kB | 75.2–75.3 kB |
| Allocations per tick | 3,931 | 662 |

This removes about 82% of this model-path time and allocated bytes. It is not
an 82% reduction in total server CPU or network latency.

### Repeated SSH comparison: 32 sessions, 225 ms RTT

Each run holds animated starter screens for ten seconds, then performs 30
confirmed selections per session (960 samples). Before and after alternate on
the same host, with the same Python terminal-emulation/relay process:

| Run | Before key p50/p95/p99 | Prototype key p50/p95/p99 |
| --- | --- | --- |
| 1 | 301/375/421 ms | 312/385/429 ms |
| 2 | 309/403/437 ms | 298/357/392 ms |
| 3 | 333/444/565 ms | 320/426/457 ms |

The overlapping, variable results do not establish a reliable input-latency
improvement. Mean idle payload remains about 6.3–6.5 kB/s per session, as expected:
we avoided work that the renderer was already suppressing on the wire. The shared
Python client remains a confounder at this concurrency.

## Reproduce

Save a baseline worktree and build its binary before modifying rendering. Run
benchmarks serially, with no SSH load or repository checks competing for CPU:

```sh
go test ./internal/tui -run '^TestIdleFramesMatchFullRepaint$' -v -count=3
go test ./internal/tui -run '^$' -bench 'BenchmarkAnimatedOutput/reading$' \
  -benchmem -count=6
```

Use the isolated-server setup in [load-baseline.md](load-baseline.md#in-session-latency-and-output-pressure),
then run each binary three times with:

```sh
# Historical probe, available at commit 8e509b6; removed from the current checkout.
uv run scripts/ssh-latency.py --sessions 32 --rtt-ms 225 --hold 10 --keys 30
```

## Next experiment

Separate **real animation traffic** from **redundant construction**. A controlled
idle-animation ablation, especially in Battle menus at constrained bandwidth,
can reveal whether animation bytes delay otherwise small menu updates. Measure
with a separate client host or lower client concurrency so Python parsing doesn't
hide server-side improvements. Keep this separate from renderer FPS changes;
raising a periodic FPS doubles wakeups even for idle sessions and needs its own
CPU-versus-latency measurement.
