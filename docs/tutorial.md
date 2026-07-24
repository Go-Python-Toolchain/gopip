# gopip tutorial

This walkthrough takes a small project from a list of requirements to a locked,
installed environment. It assumes gopip is installed. If it is not, see
[getting started](getting-started.md).

The exact versions you see will depend on what the package index offers when you
run the commands. The shapes of the output stay the same.

## 1. Start from a requirements file

Create a file called `requirements.txt` with one direct dependency:

```
rich>=13
```

`rich` is a small library with a few dependencies of its own, which makes it a
good first example.

## 2. See what gopip would resolve

```
gopip resolve -r requirements.txt
```

gopip fetches the available versions, picks a consistent set, and prints it:

```
markdown-it-py==4.2.0
mdurl==0.1.2
pygments==2.20.0
rich==15.0.0
```

You asked for one package and got four. gopip pulled in `rich` and everything it
needs, each at a version that fits.

## 3. Understand the choices

To see how the packages relate, ask gopip to explain:

```
gopip explain -r requirements.txt
```

```
rich 15.0.0
  markdown-it-py 4.2.0
    mdurl 0.1.2
  pygments 2.20.0
```

The tree reads top down: `rich` depends on `markdown-it-py` and `pygments`, and
`markdown-it-py` in turn depends on `mdurl`.

## 4. Write a lockfile

```
gopip lock -r requirements.txt
```

This writes `gpt.lock`, a deterministic JSON file that pins every package and
records the graph:

```
wrote 4 package(s) to gpt.lock
```

The lockfile is sorted and contains nothing about your machine, so running the
same command on another operating system produces a byte-identical file. Commit
`gpt.lock` to your repository so everyone resolves to the same versions.

```json
{
  "version": 1,
  "roots": [
    "rich"
  ],
  "packages": [
    {
      "name": "markdown-it-py",
      "version": "4.2.0",
      "dependencies": [
        "mdurl"
      ]
    },
    {
      "name": "mdurl",
      "version": "0.1.2"
    },
    {
      "name": "pygments",
      "version": "2.20.0"
    },
    {
      "name": "rich",
      "version": "15.0.0",
      "dependencies": [
        "markdown-it-py",
        "pygments"
      ]
    }
  ]
}
```

## 5. Install

When you are happy with the resolution, install it. gopip resolves to exact
versions and hands the install to pip:

```
gopip install -r requirements.txt
```

To see the exact pip command without running it, add `--dry-run`:

```
gopip install --dry-run -r requirements.txt
```

```
python3 -m pip install markdown-it-py==4.2.0 mdurl==0.1.2 pygments==2.20.0 rich==15.0.0
```

Because the install is delegated to pip, everything pip already knows how to do
still works. Anything you put after a bare `--` is passed straight through, for
example to install into a specific environment or to add pip options.

## 6. Target a specific Python

gopip resolves for a target interpreter, which it detects from your `python3` by
default. To resolve for a different version, pass `--python`:

```
gopip lock -r requirements.txt --python 3.9
```

This matters when a dependency is only needed on certain Python versions or
platforms, since gopip evaluates those markers against the target you choose.

## 7. The second resolve is the fast one

Run the same resolve twice and the difference is obvious:

```
gopip lock -r requirements.txt
```

The first run reads what it needs from the package index. The second answers
from a local cache and does no network work at all, so it finishes in
milliseconds. You do not have to do anything to get this; it is the default.

Three flags cover the times when it is not what you want:

```
gopip lock -r requirements.txt --refresh    # fetch again, ignoring the cache
gopip lock -r requirements.txt --offline    # use only the cache, never the network
gopip lock -r requirements.txt --no-cache   # leave the cache out entirely
```

`--offline` is the useful one in CI or on a plane: if something is missing from
the cache, gopip says so rather than quietly reaching for the network.

To see where the cache is and what it holds:

```
gopip cache info
gopip cache clear
```

The cache only decides whether an answer needed the network. It can never change
which versions you get, so a cached resolve and a fresh one produce the same
lockfile.

## Where to go next

- The [example project](../examples/basic) is ready to resolve and lock.
- The [resolver design](resolver.md) explains how gopip chooses versions and
  detects conflicts.
