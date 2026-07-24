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

## Live index

Resolution is also exercised against the live index for popular packages
(requests, flask, click, rich, httpx, pydantic). Each resolves to a complete set
that passes the same independent verification. This check is guarded by the
GOPIP_NETWORK_TESTS environment variable so the normal test run stays offline.

Resolving from the live index is currently network bound: the resolver fetches
release metadata one request at a time as it explores candidates. A concurrent
prefetch path already exists in the index client and wiring it into candidate
exploration is a planned performance improvement. It is independent of the
correctness results above.
