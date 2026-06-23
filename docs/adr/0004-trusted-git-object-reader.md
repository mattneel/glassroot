# ADR: Trusted Git object reader

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-5 introduced a `RevisionFileSource` trust contract but used only in-memory tests. The original GR-6 combined two boundaries: invoking Git to read raw objects and materializing hostile paths onto a filesystem. GR-6A separates the first boundary so Git object reading can be reviewed before GR-6B writes any target files.

The reader must resolve an approved selector to an immutable commit ID, read raw tree/blob objects without checkout or filters, and preserve the invariant that the proposed revision cannot choose how it is inspected.

## Decision

GR-6 is split into GR-6A and GR-6B. GR-6A introduces `internal/gitstore`, a standard-library-only adapter around a trusted Git CLI. GR-6B will later perform traversal-resistant filesystem materialization.

The Git directory must be a bare store created and owned by the Glassroot control plane. Arbitrary uploaded `.git` directories are rejected by contract because their config, refs, hooks, and object layout are not trusted control-plane metadata.

Glassroot uses the standard Git CLI rather than a new Git parser or third-party library. This avoids adding a Go Git implementation dependency while retaining Git's object-format support. Production `os/exec` is confined to `internal/gitstore`, uses an absolute Git executable, and never invokes a shell.

The minimum supported Git version is 2.43.0, matching the repository's fixed Ubuntu 24.04 CI environment used during implementation. The adapter records the actual Git version and rejects unsupported versions before opening a repository.

Selectors are conservative: either a full object ID for the repository object format or a fully qualified ref beginning with `refs/`. Short names, abbreviated IDs, revision expressions, reflogs, ranges, path lookups, and option-looking raw selectors are rejected. Successful resolution records the full commit ID and tree ID; later reads use those immutable IDs instead of symbolic refs.

Repository preflight rejects non-bare stores, final-path symlinks, `commondir`, linked worktrees, configured work trees, alternates, HTTP alternates, grafts, partial clones, promisor remotes or pack markers, unsupported repository extensions, config includes, and unsupported object formats. Replacement objects are disabled.

Git commands are whitelisted to version, config, check-ref-format, rev-parse, ls-tree, and cat-file. Repository commands include fixed safeguards for no pager, no replacement objects, no optional locks, explicit `--git-dir`, disabled hooks, disabled protocols, disabled automatic maintenance, and disabled submodule recursion. The command environment is an explicit allowlist and does not inherit user `GIT_*`, SSH, askpass, credential, or tracing settings.

Tree and blob access is raw. Tree output is NUL-delimited and path-validated before later GR-6B materialization. Blob reads verify cat-file headers, sizes, framing, Git object identity, and caller limits. Git object identity may be SHA-1 or SHA-256 according to the repository format; Glassroot content digests remain separate `sha256:<hex>` raw-byte digests and are not signatures or attestations.

Checkout and `git archive` are not used. `git archive` may honor export-ignore, export-subst may transform content, archive extraction adds another hostile archive-path parser, and Glassroot needs exact raw tree/blob semantics.

Tracked LFS pointer files are returned as raw blobs. Gitlinks are reported as gitlink entries and not traversed. Hooks, filters, attributes, textconv, LFS fetching, submodule initialization, clone, fetch, and remote helpers are not invoked.

## Security considerations

The adapter is read-only and creates no target workspace. It does not execute target code, initialize submodules, fetch remotes, run hooks, or claim sandboxing. Raw Git object parsing and Git subprocess behavior remain attack surfaces. The residual trust assumptions are the Git executable and the control-plane-owned bare store contract, including no concurrent mutation by an attacker during operations.

GR-6B must still use traversal-resistant filesystem APIs before writing any tree entry to disk. GR-6A path validation rejects clearly unsafe repository paths but does not by itself prove host-platform materialization safety.

## Alternatives considered

- **Single GR-6 implementation:** rejected to keep Git invocation review separate from filesystem write containment.
- **go-git or libgit2:** rejected for GR-6A to avoid adding a Git implementation dependency or CGO surface.
- **Accept arbitrary `.git` directories:** rejected because repository metadata can influence Git behavior and exceeds this trust contract.
- **Checkout or archive:** rejected because they can invoke additional semantics and path handling not needed for raw object reads.

## Consequences

Future planning can resolve refs once, keep exact commit IDs, and feed GR-5 with raw base/head configuration bytes. The package has a deliberately narrow command surface and tests for adversarial repository metadata. Future work must preserve the command inventory or update this ADR.

## Operational and migration impact

No CLI behavior changes. No repository is cloned, fetched, checked out, archived, materialized, or executed. Control-plane ingestion into the trusted bare store remains a future responsibility. GR-6B will consume the validated tree inventory and perform safe materialization.

## Validation plan

Validation uses command-adapter tests, integration tests with SHA-1 and SHA-256 bare repositories, adversarial metadata tests, tree/path/parser tests, blob identity tests, a GR-5 `RevisionFileSource` integration test, and fuzz targets for tree records, cat-file headers, and path validation. Verification runs module checks, `make verify`, race tests, schema checks, gitstore tests, all tests, vet, bounded fuzzing, govulncheck, and diff audits.

## References

- `KICKSTART.md`
- `docs/THREAT_MODEL.md`
- `docs/GIT_OBJECT_READER.md`
- `docs/adr/0003-trusted-base-configuration.md`
- `internal/config/source.go`
- `internal/gitstore`
