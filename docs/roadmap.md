# gopip Roadmap

This is where gopip is headed. It is a direction, not a dated release plan.
Items are grouped by the problem they solve, and each is written so its "done"
is obvious. The current constraints these address are in
[limitations](limitations.md), and the [benchmarks](benchmarks.md) are the
scoreboard for the performance items.

## Faster resolves

This was the priority, and it is **done**. It had three parts.

A persistent on-disk metadata cache. gopip keeps what it reads from an index
under the usual per-platform cache directory, holding version lists briefly and
individual release metadata for a week, so a warm resolve of the benchmark
projects does no network work at all and finishes in about ten milliseconds.
`--refresh`, `--offline`, and `--no-cache` cover the cases where that is not
what you want, and `gopip cache` inspects and clears it.

Parallel fetching. Requests that do not depend on each other, the dependencies
of a chosen release and the root requirements, are made together rather than one
at a time.

Not asking twice. A package document already carries the metadata of the
package's latest release, which is the version most packages resolve to, so that
is kept instead of re-requested. A cold resolve of the benchmark projects now
makes one request per resolved package, about half what it used to, and runs two
to six times faster.

What is left here is smaller and less certain to pay off: the remaining
serialization is the depth of the dependency graph, since a package's
dependencies are only known once its metadata arrives. Looking ahead
speculatively would overlap more, at the cost of fetching metadata that may
never be needed. That trade is worth measuring before it is worth building.

## Hashes in the lockfile

**Done.** `gpt.lock` records the digests of every artifact published for each
pinned version, and `gopip install --require-hashes` verifies each download
against them.

## Fuller resolution scope

Expanding extras is **done**. `flask[async]` resolves to flask plus what its
async extra requires, the selected extras are recorded in `gpt.lock`, and
install hands them to pip.

- Resolve direct URL, version control, and local path requirements, not just
  index packages.

Done when a requirements file using a direct reference resolves to a complete,
correct lock.

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

**Done.** A failing resolve is reported by walking the derivation back to the
requirements that were actually declared, so the explanation names every
constraint involved in the conflict, including the ones inside a package's
metadata that the reader never wrote.

## Private and mirrored indexes

- First-class support for authenticated private indexes and mirrors, including
  credentials handling, so resolution works against a corporate index the same
  way installation does.

Done when gopip resolves against an authenticated private index with no manual
workarounds.
