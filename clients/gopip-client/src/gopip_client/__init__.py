"""gopip-client installs and launches the native gopip binary.

gopip is a fast, deterministic dependency resolver for Python, written in Go.
This package is a thin launcher: on first use it downloads the binary that
matches your platform from the project's GitHub releases, caches it, and runs
it. Every later call reuses the cached binary.
"""

from ._binary import ensure_binary, resolve_version

__all__ = ["ensure_binary", "resolve_version"]
