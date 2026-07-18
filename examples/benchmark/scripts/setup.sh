#!/usr/bin/env bash
# Prepare the benchmark workspace: build gopip and install the competitor
# resolvers (uv and pip-tools) locally under the work directory, which is git
# ignored. Nothing here touches the global environment.
#
# Re-running is safe: existing pieces are left in place.

source "$(dirname "${BASH_SOURCE[0]}")/config.sh"

# --- gopip binary ---------------------------------------------------------
if [ -z "$GOPIP_BIN" ] || [ ! -x "$GOPIP_BIN" ]; then
  if command -v go >/dev/null 2>&1; then
    echo "building gopip"
    ( cd "$REPO_ROOT" && go build -o gopip . )
    GOPIP_BIN="$REPO_ROOT/gopip"
  else
    echo "gopip binary not found and Go is not installed; cannot continue" >&2
    exit 1
  fi
fi
echo "gopip: $("$GOPIP_BIN" version 2>/dev/null)"

# --- uv (standalone binary, pinned) --------------------------------------
if [ -x "$TOOLS_DIR/uv-bin/uv" ]; then
  echo "uv: already installed ($("$TOOLS_DIR/uv-bin/uv" --version))"
elif command -v curl >/dev/null 2>&1; then
  echo "uv: installing $UV_VERSION"
  curl -LsSf "https://astral.sh/uv/$UV_VERSION/install.sh" 2>/dev/null \
    | env UV_INSTALL_DIR="$TOOLS_DIR/uv-bin" INSTALLER_NO_MODIFY_PATH=1 sh >/dev/null 2>&1 || \
  curl -LsSf "https://astral.sh/uv/install.sh" 2>/dev/null \
    | env UV_INSTALL_DIR="$TOOLS_DIR/uv-bin" INSTALLER_NO_MODIFY_PATH=1 sh >/dev/null 2>&1 || \
    echo "uv: install failed; the uv column will be skipped" >&2
else
  echo "uv: curl not found; skipping (uv column will be skipped)" >&2
fi

# --- pip-tools (via venv, pinned) ----------------------------------------
if [ -x "$TOOLS_DIR/pip-venv/bin/pip-compile" ]; then
  echo "pip-tools: already installed ($("$TOOLS_DIR/pip-venv/bin/pip-compile" --version 2>&1))"
elif command -v python3 >/dev/null 2>&1; then
  echo "pip-tools: installing $PIPTOOLS_VERSION"
  python3 -m venv "$TOOLS_DIR/pip-venv"
  # pip-tools 7.4.1 uses a PackageFinder attribute that pip 25 removed, so pin a
  # pip it is compatible with.
  "$TOOLS_DIR/pip-venv/bin/pip" install --quiet "pip==24.2"
  "$TOOLS_DIR/pip-venv/bin/pip" install --quiet "pip-tools==$PIPTOOLS_VERSION"
else
  echo "pip-tools: python3 not found; skipping (pip-tools column will be skipped)" >&2
fi

echo ""
echo "Setup complete."
echo "  gopip:     $GOPIP_BIN"
echo "  uv:        $TOOLS_DIR/uv-bin/uv"
echo "  pip-tools: $TOOLS_DIR/pip-venv/bin/pip-compile"
