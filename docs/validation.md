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
