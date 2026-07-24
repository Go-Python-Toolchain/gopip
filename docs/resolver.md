# The gopip resolver

gopip chooses a consistent set of package versions for a set of requirements. This document explains the algorithm and the main design choices.

## Why a PubGrub-style resolver

The classic way to frame dependency resolution is boolean satisfiability: turn every package version into a variable and hand the formula to a SAT solver. That works, but it is an awkward fit. Version ranges have to be expanded into large boolean encodings, and when resolution fails the solver cannot say why in terms a developer understands.

gopip uses the PubGrub approach instead. PubGrub is a conflict-driven solver designed specifically for version resolution. It reasons over incompatibilities, which are small sets of package terms that cannot all be true at once, propagates their consequences, and when it hits a conflict it derives a new incompatibility and jumps back to the decision that caused the trouble. It is deterministic, it avoids re-exploring the same dead ends, and the incompatibilities it learns double as a human-readable explanation of any failure. This is the same family of algorithm used by Dart's package manager.

## The finite version space

A term constrains one package to a set of versions. Rather than manipulate symbolic version ranges, gopip works with the finite set of versions the index actually offers for a package. A constraint is then just a set of version strings, and the constraint algebra becomes plain set intersection, union, and complement. This keeps the core simple and fast.

One subtlety follows from using a finite universe. A dependency on any version of a package would have an empty complement, which would fail to force that package into the solution. To match PubGrub's unbounded version space, every package's universe carries a sentinel value standing for "not selected". The complement of an any-version dependency then still contains that sentinel, so the dependency correctly forces the package to be present. A package is only decided once the sentinel has been ruled out of its allowed set.

## The loop

1. Seed the solution by deciding a synthetic root package whose dependencies are the top-level requirements.
2. Unit propagation: for each incompatibility touching the package that just changed, check its relation to the partial solution. If every term is satisfied, that is a conflict. If exactly one term is undecided and the rest are satisfied, derive the negation of that term. Cascade to any package that changed.
3. Conflict resolution: walk back through the assignments that caused the conflict, merging the conflicting incompatibility with the causes of its most recent satisfiers until the result points at a single earlier decision. Backjump to just before that decision and record the learned incompatibility. A conflict that traces back to the root means the requirements are unsatisfiable.
4. Decision: among the packages that are required but not yet decided, pick one and choose the highest version that fits its allowed set and supports the target Python. Register that version's dependencies as new incompatibilities.
5. Repeat until every required package has a decision.

## Determinism and filtering

Decisions prefer the package with the fewest remaining candidates, breaking ties by name, and always take the highest allowed version. Given the same inputs the resolver produces the same result, which is what makes the lockfile reproducible.

Dependency markers are evaluated against the target environment, so a dependency that only applies on a particular operating system or Python version is included only when it should be. A release whose requires-python does not admit the target interpreter is skipped during version selection.

## Which versions are eligible

A version has to clear three things before it can be selected.

Its requires-python must admit the target interpreter, as above.

It must not have been yanked, unless a requirement named it exactly. Yanking is a maintainer saying "do not pick this", so gopip does not pick it. A pin is a different statement: it names one release deliberately, and a project pinned to a release that was later yanked still has to be lockable, so an exact `==` keeps the release eligible. This is the same rule pip follows.

Its metadata must be readable. If the index lists a version but has no metadata for it, the release is not really there and the resolver moves down to the next one. Anything else, a connection that drops or an index that errors, stops the resolve. The distinction matters more than it looks: not knowing whether a version is usable is not the same as knowing it is not, and treating a network failure as a reason to skip would quietly produce a lockfile that pins an older version and looks like a deliberate choice.

## Cancellation

Every lookup a resolve makes takes the caller's context, including the ones inside version selection, so cancelling a resolve or setting a deadline stops it promptly rather than at the next convenient point.
