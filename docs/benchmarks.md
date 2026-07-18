# Benchmarks

This document records how fast gopip resolves real projects, and how it compares
to the two tools that do the same job: uv and pip-tools. It is deliberately
honest about where gopip is slower, and precise about why, because the why
points straight at the work that will fix it. Every figure is reproducible with
the harness in [`examples/benchmark`](../examples/benchmark), which resolves the
same projects with each tool on your own machine.

## What was measured

- Subjects: five real project requirement sets, the kind a project commits.
  - `cli-tool`: click, rich, requests, pyyaml, tqdm.
  - `web-api`: fastapi, uvicorn, pydantic, httpx, jinja2.
  - `flask-app`: flask, sqlalchemy, flask-sqlalchemy, gunicorn, requests.
  - `django-stack`: django, djangorestframework, gunicorn, whitenoise,
    dj-database-url.
  - `data-science`: numpy, pandas, scikit-learn.
- Tools: gopip (this build), uv 0.11.29, and pip-tools 7.4.1. pip-tools drives
  pip's own resolver, so it stands in for pip. All three read a requirements
  file and compute a pinned set; none installs anything.
- Two phases: a cold resolve with empty caches (the realistic first run), and a
  warm resolve with caches populated.

## Environment

- CPU: 12th Gen Intel Core i9-12900H
- OS/arch: linux/amd64 (kernel 6.17)

Resolve times against the live index depend heavily on the network to the
package index. The harness runs each measurement several times and reports the
median, but absolute numbers will differ between networks. The relative standing
of the tools, and the shape of cold versus warm, are the durable part.

## How to reproduce

```
cd examples/benchmark
scripts/setup.sh      # build gopip, install uv and pip-tools
scripts/run.sh        # resolve every project with each tool, write work/raw/results.md
```

## The solver is fast

First, the resolver on its own, with no network: gopip resolves a thousand
random, satisfiable dependency graphs in well under a second.

```
resolved 1000 random graphs, 0 contradictions, 0 non-deterministic, average 7.3 packages per solution
TestResolveManyRandomGraphs (0.57s)
```

That is about half a millisecond per graph. Whatever the wall-clock numbers
below show, the solving itself is not the slow part.

## gopip resolves the same sets

Before comparing speed, it is worth confirming the tools agree on the answer.
The pinned package counts match across all three tools, to within a single
package that comes from each tool's latest-version and marker choices:

| Project | gopip | uv | pip-tools |
| --- | ---: | ---: | ---: |
| cli-tool | 12 | 12 | 12 |
| web-api | 18 | 19 | 18 |
| flask-app | 18 | 18 | 18 |
| django-stack | 8 | 9 | 8 |
| data-science | 9 | 10 | 9 |

gopip is computing the same resolution the established tools do. The question is
only how fast.

## Cold resolve

Time to resolve from an empty cache, the realistic first run on a machine.
Median of several runs. Lower is better.

| Project | gopip | uv | pip-tools |
| --- | ---: | ---: | ---: |
| cli-tool | 6.8 s | 2.5 s | 3.3 s |
| web-api | 10.8 s | 5.3 s | 6.9 s |
| flask-app | 11.2 s | 3.9 s | 5.1 s |
| django-stack | 3.6 s | 2.8 s | 2.9 s |
| data-science | 6.0 s | 3.2 s | 3.0 s |

Cold, all three are in the same order of magnitude, because all three are
waiting on the same package index. gopip is the slowest, roughly one and a half
to three times uv, because it fetches release metadata one package at a time
while uv fetches in parallel.

## Warm resolve

The same resolve with caches populated. Median of several runs. Lower is better.

| Project | gopip | uv | pip-tools |
| --- | ---: | ---: | ---: |
| cli-tool | 5.4 s | 46 ms | 3.3 s |
| web-api | 10.4 s | 50 ms | 5.2 s |
| flask-app | 10.9 s | 39 ms | 4.9 s |
| django-stack | 3.9 s | 29 ms | 2.5 s |
| data-science | 6.4 s | 37 ms | 3.2 s |

This is where the gap is widest, and it is the clearest statement of gopip's
current weakness. uv and pip-tools cache the metadata they fetched, so a second
resolve is fast, and uv, resolving entirely from its cache, finishes in tens of
milliseconds. gopip keeps no metadata cache between runs, so its warm resolve
repeats all the same network work and lands right back where its cold resolve
did. It is slower than both peers, and against uv warm it is not close.

## What the numbers say

Put together, the picture is specific rather than vague:

- gopip's solver is fast, sub-millisecond per graph offline.
- gopip resolves the same package sets as uv and pip-tools.
- gopip is slower end to end, because it fetches index metadata serially and
  caches nothing between runs. That is a fetch-strategy cost, not a solver cost.

The fix is correspondingly specific, and it is the top of the
[roadmap](roadmap.md): fetch metadata concurrently during resolution, using the
concurrent path the index client already has, and add a persistent metadata
cache so a warm resolve does little or no network work. With those in place the
warm column becomes a solver-bound number, which the offline throughput says is
fast. Until then, these tables are the honest state: correct, deterministic
resolutions, produced more slowly than the fastest peer.

## Threats to validity

These measurements were performed on Linux x86_64 with an Intel i9-12900H.
Absolute timings will vary across operating systems, CPUs, storage devices, and
the versions of the tools compared against, and, more than for a purely local
benchmark, across package-index and network latency: the resolve times here are
dominated by round-trips to the package index, so a different network or a
different index mirror will move every number. This is also why the relative
gap, gopip fetching serially while uv fetches in parallel and caches, is the
durable finding rather than any single second count. The benchmark harness is
published so readers can reproduce the measurements on their own hardware and
network and see what holds.
