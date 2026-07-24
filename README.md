# gopip

A fast, deterministic dependency resolver for Python, written in Go.

gopip computes a complete, consistent set of package versions for your project
using a pure Go solver, and writes a deterministic lockfile. It does not replace
pip. It takes over the slow part, resolution, and leaves installation to pip, so
it drops into your existing workflow without changes.

gopip is part of the [Go-Python Toolchain](https://github.com/Go-Python-Toolchain).

## Status

Released and in active development. The resolver is validated over a thousand
random dependency graphs and pinned against a frozen capture of the real package
index, and resolves the same package sets as uv and pip-tools. A repeat resolve
of a real project takes about ten milliseconds; see the
[benchmarks](docs/benchmarks.md).

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
gopip cache info                      # show what index metadata is cached
```

Requirements come from arguments or from files given with `-r`, including
extras such as `flask[async]`. The target Python is detected from your
interpreter and can be set with `--python`. The `install` command resolves to
exact versions and hands the installation to pip, so packages install exactly as
pip would while gopip does the resolving. Anything after a bare `--` is passed
straight through to pip.

gopip caches what it reads from the index, so a second resolve does no network
work. `--refresh` fetches again, `--offline` uses only what is cached and refuses
to reach the network, and `--no-cache` leaves the cache out.

`gpt.lock` records the digest of every artifact published for each pinned
version, and `gopip install --require-hashes` verifies each download against
them.

## Documentation

- [Getting started](docs/getting-started.md): install gopip and run your first resolve.
- [Tutorial](docs/tutorial.md): go from a requirements file to a locked, installed environment.
- [Architecture](docs/architecture.md): how gopip is put together, from requirements to install.
- [Resolver design](docs/resolver.md): how gopip chooses versions and detects conflicts.
- [Lockfile format](docs/lockfile.md): the exact, deterministic format of gpt.lock.
- [Benchmarks](docs/benchmarks.md): resolve time across real projects, next to uv and pip-tools.
- [Validation](docs/validation.md): how the resolver is checked for correctness.
- [Limitations](docs/limitations.md): what gopip does not do yet, and the trade-offs.
- [Roadmap](docs/roadmap.md): where gopip is headed.

## Examples

- [basic](examples/basic): a small project you can resolve and lock right away.
- [benchmark](examples/benchmark): the harness that measures resolve time across real projects and compares against uv and pip-tools.

## Design

- A pure Go solver in the PubGrub style, conflict driven and deterministic, that
  explains a failure by naming the requirements that conflict.
- Concurrent metadata fetching from the Python Package Index, with an on-disk
  cache so a repeat resolve does no network work.
- A deterministic lockfile, identical across machines and operating systems,
  carrying artifact digests for verified installs.
- Installation delegated to pip, so resolved packages install exactly as expected.

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
