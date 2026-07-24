# Benchmarks

This document records how fast gopip resolves real projects, and how it compares
to the two tools that do the same job: uv and pip-tools. Every figure is
reproducible with the harness in [`examples/benchmark`](../examples/benchmark),
which resolves the same projects with each tool on your own machine.

An earlier version of this document reported gopip as the slowest of the three,
by a wide margin when caches were warm, and said exactly why. That diagnosis is
what the work since then was aimed at, so the previous numbers are kept here for
comparison rather than quietly replaced.

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
- Two phases: a cold resolve with every cache cleared first, and a warm resolve
  with caches populated. Each tool's cache is pointed at a directory the harness
  controls, so a cold run really is cold for all three.

## Environment

- CPU: 12th Gen Intel Core i9-12900H
- OS/arch: linux/amd64 (kernel 6.17)

## How to reproduce

```
cd examples/benchmark
scripts/setup.sh      # build gopip, install uv and pip-tools
scripts/run.sh        # resolve every project with each tool, write work/raw/results.md
```

## The solver is not the slow part

The resolver on its own, with no network, resolves a thousand random satisfiable
dependency graphs in about a second:

```
resolved 1000 random graphs, 0 contradictions, 0 non-deterministic, average 7.3 packages per solution
TestResolveManyRandomGraphs (0.92s)
```

That is under a millisecond per graph. The sharper measurement is against real
data: resolving all five benchmark projects against a frozen copy of the package
index, solver and all, takes about **0.03 seconds in total**. The same five
against the live index take seconds. Whatever the tables below show, essentially
all of it is time spent talking to the index.

## gopip resolves the same sets

Before comparing speed, it is worth confirming the tools agree on the answer.
The pinned package counts match across all three, to within a single package
that comes from each tool's latest-version and marker choices:

| Project | gopip | uv | pip-tools |
| --- | ---: | ---: | ---: |
| cli-tool | 12 | 12 | 12 |
| web-api | 18 | 19 | 18 |
| flask-app | 18 | 18 | 18 |
| django-stack | 8 | 9 | 8 |
| data-science | 9 | 10 | 9 |

## Warm resolve

The same resolve repeated with caches populated. Median of five runs. Lower is
better.

| Project | gopip | uv | pip-tools | gopip before |
| --- | ---: | ---: | ---: | ---: |
| cli-tool | **11 ms** | 33 ms | 3.5 s | 5.4 s |
| web-api | **11 ms** | 46 ms | 5.4 s | 10.4 s |
| flask-app | **10 ms** | 38 ms | 5.1 s | 10.9 s |
| django-stack | **8 ms** | 41 ms | 2.6 s | 3.9 s |
| data-science | **9 ms** | 35 ms | 3.3 s | 6.4 s |

This is where the change is starkest. gopip previously kept no metadata cache at
all, so a second resolve repeated every request the first one made and landed
right back where the cold run did. It now answers a repeat resolve entirely from
disk, without touching the network, in single-digit to low double-digit
milliseconds. That is roughly the solver-bound floor the frozen-index figure
predicts, three to four times quicker than uv, and several hundred times quicker
than pip-tools.

## Cold resolve

Time to resolve from an empty cache. Median of three runs, each preceded by
clearing the cache. Lower is better.

| Project | gopip | uv | pip-tools | gopip before |
| --- | ---: | ---: | ---: | ---: |
| cli-tool | 3.2 s | 2.9 s | 3.5 s | 6.8 s |
| web-api | 4.3 s | 7.7 s | 5.5 s | 10.8 s |
| flask-app | 3.1 s | 4.8 s | 5.3 s | 11.2 s |
| django-stack | 2.0 s | 2.4 s | 2.5 s | 3.6 s |
| data-science | 4.7 s | 5.3 s | 3.4 s | 6.0 s |

gopip used to be one and a half to three times slower than uv here. It is now in
the same range as both peers, ahead on some projects and behind on others.

The honest reading of this table is that the differences between the three tools
are no longer clearly larger than the noise. A cold resolve is dominated by
round-trips to the index, and those vary: across the three samples behind this
table, uv's web-api figure ranged from 3.6 to 7.8 seconds. Take the cold column
as "all three are comparable", not as a ranking.

## What actually changed

Because wall-clock time on a cold resolve moves with the network, the durable
measurement is how many requests a resolve makes. That number is deterministic.
Counted at the HTTP layer, before and after:

| Project | Requests before | Requests after |
| --- | ---: | ---: |
| cli-tool | 24 | 12 |
| web-api | 36 | 19 |
| flask-app | 36 | 18 |
| django-stack | 16 | 8 |
| data-science | 18 | 9 |

Three things account for the tables above.

**A metadata cache.** gopip keeps what it reads from an index on disk, holding a
package's version list briefly and one release's metadata for a week, since the
latter is fixed at publication. This is what the warm column measures.

**Parallel fetching.** The dependencies of a chosen release do not depend on one
another, so they are fetched together rather than one at a time. Requests that
used to run in sequence at roughly a fifth of a second each now overlap.

**Not asking twice.** A package document already carries the full metadata of
the package's latest release, which is the version most packages resolve to.
Keeping it rather than requesting it again removes about half of a resolve's
requests, leaving one per resolved package, which is the floor.

## Threats to validity

These measurements were performed on Linux x86_64 with an Intel i9-12900H.
Absolute timings will vary across operating systems, CPUs, storage devices, and
the versions of the tools compared against.

More than for a purely local benchmark, they vary with the network and the
package index. The cold column especially should be read as an order of
magnitude rather than a ranking: it is sampled three times per tool and project
and reported as the median, and the spread within those samples is comparable to
the differences between the tools. The warm column is far more stable, because
it is local work, and the request counts do not vary at all.

The five projects are real requirement sets but they are five. A project with a
much deeper dependency graph would spend more of a cold resolve waiting between
waves of fetching, since a package's dependencies are only known once its own
metadata arrives, and no amount of parallelism inside a wave removes that.

The harness is published so readers can reproduce all of this on their own
hardware and network and see what holds.
