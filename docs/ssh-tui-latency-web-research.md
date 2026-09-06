# SSH/TUI latency: primary-source research

Research checked on 2026-09-05 for the Termon maintainers. Public upstream evidence supports reducing redundant terminal bytes and preserving snapshot order; neither change can eliminate the approximately 225 ms network round trip required for a server-authoritative response. The strongest finding is that ncurses uses the exact cost comparison proposed for the local Ultraviolet backport, while the inspected Ultraviolet upstream still contains the unreachable comparison.

The subsequent implementation, final measurements, and terminal-emulator caveats
are recorded in the [Dojo movement report](dojo-movement-latency.md).

## Scope and evidence boundaries

The supplied local context is Bubble Tea v2.0.9 and Ultraviolet `ae99b731b8c580350966069bc83037227ede021c`. The lead reports that bounded output already isolates stalled clients; local tests reproduced stale snapshots, including 32 connected Trainers with a header showing 27 and movement rewinds from out-of-order post-lock delivery. Monotonic capture sequences now guard snapshots. The lead also reports that replacing `same > end-start` with `same > inline` reduces camera-route payload by about 20% and constrained p95 from approximately 435 to 370 ms at 32 KiB/s without changing rendered cells.

Those are **provided local findings**, not measurements repeated by this research or results independently confirmed upstream. This report separates public reports, merged fixes, directly inspected implementation behavior, and recommendations. Network access was read-only; no issue, PR, dependency change, test run, or Git mutation was performed.

## Renderer bandwidth and the exact Ultraviolet condition

### 1. The pinned condition is unreachable; no exact upstream fix was found

**Direct source evidence.** In [`TerminalRenderer.putRange` at the pinned revision](https://github.com/charmbracelet/ultraviolet/blob/ae99b731b8c580350966069bc83037227ede021c/terminal_renderer.go#L718-L752), Ultraviolet computes `inline` from cursor-positioning sequence lengths, checks whether the interval exceeds that cost, then tests `same > end-start` inside its scan. At a mismatching cell, `same <= j-start <= end-start`, so the interior-run skipping branch cannot execute for a valid interval. Trailing unchanged cells are still omitted separately; this is not a claim that the renderer lacks all diffing.

Upstream `main` resolved to this same revision during the check; the [commit is dated 2026-09-03](https://github.com/charmbracelet/ultraviolet/commit/ae99b731b8c580350966069bc83037227ede021c). The expression also appears in the [initial terminal implementation dated 2025-05-02](https://github.com/charmbracelet/ultraviolet/blob/b594af6fa735ec647ed176112002fc2d86b9ae6f/terminal_screen.go#L790-L810), so it predates the current pin.

Bounded GitHub issue/PR searches for `putRange`, `"end-start"`, `"cursor cost"`, `inline`, renderer, and unchanged-run terms found no exact report or fix. The related [PR #103](https://github.com/charmbracelet/ultraviolet/pull/103), opened 2026-03-28 and closed unmerged on 2026-04-16, mentions the same-cell skip but proposes a wide-character workaround, not this threshold correction. **No exact fix found** is a search result, not proof that no discussion exists anywhere.

### 2. ncurses supplies a precise cost-based precedent

**Confirmed implementation, not a proposed optimization.** The official [ncurses 6.4 source archive](https://invisible-island.net/archives/ncurses/ncurses-6.4.tar.gz), released in 2022, contains this logic in `ncurses/tty/tty_update.c`, lines 699–712; the same code is readable in a [commit-pinned source mirror](https://github.com/mirror/ncurses/blob/79b9071f2be20a24c7be031655a5638f6032f29f/ncurses/tty/tty_update.c#L677-L721):

```c
if (same > SP_PARM->_inline_cost) {
    EmitRange(NCURSES_SP_ARGx ntext + first, j - same - first);
    GoTo(NCURSES_SP_ARGx row, first = j);
}
```

The preceding comment says to use cursor movement when an identical run is long enough to justify it. The official archive was read directly to verify that the mirror agrees. This is strong support for comparing Ultraviolet's unchanged run against `inline`, though it does not prove that every detail of Ultraviolet's cost estimate or wide-cell handling is correct.

The [official refresh manual](https://invisible-island.net/ncurses/man/curs_refresh.3x.html), accessed 2026-09-05, also describes staging window updates with `wnoutrefresh` and performing one `doupdate`: the virtual/physical screen comparison produces one output burst rather than repeated intermediate updates. For Termon, the transferable principle is to compose a coherent frame and minimize its physical update, not to replace Bubble Tea with ncurses.

### 3. A recent Ultraviolet report describes severe redundant wide-cell redraws

**Reporter-supplied reproduction; not a confirmed merged fix.** [Ultraviolet #163](https://github.com/charmbracelet/ultraviolet/issues/163), filed 2026-08-26 against `68fa937c71be` and also reporting reproduction with Bubble Tea v2.0.8's `006e29f97886`, changes one cell in a 20-line view. The reported output is 4 bytes for ASCII, 1,161 for CJK, and 1,101 for emoji. The author attributes this to the wide-cell drift fallback repainting touched lines before checking whether they changed, and reports approximately 160 KB/s from a 10 fps spinner in their larger application.

The issue is **closed because the author withdrew it after filing from the wrong account**, not because a fix landed; its [closing comment](https://github.com/charmbracelet/ultraviolet/issues/163#issuecomment-5423903987) explicitly says that. The proposed solution checks line equality before entering the conservative wide-cell repaint path. This is a separate mechanism from `same > end-start`, and its status must not be presented as confirmation of Termon's bug or a benchmark of the current pin.

Applicability: include static Unicode labels, emoji, and a changing spinner in renderer tests. A large mostly-static view can emit unnecessary bytes even when only one small area animates. Don't remove wide-character safety fallbacks merely to obtain a smaller byte count.

### 4. Bubble Tea maintainers explicitly connect unchanged-line reuse to SSH

**Merged historical renderer fix with qualitative SSH rationale.** [Bubble Tea PR #1132](https://github.com/charmbracelet/bubbletea/pull/1132), opened 2024-09-08 and merged 2024-10-30 as [`9bafc58`](https://github.com/charmbracelet/bubbletea/commit/9bafc58cc877f352240250bf2bf342ad900c8448), changes erase-and-rewrite behavior to reduce flicker. In a [2024-09-17 review comment](https://github.com/charmbracelet/bubbletea/pull/1132#issuecomment-2356280546), a maintainer argues that skipping unchanged lines is a substantial optimization for remote SSH sessions: a three-byte cursor-down sequence replaces retransmission of a whole line.

Users in the thread report reduced flicker and artifacts, but there is no controlled RTT/p95 result. This older renderer change is evidence for the bandwidth mechanism, not something to transplant into the v2 renderer. For Termon's camera route, cursor reuse and, where supported and semantically safe, scrolling a retained viewport can save bytes; scrolling needs tests that preserve the fixed header, footer, margins, and final cursor state.

## Event-driven frames and asynchronous ordering

### 5. Bubble Tea already suppresses unchanged frames

**Maintainer clarification, verified against the requested version.** [Bubble Tea #1363](https://github.com/charmbracelet/bubbletea/issues/1363), opened 2025-03-21, asks for optional event-driven rendering because the reporter assumes that the frame timer continuously repaints. The [maintainer response that day](https://github.com/charmbracelet/bubbletea/issues/1363#issuecomment-2743344887) explains that messages cause updates, changed views cause repaints, and the frame rate limits repaint frequency rather than forcing idle repainting.

In [v2.0.9 `cursed_renderer.go`](https://github.com/charmbracelet/bubbletea/blob/v2.0.9/cursed_renderer.go#L289-L323), `flush` returns early when the view and bounds are unchanged and no start/close/erase condition requires work. Its [`render` method](https://github.com/charmbracelet/bubbletea/blob/v2.0.9/cursed_renderer.go#L626-L632) replaces the pending view. A timer wakeup, a `View` computation, a cell-buffer redraw, and emitted SSH bytes are different costs.

Applicability: don't start by replacing the scheduler with a new dirty-frame architecture. First eliminate unnecessary application messages and animations, batch coherent state changes, and measure actual emitted bytes. Lower frame rate can relieve a constrained stream, but adds frame-scheduling delay; it is not an unconditional latency improvement.

### 6. Command completion order is not authoritative state order

**Documented concurrency plus a distinct historical fix.** [Bubble Tea v2.0.9 `commands.go`](https://github.com/charmbracelet/bubbletea/blob/v2.0.9/commands.go#L1-L30) documents that `Batch` executes commands concurrently without ordering guarantees, while `Sequence` executes commands one at a time. In [`handleCommands`](https://github.com/charmbracelet/bubbletea/blob/v2.0.9/tea.go#L706-L748), command execution uses goroutines and sends results afterward; independent commands can complete in a different order from their creation.

There is also a genuine historical bug report, [#847, 2023-10-23](https://github.com/charmbracelet/bubbletea/issues/847), about multiply nested `Sequence`/`Batch` ordering. Its linked [PR #848](https://github.com/charmbracelet/bubbletea/pull/848) merged on 2025-09-11; the requested version has recursive [`execSequenceMsg`/`execBatchMsg`](https://github.com/charmbracelet/bubbletea/blob/v2.0.9/tea.go#L900-L964). That repaired nested-command behavior does not promise global ordering across independent producers.

Applicability: Termon's monotonic capture sequence addresses the right boundary. Assign the sequence consistently with authoritative snapshot capture, then reject older snapshots at application time. A sequence assigned only when delivery occurs would label an old captured state as new. `tea.Sequence` cannot repair independent Hub broadcasts already delivered out of capture order, and serializing everything behind slow commands could make latency worse. Snapshot rejection must not discard distinct reliable gameplay events merely because their delivery overlaps a newer snapshot.

## Transport and high-RTT interaction

### 7. SSH flow control can block a writer without any renderer bug

**Protocol guarantee with implementation evidence.** [RFC 4254 §5.2, January 2006](https://www.rfc-editor.org/rfc/rfc4254#section-5.2) specifies that a sender must wait when its channel window is exhausted until the peer grants additional space. Section 5.1 also notes that smaller packets can improve interactive response on slow connections. In the inspected [x/crypto v0.51.0 channel implementation](https://github.com/golang/crypto/blob/v0.51.0/ssh/channel.go#L253-L276), writes reserve remote window capacity before writing packets; [`window.reserve`](https://github.com/golang/crypto/blob/v0.51.0/ssh/common.go#L697-L725) explicitly blocks when no capacity remains. This reference version is not a claim about Termon's selected x/crypto version.

Consequently, an asynchronous output queue moves the blocking boundary but does not increase the slow client's drain rate. The supplied bounded-output isolation fix protects other clients; it does not make that client's queued bytes fresh. At 32 KiB/s, an additional 1 KiB ahead of a visible update costs roughly 31.25 ms of serialization, before packet overhead and other delays. Reducing output is therefore consistent with the supplied p95 improvement, without establishing its entire causal breakdown.

Coalesce replaceable **snapshots before ANSI diff generation**, using the retained rendered state as the next diff's base. Do not discard arbitrary already-generated ANSI chunks: a later diff can depend on earlier cursor moves, modes, and cells. Once stale bytes have entered the ordered SSH stream, ordinary SSH cannot replace them with a newer screen state.

### 8. Synchronized output prevents tearing, not round trips or redundant bytes

**Terminal extension semantics.** The [synchronized-output specification pinned to `05050a1`, 2026-04-09](https://github.com/contour-terminal/vt-extensions/blob/05050a11e793c8f4362bf4e34a59ed3f7e5105fe/synchronized-output.md) describes mode 2026: the terminal continues processing incoming text while displaying its previous rendered state, then presents the updated state after synchronization ends. It documents capability detection and explicitly says that timeout behavior is not universally agreed.

This can hide partial-frame tearing over a slow link, but does not reduce the frame's payload. It can delay the first visible portion until the frame completes, and an incomplete or slow frame interacts with terminal-specific timeouts. Bubble Tea v2.0.9 already has [synchronized-output capability-query logic](https://github.com/charmbracelet/bubbletea/blob/v2.0.9/tea.go#L968-L1000), including SSH/environment heuristics; don't assume either universal activation or universal support. Test actual terminal/multiplexer combinations rather than globally forcing it as a latency fix.

### 9. Mosh improves perceived latency by changing the client and protocol

**Published experiment and implemented design.** The authors' [Mosh paper, USENIX ATC 2012](https://mosh.org/mosh-paper.pdf), reports 40 hours of traces from six users, immediate display for 70% of keystrokes, and median response below 5 ms versus 503 ms for SSH on their commercial 3G experiment. These are dated study results, not a forecast for Termon or today's TCP stacks.

Mosh's state synchronization can send the latest screen state instead of every intermediate output byte, adapting its frame rate to avoid filling network queues. Its [version-pinned 1.4.0 README](https://github.com/mobile-shell/mosh/blob/mosh-1.4.0/README.md) and [manual](https://github.com/mobile-shell/mosh/blob/mosh-1.4.0/man/mosh.1) explain speculative local echo, correction after server confirmation, and the limits: prediction covers ordinary typing/backspace and some cursor movement when confidence is sufficient. It is not a general prediction engine for movement through a multiplayer map.

Mosh requires a client and a `mosh-server`, uses SSH to launch/authenticate the latter, and then runs its terminal transport over UDP. It is not an SSH flag that makes a Wish session predictive. A separate Mosh gateway is an architectural option to evaluate, not a drop-in recommendation; game-aware local prediction would require a Termon-aware client, authoritative reconciliation, and a changed deployment contract. A server-side optimistic display still has to travel back to the terminal and cannot provide pre-RTT local feedback.

### 10. There is no evidence here that Nagle is Termon's cause

**Official implementation documentation.** [Go 1.26.0 `TCPConn.SetNoDelay`](https://github.com/golang/go/blob/go1.26.0/src/net/tcpsock.go#L259-L275) documents that the default is `true`, sending data as soon as possible after a write. This does not prove the options of every wrapper or peer in Termon's path, but it rules out treating “enable TCP_NODELAY” as an evidence-free default fix for a normal Go TCP connection.

The Mosh paper provides evidence that reliable byte-stream delivery, loss recovery, and queued obsolete output can hurt remote interaction. Its 2012 retransmission timing observations should not be copied as claims about current TCP behavior. This research has no Termon packet capture showing delayed ACK/Nagle stalls, retransmissions, window starvation, or congestion-control pathology; change transport settings only after observing the corresponding mechanism. Keepalives and faster encryption cannot eliminate propagation delay either.

## Recommended order of work

1. **Complete the two focused validations already underway.** Keep monotonic capture-order rejection and renderer byte reduction as separate fixes with separate regressions. For the `inline` backport, test unchanged interior runs below/at/above the cost threshold, multiple runs, terminal edges, styles, wide-cell continuations, and final cursor/mode state as well as visible cells. The ncurses precedent strengthens the rationale but does not replace these checks.

2. **Measure freshness at each boundary.** Record capture sequence/time, TUI application time, diff bytes, output queue age, blocked-write duration, and client-observed presentation time. Successful server writes do not mean that the client has painted. At approximately 225 ms RTT, this separates stale application state from excess frame scheduling, serialization, and transport queuing.

3. **Reduce obsolete work before changing protocols.** Prefer the latest replaceable snapshot before rendering, stop off-screen or unnecessary animation ticks, and preserve coherent frames. Test the Unicode/spinner case from #163 separately. Evaluate cursor/scroll reuse only where a controlled byte comparison and terminal-state replay show a benefit; avoid redundant layering over optimizations the current renderer already performs.

4. **Treat sub-RTT feedback as a product/client decision.** A nearby authoritative server can reduce network RTT; a predictive client can hide some of it with reconciliation. Ordinary server-rendered SSH, dirty frames, smaller diffs, synchronized output, and TCP tuning cannot make an authoritative response arrive before the input reaches the server and the response returns. With no prediction and a stable approximately 225 ms path, roughly one RTT remains the network component of input-to-authoritative-display latency.

## Limitations

This is a bounded primary-source review, not an exhaustive search or an independent reproduction of the cited applications. GitHub reports may change after the access date; source links use commits/tags where practical. Anonymous GitHub API access hit a rate limit, after which authenticated read-only `gh api` requests were used; the Mosh landing page rejected one fetch, but its paper and versioned source documentation were accessible.

The closest public reports concern redundant rendering, flicker, and nested command ordering, not the exact Termon combination of 32 Trainers, post-lock delivery, constrained bandwidth, and 225 ms RTT. No public exact UV threshold fix surfaced, no broad SSH-window tuning fix was established, and no Mosh/game-movement benefit was measured. Only this report was written; the shared checkout's source, dependencies, tasks, and Git state were left untouched by this research.
