# gopip validation

This document records the correctness validation for the resolver.

## Random graphs

The resolver is checked against one thousand randomly generated dependency
graphs. Each graph is satisfiable by construction: every package has a baseline
release whose dependencies are all satisfiable, so a valid solution always
exists. Higher releases carry a mix of satisfiable and impossible constraints,
which forces the resolver to explore and back off from versions it cannot use.

For every graph the resolver must produce a solution, and that solution is then
checked independently: every direct requirement and every dependency of every
resolved package must be present at a version that meets its constraint.

Result, over 1000 graphs:

- 0 contradictions. Every satisfiable graph resolved to a valid solution.
- 0 non-deterministic resolutions. Resolving the same graph twice always gave
  the same result, which is what makes the lockfile reproducible.
- 979 of 1000 solutions used at least one release above the baseline, so the
  resolver was doing real version selection and backtracking rather than always
  falling back to the lowest versions.
- Average of 7.3 packages per solution.
- Total time about 0.7 seconds for all 1000 graphs.

Unsatisfiable graphs are covered too: a graph whose requirements genuinely
conflict is reported as unsatisfiable rather than resolved incorrectly.

The check lives in internal/resolve as TestResolveManyRandomGraphs and runs as
part of the normal test suite.

## Determinism across operating systems

The resolver's result depends only on the requirements and the target
environment, never on the host operating system. The lockfile is a pure function
of the resolution, with all lists sorted, so the same inputs produce a
byte-identical gpt.lock on any machine. The random-graph check above confirms
resolution determinism, and the lockfile package confirms that the serialized
lock contains no host-specific content.

## Frozen index snapshot

Resolution is also pinned against a frozen capture of the real index. The
snapshot in `internal/resolve/testdata/snapshot.json` records the full version
list of every package the five benchmark projects touch, 53 packages, along with
the release metadata of the 20 newest versions of each, 988 releases in total. It
is real published metadata, with all its irregularities, held still.

Two things follow from that. First, resolving the benchmark projects becomes
deterministic in a way a live resolve cannot be: the same inputs give the same
answer on any machine, on any day, with no network. The resulting lockfiles are
committed under `internal/resolve/testdata/reference/`, and the test suite
requires every resolve to reproduce them byte for byte. That is the gate work on
fetching has to pass, because caching, prefetching, and concurrency are allowed
to change how fast an answer arrives and never what the answer is. When a change
is meant to move a pin, the references are re-recorded in the same commit so the
diff states exactly which pins moved.

Second, it gives a clean reading of where resolve time actually goes. Resolving
all five projects against the snapshot, solver and all, takes about 0.03
seconds. The same five against the live index take between four and fourteen
seconds. The difference is entirely index traffic.

The capture is reproducible: `GOPIP_CAPTURE=1` re-records the snapshot from the
live index, and `GOPIP_RECORD_REFERENCE=1` re-records the reference locks.

## Eligibility and failure

The snapshot carries fifteen genuinely yanked releases, so the rules about which
versions may be selected are checked against real data rather than a
construction. pandas 3.0.4 is yanked and sits between two releases that are not,
which makes it the clean case: a constraint that would otherwise land on it
selects 3.0.3 instead, pinning it with `==` selects it, and an unconstrained
resolve is unaffected.

Failure handling is checked by injecting faults into the source. A release the
index does not have is skipped and the resolve continues one version down. A
transient failure reading a release stops the resolve with an error naming the
release, rather than skipping the version and pinning an older one, which would
look like a decision and would really be a network error. A cancelled context
aborts a resolve from inside version selection, not only from the version
listing before it.

## Artifact digests

`gpt.lock` records the digests of every artifact published for each pinned
version, so an install can check that what it downloaded is what was locked.
Three things are checked.

That the digests are real: a guarded network test compares what gopip records
against what pypi.org publishes for the same releases. That they survive the
route from the index into a lockfile: offline tests cover parsing them from both
document shapes the index uses, keeping them when the latest release's metadata
is reused rather than refetched, storing and reloading them from the cache, and
rendering them into the requirements file pip reads.

That verification actually happens: a real install of rich into a fresh virtual
environment with `--require-hashes` succeeds, and the same install with the
digests altered is refused by pip with a hash mismatch. The second half matters
more than the first, since an install that verifies nothing also succeeds.

One detail worth stating, because it surfaced while checking this: altering only
the wheel's digest does not fail the install. Every artifact of a release is
listed, including the source distribution, and pip installs whichever one
matches. That is the intended behavior, and it is what lets a single lock verify
on Linux, macOS, and Windows.

The frozen index snapshot deliberately does not carry artifact digests. It would
roughly triple the fixture, and digests are metadata attached to a version rather
than a resolution decision, so including them would add churn to the reference
locks without adding coverage of what those locks exist to protect.

## Failure explanations

A failing resolve is checked for what it says, not only that it failed. A
conflict between a direct requirement and a dependency two levels down must name
all three constraints, including the one inside the package's metadata that the
reader never wrote and could not guess. The explanation must not repeat itself,
since a derivation visits the same requirement many times, and must not leak the
internal name of the synthetic root package. A failure that comes down to a
single fact stays on one line.

## Live index

Resolution is also exercised against the live index for popular packages
(requests, flask, click, rich, httpx, pydantic). Each resolves to a complete set
that passes the same independent verification. This check is guarded by the
GOPIP_NETWORK_TESTS environment variable so the normal test run stays offline.

Two further checks cover the parts of fetching that could quietly change an
answer rather than only the speed of reaching it.

The resolver fetches in parallel, so resolution must not depend on which
response arrives first. The reference locks above are the guard: they are
produced by the same concurrent code path and must match byte for byte, and each
project is resolved three times over to catch any dependence on ordering.

The index client reuses the release metadata that arrives alongside a version
list instead of requesting it again. That is only sound if a package document
says the same thing about its latest release as that release's own document
does. A guarded network test checks exactly that against the live index, field by
field, for several popular packages.
