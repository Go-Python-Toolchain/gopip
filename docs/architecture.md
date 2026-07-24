# gopip Architecture

This document explains how gopip is put together and records the main design
decisions. The [resolver design](resolver.md) covers the solving algorithm in
depth; this is the wider picture, from a requirements file to an installed
environment.

## What gopip is, and is not

gopip computes a complete, consistent set of package versions for a project and
writes a deterministic lockfile. It does not download wheels, build packages, or
install anything itself. It takes over the slow, fiddly part, resolution, and
hands the installation to pip. So it drops into an existing workflow without
asking a project to change its layout, its requirements files, or its primary
commands.

That boundary is the central design choice: gopip is a sibling to pip, not a
replacement for it. Everything below follows from keeping the resolver pure and
leaving side effects to the tool that already does them well.

## The pipeline

A resolve moves through a small set of packages, each with one job:

```
requirements  ->  parse  ->  fetch metadata  ->  resolve  ->  lock / print / install
```

- **`internal/version`** implements PEP 440: parsing versions, ordering them
  (including pre-releases, post-releases, and epochs), and testing version
  specifiers. Everything downstream compares versions through this package, so
  ordering is correct and consistent.
- **`internal/requirement`** implements PEP 508: requirement strings with
  extras and version specifiers, environment markers, and the recursive `-r` and
  `-c` includes of requirements files. Markers are evaluated against a target
  environment, so a dependency that only applies on a given OS or Python version
  is treated correctly.
- **`internal/pypi`** fetches metadata from a JSON package index. It is a
  `Source` interface with two methods, `Versions` and `Release`, behind an HTTP
  client with connection pooling and retry with backoff. The interface is the
  seam that keeps the resolver testable and the cache invisible to it:
  `MemSource` is an in-memory implementation used by the offline tests,
  `Snapshot` is a frozen capture of the real index that pins the regression
  tests, `CachedSource` wraps any of them with the on-disk cache, and the real
  client talks to pypi.org or any configured mirror.
- **`internal/resolve`** is the solver. It is a PubGrub-style, conflict-driven
  resolver over a finite version space, deterministic and able to explain a
  failure. It asks the `Source` for versions and release metadata as it explores.
- **`internal/lockfile`** turns a resolution into `gpt.lock` and renders the
  dependency tree for `explain`. The lockfile is a pure function of the
  resolution, with every list sorted, so it is byte-identical across machines
  and operating systems.
- **`cmd`** is the cobra command line: `resolve`, `lock`, `explain`, and
  `install`. Shared logic (gathering requirements, detecting the target Python,
  choosing the index URL) lives in `cmd/common.go`.

## The Source seam

The resolver never knows whether its data comes from the network or from memory.
It depends only on the `Source` interface:

```go
type Source interface {
    Versions(ctx, name) ([]*version.Version, error)
    Release(ctx, name, v) (*ReleaseInfo, error)
}
```

This is what lets the correctness tests run offline and deterministically, over
thousands of synthetic graphs, while the same resolver runs against the live
index in production. It also means a private index or a mirror is just a
different base URL, not a different code path.

The seam is also where the cache lives, which is why the resolver contains no
caching logic at all.

## How a resolve fetches

A resolve's wall-clock cost is almost entirely round-trips to the index. The
solver is not the slow part: resolving five real projects against a frozen copy
of the index takes about thirty milliseconds in total. So the fetch strategy is
where the time is won or lost, and two things shape it.

**Requests that do not depend on each other are made together.** When a release
is chosen, everything it depends on is needed no matter what the resolver
decides next, and those dependencies do not depend on one another. Asking for
them one at a time turns the width of a dependency layer into latency. They are
fetched in parallel instead, up to sixteen at once, along with the release
metadata of the version each constraint is most likely to select. The same
applies to the root requirements, which are all known before the resolve starts.

Fetching in parallel must not make the answer depend on which response arrives
first. Nothing is decided while requests are in flight: results are collected
and then applied in a fixed order, so the resolver walks through exactly the
state it would have built fetching one at a time. The test suite pins this by
resolving the benchmark projects to committed lockfiles that must match byte for
byte.

**Nothing is asked for twice.** A package document carries both the version list
and the full metadata of the package's latest release, which is the version the
resolver goes on to select for most packages. Keeping that metadata rather than
re-requesting it removes close to half of a resolve's requests: what remains is
one request per resolved package.

## The metadata cache

A resolve asks the index the same two questions over and over: which versions of
this package exist, and what does this exact release require. `CachedSource`
wraps any `Source` and answers them from disk when it already knows.

The two questions have very different lifetimes, and the cache treats them
differently rather than picking one compromise:

- A **version list** changes whenever anyone publishes, so it is held for ten
  minutes. A resolve that silently ignored a release published this morning
  would be answering yesterday's question.
- **One release's metadata** is fixed at publication, and the only part of it
  that can change afterwards is whether it has been yanked. It is held for a
  week.

Entries are kept per index, keyed by the index URL, so a private index and the
public one can never answer for each other. Each entry is a small JSON file
written atomically, through a temporary file renamed into place, so a reader
never sees a half-written entry and several resolves can share one cache. An
entry that cannot be read or parsed is discarded and re-fetched: a cache damaged
by a crash or a full disk costs time, never correctness.

Three flags cover the cases where the default is not what you want. `--refresh`
ignores what is stored and fetches again. `--offline` serves only what is stored
and refuses to reach the network, so a resolve that claims to be offline really
was. `--no-cache` leaves the cache out entirely. The `gopip cache` command shows
where the cache is, what it holds, and clears it.

Nothing in the cache can change which versions are chosen. It only decides
whether the answer required the network, which is why the regression suite
resolves the same projects with and without it and requires identical lockfiles.

## Install delegates to pip

`gopip install` resolves to exact versions and then runs
`<python> -m pip install <pinned>`, so packages install exactly as pip would.
Anything after a bare `--` is passed straight through to pip, and pip's own
settings, including `PIP_INDEX_URL`, still apply at install time. gopip never
becomes the thing that puts files on disk; it only decides what those files
should be.

## Determinism

The result of a resolve depends only on the requirements and the target
environment, never on the host operating system or the order metadata happened
to arrive. Decisions prefer the package with the fewest remaining candidates,
break ties by name, and always take the highest allowed version. The lockfile
then sorts every list. The upshot is that the same inputs produce the same
versions and a byte-identical `gpt.lock` anywhere, which is what makes the lock
worth committing. This is checked directly: the validation suite resolves a
thousand random graphs twice each and confirms the results never differ.
