# gopip Limitations

gopip does one thing: it resolves a project's dependencies to a consistent,
deterministic set and writes a lockfile, leaving installation to pip. This
document is an honest account of what it does not do yet, and the trade-offs of
that focus. Several of these are tracked as future work in the
[roadmap](roadmap.md), and the [benchmarks](benchmarks.md) show where they
matter.

## A cold resolve fetches one package at a time

gopip's solver is fast: it resolves a thousand synthetic graphs in well under a
second with no network involved, and it resolves five real projects against a
frozen copy of the index in about thirty milliseconds. Wall-clock time on a real
resolve is almost entirely time spent waiting on the package index.

A warm resolve no longer pays that, because gopip now keeps a metadata cache
between runs and a second resolve of the same project does no network work at
all. A cold resolve still does, and it still asks for one package at a time
rather than fetching in parallel, so it is the slower half of the comparison
against a tool like uv. The request count is already minimal, one version list
and one release per resolved package; what remains is that those requests run in
sequence. Fixing that is the top item on the [roadmap](roadmap.md), and the
[benchmarks](benchmarks.md) show where it currently stands.

## No hashes in the lockfile

`gpt.lock` records names, versions, and the dependency graph, but not artifact
hashes. It pins what to install, not a cryptographic fingerprint of each wheel.
That is enough for reproducible version selection, but it does not give you
hash-verified, tamper-evident installs the way a fully hashed lock would.
Recording hashes is a planned addition that would add fields without changing
the meaning of the existing ones.

## Resolution scope

gopip resolves the dependency graph from the requirements you give it, evaluated
against a target environment. Some things are out of scope today:

- **Extras** are parsed in requirement strings, but the resolver does not yet
  expand a package's optional extras into their own dependencies.
- **Non-index requirements**, such as editable installs, direct URLs, version
  control references, and local paths, are not resolved. gopip works from the
  package index.
- **Build-time dependencies** are not resolved. gopip reasons about runtime
  requirements as published in release metadata, not the requirements needed to
  build a package from source.

For any of these, the usual pip workflow still applies; gopip simply does not
take them over.

## Metadata comes from the index

gopip reads release metadata from a JSON package index. A package whose runtime
dependencies are not exposed in that metadata, for example an older
source-only distribution whose requirements are only discoverable by building
it, cannot be resolved from metadata alone. Modern wheels publish their
requirements, so this is rare in practice, but it is a real edge.

## One target environment per resolve

A resolve is for a single target environment: one Python version and one set of
markers, detected from your interpreter or set with `--python`. It produces a
lock for that target. It does not yet produce a single lock that is valid across
many platforms and Python versions at once; for a different target you run a
different resolve.

## Installation is pip's job

By design gopip does not install anything. It hands the pinned set to pip. So
anything about installation itself, wheel building, install-time environment
markers as pip evaluates them, or install ordering, is pip's behavior, not
gopip's. This is a deliberate boundary, not a gap, but it is worth stating: gopip
decides versions, pip puts files on disk.

## Platform coverage of this build

gopip is pure Go with no platform-specific resolution logic, and its output is
byte-identical across operating systems by construction. It is developed and
exercised primarily on Linux; the released binaries cover Linux, macOS, and
Windows, and the determinism guarantee is what makes cross-platform results
trustworthy.
