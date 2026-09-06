#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.11"
# dependencies = ["asyncssh==2.21.1", "pyte==0.8.2"]
# ///
"""Sustained Dojo input with real SSH peers; isolated disposable fixtures only."""

import argparse
import asyncio
import contextlib
import importlib.util
import time
from itertools import pairwise
from pathlib import Path

import asyncssh

spec = importlib.util.spec_from_file_location(
    "ssh_latency", Path(__file__).with_name("ssh-latency.py")
)
probe = importlib.util.module_from_spec(spec)
spec.loader.exec_module(probe)


async def crowded_session(args, address, _barrier):
    terminals, tasks = [], []
    stop = asyncio.Event()
    loop_lag = []
    async with contextlib.AsyncExitStack() as stack:

        async def connect(index):
            key = asyncssh.read_private_key(str(Path(args.fixture) / f"key-{index:02}"))
            conn = await stack.enter_async_context(
                asyncssh.connect(
                    *address,
                    username="latency-probe",
                    client_keys=[key],
                    known_hosts=None,
                    agent_path=None,
                    config=None,
                    encoding=None,
                )
            )
            process = await conn.create_process(
                term_type="xterm-256color", term_size=(120, 40), encoding=None
            )
            terminal = probe.Terminal(process)
            terminals.append((terminal, conn))
            await terminal.expect("◎", args.timeout)
            return terminal, conn

        async def wave(terminal, offset):
            await asyncio.sleep(offset)
            n = 0
            while not stop.is_set():
                terminal.key("w" if n % 2 == 0 else "s")
                n += 1
                await asyncio.sleep(0.1)

        async def watch_loop():
            while not stop.is_set():
                due = time.monotonic() + 0.01
                await asyncio.sleep(0.01)
                loop_lag.append(max(0, time.monotonic() - due) * 1000)

        try:
            opened = time.monotonic()
            terminal, conn = await connect(0)  # reserve the westmost entrance first
            first_render = (time.monotonic() - opened) * 1000
            async with asyncio.TaskGroup() as group:
                connecting = [
                    group.create_task(connect(i)) for i in range(1, args.peers + 1)
                ]
            peers = [task.result() for task in connecting]
            for peer, _ in peers:
                peer.emulate = (
                    False  # drain output without 31 extra Python screen parsers
                )
            if args.refresh_population:
                await asyncio.sleep(1)
                await terminal.move("a", args.timeout)
                await terminal.move("d", args.timeout)
            try:
                await terminal.expect(f"{args.peers + 1} inside", args.timeout)
            except TimeoutError as error:
                raise RuntimeError(
                    "population not visible:\n" + "\n".join(terminal.screen.display)
                ) from error
            await asyncio.sleep(args.hold)
            initial = terminal.player_position()
            if len(initial) != 1:
                raise RuntimeError("expected one local Trainer marker")
            row, column = initial[0]
            keys = "aaaadddd" * args.cycles + "aaaaaa"
            offset = 0
            expected = []
            for key in keys:
                offset += -1 if key == "a" else 1
                expected.append((row, column + offset * 9))
            if args.moving_peers:
                tasks.extend(
                    asyncio.create_task(wave(peer, i * 0.1 / max(1, args.peers)))
                    for i, (peer, _) in enumerate(peers)
                )
            tasks.append(asyncio.create_task(watch_loop()))
            started = time.monotonic()
            initial_bytes = terminal.total
            initial_scrolls = terminal.screen.scroll_operations
            terminal.positions = [(started, initial[0], initial_bytes)]
            sent = []
            for index, key in enumerate(keys):
                await asyncio.sleep(
                    max(
                        0,
                        started
                        + index * args.key_interval_ms / 1000
                        - time.monotonic(),
                    )
                )
                sent.append(time.monotonic())
                terminal.key(key)
            timed_out = False
            try:
                async with asyncio.timeout(args.timeout):
                    async with terminal.changed:
                        await terminal.changed.wait_for(
                            lambda: (
                                terminal.player_position() == [expected[-1]]
                                or terminal.closed
                            )
                        )
            except TimeoutError:
                timed_out = True
            await asyncio.sleep(0.5)
            observations = terminal.positions[1:]
            terminal.positions = None
            stop.set()
            await asyncio.gather(*tasks)
            exact = [p for _, p, _ in observations] == expected
            samples = (
                [(t - s) * 1000 for (t, _, _), s in zip(observations, sent)]
                if exact
                else []
            )
            final_correct = terminal.player_position() == [expected[-1]]
            final_arrivals = [t for t, p, _ in observations if p == expected[-1]]
            # The final x=2 tile is never visited in the oscillating x=4..8 path.
            drain_ms = (final_arrivals[0] - sent[-1]) * 1000 if final_arrivals else None
            collision_ms = None
            payload = terminal.total - initial_bytes
            if final_correct and not timed_out:
                for peer, _ in peers:
                    peer.key("s")  # settle every peer on the entrance row
                await asyncio.sleep(args.rtt_ms / 1000 + 1)
                for _ in range(6):
                    await terminal.move(
                        "d", args.timeout
                    )  # reset to x=8 for repeat runs
                if peers and not args.moving_peers:
                    collision_sent = time.monotonic()
                    terminal.key("d")
                    await terminal.expect("lobby: blocked", args.timeout)
                    collision_ms = (time.monotonic() - collision_sent) * 1000
            return {
                "mode": "movement-sustained",
                "first_render_ms": first_render,
                "key_ms": samples,
                "input_count": len(keys),
                "observed_positions": len(observations),
                "exact_trace": exact,
                "final_correct": final_correct,
                "timed_out": timed_out,
                "final_key_to_visible_ms": drain_ms,
                "max_visual_gap_ms": max(
                    (b[0] - a[0]) * 1000 for a, b in pairwise(observations)
                )
                if len(observations) > 1
                else None,
                "client_loop_lag_ms": probe.percentiles(loop_lag),
                "collision_ms": collision_ms,
                "payload_bytes": payload,
                "scroll_operations": terminal.screen.scroll_operations
                - initial_scrolls,
                "peer_count": len(peers),
                "peer_disconnected": conn.is_closed()
                or any(c.is_closed() for _, c in peers),
                "sent_ms": [(t - started) * 1000 for t in sent],
                "observations": [
                    [(t - started) * 1000, p, n - initial_bytes]
                    for t, p, n in observations
                ],
            }
        finally:
            stop.set()
            for task in tasks:
                task.cancel()
            await asyncio.gather(*tasks, return_exceptions=True)
            for terminal, conn in terminals:
                conn.close()
            await asyncio.gather(*(conn.wait_closed() for _, conn in terminals))
            await asyncio.gather(
                *(terminal.task for terminal, _ in terminals), return_exceptions=True
            )


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--address", default="127.0.0.1:2222")
    parser.add_argument("--fixture", required=True)
    parser.add_argument("--peers", type=int, default=31)
    parser.add_argument("--moving-peers", action="store_true")
    parser.add_argument(
        "--refresh-population",
        action="store_true",
        help="explicit baseline workaround: refresh after concurrent joins",
    )
    parser.add_argument("--cycles", type=int, default=75)
    parser.add_argument("--key-interval-ms", type=float, default=33)
    parser.add_argument("--rtt-ms", type=float, default=225)
    parser.add_argument("--bytes-per-second", type=int, default=0)
    parser.add_argument("--timeout", type=float, default=30)
    parser.add_argument("--hold", type=float, default=1)
    args = parser.parse_args()
    if (
        not 0 <= args.peers <= 31
        or args.cycles < 1
        or args.timeout <= 0
        or args.key_interval_ms <= 0
        or min(args.rtt_ms, args.bytes_per_second, args.hold) < 0
    ):
        parser.error(
            "peers must be 0..31; cycles/timeout/interval positive; delays/rate nonnegative"
        )
    args.sessions = 1
    args.keys = args.cycles * 8 + 6
    args.stalled_companions = args.slow_companions = 0
    asyncio.run(probe.main(args, crowded_session))
