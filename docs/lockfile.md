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
| `dependencies` | array of strings, optional | The names of the resolved packages this one depends on, sorted. Omitted when empty. |

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
