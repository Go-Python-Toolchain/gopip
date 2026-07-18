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
  seam that keeps the resolver testable: `MemSource` is an in-memory
  implementation used by the offline tests, and the real client talks to
  pypi.org or any configured mirror.
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

Today the resolver calls `Versions` and `Release` one package at a time as it
explores, caching within a single run but keeping nothing on disk between runs.
That keeps the core simple, and it is the main thing standing between gopip and
the speed of a tool like uv on a cold cache. The index client already has a
concurrent `FetchReleases`; wiring it into the resolver's exploration is the top
performance item on the [roadmap](roadmap.md).

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
