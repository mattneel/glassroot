# Safe revision materialization

GR-6B materializes one already-resolved Git tree into a fresh private workspace. The input revision must come from `internal/gitstore` as an immutable `ResolvedRevision`; the materializer never resolves or re-reads a symbolic ref.

Materialization writes hostile source data. It is not a sandbox, and a successful workspace is not safe to execute directly on the host.

## Platform support

Initial security support is Linux-only. The package compiles on other Go platforms, but materialization fails closed with `unsupported-platform` outside Linux. No security claim is made for untested `os.Root` implementations.

## Destination parent trust contract

The destination parent is trusted control-plane configuration, not repository or pipeline content. It must be an absolute, clean, existing directory whose final component is not a symlink. It must not equal or live under the source bare Git store.

The parent and backing filesystem must not be writable or replaceable by the analyzed workload. Materialization assumes no hostile same-UID process mutates the generated workspace concurrently, no attacker-controlled mounts or device nodes are present, and the host kernel and filesystem enforce normal semantics. These assumptions do not isolate future target execution.

## Workspace lifecycle

For each attempt, Glassroot generates a random fixed-format directory name and creates a new `0700` workspace under the trusted parent. Repository text is never used in the workspace name. The workspace is opened with `os.OpenRoot` and all repository-selected paths are passed through that root.

The caller owns the `Workspace` only after `Materialize` returns success. `Workspace.Close` is idempotent: it closes the root handle and removes the generated directory. Partial output is removed after every failed attempt. If cleanup itself fails, the primary failure is preserved under a `cleanup-failed` classification and the result is not usable.

## Source and write order

The materializer calls `ListTree` once for the exact resolved revision, copies the inventory into owned structures, and preflights every entry before creating the first repository-derived destination entry. It rejects unsafe paths, duplicate entries, file/directory conflicts, missing explicit parent directories, unknown modes, invalid object IDs, count limits, byte limits, path limits, and metadata limits before writing.

Creation order is deterministic:

1. directories sorted by depth and path;
2. regular and executable files sorted by path;
3. symlinks sorted by path;
4. gitlinks are reported but never created.

No repository-derived filesystem operation occurs after symlink creation begins.

## Paths and modes

Repository paths remain slash-separated POSIX paths. They must be valid UTF-8, relative, clean, bounded, free of NUL/control characters and backslashes, and must not contain `.git` under ASCII case-insensitive comparison. Unicode normalization is not performed; byte-distinct Unicode names remain distinct.

Directories are created with mode `0755`. Regular files are created with `0600`, populated from the exact Git blob, and then normalized through descriptor-based `File.Chmod` to `0644`. Executable files are normalized to `0755`. Setuid, setgid, sticky bits, timestamps, owners, groups, ACLs, xattrs, sparse-file intent, and hard links are not preserved.

Executable mode is repository metadata only. GR-6B never runs a materialized file.

## Symlinks

Symlink targets are read as bounded raw blob bytes. A target must be valid UTF-8, non-empty, clean, relative, and free of NUL/control characters and backslashes. Absolute targets are rejected. Relative `..` segments are accepted only when lexical resolution from the link's parent remains inside the workspace and avoids `.git` components.

Targets are not followed, chained, normalized, or required to exist. The raw target is not included in results or errors; only its byte length and SHA-256 digest are reported. `os.Root.Symlink` is used only after directories and regular files are complete.

## Gitlinks and Git LFS

Gitlinks are not traversed, fetched, or materialized. They are reported with disposition `skipped-gitlink`, their commit object ID, and a limitation noting the source tree is incomplete at that path.

Tracked Git LFS pointer files are materialized as their actual raw Git blob bytes. Glassroot detects canonical v1 pointer text, records the declared SHA-256 OID and declared size, and marks the entry `materialized-lfs-pointer`. It never invokes `git-lfs`, fetches the external object, contacts a remote, or allocates based on the declared LFS size.

## Limits

Initial absolute ceilings are:

- entries: 100,000;
- directories: 50,000;
- regular/executable files: 75,000;
- symlinks: 10,000;
- gitlinks: 10,000;
- single blob: 128 MiB;
- total blob bytes, including symlink blobs: 8 GiB;
- symlink target: 4 KiB;
- inventory metadata: 64 MiB;
- path: 4,096 bytes;
- component: 255 bytes;
- depth: 128 components;
- reported entries: 100,000;
- reported limitations: 1,000;
- duration: 10 minutes.

These are adapter ceilings. Future platform and organization policy may narrow them; pipeline content cannot raise them in GR-6B.

## Digests

GR-6B emits two SHA-256 digests using length-prefixed binary records sorted by exact path bytes:

- `MaterializedTreeDigest` covers only entries actually created: directories, regular files, executable files, and symlinks.
- `MaterializationManifestDigest` covers all source entries and dispositions, including skipped gitlinks and detected LFS pointers.

The domains are `glassroot.dev/materialized-tree/v1\0` and `glassroot.dev/materialization-manifest/v1\0`. The digests exclude workspace paths, inodes, devices, timestamps, ownership, umask, ACLs, and xattrs. They are not signatures, attestations, canonical filesystem representations, or proof of storage integrity.

## Exclusions

GR-6B does not clone, fetch, check out, use `git archive`, initialize submodules, fetch LFS objects, run filters or hooks, execute target code, create a runner, build a run plan, collect evidence, compare behavior, or make a sandbox security claim. GR-7 and later runner work must provide the execution boundary and must not run hostile workspaces directly on the host.
