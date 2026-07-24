#!/usr/bin/env bash
# Resolve every project requirement set with each tool, and time it. gopip,
# uv, and pip-tools all do the same job here: read a requirements file and
# compute a pinned set. None of them installs anything; this measures resolution
# only.
#
# Two phases are measured:
#   cold  the first resolve with an empty cache (the realistic first run)
#   warm  a repeat resolve with the cache populated
# All three tools keep a metadata cache, and each tool's cache is pointed at a
# directory under CACHE_DIR so a cold run really is cold. gopip reads its cache
# location from the environment, which is why its command runs with
# XDG_CACHE_HOME set: without that it would use the developer's own cache and
# every cold measurement here would silently be a warm one.
#
# Absolute numbers depend heavily on the network to the package index. The
# harness runs each measurement several times and reports the median, and
# records every sample so a reader can see the spread.

source "$(dirname "${BASH_SOURCE[0]}")/config.sh"
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

csv="$RAW_DIR/resolve.csv"
counts="$RAW_DIR/counts.csv"
: >"$csv"; echo "label,wall_seconds,max_rss_kb,exit_code" >"$csv"
: >"$counts"; echo "tool,project,packages" >"$counts"
mkdir -p "$RAW_DIR/resolved"

runs="$RESOLVE_RUNS"
cold_runs="$COLD_RUNS"

# --- per-tool resolve commands (resolution only, output to stdout) --------
# Each takes a requirements file path. Caches live under CACHE_DIR so cold runs
# can clear them.

gopip_cmd()      { XDG_CACHE_HOME="$CACHE_DIR/gopip" "$GOPIP_BIN" resolve -r "$1"; }
uv_cmd()         { "$UV_BIN" pip compile --cache-dir "$CACHE_DIR/uv" --quiet "$1"; }
pipcompile_cmd() { PIP_CACHE_DIR="$CACHE_DIR/pip" "$PIPCOMPILE_BIN" --quiet --no-header --output-file - "$1"; }

clear_cache() {
  case "$1" in
    uv)         rm -rf "$CACHE_DIR/uv" ;;
    pip-compile) rm -rf "$CACHE_DIR/pip" ;;
    gopip)      rm -rf "$CACHE_DIR/gopip" ;;
  esac
}

have_tool() {
  case "$1" in
    gopip)       [ -n "$GOPIP_BIN" ] && [ -x "$GOPIP_BIN" ] ;;
    uv)          [ -x "$UV_BIN" ] ;;
    pip-compile) [ -x "$PIPCOMPILE_BIN" ] ;;
  esac
}

# count_packages TOOL FILE -> capture the resolved set once for auditing and
# count the pinned packages (lines containing ==).
capture_and_count() {
  local tool="$1" project="$2" req="$3"
  local outfile="$RAW_DIR/resolved/$tool-$project.txt"
  case "$tool" in
    gopip)       gopip_cmd "$req" >"$outfile" 2>/dev/null || true ;;
    uv)          uv_cmd "$req" 2>/dev/null | grep -vE '^\s*#|^\s*$' >"$outfile" || true ;;
    pip-compile) pipcompile_cmd "$req" 2>/dev/null | grep -vE '^\s*#|^\s*$' >"$outfile" || true ;;
  esac
  local n
  n="$(grep -c '==' "$outfile" 2>/dev/null || echo 0)"
  printf '%s,%s,%s\n' "$tool" "$project" "$n" >>"$counts"
}

TIMEOUT="${GOPIP_BENCH_TIMEOUT:-180}"
cap() { command -v timeout >/dev/null 2>&1 && echo "timeout $TIMEOUT"; }

for project in "${PROJECTS[@]}"; do
  req="$PROJECTS_DIR/$project.txt"
  [ -f "$req" ] || { echo "skip $project: no requirements file" >&2; continue; }

  for tool in gopip uv pip-compile; do
    have_tool "$tool" || { echo "skip $tool: not installed" >&2; continue; }
    section "$tool resolve: $project"

    # Verify it produces a set, and record the package count.
    capture_and_count "$tool" "$project" "$req"

    # Cold: clear the tool's cache before each sample, so every one of them is
    # a genuine first run. A cold resolve is dominated by round-trips to the
    # index, which vary far more than local work does, so it is sampled several
    # times rather than once and reported as the median.
    for _ in $(seq 1 "$cold_runs"); do
      clear_cache "$tool"
      case "$tool" in
        gopip)       time_run "$csv" "cold,$tool,$project" -- $(cap) env XDG_CACHE_HOME="$CACHE_DIR/gopip" "$GOPIP_BIN" resolve -r "$req" ;;
        uv)          time_run "$csv" "cold,$tool,$project" -- $(cap) "$UV_BIN" pip compile --cache-dir "$CACHE_DIR/uv" --quiet "$req" ;;
        pip-compile) time_run "$csv" "cold,$tool,$project" -- $(cap) env PIP_CACHE_DIR="$CACHE_DIR/pip" "$PIPCOMPILE_BIN" --quiet --no-header --output-file - "$req" ;;
      esac
    done

    # Warm: repeat with the cache populated.
    for _ in $(seq 1 "$runs"); do
      case "$tool" in
        gopip)       time_run "$csv" "warm,$tool,$project" -- $(cap) env XDG_CACHE_HOME="$CACHE_DIR/gopip" "$GOPIP_BIN" resolve -r "$req" ;;
        uv)          time_run "$csv" "warm,$tool,$project" -- $(cap) "$UV_BIN" pip compile --cache-dir "$CACHE_DIR/uv" --quiet "$req" ;;
        pip-compile) time_run "$csv" "warm,$tool,$project" -- $(cap) env PIP_CACHE_DIR="$CACHE_DIR/pip" "$PIPCOMPILE_BIN" --quiet --no-header --output-file - "$req" ;;
      esac
    done
  done
done

echo "" >&2
echo "resolve samples in $csv, package counts in $counts, resolved sets in $RAW_DIR/resolved/" >&2
