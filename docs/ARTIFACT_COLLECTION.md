# Safe post-run artifact collection

GR-13B adds a bounded collector for artifacts produced inside one private
materialized workspace. It runs after the future docker-dev attempt has
terminated and been reaped, and before the enclosing evidence transaction is
committed. It does not execute workspace content, observe filesystem activity
while a target is running, invoke Docker, invoke Git, invoke a shell, or write an
evidence bundle by itself.

## Trust boundary

The workspace path is trusted control-plane state supplied by later run
orchestration. The collector opens and retains the workspace root before target
execution. After execution, every name, permission bit, link, file type, and byte
inside that workspace is hostile.

The collector uses `os.Root` as a traversal-resistant descriptor API. `os.Root`
is not a sandbox. Same-UID host mutation, hostile mounts, filesystem bugs,
kernel compromise, daemon compromise, and unreliable identity metadata remain
residual risks.

## Collection plan

A collection plan is trusted effective-run-plan data and contains:

- the exact run-plan digest;
- one attempt identity;
- the absolute POSIX sandbox workdir;
- ordered artifact rules with stable IDs, absolute POSIX patterns, and `maxBytes`.

GR-13B collects only logical paths equal to or lexically beneath the planned
workdir. `/workspace2` is not beneath `/workspace`. Patterns outside the workdir,
relative patterns, backslashes, control characters, `.` or `..` segments, and
malformed glob syntax are rejected before any artifact sink call.

Head configuration cannot add collection patterns for a trusted-base run. Later
GR-13C must bind the collector to the effective base plan before starting the
container.

## Glob semantics

Matching is performed in memory against a complete validated inventory. The
collector never calls filesystem glob expansion.

Supported syntax is bytewise and case-sensitive:

- literal path components;
- `*` and `?` within one component;
- Go-style character classes inside one component;
- `**` only as an entire component, matching zero or more complete components.

Dotfiles have no shell-specific exception. Unicode is not normalized. A pattern
ending in `/**` matches eligible descendant files, not the directory object
itself. One file may match multiple rules but is stored at most once, and the
result records every matching rule ID.

## Inventory and type policy

Before the first sink call, the collector inventories the complete workspace
through the retained root descriptor. Every entry is `Lstat`-checked. Directory
entries are opened and identity-checked before traversal. Names must be valid
UTF-8, free of NUL/control characters and backslashes, clean, bounded, and must
not contain a `.git` component under ASCII case-insensitive comparison.

Entry handling is intentionally narrow:

| Entry type | Behavior |
| --- | --- |
| Directory | Traversed after identity checks; never stored as an artifact. |
| Regular file | Eligible only when link count is exactly one. |
| Hard-linked regular file | Collection error in v1; no successful result. |
| Symlink | Never followed or opened. Direct matches become `omitted-symlink`; descendant patterns become blocked. |
| FIFO/socket/device/other special | Never opened. Direct matches become `omitted-special`; descendant patterns become blocked. |
| Different filesystem device | Rejected as a filesystem-boundary failure. |

The collector does not read symlink targets, create placeholders, infer file
formats, or treat filename extensions as executable evidence.

## Stable regular-file reads

For each matched regular file the collector:

1. compares the current `Lstat` metadata with the preflight inventory;
2. opens the file through the retained root descriptor;
3. verifies the descriptor identity with `File.Stat`;
4. verifies the path still names the same file;
5. streams bytes once to the synchronous artifact sink while hashing and
   counting independently;
6. verifies the sink-reported digest and size;
7. verifies descriptor and path metadata again after streaming;
8. propagates close errors.

Empty, binary, invalid UTF-8, ANSI, CRLF, and NUL-containing bytes are preserved
exactly by the stream. The result never contains raw artifact bytes.

## Modes and limits

Stored metadata records source mode bits, executable state from execute bits,
and setuid/setgid/sticky observations. The collector never executes, inspects,
chmods, or preserves the source file mode in evidence storage. Evidence objects
written later remain responsible for their own storage mode.

If a matched file exceeds its artifact rule, per-file, or total byte limit, the
collector does not open it. It records `omitted-limit`, known size, applicable
limit, executable metadata, matching rule IDs, and a fixed limitation. No
truncated artifact object is stored.

## Completeness and caller responsibilities

A successful collector transaction has separate completeness:

- `collectionComplete=true` means the stable inventory was complete and every
  matched regular file was stored; zero matches can still be complete.
- `collectionComplete=false` means omissions or blocked traversal are explicit.
- mutation, identity failure, I/O failure, sink failure, or model invariant
  failure returns an error and no result.

After all sink calls, the collector inventories the workspace again and requires
exact metadata equality with the preflight inventory. A new, removed, renamed,
replaced, remoded, relinked, or resized entry fails collection.

If any error occurs after sink writes, GR-13C must abort the enclosing evidence
transaction. A successful incomplete result must be recorded as incomplete
evidence; it must not be converted to complete evidence by policy or rendering.

## Non-claims

Artifact collection is not filesystem observation during execution. It does not
prove no other file existed earlier, no artifact escaped before collection, or
that uncollected behavior was absent. It adds no CLI behavior, policy decision,
report rendering, Docker integration, hardened sandbox, signing, authentication,
attestation, or provenance claim.

Initial hardened support is Linux-only. Other platforms compile but binding and
collection fail closed as `unsupported-platform`.
