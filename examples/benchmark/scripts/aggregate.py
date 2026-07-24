#!/usr/bin/env python3
"""Turn the raw benchmark CSVs into the published markdown tables.

Reads resolve.csv and counts.csv from the raw directory and writes results.md.
Standard library only.
"""

import csv
import os
import statistics
import sys

RAW = os.environ.get("GOPIP_BENCH_RAW")
if not RAW:
    here = os.path.dirname(os.path.abspath(__file__))
    work = os.environ.get("GOPIP_BENCH_WORK") or os.path.join(here, "..", "work")
    RAW = os.path.join(work, "raw")

PROJECT_ORDER = ["cli-tool", "web-api", "flask-app", "django-stack", "data-science"]
TOOL_ORDER = ["gopip", "uv", "pip-compile"]
TOOL_LABEL = {"gopip": "gopip", "uv": "uv", "pip-compile": "pip-tools"}


def load(name):
    path = os.path.join(RAW, name)
    if not os.path.exists(path):
        return []
    with open(path, newline="") as fh:
        return list(csv.DictReader(fh))


def medians(rows):
    """(phase,tool,project) -> median wall ms."""
    buckets = {}
    for r in rows:
        parts = r.get("label", "").split(",")
        if len(parts) != 3:
            continue
        try:
            ms = float(r["wall_seconds"]) * 1000.0
        except (ValueError, KeyError):
            continue
        buckets.setdefault(tuple(parts), []).append(ms)
    return {k: statistics.median(v) for k, v in buckets.items()}


def counts(rows):
    out = {}
    for r in rows:
        out[(r.get("tool"), r.get("project"))] = r.get("packages", "?")
    return out


def fmt(ms):
    if ms is None:
        return "n/a"
    if ms >= 1000:
        return f"{ms/1000:.1f} s"
    return f"{ms:.0f} ms"


def phase_table(med, phase, title, note):
    projects = [p for p in PROJECT_ORDER if any(k[2] == p for k in med if k[0] == phase)]
    lines = [f"### {title}\n", note + "\n"]
    header = "| Project | " + " | ".join(TOOL_LABEL[t] for t in TOOL_ORDER) + " |"
    sep = "| --- | " + " | ".join("---:" for _ in TOOL_ORDER) + " |"
    lines += [header, sep]
    for p in projects:
        cells = [fmt(med.get((phase, t, p))) for t in TOOL_ORDER]
        lines.append(f"| {p} | " + " | ".join(cells) + " |")
    return "\n".join(lines) + "\n"


def counts_table(cnt):
    projects = [p for p in PROJECT_ORDER if any(k[1] == p for k in cnt)]
    lines = ["### Packages resolved\n",
             "The pinned package count each tool produced. Small differences",
             "come from each tool's latest-version choices and marker handling;",
             "each set is internally consistent.\n"]
    header = "| Project | " + " | ".join(TOOL_LABEL[t] for t in TOOL_ORDER) + " |"
    sep = "| --- | " + " | ".join("---:" for _ in TOOL_ORDER) + " |"
    lines += [header, sep]
    for p in projects:
        cells = [str(cnt.get((t, p), "n/a")) for t in TOOL_ORDER]
        lines.append(f"| {p} | " + " | ".join(cells) + " |")
    return "\n".join(lines) + "\n"


def main():
    med = medians(load("resolve.csv"))
    cnt = counts(load("counts.csv"))

    parts = ["# gopip benchmark results\n"]
    machine = os.path.join(RAW, "machine.txt")
    if os.path.exists(machine):
        parts.append("```\n" + open(machine).read().rstrip() + "\n```\n")
    algo = os.path.join(RAW, "algorithm.txt")
    if os.path.exists(algo) and open(algo).read().strip():
        parts.append("### Resolver throughput (offline, no network)\n")
        parts.append("```\n" + open(algo).read().rstrip() + "\n```\n")

    if med:
        parts.append(phase_table(
            med, "cold", "Cold resolve (empty caches)",
            "Time to resolve a requirements file to a pinned set from a cold "
            "cache, the realistic first run. Every sample clears the tool's "
            "cache first. Median. Lower is better."))
        parts.append(phase_table(
            med, "warm", "Warm resolve (caches populated)",
            "The same resolve repeated with each tool's cache warm. All three "
            "tools cache index metadata, so this measures how much of a repeat "
            "resolve each of them can answer without the network. Median. "
            "Lower is better."))
    if cnt:
        parts.append(counts_table(cnt))
    if not med:
        parts.append("_No resolve samples found. Run resolve_bench.sh._\n")

    report = "\n".join(parts)
    out = os.path.join(RAW, "results.md")
    with open(out, "w") as fh:
        fh.write(report)
    sys.stdout.write(report)
    sys.stderr.write(f"\nwrote {out}\n")


if __name__ == "__main__":
    main()
