# ADR: Safe post-run artifact collection

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-13A introduced a development-only Docker runner core. The remaining local run
work needs two distinct reviews: hostile post-run artifact collection and the
user-facing run orchestration that will connect Docker execution, evidence
writing, inspection, and CLI acknowledgement. The writable bind-mounted
workspace is trusted before execution but hostile after execution. It may contain
attacker-selected names, links, modes, file types, bytes, and mutation traps.

## Decision

Split the previous GR-13B scope. GR-13B implements only
`internal/artifactcollect`; GR-13C will add `glassroot run`, Docker execution
orchestration, log/evidence adapters, report production, cleanup, and the
unsafe-development acknowledgement.

The collector binds a private workspace before execution by opening and retaining
an `os.Root`. On Linux it records device, inode, mode, link count, size, mtime,
and ctime facts for identity checks. `os.Root` is a traversal-resistant API, not
a sandbox, and non-Linux collection fails closed.

Collection uses a trusted typed plan: run-plan digest, attempt identity, planned
absolute POSIX workdir, and ordered artifact rules. Rules are validated before
any sink call. Only paths equal to or beneath the workdir are supported.

The collector performs a complete preflight inventory through the retained root
before writing to the sink. It validates names and paths, rejects `.git`
components, rejects device-boundary crossings, opens directories for identity
checks before traversal, and sorts inventory records deterministically.

Pattern matching is in memory. No filesystem glob expansion is used. The v1
matcher supports literals, `*`, `?`, character classes within one component, and
`**` only as a complete component. Matching is bytewise, case-sensitive, and
bounded to avoid exponential behavior.

Entry policy is failure-closed. Directories are traversed but never stored.
Regular files are eligible only with link count one. Hard-linked regular files
cause an infrastructure error in v1. Symlinks are never followed or opened;
directly matched symlinks are recorded as `omitted-symlink`, and descendant
patterns are blocked. FIFOs, sockets, devices, and other specials are never
opened; direct matches are `omitted-special` and descendant patterns are blocked.

Regular files are opened through the root descriptor after preflight identity
comparison. The collector verifies descriptor identity and path identity before
and after streaming, hashes/counts the bytes independently while the synchronous
sink consumes them, and requires the sink-reported digest and size to match. A
changed file is not retried.

Omission is distinct from infrastructure failure. Files over per-rule, per-file,
or total byte limits are not opened and are represented as `omitted-limit` with
fixed limitations. Mutation, identity failure, I/O failure, sink failure, or
model invariant failure returns no successful result and requires the caller to
abort the enclosing evidence transaction.

After sink calls, the collector inventories the workspace again and requires the
same paths, entry types, identities, modes, link counts, sizes, directory
membership, mtime, and ctime facts. It does not merge inventories or return
pre-mutation artifacts as complete.

Results contain deterministic typed metadata only: no host paths, no inode/device
values, no symlink targets, and no raw artifact bytes. Collection completeness is
separate from evidence completeness; GR-13C must record incomplete collection so
evidence cannot remain complete.

## Consequences

GR-13B gives the future run command a reviewed artifact-collection boundary
without adding execution, CLI behavior, Docker imports, evidence writer imports,
policy changes, or rendering changes. The design is conservative: unsupported
links, special files, hard links, out-of-workdir paths, and unstable filesystem
state either become explicit omissions or fail the collection transaction.

Residual risk remains in same-UID host mutation, hostile filesystems, malicious
mount namespaces, kernel compromise, daemon compromise, and ctime/identity
reliability. The collector does not prove no other files existed during
execution, that no artifact escaped, or that uncollected behavior was absent.

## Alternatives considered

- **Fold collection into `glassroot run`:** rejected to review hostile
  filesystem handling independently from CLI, Docker execution, and evidence
  orchestration.
- **Use filesystem globbing:** rejected because hostile names and symlinks must
  not drive host path expansion.
- **Follow symlinks within the workspace:** rejected because post-run links are
  attacker-controlled and may race or obscure identity.
- **Collect hard links:** rejected for v1 because aliasing would weaken stable
  identity and ownership semantics.
- **Open FIFOs or special files:** rejected to avoid blocking, device access, or
  interpreting non-file objects as artifact bytes.
- **Return partial success after mutation:** rejected because callers must abort
  evidence transactions after any post-sink instability.
- **Store artifact bytes in the collector result:** rejected to keep evidence
  storage behind a narrow synchronous sink and avoid unbounded memory retention.
