#!/usr/bin/env bash
# Measure gopip's resolver throughput with no network involved, so the number
# reflects the solver itself rather than the speed of the package index. This
# runs the resolver over one thousand randomly generated, satisfiable dependency
# graphs (the same check the validation doc describes) and records how long it
# takes. Requires the Go toolchain and the gopip source; skipped otherwise.

source "$(dirname "${BASH_SOURCE[0]}")/config.sh"

out="$RAW_DIR/algorithm.txt"
: >"$out"

if ! command -v go >/dev/null 2>&1; then
  echo "go not found; skipping the offline resolver throughput measurement" | tee "$out" >&2
  exit 0
fi

echo "resolving 1000 random satisfiable graphs (offline, solver only)" >&2
( cd "$REPO_ROOT" && go test ./internal/resolve -run TestResolveManyRandomGraphs -v 2>&1 ) \
  | grep -iE "resolved [0-9]+ random|PASS: TestResolveManyRandomGraphs" | tee "$out"
