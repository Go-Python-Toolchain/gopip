# gopip benchmark

Reproduce the numbers behind gopip's resolver on your own machine, and compare
gopip against the two tools that do the same job: uv and pip-tools. This harness
resolves several real project requirement sets with each tool and times them,
and it measures gopip's solver on its own, with no network, so you can see the
algorithm's speed separately from the cost of talking to the package index.

The published figures live in [`docs/benchmarks.md`](../../docs/benchmarks.md).
This directory is the harness that produces them.

## What it measures

1. **Resolver throughput (offline).** gopip resolves a thousand random,
   satisfiable dependency graphs with no network involved. This is the solver
   itself, decoupled from the index.
2. **Cold resolve.** Each tool resolves a project's requirements to a pinned set
   with an empty cache, the realistic first run.
3. **Warm resolve.** The same resolve repeated with each tool's cache populated.
   gopip keeps no metadata cache today, so its warm figure barely moves; uv and
   pip-tools cache aggressively.

None of the tools installs anything here. This measures resolution only:
requirements in, a pinned set out.

## The tools, and a fair-comparison note

- **gopip**: `gopip resolve -r <project>`.
- **uv**: `uv pip compile <project>`.
- **pip-tools**: `pip-compile <project>`, which drives pip's own resolver, so it
  stands in for pip.

All three read a requirements file and compute a consistent pinned set. They may
produce slightly different sets: each picks the latest versions that fit at the
moment it runs, and they handle markers and pre-releases with small differences,
so counts can vary by a package or two. Every set is internally consistent. The
harness records each tool's resolved set under `work/raw/resolved/` so you can
compare them.

The comparison is deliberately unflattering where gopip is weak: it shows that
gopip's cold and warm resolves are slower than uv's, because gopip fetches
metadata serially and does not cache it between runs. The offline throughput
number shows that this is a fetch-strategy cost, not a solver cost. Both are the
honest picture, and both are why concurrent fetch and a cache are the top items
on gopip's roadmap.

## Requirements

- `python3` (runs the measurement wrapper and the aggregator, standard library
  only)
- `go` (to build gopip, if a `gopip` binary is not already present)
- `curl` (to install uv), and a network connection (the resolves hit the live
  package index)

The competitors are installed into `work/tools/`, which is git ignored. Nothing
touches your global environment.

## Run it

```
# From this directory (examples/benchmark).

# 1. One time: build gopip, install uv and pip-tools.
scripts/setup.sh

# 2. Measure everything and write work/raw/results.md.
scripts/run.sh
```

Or run a single stage:

```
scripts/machine.sh          # record CPU / OS / tool versions
scripts/algorithm_bench.sh  # gopip's offline resolver throughput
scripts/resolve_bench.sh    # cold and warm resolves across tools and projects
python3 scripts/aggregate.py # rebuild results.md from the raw CSVs
```

Results land in `work/raw/`:

- `results.md` - the assembled tables
- `resolve.csv`, `counts.csv` - one row per sample, and the package counts
- `resolved/<tool>-<project>.txt` - each tool's resolved set, for comparison
- `algorithm.txt` - the offline throughput measurement
- `machine.txt` - CPU, OS, and tool versions

## A note on network variance

Resolve times against the live index depend heavily on the network between your
machine and the package index. The harness runs each measurement several times
and reports the median, but absolute numbers will differ between networks. The
relative standing of the tools, and the shape of cold versus warm, are the
durable part.

## Projects and tuning

The requirement sets live in `projects/` and are real dependency lists (a CLI
tool, a FastAPI service, a Flask app, a Django stack, and a data-science stack).

Environment variables (all optional):

- `GOPIP_BENCH_RUNS` - warm repetitions per measurement (default 5, median wins)
- `GOPIP_BENCH_WORK` - where tools, caches, and raw output live (default `./work`)
- `GOPIP_BENCH_TIMEOUT` - per-resolve ceiling in seconds (default 180)
- `GOPIP_BIN`, `UV_BIN`, `PIPCOMPILE_BIN` - point at specific tool binaries

## Pinned versions

| Piece | Version |
| --- | --- |
| uv | 0.11.29 |
| pip-tools | 7.4.1 (with pip 24.2) |
