# GitHub source store (GR-15B3)

A GitHub source store is a trusted-control-plane, immutable-by-service-contract, shallow bare Git object store created by the GR-15B3 source ingester. Future workers receive only an opaque `SourceStoreID`; they do not receive host paths or GitHub credentials.

## Source root contract

The configured source root is trusted control-plane state. It must be absolute, clean, UTF-8, control-free, existing, non-symlink, owned by the current effective UID on Linux, mode `0700`, and backed by a local trusted filesystem. Network filesystems, hostile mounts, malicious kernels, or same-UID host compromise are unsupported.

A quota-backed filesystem is recommended. The userspace size checks are not a hard disk-exhaustion defense during Git pack ingestion.

## Deterministic ID and layout

SourceStoreID domain:

```text
glassroot.dev/github-source-store-id/v1\0
```

It binds the import profile, target ID, base repository ID, head repository ID, pull-request number, exact base commit ID, and exact head commit ID. It excludes job ID, controller generation, source request ID, route names, tokens, timestamps, Git version, and host paths.

Rendered form:

```text
source-<64 lowercase SHA-256 hex>
```

Fixed layout:

```text
<source-root>/
  stores/
    sha256/
      <first-two-hex>/
        <source-store-id>/
          source.json
          repository.git/
```

Repository data, route names, and commits do not choose paths except through the deterministic SourceStoreID hash.

## `source.json` metadata

Schema:

```text
glassroot.dev/github-source-store/v1alpha1
```

Metadata includes import profile, SourceStoreID, target ID, numeric repository IDs, pull-request number, object format, exact base/head commit and tree IDs, `shallow: true`, fixed local refs, and fixed limitations. It excludes tokens, remote URLs, route names, host paths, Git stderr, timestamps, random values, PR prose, branch names, and source contents.

Metadata digest domain:

```text
glassroot.dev/github-source-store-metadata-json/v1\0
```

The digest is over the exact compact metadata JSON bytes with length prefixing. It is a control-plane integrity check, not authentication or provenance.

## Bare repository profile

Allowed fixed refs:

- `refs/glassroot/base`
- `refs/glassroot/head`

`HEAD` is set only as benign bare-repository metadata; consumers use exact commit IDs from metadata/controller state.

The final store must not contain remote configuration, remote-tracking refs, tags, `FETCH_HEAD`, worktree metadata, index files, hooks, alternates, HTTP alternates, grafts, promisor markers, partial-clone config, credential helpers, persisted extra headers, proxy config, URL rewrites, submodule recursion config, unknown fixed refs, writable files, symlinks, or special files.

Final ordinary files are mode `0400`; directories are read-only (`0500`) for future control-plane reads. Unix mode bits do not defend against same-UID or privileged host compromise.

## Publication and reuse

The source ingester publishes from a fresh staging directory only after Git import, exact ref verification, tree identity verification, shallow metadata checks, GR-6A Git object-reader preflight, metadata writing, permission normalization, and directory sync attempts.

Existing deterministic stores are opened and fully verified before reuse. Reuse avoids requesting a new token. Conflicting, corrupt, or unsafe existing stores fail closed; the ingester never overwrites, merges, repairs, or prunes stores in v1.

A crash after publication but before controller completion is recovered by retrying the source request, re-opening the deterministic store, and applying the result idempotently. A crash before publication leaves no final store by contract; staging cleanup failures are surfaced.

## GR-6A boundary

GR-6A remains the Git object-reader boundary. GR-15B3 opens imported stores through `internal/gitstore` before reporting success and again through `OpenByID`. Shallow stores are allowed only under the exact control-plane import profile and are documented as complete for selected revisions, not complete history.

## Future use

`OpenByID` derives the physical path internally from SourceStoreID, revalidates metadata and repository identities on every open, and exposes no mutation, deletion, fetch, repair, or path API. Future GR-15C workers must receive SourceStoreID, never a host path.
