# Glassroot pipeline configuration

Glassroot v1alpha1 reads a single pipeline file from `.glassroot/pipeline.yaml` by default. The file is repository-controlled input, so GR-4 accepts only a narrow YAML subset and validates it before any later planning work. Validation never executes `run`, does not inspect a repository, does not expand globs, does not resolve images, and does not prove that a command or image is safe.

Use:

```sh
glassroot validate
glassroot validate --file path/to/pipeline.yaml
```

Exit codes:

- `0`: configuration is valid.
- `2`: usage error, missing configuration, parse failure, or validation failure.
- `3`: unexpected I/O or internal failure.

`validate` reads only the selected file, applies a 1 MiB input bound, and does not recurse, include other files, access the network, construct a runner, or create a run plan.

## Configuration trust in pull requests

For a pull request from trusted base revision `A` to proposed head revision `B`, Glassroot uses only the base revision's `.glassroot/pipeline.yaml` as the effective pipeline configuration for the current request.

- Base `A` supplies the effective pipeline for both future base and head executions.
- Head `B` is read from the same fixed path only for change assessment.
- Head configuration is never merged, selected, or used to fill missing base fields.
- Formatting-only, comment-only, or key-order-only head changes can be reported as content changes that are semantically equivalent.
- Semantic head changes are reported with deterministic fields, change kinds, and security-effect classifications.
- Invalid, unsupported, oversized, removed, or non-regular head configuration is still reported; it does not become effective.
- Missing, unreadable, unsupported, oversized, syntactically invalid, or semantically invalid base configuration fails closed and prevents later planning.
- There is no approved-rerun or override mechanism in GR-5.
- GR-6 will provide exact Git commit resolution and raw Git object loading. GR-5 trusts the abstract revision-file source contract.
- Validation and trusted loading do not execute, expand, interpolate, or shell-parse `run` strings.

Compact example:

```text
base:      resources.cpu = 2
head:      resources.cpu = 64
effective: resources.cpu = 2
assessment: privilege-increase proposed in head
```

Configuration trust prevents the proposed revision from choosing how this request is inspected. It is not isolation by itself and does not make a sandbox security claim.

## v1alpha1 shape

```yaml
apiVersion: glassroot.dev/v1alpha1
kind: Pipeline
metadata:
  name: default
spec:
  environment:
    image: docker.io/library/golang:1.26@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
    workdir: /workspace
  resources:
    cpu: 2
    memory: 2GiB
    disk: 4GiB
    processes: 256
    timeout: 15m
  network:
    mode: deny
    allow: []
  scenarios:
    - id: test
      name: Unit tests
      shell: /bin/sh
      run: go test ./...
      timeout: 10m
  collect:
    filesystem:
      roots:
        - /workspace
        - /tmp
      contents: metadata-and-digests
    artifacts:
      - path: /workspace/bin/**
        maxBytes: 50MiB
    logs:
      maxBytesPerStream: 10MiB
  compare:
    ignore:
      - field: event.timestamp
      - field: process.pid
    repetitions: 1
  policy:
    profile: strict
```

All fields shown above are required. Unknown keys are rejected at every level. Explicit `null` does not satisfy a required object, array, or scalar.

## Accepted values and bounds

| Field | Accepted values |
| --- | --- |
| `apiVersion` | exactly `glassroot.dev/v1alpha1` |
| `kind` | exactly `Pipeline` |
| `metadata.name`, scenario `id` | ASCII identifier `^[a-z][a-z0-9._-]{0,63}$` |
| scenario `name` | non-empty Unicode string, no control characters, up to 256 UTF-8 bytes |
| `environment.image` | immutable reference with `@sha256:` and exactly 64 lowercase hex digest characters, up to 512 bytes |
| `environment.workdir` | absolute lexical POSIX path |
| `resources.cpu` | integer 1 through 64 |
| `resources.memory` | 16 MiB through 1 TiB |
| `resources.disk` | 16 MiB through 16 TiB |
| `resources.processes` | 1 through 65,535 |
| `resources.timeout` | 100 ms through 24 h |
| `scenarios` | 1 through 64 entries; IDs are case-sensitive and unique |
| scenario `shell` | `/bin/sh`, `/bin/bash`, or `/usr/bin/bash` only; no arguments |
| scenario `run` | non-empty literal data, may contain shell syntax and newlines, no NUL, up to 64 KiB |
| scenario `timeout` | 100 ms through the global timeout |
| `network.mode` | `deny` only |
| `network.allow` | explicitly empty array only |
| filesystem `roots` | up to 16 absolute lexical POSIX paths; no duplicates |
| `filesystem.contents` | `metadata-and-digests` only |
| `artifacts` | up to 64 entries; exact duplicate patterns rejected |
| artifact `maxBytes` | 1 B through 1 GiB |
| `logs.maxBytesPerStream` | 1 B through 100 MiB |
| `compare.ignore[].field` | `event.timestamp` or `process.pid`; no duplicates |
| `compare.repetitions` | 1 through 10 |
| `policy.profile` | `strict` only |

These are broad parser safety bounds. Deployment and platform policy can narrow them later.

## Units

Byte sizes use binary units only:

```text
<positive base-10 integer><unit>
```

Accepted units are `B`, `KiB`, `MiB`, `GiB`, and `TiB`. Decimal fractions, signs, whitespace, scientific notation, and SI units such as `KB`, `MB`, and `GB` are rejected.

Durations use:

```text
<positive base-10 integer><unit>
```

Accepted units are `ms`, `s`, `m`, and `h`. Compound durations such as `1h30m`, fractions, signs, whitespace, and day units are rejected. Values normalize internally to integer milliseconds.

## Paths and globs

Paths are untrusted lexical POSIX strings. GR-4 does not stat, open, create, clean on disk, or materialize them.

For `workdir` and filesystem roots, paths must:

- start with `/`;
- fit within 4,096 UTF-8 bytes;
- contain no NUL, control characters, or backslashes;
- contain no `.` or `..` segments;
- be unchanged by POSIX `path.Clean`.

Artifact paths must be absolute POSIX glob patterns, may use `**`, and are syntax-checked but never expanded. Lexical validation is not a complete defense against future extraction or materialization attacks; those protections belong to GR-6 and GR-8 work.

## YAML subset

Glassroot accepts exactly one non-empty YAML document and rejects:

- invalid UTF-8 and NUL bytes;
- `%YAML` and `%TAG` directives;
- aliases, anchors, and merge keys;
- explicit/custom tags;
- complex mapping keys;
- duplicate mapping keys;
- unknown fields;
- inputs over 1 MiB;
- nesting deeper than 32 levels;
- more than 20,000 YAML nodes;
- general scalars over 4 KiB, except `run` which is capped at 64 KiB.

Diagnostics are bounded, deterministic, unstyled, and avoid printing source lines or complete untrusted values.

## Public JSON Schema

The hand-maintained Draft 2020-12 schema is committed at `api/v1alpha1/pipeline.schema.json` with `$id` `https://glassroot.dev/schemas/v1alpha1/pipeline.schema.json`.

The schema supplements but does not replace Go validation. JSON Schema cannot directly express every GR-4 rule, including YAML aliases, anchors, merge keys, directives, duplicate keys, multiple documents, scenario ID or artifact-path uniqueness by object property, scenario timeout not exceeding the global timeout, all path-cleanliness checks, numeric bounds hidden inside unit strings, and semantic overflow checks.

## Security notes

A validated `run` string is only well-formed configuration intended for later sandboxed execution work. It is not safe, trusted, expanded, interpolated, syntax-checked by invoking a shell, or executed by `glassroot validate`.

Image validation checks conservative syntax and immutability only. Glassroot does not contact a registry, resolve tags, pull images, or verify digest contents in GR-4.
