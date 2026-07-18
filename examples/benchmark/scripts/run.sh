#!/usr/bin/env bash
# One command to reproduce the whole gopip benchmark: capture the machine,
# measure the offline resolver throughput, resolve every project with each tool
# cold and warm, and write results.md. Run setup.sh first to build gopip and
# install the competitors.
#
#   scripts/setup.sh     # once: build gopip, install uv and pip-tools
#   scripts/run.sh       # measure and report

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$here/config.sh"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 not found; the measurement wrapper needs it." >&2
  exit 1
fi
if [ -z "$GOPIP_BIN" ] || [ ! -x "$GOPIP_BIN" ]; then
  echo "gopip binary not found; run setup.sh or build it: (cd $REPO_ROOT && go build -o gopip .)" >&2
  exit 1
fi

bash "$here/machine.sh"
bash "$here/algorithm_bench.sh"
bash "$here/resolve_bench.sh"
python3 "$here/aggregate.py"

echo ""
echo "Done. Raw logs and results.md are in: $RAW_DIR"
