"""pyte adapter for xterm scrolling emitted by Ultraviolet.

pyte 0.8.2 does not dispatch CSI S/T. Ignoring them makes valid vertical scrolls
look like stale or missing movement, so the SSH and replay probes share this
explicit implementation. This is a measurement adapter, not a production client.
"""

from typing import ClassVar

import pyte


class Screen(pyte.Screen):
    def __init__(self, columns, lines):
        super().__init__(columns, lines)
        self.scroll_operations = 0

    def draw(self, data):
        super().draw(data)
        if pyte.modes.DECAWM not in self.mode:
            self.cursor.x = min(self.cursor.x, self.columns - 1)

    def scroll_up(self, count=1):
        self.scroll_operations += 1
        top, bottom = self.margins or (0, self.lines - 1)
        previous_y = self.cursor.y
        self.cursor.y = bottom
        for _ in range(min(count or 1, bottom - top + 1)):
            self.index()
            self.erase_in_line(2)
        self.cursor.y = previous_y

    def scroll_down(self, count=1):
        self.scroll_operations += 1
        top, bottom = self.margins or (0, self.lines - 1)
        previous_y = self.cursor.y
        self.cursor.y = top
        for _ in range(min(count or 1, bottom - top + 1)):
            self.reverse_index()
            self.erase_in_line(2)
        self.cursor.y = previous_y


class Stream(pyte.Stream):
    csi: ClassVar[dict[str, str]] = {**pyte.Stream.csi, "S": "scroll_up", "T": "scroll_down"}
    events = pyte.Stream.events | {"scroll_up", "scroll_down"}
