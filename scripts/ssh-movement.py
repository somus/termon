#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.11"
# dependencies = ["asyncssh==2.21.1", "pyte==0.8.2"]
# ///
"""Sustained Dojo input with real SSH peers; isolated disposable fixtures only."""

import argparse
import asyncio
import codecs
import contextlib
import json
import time
from itertools import pairwise
from pathlib import Path

import asyncssh
from terminal_emulator import Screen, Stream


async def relay(reader, writer, delay, rate):
    queue = asyncio.Queue(maxsize=16)

    async def receive():
        available = 0.0
        try:
            while data := await reader.read(16384):
                available = max(available, time.monotonic()) + (
                    len(data) / rate if rate else 0
                )
                await queue.put((available + delay, data))
        except ConnectionError:
            pass
        await queue.put((0, b""))

    task = asyncio.create_task(receive())
    try:
        while True:
            due, data = await queue.get()
            if not data:
                break
            await asyncio.sleep(max(0, due - time.monotonic()))
            writer.write(data)
            await writer.drain()
    finally:
        task.cancel()
        with contextlib.suppress(asyncio.CancelledError, ConnectionError):
            await task
        writer.close()


class Terminal:
    def __init__(self, process):
        self.process = process
        self.screen = Screen(120, 40)
        self.stream = Stream(self.screen)
        self.changed = asyncio.Condition()
        self.total = 0
        self.closed = False
        self.positions = None
        self.emulate = True
        self.task = asyncio.create_task(self.read())

    async def read(self):
        decoder = codecs.getincrementaldecoder("utf-8")("replace")
        try:
            while data := await self.process.stdout.read(16384):
                async with self.changed:
                    self.total += len(data)
                    if self.emulate:
                        self.stream.feed(decoder.decode(data))
                    if self.positions is not None:
                        position = self.player_position()
                        if len(position) == 1 and (
                            not self.positions or position[0] != self.positions[-1][1]
                        ):
                            self.positions.append(
                                (time.monotonic(), position[0], self.total)
                            )
                    self.changed.notify_all()
        except (asyncssh.ConnectionLost, OSError):
            pass
        finally:
            async with self.changed:
                self.closed = True
                self.changed.notify_all()

    async def expect(self, text, timeout):
        async with asyncio.timeout(timeout):
            async with self.changed:
                await self.changed.wait_for(
                    lambda: text in "\n".join(self.screen.display) or self.closed
                )
                if self.closed:
                    raise RuntimeError(f"SSH closed waiting for {text!r}")

    def player_position(self):
        return [
            (y, row.index("◎"))
            for y, row in enumerate(self.screen.display)
            if "◎" in row
        ]

    async def move(self, key, timeout):
        previous = self.player_position()
        self.key(key)
        async with asyncio.timeout(timeout):
            async with self.changed:
                await self.changed.wait_for(
                    lambda: (
                        bool(self.player_position())
                        and self.player_position() != previous
                    )
                    or self.closed
                )
                if self.closed:
                    raise RuntimeError("SSH closed waiting for movement")

    def key(self, text):
        self.process.stdin.write(text.encode())


def percentiles(values):
    values = sorted(values)
    return (
        {
            f"p{p}": values[
                min(len(values) - 1, int((len(values) - 1) * p / 100 + 0.5))
            ]
            for p in (50, 95, 99)
        }
        if values
        else {}
    )


async def crowded_session(args, address):
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
            terminal = Terminal(process)
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
                "client_loop_lag_ms": percentiles(loop_lag),
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


async def main(args):
    host, port = args.address.rsplit(":", 1)
    address = (host, int(port))
    handlers = set()

    async def accept(reader, writer):
        current = asyncio.current_task()
        handlers.add(current)
        try:
            remote_reader, remote_writer = await asyncio.open_connection(*address)
            async with asyncio.TaskGroup() as group:
                group.create_task(
                    relay(reader, remote_writer, args.rtt_ms / 2000, args.bytes_per_second)
                )
                group.create_task(
                    relay(remote_reader, writer, args.rtt_ms / 2000, args.bytes_per_second)
                )
        except* ConnectionError:
            pass
        finally:
            writer.close()
            handlers.discard(current)

    proxy = None
    try:
        target = address
        if args.rtt_ms or args.bytes_per_second:
            proxy = await asyncio.start_server(accept, "127.0.0.1", 0)
            target = proxy.sockets[0].getsockname()[:2]
        async with asyncio.timeout(
            args.timeout + 60 + args.hold + (args.cycles * 8 + 6) * (args.rtt_ms / 1000 + 1)
        ):
            row = await crowded_session(args, target)
        print(
            json.dumps(
                {
                    "config": vars(args),
                    "first_render_ms": percentiles([row["first_render_ms"]]),
                    "key_ms": percentiles(row["key_ms"]),
                    "samples": len(row["key_ms"]),
                    "sessions": [row],
                },
                indent=2,
            )
        )
    finally:
        if proxy:
            proxy.close()
            await proxy.wait_closed()
        pending = list(handlers)
        for task in pending:
            task.cancel()
        await asyncio.gather(*pending, return_exceptions=True)


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
    asyncio.run(main(args))
