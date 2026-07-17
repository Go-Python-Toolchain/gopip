# gopip

A fast, deterministic dependency resolver for Python, written in Go.

gopip computes a complete, consistent set of package versions for your project
using a pure Go solver, and writes a deterministic lockfile. It does not replace
pip. It takes over the slow part, resolution, and leaves installation to pip, so
it drops into your existing workflow without changes.

gopip is part of the [Go-Python Toolchain](https://github.com/Go-Python-Toolchain).

## Status

Active development. The version model, requirement parsing, PyPI fetcher,
resolver, lockfile, and command line are in place and working. Distribution and
large-scale validation are next.

## Install

The easiest way is the Python launcher, which downloads the native binary for
your platform on first use:

```
pip install gopip-client
gopip version
```

Or build from source:

```
git clone https://github.com/Go-Python-Toolchain/gopip
cd gopip
go build -o gopip .
./gopip version
```

Building from source requires Go 1.22 or newer.

In GitHub Actions, install gopip with the bundled action:

```yaml
- uses: Go-Python-Toolchain/gopip/.github/actions/setup-gopip@v0.1.0
- run: gopip lock -r requirements.txt
```

## Use

```
gopip resolve requests flask>=2.0     # print pinned name==version lines
gopip lock -r requirements.txt        # write a deterministic gpt.lock
gopip explain requests                # print the resolved dependency tree
gopip install -r requirements.txt     # resolve, then install with pip
```

Requirements come from arguments or from files given with `-r`. The target
Python is detected from your interpreter and can be set with `--python`. The
`install` command resolves to exact versions and hands the installation to pip,
so packages install exactly as pip would while gopip does the resolving. Anything
after a bare `--` is passed straight through to pip.

## Design

- A pure Go solver with conflict-driven clause learning.
- Concurrent metadata fetching from the Python Package Index.
- A deterministic lockfile that is identical across machines and operating systems.
- Installation delegated to pip, so resolved packages install exactly as expected.

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
