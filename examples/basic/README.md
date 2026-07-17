# Basic gopip example

A one line requirements file that pulls in `rich` and its dependencies. Use it to
try gopip end to end.

## Resolve

```
gopip resolve -r requirements.txt
```

Prints the pinned set (exact versions depend on the current index):

```
markdown-it-py==4.2.0
mdurl==0.1.2
pygments==2.20.0
rich==15.0.0
```

## Explain

```
gopip explain -r requirements.txt
```

```
rich 15.0.0
  markdown-it-py 4.2.0
    mdurl 0.1.2
  pygments 2.20.0
```

## Lock

```
gopip lock -r requirements.txt
```

Writes a deterministic `gpt.lock` you can commit. Running it again on any machine
produces the same file for the same inputs.

## Install

```
gopip install -r requirements.txt
```

Resolves to exact versions and installs them with pip. Add `--dry-run` to see the
pip command first.
