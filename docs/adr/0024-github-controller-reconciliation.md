# ADR: GitHub controller reconciliation and durable job state

## Status

Accepted for GR-15B2 implementation. Not a public-execution approval.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-15B was split because GitHub App private-key custody, controller reconciliation, and exact source ingestion have different trust boundaries. GR-15A now provides authenticated webhook projections and a durable at-least-once inbox/outbox. GR-15B1 now provides repository-scoped installation tokens through a local credential broker. GR-15B2 needs to turn authenticated webhook hints into durable immutable controller state without fetching source, assigning workers, executing code, or publishing checks.

Webhook payloads can be duplicated, delayed, out of order, stale, or maliciously crafted within GitHub's signed envelope. Event-reported SHAs are hints, not immutable source authority. Controller state therefore needs a fresh GitHub API read and monotonic generation model before any future source import or worker handoff can exist.

## Decision

Add `internal/githubcontroller`, `internal/githubcontrollerstore`, and `cmd/glassroot-controller`.

The controller consumes one GR-15A outbox record at a time, acquires a per-PR reconciliation lease, requests only a `pull-request-read` token for the base repository through GR-15B1, and calls exactly one new installation-authenticated REST operation:

```text
GET /repos/{owner}/{repo}/pulls/{pull_number}
```

Route owner/name strings are bounded routing hints retained from pull_request projections and current API snapshots. Numeric repository IDs and exact commit IDs are identity authority. The API snapshot is reduced to PR number, state/draft/merged, base/head repository IDs, route hints, and exact base/head commit IDs. Prose, branch refs, URLs, labels, comments, and commit messages are not retained.

The controller store persists processed outbox records, PR generations, immutable targets, jobs, attempts, source-import requests, Check Run bindings, source-result handling, and worker-result freshness classifications. Controller state commits before receiver outbox acknowledgement. Reprocessing after crash is idempotent by receiver/outbox ID and projection digest.

A newly eligible target creates one job, one initial attempt, and one credential-free source-import request. The required future runner tiers are `hardened-container` and `microvm`; no WorkerAssignment is emitted. Draft, closed, inaccessible source, and installation-blocked states create no replacement work.

Check Run rerequests are current-target-only in v1. A known binding must match configured App ID, installation, repository, Check Run ID, external ID, target, job, and current generation. The controller re-reads current PR state and creates a new attempt only when the fresh snapshot still maps to the exact bound current target. Historical target rerequest is deferred.

Installation lifecycle events are conservative invalidation hints. Deletion, suspension, and repository removal can cancel affected current jobs and increment generations. Creation, unsuspension, and repository addition are recorded only and cannot restore eligibility without a later PR API reconciliation.

## Alternatives considered

- Trust webhook base/head SHAs directly: rejected because webhook payloads are triggers and hints, not immutable execution authority.
- Let GR-15B2 fetch source immediately: rejected; Contents-read tokens and exact Git ingestion are GR-15B3.
- Emit worker assignments when jobs become `awaiting-runner`: rejected; the hardened worker protocol is GR-15C and no reviewed runtime is currently eligible.
- Support historical Check Run rerequests now: rejected to avoid substituting stale targets or current head commits incorrectly.
- Use branch names, clone URLs, archive URLs, or repository names as authority: rejected; route names are only bounded API path hints, numeric IDs and exact commits are authority.
- Process PRs concurrently in v1: rejected; single-record processing plus a durable per-PR lease is simpler and failure-closed.

## Security considerations

The controller has no App private key and no webhook secret. It transiently uses one-repository `pull-request-read` tokens and closes token leases after the single API read. Tokens are not persisted, logged, placed in IDs, source-import requests, jobs, attempts, errors, or worker-facing contracts.

The GitHub API and broker are trusted dependencies, but API responses remain bounded external platform input. Current PR state is reconciliation authority; it is not proof that source content is safe. A controller database compromise can forge, drop, or reorder jobs and source-import requests. Route renames, private forks, inaccessible heads, delayed installation events, and API ordering remain residual operational risks.

Public PR execution remains prohibited. Jobs are durable planning state only and do not authorize target execution. The gVisor spike and docker-dev runner are not production public-PR runtimes.

## Consequences

GR-15B3 can consume source-import requests at least once without receiving webhook bodies or PR prose. GR-15C can later validate worker results against durable generation/attempt state. GR-15D can register Check bindings and publish advisory neutral checks through a separate Checks-only token path.

## Validation plan

Tests cover the exact PR endpoint, token use, minimal PR snapshots, route hints, generation monotonicity, supersession, duplicate outbox processing, source-import leasing/results, Check Run rerequest current-target-only behavior, installation invalidation, stale worker result rejection, CLI parsing, fuzz seeds, and data-minimization canaries. `make test-github-controller-integration` remains gated and is not runtime validation when skipped.

## References

- ADR 0021: GitHub App advisory boundaries
- ADR 0022: Durable GitHub webhook intake
- ADR 0023: GitHub App credential broker
- GitHub Docs: REST API endpoint `GET /repos/{owner}/{repo}/pulls/{pull_number}`
- GitHub Docs: REST API versioning and installation-token scoping
