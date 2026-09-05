#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.11"
# dependencies = ["asyncssh==2.21.1", "pyte==0.8.2"]
# ///
"""Measure terminal-visible SSH latency, not the arrival of arbitrary bytes.

Use only against an isolated disposable Termon server: sessions register fresh
credentials unless --fixture selects returning Trainers. Host-key verification
is intentionally disabled for these ephemeral servers. The bounded TCP relay adds
propagation delay independently of serialization; it does not sleep once per packet and accidentally limit throughput.
"""

import argparse
import asyncio
import codecs
import contextlib
import copy
import json
from pathlib import Path
import time

import asyncssh
import pyte


async def relay(reader, writer, delay, rate):
    queue = asyncio.Queue(maxsize=16)  # at most 16 * 16 KiB per direction

    async def receive():
        available = 0.0
        try:
            while data := await reader.read(16384):
                now = time.monotonic()
                available = max(available, now) + (len(data) / rate if rate else 0)
                await queue.put((available + delay, data))
        except ConnectionError:
            pass  # normal when an ephemeral client closes after its final sample
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
        self.screen = pyte.Screen(120, 40)
        self.stream = pyte.Stream(self.screen)
        self.changed = asyncio.Condition()
        self.total = 0
        self.closed = False
        self.read_rate = 0
        self.failure = None
        self.task = asyncio.create_task(self.read())
        self.pause = asyncio.Event()
        self.pause.set()

    async def read(self):
        decoder = codecs.getincrementaldecoder("utf-8")("replace")
        try:
            while True:
                await self.pause.wait()
                data = await self.process.stdout.read(256 if self.read_rate else 16384)
                if not data:
                    break
                async with self.changed:
                    self.total += len(data)
                    self.stream.feed(decoder.decode(data))
                    self.changed.notify_all()
                if self.read_rate:
                    await asyncio.sleep(len(data) / self.read_rate)
        except (asyncssh.ConnectionLost, OSError) as error:
            self.failure = str(error)
        finally:
            async with self.changed:
                self.closed = True
                self.changed.notify_all()

    async def expect(self, text, timeout=30):
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

    async def battle(self):
        self.key("f")
        await self.expect("FIND BATTLE")
        self.key("\r")
        await self.expect("Go! ROOTKIT!  ▼")
        self.key("\r")
        await self.expect("FIGHT")

    async def starter(self):
        self.key(" ")
        for text in (
            "I am Master Sable. This Dojo is my hall.",
            "TERMON are creatures you raise and battle over SSH.",
            "You are a Trainer. Pick a partner, learn their Moves, fight others.",
            "First, who are you?",
        ):
            await self.expect(text)
            self.key("\r")
        await self.expect("What is your name?")
        self.key("e")
        await self.expect("> ")
        self.key("latency-probe\r")
        await self.expect("Right! So you are LATENCY-PROBE!")
        self.key("\r")
        await self.expect("ROOTKIT, the")


async def session(args, address, barrier):
    started = time.monotonic()
    key = (
        asyncssh.read_private_key(str(Path(args.fixture) / f"key-{args.index:02}"))
        if args.fixture
        else asyncssh.generate_private_key("ssh-ed25519")
    )
    async with asyncssh.connect(
        *address,
        username="latency-probe",
        client_keys=[key],
        known_hosts=None,
        agent_path=None,
        config=None,
        encoding=None,
    ) as conn:
        process = await conn.create_process(
            term_type="xterm-256color",
            term_size=(120, 40),
            encoding=None,
            window=args.window,
        )
        terminal = Terminal(process)
        try:
            await terminal.expect(
                "◎" if args.fixture else "press any key", args.timeout
            )
            first = time.monotonic() - started
            if not args.fixture and args.mode != "welcome":
                await terminal.starter()
            if args.mode == "battle":
                await terminal.battle()
            await barrier.wait()
            before = terminal.total
            idle_start = time.monotonic()
            if args.mode == "stalled":
                terminal.pause.clear()
            elif args.mode == "slow":
                terminal.read_rate = args.read_bytes_per_second
            await asyncio.sleep(args.hold)
            terminal.pause.set()
            idle_bps = (terminal.total - before) / (time.monotonic() - idle_start)
            samples = []
            before = terminal.total
            active_start = time.monotonic()
            if args.mode == "movement":
                for i in range(args.keys):
                    sent = time.monotonic()
                    await terminal.move("a" if i % 2 == 0 else "d", args.timeout)
                    samples.append((time.monotonic() - sent) * 1000)
            elif args.mode == "battle":
                for i in range(args.keys):
                    sent = time.monotonic()
                    terminal.key("\r" if i % 2 == 0 else "\x1b")
                    await terminal.expect(
                        "ROOT ACCESS" if i % 2 == 0 else "FIGHT", args.timeout
                    )
                    samples.append((time.monotonic() - sent) * 1000)
            elif args.mode == "navigation":
                for i in range(args.keys):
                    expected, key = (
                        ("EMBERBYTE, the", "2") if i % 2 == 0 else ("ROOTKIT, the", "1")
                    )
                    sent = time.monotonic()
                    terminal.key(key)
                    await terminal.expect(expected, args.timeout)
                    samples.append((time.monotonic() - sent) * 1000)
            elapsed = time.monotonic() - active_start
            active_bytes = terminal.total - before
            action_ms = None
            if args.mode == "battle":
                await barrier.wait()
                if args.index == 0:
                    if args.keys % 2 == 0:
                        terminal.key("\r")
                        await terminal.expect("ROOT ACCESS")
                    sent = time.monotonic()
                    terminal.key("1")
                    await terminal.expect("Waiting for opponent…")
                    action_ms = (time.monotonic() - sent) * 1000
                await barrier.wait()
                if args.index == 1:
                    terminal.key("f")
                await terminal.expect("forfeited")
            if args.mode != "stalled" and conn.is_closed():
                raise RuntimeError(
                    f"{args.mode} client disconnected during the workload"
                )
            return {
                "mode": args.mode,
                "first_render_ms": first * 1000,
                "key_ms": samples,
                "peer_disconnected": conn.is_closed(),
                "battle_action_ms": action_ms,
                "idle_bytes_per_second": idle_bps,
                "active_bytes_per_second": active_bytes / elapsed,
            }
        finally:
            terminal.pause.set()
            terminal.read_rate = 0
            conn.close()
            await conn.wait_closed()
            await terminal.task


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


async def main(args):
    host, port = args.address.rsplit(":", 1)
    address = (host, int(port))
    proxy = None
    handlers = set()

    async def accept(reader, writer):
        current = asyncio.current_task()
        handlers.add(current)
        try:
            remote_reader, remote_writer = await asyncio.open_connection(*address)
            async with asyncio.TaskGroup() as group:
                group.create_task(
                    relay(
                        reader, remote_writer, args.rtt_ms / 2000, args.bytes_per_second
                    )
                )
                group.create_task(
                    relay(
                        remote_reader, writer, args.rtt_ms / 2000, args.bytes_per_second
                    )
                )
        except* ConnectionError:
            pass
        finally:
            writer.close()
            handlers.discard(current)

    target = address
    if args.rtt_ms or args.bytes_per_second:
        proxy = await asyncio.start_server(accept, "127.0.0.1", 0)
        target = proxy.sockets[0].getsockname()[:2]
    try:
        configs = []
        for index in range(args.sessions):
            config = copy.copy(args)
            config.index = index
            configs.append(config)
        for mode, count in (
            ("stalled", args.stalled_companions),
            ("slow", args.slow_companions),
        ):
            for _ in range(count):
                companion = copy.copy(args)
                companion.mode = mode
                companion.fixture = None
                companion.window = 8192
                companion.hold += args.keys * (args.rtt_ms / 1000 + 0.5)
                configs.append(companion)
        barrier = asyncio.Barrier(len(configs))
        async with asyncio.timeout(
            args.timeout + 60 + args.hold + args.keys * (args.rtt_ms / 1000 + 1)
        ):
            async with asyncio.TaskGroup() as group:
                jobs = [
                    group.create_task(session(config, target, barrier))
                    for config in configs
                ]
            rows = [job.result() for job in jobs]
        keys = [value for row in rows for value in row["key_ms"]]
        print(
            json.dumps(
                {
                    "config": vars(args),
                    "first_render_ms": percentiles(
                        [r["first_render_ms"] for r in rows]
                    ),
                    "key_ms": percentiles(keys),
                    "samples": len(keys),
                    "sessions": rows,
                },
                indent=2,
            )
        )
    finally:
        if proxy:
            proxy.close()
            await proxy.wait_closed()
        for task in list(handlers):
            task.cancel()
        await asyncio.gather(*list(handlers), return_exceptions=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--address", default="127.0.0.1:2222")
    parser.add_argument("--sessions", type=int, default=1)
    parser.add_argument("--keys", type=int, default=100)
    parser.add_argument("--stalled-companions", type=int, default=0)
    parser.add_argument("--slow-companions", type=int, default=0)
    parser.add_argument("--hold", type=float, default=5)
    parser.add_argument("--timeout", type=float, default=30)
    parser.add_argument("--rtt-ms", type=float, default=0)
    parser.add_argument("--bytes-per-second", type=int, default=0)
    parser.add_argument(
        "--mode",
        choices=("welcome", "navigation", "stalled", "slow", "movement", "battle"),
        default="navigation",
    )
    parser.add_argument(
        "--fixture", help="directory prepared by TestPrepareLatencyFixture"
    )
    parser.add_argument("--read-bytes-per-second", type=int, default=1024)
    parser.add_argument("--window", type=int, default=2097152)
    options = parser.parse_args()
    if (
        options.sessions < 1
        or options.keys < 1
        or options.timeout <= 0
        or min(
            options.hold,
            options.rtt_ms,
            options.bytes_per_second,
            options.stalled_companions,
            options.slow_companions,
        )
        < 0
        or options.read_bytes_per_second < 1
        or options.window < 1
    ):
        parser.error("counts must be positive and delay/rate/hold nonnegative")
    if options.mode in ("movement", "battle") and not options.fixture:
        parser.error("movement and battle need --fixture")
    if options.fixture and (
        options.sessions > 32 or options.mode not in ("movement", "battle", "welcome")
    ):
        parser.error(
            "fixtures support up to 32 sessions in movement, battle, or welcome mode"
        )
    if options.mode == "battle" and (
        options.sessions != 2 or options.stalled_companions or options.slow_companions
    ):
        parser.error("battle mode requires exactly two sessions and no companions")
    asyncio.run(main(options))
