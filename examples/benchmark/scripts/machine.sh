#!/usr/bin/env bash
# Capture the machine identity and tool versions for a benchmark run. Resolve
# times depend on the machine and, heavily, on the network, so every published
# number should carry this context. Writes machine.txt into the raw directory.

source "$(dirname "${BASH_SOURCE[0]}")/config.sh"

cpu_model() {
  if [ -r /proc/cpuinfo ]; then grep -m1 'model name' /proc/cpuinfo | sed 's/.*: //'
  elif command -v sysctl >/dev/null 2>&1; then sysctl -n machdep.cpu.brand_string 2>/dev/null
  else echo "unknown"; fi
}
cpu_cores() {
  if command -v nproc >/dev/null 2>&1; then nproc
  elif command -v sysctl >/dev/null 2>&1; then sysctl -n hw.ncpu 2>/dev/null
  else echo "unknown"; fi
}

out="$RAW_DIR/machine.txt"
{
  echo "CPU:       $(cpu_model)"
  echo "Cores:     $(cpu_cores)"
  echo "OS/arch:   $(uname -s -m)"
  echo "Kernel:    $(uname -r)"
  echo "gopip:     $("$GOPIP_BIN" version 2>/dev/null || echo 'n/a')"
  echo "uv:        $("$UV_BIN" --version 2>/dev/null | awk '{print $2}' || echo 'n/a')"
  echo "pip-tools: ${PIPTOOLS_VERSION}"
} | tee "$out"
