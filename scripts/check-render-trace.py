#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.11"
# dependencies = ["pyte==0.8.2"]
# ///
"""Compare incremental renderer output with full repaints, including cell styles.

Generate input with TERMON_RENDER_TRACE=<new-file> go test ./internal/tui
-run '^TestExportLobbyRendererTrace$' -count=1. No SSH server is required.
"""

import argparse
import json
from pathlib import Path

from terminal_emulator import Screen, Stream


def compare(frames):
    screen = stream = None
    for index, frame in enumerate(frames):
        w, h = frame["width"], frame["height"]
        if frame["reset"]:
            screen = Screen(w, h)
            stream = Stream(screen)
        screen.resize(lines=h, columns=w)
        stream.feed(frame["delta"])
        reference = Screen(w, h)
        Stream(reference).feed(frame["full"])
        if (screen.cursor.x, screen.cursor.y) != (frame["cursor_x"], frame["cursor_y"]):
            raise AssertionError(
                f"frame {index}: renderer cursor tracking differs from terminal"
            )
        if screen.mode != reference.mode:
            raise AssertionError(
                f"frame {index}: terminal modes differ from full repaint"
            )
        for y in range(h):
            for x in range(w):
                actual, wanted = screen.buffer[y][x], reference.buffer[y][x]
                # Erased/scrolled plain spaces need not restore an invisible
                # foreground color or its bold intensity. Preserve backgrounds
                # and every attribute which paints ink on a space.
                if actual.data == wanted.data == " " and not any(
                    (c.underscore or c.strikethrough or c.reverse)
                    for c in (actual, wanted)
                ):
                    actual, wanted = (
                        actual._replace(fg="", bold=False),
                        wanted._replace(fg="", bold=False),
                    )
                if actual != wanted:
                    raise AssertionError(
                        f"frame {index}, cell {x},{y}: {screen.buffer[y][x]!r} != {reference.buffer[y][x]!r}"
                    )
    return len(frames)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("trace", type=Path)
    args = parser.parse_args()
    count = compare(json.loads(args.trace.read_text()))
    print(
        f"All {count} frames match full repaints: visible styles, cursor tracking, and modes."
    )
