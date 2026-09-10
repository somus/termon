#!/usr/bin/env python3
"""Rebuild the website gameplay cut from the two original screen recordings."""

import argparse
import subprocess
from pathlib import Path

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("intro", type=Path)
parser.add_argument("battle", type=Path)
parser.add_argument("output", type=Path)
args = parser.parse_args()

# Source index, in/out seconds, playback speed. Keep the title, starter reveal,
# multiplayer encounter, battle introduction, and the finishing attack.
clips = [(0, 0.1, 1.75, 1), (0, 7.45, 10, 1), (1, 0.4, 8.7, 1.5),
         (1, 8.7, 12.9, 1.2), (1, 18.3, 34, 1.3)]
filters = []
duration = 0
for index, (source, start, end, speed) in enumerate(clips):
    filters.append(
        f"[{source}:v]trim=start={start}:end={end},setpts=(PTS-STARTPTS)/{speed},"
        f"crop=1150:874:8:46,fps=30,settb=AVTB,format=yuv420p[v{index}]"
    )
    length = (end - start) / speed
    if index == 0:
        duration = length
        joined = "v0"
    else:
        filters.append(
            f"[{joined}][v{index}]xfade=transition=fade:duration=0.2:"
            f"offset={duration - 0.2:.6f}[join{index}]"
        )
        duration += length - 0.2
        joined = f"join{index}"
filters.append(
    f"[{joined}]fade=t=in:st=0:d=0.2,"
    f"fade=t=out:st={duration - 0.35:.6f}:d=0.35,format=yuv420p[out]"
)
subprocess.run([
    "ffmpeg", "-hide_banner", "-loglevel", "error", "-n",
    "-i", str(args.intro), "-i", str(args.battle),
    "-filter_complex", ";".join(filters), "-map", "[out]", "-an",
    "-c:v", "libx264", "-preset", "slow", "-crf", "20",
    "-movflags", "+faststart", str(args.output),
], check=True)
subprocess.run([
    "ffmpeg", "-hide_banner", "-loglevel", "error", "-n", "-ss", "14",
    "-i", str(args.output), "-frames:v", "1", "-q:v", "2",
    str(args.output.with_suffix(".jpg")),
], check=True)
print(f"Created {args.output} ({duration:.2f}s) and its poster.")
