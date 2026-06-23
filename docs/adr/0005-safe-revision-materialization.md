# ADR: Safe revision materialization

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-6A separated trusted Git object reading from filesystem writes. It resolves selectors to immutable commit/tree identities and exposes raw tree/blob metadata without checkout, filters, LFS fetching, or submodule traversal. GR-6B is the next boundary: transforming one resolved tree into a fresh workspace without allowing hostile paths or symlink targets to escape the generated directory.

The workspace will later be input to a runner, but no runner exists in this issue. Materialization must not execute target code or imply that the workspace is safe to execute on the host.

## Decision

Glassroot adds `internal/materialize`, a standard-library-only materializer that consumes an already-resolved `gitstore.ResolvedRevision` and a trusted `gitstore.Repository`. It is initially Linux-only for security support and fails closed with `unsupported-platform` elsewhere.

The destination parent is trusted control-plane state. The materializer validates that the parent is absolute, clean, existing, a directory, not a final-component symlink, and not inside the source bare Git store. It creates a random `0700` workspace and opens it with `os.OpenRoot`. Repository content cannot choose the parent or workspace name.

The materializer preflights the complete tree inventory before writing. It validates paths defensively even though `gitstore` already validates them, requires explicit parent directories, rejects duplicate entries and path conflicts, enforces object ID and mode coherence, and applies entry, byte, path, depth, metadata, and time limits. It copies inventory values into owned structures before writing.

Creation order is deterministic: directories first, files second, symlinks last. Directories use `Root.Mkdir` with `0755`; `MkdirAll` is not used for hostile paths. Files use `Root.OpenFile` with `O_CREATE|O_EXCL`, are initially `0600`, are populated through exact-object `gitstore.CopyBlob`, and are normalized with descriptor-based `File.Chmod` to `0644` or `0755`. No path-based chmod/chown/timestamp operation is used.

Symlink blobs are read as bounded data. Targets must be valid UTF-8, non-empty, relative, clean, and lexically contained within the workspace when resolved from the link's parent. Absolute targets and `.git` components are rejected. Symlinks are created last with `Root.Symlink`, and no repository-derived filesystem operations follow symlink creation.

Gitlinks are skipped and reported with limitations. Git LFS pointer files are materialized as their raw pointer blobs; canonical v1 pointers are detected and annotated without fetching external content.

Two deterministic SHA-256 digests are produced. `MaterializedTreeDigest` covers created directories, files, and symlinks under Glassroot's mode and path normalization. `MaterializationManifestDigest` covers all source entries and dispositions, including skipped gitlinks and detected LFS pointers. Both use domain-separated, length-prefixed binary records sorted by exact path bytes. The digests are not signatures or canonical filesystem proofs.

On any failure after workspace creation, the materializer closes open descriptors, closes the root, and removes the exact generated workspace directory. No partial workspace is returned as usable output. `Workspace.Close` gives successful callers idempotent ownership cleanup.

## Security considerations

`os.Root` is a traversal-resistant filesystem primitive, not a sandbox. It does not defend against a malicious kernel, compromised materializer UID, attacker-controlled mount namespace, or hostile same-UID process racing the generated workspace. The accepted contract is that the destination parent and backing filesystem are control-plane trusted and that target code is not running concurrently with materialization.

The materializer does not preserve timestamps, owners, groups, ACLs, xattrs, setuid/setgid/sticky bits, sparse-file intent, hard links, devices, FIFOs, or sockets. It never runs executable files merely because Git marks them executable.

Residual races remain possible if the control-plane trust contract is violated. The implementation still uses `os.Root`, `O_EXCL`, descriptor-based chmod, symlinks-last ordering, and full cleanup to reduce exposure under the accepted contract.

## Alternatives considered

- **Checkout:** rejected because checkout can apply filters, attributes, LFS, working-tree semantics, and additional path behavior.
- **`git archive`:** rejected for the GR-6A reasons: export attributes can omit or transform content, and archive extraction would add another hostile path parser.
- **Materialize into an existing directory:** rejected because merging with prior content expands overwrite and cleanup ambiguity.
- **`os.DirFS` or absolute path joins:** rejected as confinement mechanisms because repository-selected paths must not be appended to host paths.
- **Preserve all Git/host metadata:** rejected to keep the first materializer narrow and deterministic.

## Consequences

Future planning can refer to a deterministic materialized-tree digest and a manifest digest while keeping Gitlinks and LFS omissions explicit. Later runner work must treat the workspace as hostile data and must not execute it directly on the host.

The materializer depends on the GR-6A trusted bare-store contract and the host parent-directory contract. GR-6B does not solve cloning, fetch, organization policy, run planning, sandboxing, execution, artifact collection, evidence bundles, or report rendering.

## Operational and migration impact

No CLI behavior changes. No Go dependencies are added. `internal/model`, `internal/config`, public pipeline schema, workflows, and GR-3 fixtures are unchanged. Production `os/exec` remains confined to `internal/gitstore`.

## Validation plan

Validation covers preflight path and inventory rejection, file mode normalization, raw byte preservation, symlink containment, Gitlink reporting, LFS pointer detection, deterministic digests, cleanup after failure, idempotent close, hostile symlink-race tests, descriptor-based chmod redirection tests, SHA-1 and SHA-256 Git integration, ref-mutation noninterference, and fuzz targets for inventory, symlink targets, LFS parsing, and digest encoding.

## References

- `KICKSTART.md`
- `docs/THREAT_MODEL.md`
- `docs/GIT_OBJECT_READER.md`
- `docs/MATERIALIZATION.md`
- `docs/adr/0004-trusted-git-object-reader.md`
- `internal/gitstore`
- `internal/materialize`
