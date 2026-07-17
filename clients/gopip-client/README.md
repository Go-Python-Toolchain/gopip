# gopip-client

A small installer and launcher for [gopip](https://github.com/Go-Python-Toolchain/gopip), a fast, deterministic dependency resolver for Python written in Go.

Installing this package gives you a `gopip` command. The first time you run it, it downloads the native binary that matches your platform from the project's GitHub releases, verifies its checksum, and caches it. Every later run reuses the cached binary, so there is no per-run overhead.

## Install

```
pip install gopip-client
```

## Use

```
gopip resolve requests flask>=2.0
gopip lock -r requirements.txt
gopip explain requests
gopip install -r requirements.txt
```

`gopip` computes a consistent set of versions and writes a deterministic
lockfile, then hands installation to pip. See the [main project](https://github.com/Go-Python-Toolchain/gopip) for full documentation.

## Supported platforms

Linux and macOS on x86_64 and arm64, and Windows on x86_64.

## License

Apache License 2.0.
