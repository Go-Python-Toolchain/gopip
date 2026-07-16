# gopip

A fast, deterministic dependency resolver for Python, written in Go.

gopip computes a complete, consistent set of package versions for your project
using a pure Go solver, and writes a deterministic lockfile. It does not replace
pip. It takes over the slow part, resolution, and leaves installation to pip, so
it drops into your existing workflow without changes.

gopip is part of the [Go-Python Toolchain](https://github.com/Go-Python-Toolchain).

## Status

Early development. The command line skeleton and build pipeline are in place. The
version model, requirement parsing, PyPI fetcher, solver, and lockfile are being
built in order.

## Install

While pre-release, build from source:

```
git clone https://github.com/Go-Python-Toolchain/gopip
cd gopip
go build -o gopip .
./gopip version
```

Requires Go 1.22 or newer.

## Design

- A pure Go solver with conflict-driven clause learning.
- Concurrent metadata fetching from the Python Package Index.
- A deterministic lockfile that is identical across machines and operating systems.
- Installation delegated to pip, so resolved packages install exactly as expected.

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
