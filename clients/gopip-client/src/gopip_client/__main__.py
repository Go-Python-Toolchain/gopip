"""Entry point that launches the native gopip binary with the given arguments."""

import os
import subprocess
import sys

from ._binary import ensure_binary


def main() -> int:
    try:
        binary = ensure_binary()
    except Exception as exc:  # noqa: BLE001 - report any failure to the user
        print(f"gopip-client: could not obtain the gopip binary: {exc}", file=sys.stderr)
        return 1

    args = [str(binary), *sys.argv[1:]]
    if os.name == "nt":
        return subprocess.run(args).returncode
    os.execv(str(binary), args)
    return 0  # not reached on success


if __name__ == "__main__":
    raise SystemExit(main())
