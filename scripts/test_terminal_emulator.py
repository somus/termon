#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.11"
# dependencies = ["pyte==0.8.2"]
# ///
"""Regression tests for the terminal behaviors used by the SSH probes."""

import unittest

from terminal_emulator import Screen, Stream


class ScrollTests(unittest.TestCase):
    def setUp(self):
        self.screen = Screen(5, 5)
        self.stream = Stream(self.screen)
        for y, letter in enumerate("ABCDE", 1):
            self.stream.feed(f"\x1b[{y};1H{letter * 5}")
        self.stream.feed("\x1b[2;4r\x1b[1;3H")

    def test_scroll_up_preserves_cursor_and_region(self):
        self.stream.feed("\x1b[41m\x1b[2S")
        self.assertEqual(
            self.screen.display, ["AAAAA", "DDDDD", "     ", "     ", "EEEEE"]
        )
        self.assertEqual((self.screen.cursor.x, self.screen.cursor.y), (2, 0))
        self.assertEqual(self.screen.buffer[3][0].bg, "red")

    def test_scroll_down_default_count(self):
        self.stream.feed("\x1b[44m\x1b[0T")
        self.assertEqual(
            self.screen.display, ["AAAAA", "     ", "BBBBB", "CCCCC", "EEEEE"]
        )
        self.assertEqual((self.screen.cursor.x, self.screen.cursor.y), (2, 0))
        self.assertEqual(self.screen.buffer[1][0].bg, "blue")

    def test_large_scroll_is_bounded_to_region(self):
        self.stream.feed("\x1b[999999999S")
        self.assertEqual(
            self.screen.display, ["AAAAA", "     ", "     ", "     ", "EEEEE"]
        )

    def test_disabled_autowrap_does_not_leave_pending_wrap(self):
        screen = Screen(5, 2)
        Stream(screen).feed("\x1b[?7l\x1b[1;5HX\x1b[?7hYZ")
        self.assertEqual(screen.display, ["    Y", "Z    "])
        self.assertEqual((screen.cursor.x, screen.cursor.y), (1, 1))


if __name__ == "__main__":
    unittest.main()
