# Shared configuration for the gopip benchmark harness.
#
# Every script sources this file. It lists the project requirement sets to
# resolve and the competitor tools, and defines the directory layout. Machine
# details are captured at run time by machine.sh.

set -euo pipefail

BENCH_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_ROOT="$(cd "$BENCH_ROOT/../.." && pwd)"
PROJECTS_DIR="$BENCH_ROOT/projects"

# Heavy, uncommitted artifacts: installed competitors, tool caches, raw logs.
WORK_DIR="${GOPIP_BENCH_WORK:-$BENCH_ROOT/work}"
TOOLS_DIR="$WORK_DIR/tools"
RAW_DIR="${GOPIP_BENCH_RAW:-$WORK_DIR/raw}"
CACHE_DIR="$WORK_DIR/caches"

# The project requirement sets, real dependency lists of the kind a project
# commits. Each name maps to projects/<name>.txt.
PROJECTS=(
  "cli-tool"
  "web-api"
  "flask-app"
  "django-stack"
  "data-science"
)

# Repetitions per (tool, project, phase). The reported figure is the median.
RESOLVE_RUNS="${GOPIP_BENCH_RUNS:-5}"

# Repetitions of the cold phase. Fewer than the warm phase, because every cold
# sample re-fetches everything from the package index, but more than one:
# a cold resolve is network bound and a single sample of it is not a measurement.
COLD_RUNS="${GOPIP_BENCH_COLD_RUNS:-3}"

# Pinned competitors. uv and pip-tools both compile a requirements file to a
# pinned set, which is the same job gopip's resolve does, so they are the fair
# peers. pip-tools drives pip's own resolver, so it stands in for pip.
UV_VERSION="0.11.29"
PIPTOOLS_VERSION="7.4.1"

# Tool entry points.
find_gopip() {
  if command -v gopip >/dev/null 2>&1 && [ -z "${GOPIP_PREFER_LOCAL:-}" ]; then
    command -v gopip; return
  fi
  for c in "$REPO_ROOT/gopip" "$REPO_ROOT/gopip.exe"; do
    [ -x "$c" ] && { echo "$c"; return; }
  done
  command -v gopip 2>/dev/null || echo ""
}
GOPIP_BIN="${GOPIP_BIN:-$(find_gopip)}"
UV_BIN="${UV_BIN:-$TOOLS_DIR/uv-bin/uv}"
PIPCOMPILE_BIN="${PIPCOMPILE_BIN:-$TOOLS_DIR/pip-venv/bin/pip-compile}"

mkdir -p "$WORK_DIR" "$TOOLS_DIR" "$RAW_DIR" "$CACHE_DIR"
