# The gpt.lock Format

`gpt.lock` is the file gopip writes to pin a project's dependencies. This
document specifies its format so it can be read, diffed, or produced by other
tools with confidence. The guiding property is determinism: the lock is a pure
function of the resolution, so the same requirements produce a byte-identical
file on any machine or operating system, which is what makes it worth committing
to a repository.

## The file

`gpt.lock` is UTF-8 JSON, pretty-printed with two-space indentation and a
trailing newline. HTML escaping is off, so package names appear verbatim. It has
three top-level fields:

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

## Fields

| Field | Type | Meaning |
| --- | --- | --- |
| `version` | integer | The lock schema version. Currently `1`. A reader that does not recognize the version should refuse the file rather than guess. |
| `roots` | array of strings | The names of the direct requirements the resolution started from, sorted. |
| `packages` | array of objects | Every resolved package, one entry each, sorted by name. |

Each entry in `packages` is:

| Field | Type | Meaning |
| --- | --- | --- |
| `name` | string | The package's normalized name. |
| `version` | string | The exact resolved version, a PEP 440 version. |
| `extras` | array of strings, optional | The package's optional features the resolution selected, normalized and sorted. Omitted when none were, so a lock for a project that uses no extras is unaffected by this field existing. |
| `hashes` | array of strings, optional | The digests of every artifact published for this version, each written as `sha256:<hex>`, sorted. Omitted when the index published none. |
| `dependencies` | array of strings, optional | The names of the resolved packages this one depends on, sorted. Omitted when empty. |

A package with extras appears once, with the extras it was resolved with:

```json
{
  "name": "flask",
  "version": "3.1.3",
  "extras": [
    "async"
  ],
  "dependencies": [
    "asgiref",
    "blinker",
    "click",
    "itsdangerous",
    "jinja2",
    "markupsafe",
    "werkzeug"
  ]
}
```

The extra's own requirements are entries in `packages` like any other, and they
appear in the package's `dependencies`, so the graph the file records is the one
that will actually be installed.

### Hashes

A release is usually published as several artifacts: a source distribution and
one wheel per platform. `hashes` lists all of them, not only the one the machine
that produced the lock would install, so the same lock verifies on Linux, macOS,
and Windows. An install checks the artifact it actually downloads against the
list and fails if none of them match.

```json
{
  "name": "mdurl",
  "version": "0.1.2",
  "hashes": [
    "sha256:84008a41e51615a49fc9966191ff91509e3c40b939176e643fd50a5c2196b8f8",
    "sha256:bb413d29f5eea38f31dd4754dd7377d4465116fb207585f97bf925588687c1ba"
  ]
}
```

The `dependencies` lists reference other entries in `packages` by name, so the
file records the full resolved graph, not just a flat list of pins. `explain`
renders that graph as a tree; the lock stores it as edges.

## Determinism rules

These rules are what make the file reproducible, and any producer of a
compatible lock must follow them:

- `roots` is sorted.
- `packages` is sorted by `name`.
- Each `dependencies` list is sorted.
- The file contains nothing about the host: no paths, no timestamps, no wheel
  URLs, no platform, no interpreter details. It is exactly the resolution and
  nothing else.

Because of these rules, two people on different operating systems who resolve
the same requirements against the same index state get the same bytes, and a
change to the lock in a code review is always a real change to the resolution,
never noise.

## What the lock does not contain

The lock records what to install, not how to fetch or verify it. It deliberately
does not carry wheel URLs, file hashes, or per-platform artifact information,
because installation is delegated to pip, which resolves those at install time.
Recording hashes for verified, offline-friendly installs is a planned addition
tracked in the [roadmap](roadmap.md); it would add fields without changing the
meaning of the ones above.

## Reading and writing

gopip reads a lock with a plain JSON parse and writes one by sorting as above
and encoding. Any tool that emits the same fields under the same sorting rules
produces a file gopip can read, and gopip's output can be consumed by anything
that reads JSON. The format is intentionally small so that staying compatible is
easy.
