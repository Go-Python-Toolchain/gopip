# Getting started with gopip

gopip works out what versions of your Python dependencies to install. It reads
your requirements, computes a single consistent set of versions with a fast
solver, writes a lockfile you can commit, and hands the actual install to pip. It
does not replace pip. It takes over the slow, fiddly part, resolution, and leaves
installation to the tool that already does it well.

## Install

The quickest way is the Python launcher, which downloads the native binary for
your platform the first time you run it:

```
pip install gopip-client
gopip version
```

Or build from source with Go 1.22 or newer:

```
git clone https://github.com/Go-Python-Toolchain/gopip
cd gopip
go build -o gopip .
./gopip version
```

## Your first resolve

Point gopip at some requirements. You can pass them as arguments:

```
gopip resolve requests rich
```

gopip prints the resolved versions as `name==version` lines, the exact form pip
understands:

```
certifi==2026.6.17
charset-normalizer==3.4.9
idna==3.18
markdown-it-py==4.2.0
mdurl==0.1.2
pygments==2.20.0
requests==2.34.2
rich==15.0.0
urllib3==2.7.0
```

Exact versions depend on what the index offers when you run it, but the set is
always internally consistent: every package sits at a version that satisfies
every other package that depends on it.

## The mental model

Give gopip your direct requirements. It fetches the available versions and their
dependencies from the package index, then chooses the newest version of each
package that keeps every constraint satisfied. If two requirements cannot both be
met, it tells you rather than installing something broken. The result is
reproducible: the same inputs produce the same versions and the same lockfile on
any machine.

## Where to go next

- The [tutorial](tutorial.md) walks through locking a project, reading the
  dependency tree, and installing.
- The [example project](../examples/basic) is a tiny project you can resolve and
  lock right away.
