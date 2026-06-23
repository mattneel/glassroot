# ADR: Exact GitHub source ingestion into trusted bare stores

## Status

Accepted for GR-15B3 implementation. Not a public-execution approval.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-15B was split because credential custody, controller reconciliation, source ingestion, worker execution, and Check publication have different trust boundaries. GR-15B2 now emits credential-free source-import requests containing immutable target, job, generation, repository, PR, and commit identities. GR-15B3 needs to import exact Git objects without handing credentials to workers or treating mutable refs as authority.

## Decision

Add `internal/githubsource`, `internal/githubsourcestore`, a narrow Git import adapter under `internal/gitstore`, and `cmd/glassroot-source-ingester`.

The source ingester consumes one controller source-import request at a time, asks the GR-15B1 broker only for a one-repository `source-read` token scoped to the base repository ID, and imports through Git smart HTTP from the fixed GitHub.com base repository remote. It fetches the expected base commit by object ID and the PR head through `refs/pull/<number>/head` as a transport hint, then requires the fetched head ref to equal the controller-authorized head commit exactly.

The importer uses native Git rather than go-git/libgit2 because Glassroot already has a GR-6A Git object-reader boundary and native Git supports protocol v2, shallow smart-HTTP fetch, SHA-1/SHA-256 repositories, and `--config-env`. REST object reconstruction and archives were rejected because they would add a separate object reconstruction/verifier and archive extraction boundary.

Tokens are converted to an HTTP Basic authorization header inside `TokenLease.Use` and passed only through URL-scoped `--config-env` to one child environment variable. Tokens are absent from argv, URLs, Git config on disk, refs, metadata, controller records, logs, errors, and future worker contracts. Same-UID/environment exposure remains a deployment limitation.

The store profile is shallow depth-one, not partial clone. It imports selected commit/tree/blob closures, omits unrelated history, imports no tags, traverses no submodules, and fetches no LFS objects. Stores are identified by deterministic opaque SourceStoreID values and published under a trusted source root after GR-6A verification. Existing deterministic stores are reused only after full verification.

## Alternatives considered

- Fetch the head repository directly: rejected for v1 because it would require a separate head-repository token and broader access model. The base repository PR-head ref is used only as a transport locator and verified against exact head SHA.
- Use unauthenticated public fetch fallback: rejected because source availability must not depend on broadening trust or bypassing credential scope.
- Use GitHub archives or REST object APIs: rejected to avoid archive extraction and custom object-closure reconstruction boundaries.
- Use partial clone or filters: rejected because promisor/lazy fetching weakens local completeness for selected revisions.
- Create working trees or checkouts: rejected; materialization is a later reviewed boundary.
- Store physical paths in controller/worker protocols: rejected; SourceStoreID is the opaque control-plane capability descriptor.

## Security considerations

Git smart HTTP, Git's pack parser, repository objects, trees, blobs, gitlinks, and LFS pointer files are attack surfaces and hostile input. The Git executable, GitHub, TLS roots, broker, controller store, source root filesystem, local clock, and host kernel are trusted dependencies. Disk quota remains required for hard storage exhaustion defense.

A successful import does not prove source is benign. A commit ID is not authentication or provenance. SourceStoreID is not a signature or digest of physical pack bytes. The control-plane-created bare store is not safe to execute.

## Consequences

GR-15C can later consume opaque source-store identities without receiving GitHub tokens or host paths. Jobs can advance to `awaiting-runner` after exact source import, but no WorkerAssignment is emitted by GR-15B3. GR-15D remains separate.

## Validation plan

Tests cover request validation, route rejection, exact remote/refspec command construction, token absence from argv/URL/metadata/results, source-store ID and metadata determinism, controller source-result application, CLI flag restrictions, and fuzz seeds for routes, command construction, metadata, shallow files, publication identity, and result validation. `make test-github-source-ingester-integration` remains gated and skipped without operator-provided credentials and broker fixtures.

## References

- ADR 0004: Git object reader boundary
- ADR 0021: GitHub App advisory boundaries
- ADR 0023: GitHub App credential broker
- ADR 0024: GitHub controller reconciliation
- Git docs: `git fetch`, `git init`, `git --config-env`, Git HTTP config
- GitHub docs: installation access tokens and checking out pull requests locally
