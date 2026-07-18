# Shared helpers for the gopip benchmark harness.

MEASURE_PY="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/measure.py"

# time_run OUT_CSV LABEL -- CMD...
# Runs CMD once under measure.py and appends a CSV row:
#   "label",wall_seconds,max_rss_kb,exit_code
time_run() {
  local out_csv="$1"; shift
  local label="$1"; shift
  [ "$1" = "--" ] && shift
  local line
  line="$(python3 "$MEASURE_PY" "$@" 2>/dev/null || echo "NA,NA,127")"
  printf '"%s",%s\n' "$label" "$line" >>"$out_csv"
}

section() { echo "" >&2; echo "== $* ==" >&2; }
