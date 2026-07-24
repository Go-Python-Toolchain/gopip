# gopip Roadmap

This is where gopip is headed. It is a direction, not a dated release plan.
Items are grouped by the problem they solve, and each is written so its "done"
is obvious. The current constraints these address are in
[limitations](limitations.md), and the [benchmarks](benchmarks.md) are the
scoreboard for the performance items.

## Faster resolves: concurrent fetch

This is the priority. gopip's solver is fast; its wall-clock time on real
projects is spent waiting on the package index, fetched one request at a time.

- Wire the index client's existing concurrent `FetchReleases` into the
  resolver's candidate exploration, so metadata for packages the resolver will
  need is fetched in parallel rather than serially.

Done when a cold resolve of the benchmark projects lands in the same range as
the fastest peer tools.

The other half of this, a persistent on-disk metadata cache, is **done**. gopip
keeps what it reads from an index under the usual per-platform cache directory,
holding version lists briefly and individual release metadata for a week, so a
warm resolve of the benchmark projects does no network work and finishes in
about ten milliseconds. `--refresh`, `--offline`, and `--no-cache` cover the
cases where that is not what you want, and `gopip cache` inspects and clears it.

## Hashes in the lockfile

- Record artifact hashes in `gpt.lock` alongside each pinned version, and have
  `install` pass them to pip for hash-verified installs.

Done when a locked project can be installed with hash checking enabled and no
extra configuration.

## Fuller resolution scope

- Expand a package's requested extras into their dependencies during resolution.
- Resolve direct URL, version control, and local path requirements, not just
  index packages.

Done when a requirements file that uses extras and a direct reference resolves to
a complete, correct lock.

## Cross-environment locks

- Produce a single lock that is valid across several Python versions and
  platforms at once, rather than one lock per target.

Done when one `gpt.lock` can drive correct installs on multiple targets from a
single resolve.

## A sync command

- A command that brings an environment exactly into line with `gpt.lock`,
  installing what is missing and removing what should not be there, still by
  driving pip.

Done when one command makes an environment match the lock precisely.

## Better failure explanations

The PubGrub core already learns human-readable incompatibilities when resolution
fails.

- Surface them as a clear, minimal explanation of why a set of requirements
  cannot be satisfied, naming the conflicting constraints.

Done when an unsatisfiable input prints an explanation a developer can act on
without guessing.

## Private and mirrored indexes

- First-class support for authenticated private indexes and mirrors, including
  credentials handling, so resolution works against a corporate index the same
  way installation does.

Done when gopip resolves against an authenticated private index with no manual
workarounds.
