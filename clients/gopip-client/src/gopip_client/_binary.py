"""Platform detection, download, and caching of the native gopip binary."""

import hashlib
import io
import os
import platform
import stat
import tarfile
import urllib.request
import zipfile
from pathlib import Path

OWNER = "Go-Python-Toolchain"
REPO = "gopip"


def resolve_version() -> str:
    """Return the installed package version, which selects the release to fetch."""
    try:
        from importlib.metadata import version

        return version("gopip-client")
    except Exception:
        return "0.1.0"


def _platform_tags() -> tuple[str, str]:
    """Return the (os, arch) tags used in release archive names."""
    system = platform.system().lower()
    machine = platform.machine().lower()

    if system == "linux":
        goos = "linux"
    elif system == "darwin":
        goos = "darwin"
    elif system in ("windows", "win32"):
        goos = "windows"
    else:
        raise RuntimeError(f"unsupported operating system: {platform.system()}")

    if machine in ("x86_64", "amd64"):
        goarch = "amd64"
    elif machine in ("arm64", "aarch64"):
        goarch = "arm64"
    else:
        raise RuntimeError(f"unsupported architecture: {platform.machine()}")

    if goos == "windows" and goarch != "amd64":
        raise RuntimeError("only amd64 builds are published for Windows")

    return goos, goarch


def _binary_name() -> str:
    return "gopip.exe" if os.name == "nt" else "gopip"


def _cache_dir(version: str) -> Path:
    if os.name == "nt":
        base = os.environ.get("LOCALAPPDATA") or os.path.expanduser("~\\AppData\\Local")
    else:
        base = os.environ.get("XDG_CACHE_HOME") or os.path.expanduser("~/.cache")
    return Path(base) / "gopip-client" / version


def _download(url: str) -> bytes:
    with urllib.request.urlopen(url) as response:  # noqa: S310 - fixed release host
        return response.read()


def _expected_checksum(checksums: str, archive_name: str) -> str | None:
    for line in checksums.splitlines():
        parts = line.split()
        if len(parts) == 2 and parts[1] == archive_name:
            return parts[0]
    return None


def _extract_binary(archive_name: str, data: bytes, binary_name: str) -> bytes:
    if archive_name.endswith(".zip"):
        with zipfile.ZipFile(io.BytesIO(data)) as zf:
            return zf.read(binary_name)
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as tf:
        member = tf.extractfile(binary_name)
        if member is None:
            raise RuntimeError(f"{binary_name} not found in {archive_name}")
        return member.read()


def ensure_binary() -> Path:
    """Return the path to the cached native binary, downloading it if needed."""
    version = resolve_version()
    binary_name = _binary_name()
    target = _cache_dir(version) / binary_name
    if target.exists() and os.access(target, os.X_OK):
        return target

    goos, goarch = _platform_tags()
    suffix = "zip" if goos == "windows" else "tar.gz"
    archive_name = f"{REPO}_{version}_{goos}_{goarch}.{suffix}"
    base_url = f"https://github.com/{OWNER}/{REPO}/releases/download/v{version}/"

    archive = _download(base_url + archive_name)

    checksums = _download(base_url + "checksums.txt").decode("utf-8")
    expected = _expected_checksum(checksums, archive_name)
    if expected is None:
        raise RuntimeError(f"no checksum published for {archive_name}")
    actual = hashlib.sha256(archive).hexdigest()
    if actual != expected:
        raise RuntimeError(
            f"checksum mismatch for {archive_name}: expected {expected}, got {actual}"
        )

    binary = _extract_binary(archive_name, archive, binary_name)

    target.parent.mkdir(parents=True, exist_ok=True)
    tmp = target.with_suffix(target.suffix + ".partial")
    tmp.write_bytes(binary)
    mode = tmp.stat().st_mode
    tmp.chmod(mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    tmp.replace(target)
    return target
