# Multi-Dojo load baseline

For key-to-visible measurements and slow-reader tests, use the
[in-session latency procedure](#in-session-latency-and-output-pressure). The older
SSH startup harness below only checks initial control bytes; it does not measure
usable screens or continuous output delivery.

The 4 CPU / 4 GiB Docker baseline supports 512 concurrent Trainers and 256 simultaneous Battles with zero correctness failures at 59.8 ms median p95 Battle Result latency under the production WAL/NORMAL profile. Keep 256 Trainers as the simultaneous-login baseline because a cold burst of 512 SSH sessions exceeds the 15-second startup budget, even though the server can hold all 512 once they render.

This is a coordination and persistence baseline, not an SSH connection ceiling. The harness exercises authentication, onboarding, attached Hub clients, multi-Dojo presence fan-out, the global Queue, Battle creation, concurrent forfeits, and SQLite result transactions. It doesn't open SSH sockets or render Bubble Tea views.

## Workload

Each level creates a fresh SQLite database inside a container constrained by `--cpus=4`, `--memory=4g`, and `--memory-swap=4g`. Every Trainer receives Hub messages through an attached callback. Each of 10 rounds globally pairs every Trainer, creates the maximum number of simultaneous Battles, and forfeits one side of every Battle concurrently.

The harness verifies that every Battle closes and that durable win and loss totals equal the completed Battle count. A run exits with an error if pairing, completion, or durable records disagree.

## Original WAL/FULL results

The original operating baseline used the host-crash-durable `FULL` profile and a 500 ms p95 commit threshold. The 256-Trainer level stayed below that threshold; 512 Trainers exceeded it even though the run remained correct.

| Trainers | Dojos | Peak Battles | Completed | Failures | Matchmaking p95 | Commit p95 | Battles/s | Peak system memory |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 32 | 1 | 16 | 160 | 0 | 24.2 ms | 46.2 ms | 243.9 | 13.1 MiB |
| 64 | 2 | 32 | 320 | 0 | 71.9 ms | 113.4 ms | 190.5 | 17.1 MiB |
| 128 | 4 | 64 | 640 | 0 | 148.3 ms | 213.9 ms | 193.4 | 17.4 MiB |
| 256 | 8 | 128 | 1,280 | 0 | 300.5 ms | 413.8 ms | 184.1 | 21.1 MiB |
| 512 | 16 | 256 | 2,560 | 0 | 451.5 ms | 733.5 ms | 215.5 | 25.4 MiB |
| 1,024 | 32 | 512 | 5,120 | 0 | 593.1 ms | 1,482.4 ms | 258.8 | 33.1 MiB |

The 1,024-Trainer run proves correctness at that level, but it isn't an acceptable latency baseline. SQLite uses one connection as the durable serialization boundary, so concurrent Battle Result bursts wait behind earlier commits while CPU and memory remain low.

## Production WAL/NORMAL results

The production `NORMAL` profile removes the per-commit WAL synchronization. Each level ran three times; this table reports median latency and throughput with zero correctness failures in every run.

| Trainers | Peak Battles | Commit p95 | Commit p99 | Battles/s |
|---:|---:|---:|---:|---:|
| 256 | 128 | 36.3 ms | 43.5 ms | 476.0 |
| 512 | 256 | 59.8 ms | 66.8 ms | 549.5 |

## Reproduce the baseline

Run the default WAL/NORMAL matrix from the repository root:

```sh
./scripts/load-baseline.sh
```

Run extended levels with the same container limits:

```sh
TERMON_LOAD_LEVELS="512 1024" ./scripts/load-baseline.sh
```

Set `TERMON_LOAD_ROUNDS` to change the number of complete Battle rounds per Trainer. Keep the default 10 rounds when comparing a result with this baseline.

## Production interpretation

Persistence no longer limits the process at 512 Trainers under the current 500 ms p95 target. SSH cold-start latency now sets the lower admission limit: allow at most 256 simultaneous login starts, while separately allowing up to 512 established sessions. Add connection-rate smoothing before raising the login-start limit.

Persistence tradeoffs behind these numbers live in [design/durability.md](design/durability.md).

## SSH session baseline

The SSH baseline runs the real termond binary in a Docker container constrained to 4 CPUs and 4 GiB. Independent generated credentials authenticate through Wish, request PTYs, wait for Bubble Tea terminal output, hold every successful session concurrently for two seconds, and then disconnect.

| Trainers | Startup timeout | Connected | Peak sessions | Failures | Connect p95 | Connect p99 |
|---:|---:|---:|---:|---:|---:|---:|
| 256 | 15 s | 256 | 256 | 0 | 6.37 s | 7.01 s |
| 512 | 15 s | 390 | 390 | 122 | 14.79 s | 25.68 s |
| 512 | 60 s | 512 | 512 | 0 | 32.55 s | 35.24 s |

The server can hold at least 512 rendered SSH sessions without exceeding the container memory limit, but it cannot cold-start all 512 within 15 seconds. Keep 256 as the simultaneous-login admission baseline. Treat 512 as a live-session capacity result and smooth new connection bursts before raising the admission limit.

Reproduce the default 256 and 512 session gate with:

```sh
./scripts/ssh-baseline.sh
```

Use a longer startup budget to verify eventual 512-session capacity:

```sh
TERMON_SSH_LEVELS=512 TERMON_SSH_STARTUP_TIMEOUT=60s ./scripts/ssh-baseline.sh
```

This scenario covers SSH authentication, session lifecycle, PTY allocation, and initial TUI rendering. The Hub load scenario above covers matchmaking and Battle Result persistence; it does not drive Battle input through SSH.

Login-start smoothing now ships by default (25 starts/second, burst 128) so cold bursts queue instead of stampeding startup; see [operations.md](operations.md) for tuning flags. The table above predates it and is unchanged.

### Sustained idle-render profile (512 sessions)

Measured locally on an Apple M1 Pro (10 cores, 32 GiB, macOS, no container limits) — a first-signal laptop number, not the 4 CPU container baseline. termond ran from this tree with pprof served next to `/metrics` on the loopback metrics listener; `go run ./cmd/termon-ssh-load -address <addr> -trainers 512 -hold 60s -startup-timeout 180s` connected all 512 sessions with zero failures. While every session sat idle in the lobby re-rendering at 10 Hz, a 30-second CPU profile collected 166.95 s of samples over 30.18 s of wall clock — about **553% of one core (~55% of the machine)**, i.e. roughly **11 ms of CPU per second per held session**. Half of all CPU lands under `charm.land/bubbletea/v2.(*Program).render`; the leaders are terminal-width measurement (`x/ansi.stringWidth`, 33% cum) and `lipgloss.Style.Render` (15% cum). Heap in-use during the hold was about **1.21 GiB (~2.4 MiB/session)**, mostly ultraviolet screen buffers and per-frame string builders. An independent identical run reproduced the CPU figure within 1% (557%).

That sustained idle cost is far above the ~25%-of-one-core guideline for 512 sessions, so a follow-up finding is recorded here: the lobby re-renders its full frame every tick, re-measuring cell widths for the entire frame each time. TUI-level memoization or render diffing is the candidate fix and is tracked separately; it is deliberately not implemented in this change. Until that lands, plan roughly one core per ~90 idle rendered sessions on laptop-class hardware. (Memoization has since landed — see below; component-level caching for busy frames is specified in [design/render-caching.md](design/render-caching.md).)

#### After render memoization

Re-measured with the identical local procedure after TUI frame memoization landed (dirty-flag invalidation in `internal/tui`; termond needs `-exempt-loopback-rate-limit -registrations-per-ip -1` on loopback or the registration quota denies the burst): 512/512 connected, zero failures, zero login drops. The 30-second CPU profile collected 15.81 s of samples over 30.08 s of wall clock — about **53% of one core (~5% of the machine)**, i.e. roughly **1 ms of CPU per second per held session**: a **~10.5× reduction** against the 553%-of-one-core figure above (~11 ms → ~1 ms CPU/s/session). Remaining CPU sits in the frame builds that legitimately still happen (`tui.Model.Update`/`buildFrame`, 35% cum), the renderer diff/flush path (`cursedRenderer.flush`, 24% cum), terminal-width measurement (`x/ansi.stringWidth`, 22% cum), and `lipgloss.Style.Render` (11% cum). Heap in-use during the hold was about **1.29 GB (~2.4 MiB/session)** — unchanged in size and shape versus the pre-memoization hold (ultraviolet screen buffers plus per-frame string builders), because memoization removes repeated frame construction, not retained buffers. One correction to the paragraph above: this harness never sends keystrokes, so fresh trainers sit on the animated onboarding welcome screen (its "press any key" prompt blinks by design), which therefore still rebuilds every tick; genuinely static screens such as a settled lobby or queue now reuse the memoized frame verbatim and skip rendering entirely between messages, so 53% is an upper bound for the idle-session cost going forward. Same hardware caveats as above: Apple M1 Pro laptop, no container limits, first-signal number. `termon_login_wait_seconds` and `termon_login_drops_total` stayed exposed throughout the run with zero drops.

## In-session latency and output pressure

TERM-70 adds a terminal-emulating probe and a bounded output policy. The
[diagnosis and results](ssh-session-latency.md) explain the failure and
measurement limits; [per-run CSV results](ssh-session-latency-results.csv)
retain the repeated samples' summaries. These are Apple M1 Pro/macOS measurements,
not a replacement for the 4 CPU Docker capacity gate.

| Workload | Before | After |
| --- | --- | --- |
| Starter navigation, loopback, p95 across three 60-key runs | 17.5–18.9 ms | 17.9–18.8 ms |
| Starter navigation, 225 ms RTT, p95 | 268.5–275.9 ms | 269.5–273.1 ms |
| Movement, 225 ms RTT, p95 across three 60-key runs | 271.6–283.0 ms | 272.9–273.7 ms |
| Battle Move acknowledgement, 225 ms RTT, three samples | 254.8–272.1 ms | 257.5–273.4 ms |
| Healthy navigation alongside a stalled and 1 KiB/s reader, p95 | 270.7–271.7 ms | 269.9–273.7 ms |
| Stalled SSH clients disconnected during three runs | 0/3 | 3/3 |
| 1 KiB/s readers kept connected during those runs | 3/3 | 3/3 |

Healthy latency is unchanged within run-to-run variation; the improvement is
bounded output and isolation when SSH writes block. The 100 ms tick and 60 FPS
renderer weren't retuned. Bare Escape has a separate 50 ms decoder ambiguity
window: Battle menu probes alternating Enter/Escape measured p95 of 318–324 ms
before and 320–336 ms after at 225 ms RTT. Move acknowledgement doesn't pay that
Escape delay. Don't combine these input classes into a claimed render delay.

### Prepare an isolated server

The Python probe uses `uv` to resolve its pinned AsyncSSH and pyte test-only
dependencies. It disables host-key verification and the local SSH agent, so use
it only against disposable local servers. It never prints private keys. Fixtures
use public Store/Hub operations, not a production gameplay bypass, and refuse an
existing directory.

```sh
work=$(mktemp -d)
TERMON_LATENCY_FIXTURE="$work/fixture" \
  go test ./internal/sshload -run '^TestPrepareLatencyFixture$' -v

go build -o "$work/termond" ./cmd/termond
"$work/termond" -listen 127.0.0.1:2222 \
  -metrics-listen 127.0.0.1:9191 \
  -database "$work/fixture/termon.db" -host-key "$work/host-key" \
  -exempt-loopback-rate-limit -registrations-per-ip -1 \
  >"$work/server.log" 2>&1 &
server_pid=$!
trap 'kill "$server_pid"; wait "$server_pid"' EXIT
```

Wait for `curl --fail http://127.0.0.1:9191/readyz` to succeed before probing.
Keep the server, probe, and profilers on otherwise idle hardware. Preserve each
revision's binary and use separate disposable databases for before/after runs.
Never run two servers against one fixture database.

### Run the latency matrix

The relay adds 112.5 ms in each direction for `--rtt-ms 225`. It schedules arrival
times independently of serialization and retains at most sixteen 16 KiB chunks
per direction, plus asyncio/kernel buffering. It is a user-space TCP relay, not
a model of every TCP congestion-control effect. `--bytes-per-second 32768` adds
32 KiB/s per-direction serialization when testing constrained bandwidth; expected
latency must then include the frame's transmission time as well as RTT.

```sh
for run in 1 2 3; do
  for rtt in 0 225; do
    uv run scripts/ssh-latency.py --rtt-ms "$rtt" --keys 60 --hold 3 \
      >"$work/starter-$rtt-$run.json"
    uv run scripts/ssh-latency.py --rtt-ms "$rtt" --keys 60 --hold 3 \
      --fixture "$work/fixture" --mode movement \
      >"$work/movement-$rtt-$run.json"
    uv run scripts/ssh-latency.py --rtt-ms "$rtt" --keys 20 --hold 3 \
      --fixture "$work/fixture" --mode battle --sessions 2 \
      >"$work/battle-$rtt-$run.json"
    uv run scripts/ssh-latency.py --rtt-ms "$rtt" --mode welcome \
      --sessions 32 --hold 1 >"$work/first32-$rtt-$run.json"
  done
  uv run scripts/ssh-latency.py --rtt-ms 225 --keys 30 --hold 12 \
    --stalled-companions 1 --slow-companions 1 \
    >"$work/isolation-$run.json"
done

uv run scripts/ssh-latency.py --sessions 32 --rtt-ms 225 \
  --hold 20 --keys 30 >"$work/navigation32.json"
uv run scripts/ssh-latency.py --rtt-ms 225 --bytes-per-second 32768 \
  --hold 12 --keys 30 >"$work/bandwidth.json"
```

Starter navigation waits for the corresponding selected Species heading on the
emulated terminal. Movement waits for the local Trainer's glyph to appear at a
new position, not for changing walking-animation bytes. Battle probes enter the
real Queue, alternate the Move menu and command menu, acknowledge one immutable
Move selection, and finish with a forfeit visible to both participants. Query
`SELECT count(*) FROM battle_results` and `SELECT sum(wins), sum(losses) FROM trainers`
on the disposable SQLite database to verify one new result and one win/loss per
completed two-session Battle probe.

Companions share the healthy client's start barrier. They use an 8 KiB SSH receive
window to expose pressure quickly rather than spending minutes filling a normal
2 MiB channel window. The stalled companion stops consuming stdout; the slow one
reads at 1 KiB/s. A final `peer_disconnected` value distinguishes a server-initiated
disconnection from the probe's own cleanup. The separate zero-buffer transport
regression test checks closure when the underlying connection writer itself is
blocked. Increasing `--window` delays detection by allowing more client-side
buffering; it doesn't increase Termon's application queue bound.

### Profile rendering and blocked output

Restart the disposable server with `-profile-contention` for mutex/block profiles;
keep that overhead out of normal latency comparisons. While the 32-session hold
is active, collect:

```sh
curl -fsS 'http://127.0.0.1:9191/debug/pprof/profile?seconds=15' -o "$work/cpu.pprof"
for kind in heap allocs goroutine mutex block; do
  curl -fsS "http://127.0.0.1:9191/debug/pprof/$kind" -o "$work/$kind.pprof"
  go tool pprof -top "$work/$kind.pprof" >"$work/$kind.txt"
done
curl -fsS http://127.0.0.1:9191/metrics >"$work/metrics.txt"
go tool pprof -top "$work/cpu.pprof"
go test ./internal/tui -run '^$' -bench BenchmarkAnimatedOutput \
  -benchmem -count=6 >"$work/render-bench.txt"
```

Take metrics snapshots before and after the hold to calculate output-byte and
allocation rates. Inspect pending output **during** the stall, then confirm zero
sessions and zero pending bytes after cleanup. Don't run benchmarks concurrently
with the SSH matrix or repository checks. Keep raw profiles private; only the
summary belongs in a task or repository.
