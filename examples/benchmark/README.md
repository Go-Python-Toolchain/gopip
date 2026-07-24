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
   All three tools cache index metadata, so this measures how much of a repeat
   resolve each of them can answer without going back to the network.

Each tool's cache is pointed at a directory under `work/caches`, which the
harness clears before every cold sample. That includes gopip, which reads its
cache location from the environment: without setting it, gopip would use the
cache in your home directory and every cold measurement here would silently be a
warm one.

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

The comparison is meant to be readable in either direction. It once showed
gopip clearly slower than uv in both phases, which is what drove the work on how
gopip fetches; it now shows gopip ahead on warm resolves and comparable on cold
ones. Read the cold column with care: it is network bound, and the spread
between repeated samples is about as large as the difference between the tools.
The warm column and the offline throughput number are the stable ones.

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
machine and the package index. The harness repeats every measurement and reports
the median, three times for the cold phase and five for the warm one, but
absolute numbers will differ between networks.

The cold phase is the volatile one, since every sample re-fetches everything. In
one published run a single tool's cold figure for one project ranged from 3.6 to
7.8 seconds across three samples, which is wider than the gap between the tools.
Treat cold numbers as an order of magnitude rather than a ranking. Warm numbers
are mostly local work and are far steadier.

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
