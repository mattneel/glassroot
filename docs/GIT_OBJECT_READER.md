# Git object reader

GR-6A adds a read-only Git object reader for Glassroot's control plane. It exists to enforce the GR-5 `RevisionFileSource` contract against a trusted bare Git object store before any future source materialization work.

## Trust contract

The adapter opens only a bare or isolated Git store created and owned by the Glassroot control plane. The target repository does not provide the `.git` directory directly. By contract:

- repository config, hooks, refs storage, and object-store layout are control-plane metadata;
- commit, tree, blob, tag, ref, and path contents remain hostile;
- the analyzed workload cannot write to the store;
- the store is not mutated during a repository operation;
- arbitrary uploaded `.git` directories are unsupported;
- clone/fetch/ingestion into the trusted store is outside GR-6A.

This is not a sandbox and does not make repository contents trustworthy.

## Immutable revision resolution

Inputs are either a full object ID or a fully qualified ref beginning with `refs/`. Short branch names, abbreviated object IDs, and revision expressions such as parent operators, reflogs, ranges, path lookups, or wildcard syntax are rejected.

Resolution peels the approved selector to a commit and records the full lowercase commit ID and tree ID. After that point, reads use the immutable commit/tree object IDs and do not re-read a symbolic ref. A later ref mutation cannot change a previously resolved revision.

Supported object formats are `sha1` and `sha256`. These are Git object identities only; they are not Glassroot evidence attestations.

## Command inventory and environment

Production Git subprocesses are constructed by typed code and use an absolute Git executable. The allowed command inventory is:

- `git version`
- `git config --file ... --no-includes --null --list`
- `git check-ref-format`
- `git rev-parse`
- `git ls-tree`
- `git cat-file`

Repository commands include fixed safeguards such as `--no-pager`, `--no-replace-objects`, `--no-optional-locks`, explicit `--git-dir`, `core.hooksPath=/dev/null`, `protocol.allow=never`, disabled automatic maintenance, and disabled submodule recursion. Commands run from a private control-plane working directory, not inside the repository.

The environment is an allowlist with deterministic locale/time settings and Git controls such as `GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_SYSTEM=/dev/null`, `GIT_CONFIG_NOSYSTEM=1`, `GIT_TERMINAL_PROMPT=0`, `GIT_NO_REPLACE_OBJECTS=1`, `GIT_NO_LAZY_FETCH=1`, and `GIT_PROTOCOL_FROM_USER=0`. User-provided `GIT_*`, SSH, askpass, credential, tracing, and repository-derived environment values are not inherited.

## Bounds

Initial safety ceilings include:

- revision selector: 1 KiB;
- repository path: 4 KiB;
- config file: 1 MiB;
- small stdout: 1 MiB;
- stderr: 64 KiB;
- tree listing: 64 MiB;
- tree entries: 100,000;
- tree depth: 128;
- tree path: 4,096 bytes;
- tree path component: 255 bytes;
- blob: 128 MiB;
- default Git command timeout: 30 seconds.

Stdout and stderr are bounded and sanitized in errors. Cancellation and timeouts terminate Git subprocesses.

## Repository preflight

Opening rejects non-bare repositories, final-path symlinks, linked worktree metadata, `commondir`, configured work trees, alternates, HTTP alternates, grafts, partial-clone and promisor state, unsupported extensions, unsupported object formats, config includes, and oversized or malformed config.

The adapter does not repair repositories.

## Raw tree and blob semantics

Tree enumeration uses NUL-delimited `ls-tree` output. It validates modes, object types, object IDs, sizes, duplicate paths, path conflicts, and hostile path bytes. Exposed paths must be valid UTF-8 relative slash-separated paths with no control characters, backslashes, empty segments, `.` or `..`, overlong components, excessive depth, or `.git` components under ASCII case-insensitive comparison. Unicode normalization is not performed; byte-distinct Unicode paths remain distinct.

Blob reads use exact object IDs and `cat-file --batch`. The adapter checks the returned object ID, object type, announced size, framing, caller byte limit, and absolute blob limit. It recomputes the Git blob object identity from raw bytes and also records a `sha256:<hex>` raw content digest when returning file data. No textconv, filters, end-of-line conversion, Git LFS dereference, remote lookup, or checkout occurs.

Symlinks are returned as symlink blobs and are not followed. Gitlinks are returned as gitlink entries and are not traversed. LFS pointer files are returned as ordinary raw blob bytes. Submodules are not initialized and `.gitmodules` is not interpreted.

## No checkout or archive

GR-6A does not create a target workspace and does not use `git archive`. Archive output can be affected by export attributes, export-subst can transform content, and archive extraction would introduce a second hostile path parser. GR-6B will materialize the already validated tree inventory directly with traversal-resistant filesystem operations.

## Operational limitations

The adapter relies on the trusted Git executable and on the control-plane-owned bare store contract. It is not safe for arbitrary attacker-supplied `.git` directories. GR-6B remains responsible for filesystem-write containment, destination path policy, cleanup after partial writes, and materialized-tree digests.
