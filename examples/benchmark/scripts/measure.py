#!/usr/bin/env python3
"""Run a command once and print its wall time and peak resident memory.

Uses perf_counter for microsecond wall resolution and
getrusage(RUSAGE_CHILDREN).ru_maxrss for the child's peak resident set size, so a
single process launch yields both numbers accurately.

Output (one line, to stdout):  <wall_seconds>,<max_rss_kb>,<exit_code>
The measured command's own stdout and stderr are discarded.
"""

import resource
import subprocess
import sys
import time

# ru_maxrss units differ by platform: kilobytes on Linux, bytes on macOS.
_RSS_DIVISOR = 1024 if sys.platform == "darwin" else 1


def main():
    if len(sys.argv) < 2:
        sys.stderr.write("usage: measure.py CMD [ARG...]\n")
        return 2
    cmd = sys.argv[1:]

    start = time.perf_counter()
    rc = subprocess.run(
        cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL
    ).returncode
    wall = time.perf_counter() - start
    rss_kb = resource.getrusage(resource.RUSAGE_CHILDREN).ru_maxrss / _RSS_DIVISOR

    sys.stdout.write(f"{wall:.6f},{rss_kb:.0f},{rc}\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
