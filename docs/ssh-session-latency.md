# SSH session latency investigation

## Diagnosis before production changes

Measured on the Apple M1 Pro macOS host, with the unmodified `termond` binary,
120×40 xterm-256color PTYs, SQLite WAL/NORMAL, and loopback admission exemptions.
The probe parses received ANSI into a terminal screen and waits for the selected
Species heading, rather than treating an animation byte as a response. It selects
alternating starters; it does not complete onboarding or claim to measure Battle
input. The TCP relay pipelines 112.5 ms propagation in each direction. Python
terminal emulation and relay scheduling are included in these client measurements.

Three independent one-session runs, 60 alternating selections per run:

| Network | Key-to-visible p50 | p95 | p99 | First usable screen |
| --- | --- | --- | --- | --- |
| Loopback, run 1 | 16.6 ms | 17.5 ms | 18.7 ms | 57.1 ms |
| Loopback, run 2 | 16.8 ms | 18.9 ms | 19.4 ms | 57.9 ms |
| Loopback, run 3 | 16.2 ms | 18.5 ms | 19.1 ms | 55.9 ms |
| 225 ms RTT, run 1 | 252.3 ms | 268.5 ms | 269.6 ms | 2180.1 ms |
| 225 ms RTT, run 2 | 261.8 ms | 271.5 ms | 279.9 ms | 2155.3 ms |
| 225 ms RTT, run 3 | 263.8 ms | 275.9 ms | 279.8 ms | 2181.0 ms |

Idle starter animation sends about 6.5 KiB/s; continuous RTT-limited selection
sends 26–28 KiB/s. Loopback selection sends about 326 KiB/s when driven as fast as
responses arrive. These are terminal payload bytes, not encrypted TCP wire bytes.

A 32-session run (960 selections) measured first-render p50/p95/p99 of
2441/2498/2507 ms and key-to-visible 307/353/387 ms. One Python event loop parses
all 32 screens, so this is a combined client/server load result, not a server-only
capacity claim. The 15-second server CPU profile contained 8.61 CPU seconds
(57% of one core), dominated by runtime condition signalling (55% flat), with
ANSI width measurement 8.4% cumulative. At sampling there were 492 goroutines,
102 MB allocated heap and 117 MB heap in use. No evidence here justifies changing
SQLite or the Hub's authoritative timing.

The existing Go SSH harness reads only 512 bytes and then stops reading; any
escape sequence passes its startup assertion. Its historical first-render and
idle-session claims cannot establish first visible content or sustained output
throughput. The new probe explicitly disables the local SSH agent, since otherwise
an agent credential can override generated keys and displace concurrent sessions.

### Reproduced failure

With an 8 KiB receive window, a client that stops consuming an animated starter
screen leaves Bubble Tea's flush goroutine blocked in `ssh.window.reserve` through
`channel.WriteExtended`. Its event loop then blocks on `cursedRenderer.render`'s
mutex. The renderer holds that mutex while writing output. Termon's Hub outbox
calls `Program.Send` synchronously, so an output stall can delay later recipients
of the same broadcast, without needing to hold the Hub mutex itself. Waiting on
SSH idle timeout is not an adequate output-progress policy.

Bubble Tea already retains only its latest requested View before a flush. Dropping
bytes **after** its ANSI diff would corrupt later frames: their cursor movements
and cell changes depend on preceding output. The first hypothesis is to remove
network blocking from the renderer with a bounded, ordered writer, and suppress
cosmetic frame construction before diffing while that writer is busy. State
updates and commands must still run; direct input must still rebuild immediately.
No change to the 100 ms tick or renderer FPS is justified by the healthy-client
baseline, which already responds substantially faster than one tick locally.

### late.sh comparison

At pinned commit `bec3d9e1b3042a2286d206002ae63f645ec1f111`,
[`late-ssh/src/ssh.rs`](https://github.com/mpiorowski/late-sh/blob/bec3d9e1b3042a2286d206002ae63f645ec1f111/late-ssh/src/ssh.rs)
lines 57–109 describe a 32 MiB outstanding-output budget and 30-second stall
threshold. Lines 1237–1318 pause rendering before Ratatui's diff state advances.
Its comments identify an uncapped russh output queue as the failure mechanism.
Go's SSH channel instead blocks its writer when channel credit is exhausted;
Termon needs isolation from that blocking, not a copy of russh's budget.
[`SCALE.md`](https://github.com/mpiorowski/late-sh/blob/bec3d9e1b3042a2286d206002ae63f645ec1f111/SCALE.md)
also reports input-triggered rendering and warns that its stall metrics don't
fully account for observed frame drops. These are source-backed design comparisons,
not measured claims about late.sh's current end-to-end latency.

## Results after bounded output — TERM-70

The writer accepts at most 256 KiB including its active write, serializes all ANSI
bytes, and closes the transport if one write cannot finish in 10 seconds. Cosmetic
frame requests are coalesced before diffing while output is pending. This leaves
authoritative commands and messages untouched. No tick or FPS setting changed.

Three repeat runs reproduced healthy starter navigation at 269.5–273.1 ms p95
under 225 ms RTT, versus 268.5–275.9 ms before. This is **not a measured improvement
in healthy-client latency**. It removes a reproduced failure when output stops
progressing. With both a stalled companion and a 1 KiB/s companion, healthy-client
p95 was 269.9/271.4/273.7 ms after, versus 271.7/271.0/270.7 ms before. No stalled
companion disconnected during the three baseline holds; all three disconnected
after the change. All slow-reading companions stayed connected in both versions.

During a real stalled SSH session, the pending-byte gauge remained at 3,953 bytes
(one blocked animation write). After its deadline, pending bytes and active
sessions returned to zero, and a single `stalled` counter increment and correlated
warning recorded the reason. Before the fix the flush and event-loop stacks stayed
blocked until the probe resumed reading. The unit tests additionally exercise an
underlying connection with zero buffering and no reader, overflow including active
bytes, byte ordering and copying, short writes, cancellation, and worker cleanup.
A real Bubble Tea regression verifies that a blocked stream no longer prevents
later sequential broadcast callbacks from running.

A lifecycle follow-up joins the output worker when its shell Program finishes,
rather than waiting for a potentially long-lived shared SSH connection. Healthy
shutdown drains accepted terminal-reset bytes without closing that transport.
A regression opens three successive shell channels on one connection. A final
recheck after this follow-up measured isolation p95 273.5 ms, with the stalled
companion disconnected and the 1 KiB/s reader still connected.

### Movement and Battle input

Returning-Trainer fixtures are created through Hub/Store operations in a fresh
SQLite database. Movement measures the new visible location of the local Trainer
glyph, excluding leg-animation changes. Three 60-key runs per revision at 225 ms
RTT measured movement p95 283.0/273.6/271.6 ms before and 273.7/273.6/272.9 ms after.
Loopback movement p95 stayed around 18–19 ms. A settled Lobby sent zero idle bytes;
RTT-limited movement sent about 1.9 kB/s, much less than starter navigation.

Battle probes use two real SSH Trainers and the real Queue. They alternate opening
and leaving the Move menu, then one Trainer selects a Move and waits for the
`Waiting for opponent…` screen. The opponent forfeits, and both clients wait for
that result text. Three Move acknowledgements at 225 ms RTT took 261.0/272.1/254.8 ms
before and 269.5/273.4/257.5 ms after; three samples are a smoke test, not a credible
Battle-action p99 estimate. The full before/after Battle matrices each completed
six Battles. SQLite contained matching result, win, and loss counts; extra debug
and final recheck Battles were accounted for separately.

Enter/Escape Battle-menu p95 was 318–324 ms before and 320–336 ms after. The extra
Escape delay is explained by the pinned Ultraviolet `DefaultEscTimeout` of 50 ms:
a bare Escape could be the prefix of a longer key sequence. Loopback samples
alternate roughly 15–20 ms for Enter and 60–75 ms for Escape. Reducing that timeout
without testing fragmented escape sequences risks corrupting input, so this
change leaves it alone. Battle idle animation sent roughly 13 kB/s, and this menu
workload roughly 14 kB/s per Trainer under RTT simulation.

### First usable screen and concurrent sessions

Each of three independent 32-session runs contributes 32 first-render samples:

| Network/revision | Run 1 p50/p95/p99 | Run 2 p50/p95/p99 | Run 3 p50/p95/p99 |
| --- | --- | --- | --- |
| Loopback, before | 208/251/258 ms | 187/228/233 ms | 215/252/259 ms |
| Loopback, after | 203/242/251 ms | 189/227/232 ms | 181/233/237 ms |
| 225 ms RTT, before | 2298/2366/2368 ms | 2451/2480/2481 ms | 2358/2401/2406 ms |
| 225 ms RTT, after | 2395/2438/2443 ms | 2371/2422/2427 ms | 2339/2416/2423 ms |

Startup involves multiple SSH handshake/authentication/channel/PTY round trips;
it should not be compared with the single-round-trip in-session target. No ingress
changes were made. Public IPv4:22 still publishes termond:2222 directly.

The 32-session active-navigation comparison measured key p50/p95/p99 of
307/353/387 ms before and 295/361/388 ms after. These runs deliberately include the
same Python emulator and relay event loop for every client, so they don't prove
that termond alone adds the entire observed allowance. They also do not certify
512-session production latency. A separate host or Linux netem run is needed to
separate client saturation from server scheduling at higher load.

### Profiles, allocations, and bounds

The comparable 15-second 32-session CPU profiles contained 8.61 CPU seconds before
and 8.77 after (57% and 58% of one core). Heap snapshots were 102/96 MB allocated
and 117/114 MB in use; these are snapshots, not a claim of reduced memory. Goroutine
counts were 492/525, consistent with one new output worker per session and scrape
activity. Network writes no longer happen under Bubble Tea's renderer mutex.

Six repeats of `BenchmarkAnimatedOutput` measured the model's animated
Update/View path at 1.017–1.034 ms, approximately 407 kB and 3,931 allocations per
tick while reading. Under output pressure the same state advances in 2.390–2.459 µs,
8,348 bytes and 9 allocations per tick because frame construction is omitted.
This is a comparison of paths, not a claim that healthy rendering became faster.
At 10 ticks/s, the remaining pressured model allocations are about 83 kB/s until
the write completes or the deadline disconnects the session.

A final diagnostic run with contention profiling enabled observed about 133 MB/s
of allocations and 221 kB/s of output during the 32-session animated hold. The
mutex profile recorded 215 ms cumulative contention, with about 25 ms attributed
to renderer flushing. The block profile was dominated by normal channel, timer,
and input waits across many goroutines; its accumulated hours are not request
latency. After the run, active sessions and pending output were zero. Profiling
was kept out of the main before/after latency runs.

## Reproduction and residual uncertainty

Use the commands in [load-baseline.md](load-baseline.md#in-session-latency-and-output-pressure).
The [CSV](ssh-session-latency-results.csv) records per-run summaries without keys,
Trainer IDs, source addresses, or private profile data. Raw JSON samples and
profiles were retained as local investigation artifacts.

A 229 ms production RTT remains a physical lower bound for acknowledged input.
The controlled low-load tests leave roughly 45–50 ms above the synthetic RTT,
including renderer scheduling, SSH handling, the relay, and Python terminal
emulation. They do not establish that all remaining allowance belongs to Termon,
or explain every report of production sluggishness. Bare Escape decoding,
intentional Battle playback, constrained-link serialization, and terminal-specific
rendering remain distinct from the fixed slow-writer failure. Observe the new
write-duration and pressure metrics in production before claiming that this
failure was the dominant cause there.

All requested Go formatting, test, race, lint, vet, build, vulnerability, and
`scripts/check.sh` gates passed, including the Balance Run. Gameplay content,
SQLite durability, and authoritative timing were unchanged.
