# GitHub controller reconciliation (GR-15B2)

GR-15B2 adds `glassroot-controller`, the first controller component for the GitHub App path. It consumes authenticated GR-15A receiver outbox records at least once, asks the GR-15B1 credential broker only for repository-scoped `pull-request-read` tokens, re-reads current pull-request state through one GitHub REST endpoint, and records durable target/job/source-import state.

It is not a public-execution approval. It performs no source ingestion, creates no Git store, emits no `WorkerAssignment`, executes no target code, publishes no Check Run, and does not make a sandbox, provenance, authentication, attestation, or safety claim.

## Service CLI

```text
glassroot-controller version

glassroot-controller serve \
  --inbox-state-dir ABSOLUTE_DIRECTORY \
  --receiver-id RECEIVER_ID \
  --controller-state-dir ABSOLUTE_DIRECTORY \
  --controller-id CONTROLLER_ID \
  --credential-broker-unix ABSOLUTE_SOCKET_PATH \
  --app-id POSITIVE_INTEGER
```

There are no defaults, environment-variable equivalents, GitHub token flags, App private-key flags, webhook-secret flags, source-ingester flags, worker flags, publisher flags, runner-selection flags, public-execution flags, or arbitrary lease/poll tuning flags.

The command opens the GR-15A inbox through `internal/githubinbox`, opens its own controller store, dials the GR-15B1 broker over the explicit Unix socket, constructs the fixed GitHub.com installation-token REST client, then claims one receiver outbox record at a time.

## Inbox consumption

The controller uses GR-15A outbox leases with at-least-once semantics. For each claimed record:

1. Validate the leased projection and projection digest again.
2. Acquire a per-PR reconciliation lease when the event relates to a pull request.
3. Obtain a `pull-request-read` token from the broker only for the durable installation ID and base repository numeric ID.
4. Re-read current PR state through GitHub.
5. Commit controller state and any source-import request.
6. Acknowledge the receiver outbox only after controller commit succeeds.

Transient broker, GitHub, lease, or database failures release the receiver outbox lease with a stable code. A crash after controller commit but before receiver acknowledgement reprocesses idempotently through the processed-outbox table and does not duplicate targets, jobs, attempts, or source-import requests. This is at-least-once processing, not exactly-once delivery.

## GitHub API authority

Webhook fields are hints. Event-reported base/head SHAs and actions never become target authority. The controller revalidates current PR state with:

```text
GET /repos/{owner}/{repo}/pulls/{pull_number}
```

using `Authorization: Bearer <installation-token>`, `Accept: application/vnd.github+json`, and `X-GitHub-Api-Version: 2026-03-10`.

The only retained API authority fields are:

- PR number;
- `state`, `draft`, and `merged`;
- base repository numeric ID;
- base owner/name route hints;
- exact base commit ID;
- head availability;
- head repository numeric ID and route hints when available;
- exact head commit ID when available.

The controller does not retain PR title/body, branch refs, labels, comments, sender identity, URLs, diff/patch text, merge commit SHA, reviewer data, or commit messages.

Route owner/name strings are bounded routing hints, not identity authority. Numeric repository IDs and exact commit IDs form durable target identity.

## PR eligibility and generation rules

A PR is eligible only when the current API snapshot is:

- open;
- not draft;
- not merged/closed;
- base repository ID matches the durable projection;
- base commit ID is exact;
- head repository is available;
- head repository ID is positive;
- head commit ID is exact.

Draft, closed, merged, inaccessible/deleted head, route mismatch, missing commits, or token/API unavailability create no worker assignment.

The durable PR key is `(installation ID, base repository ID, pull request number)`. Generations start at 1 and never decrease.

- First eligible snapshot creates generation 1.
- Duplicate reconciliation of the same current target does not increment.
- Base/head/target changes increment and supersede the old current job.
- Eligible to draft/closed/unavailable increments and cancels current work.
- Draft/closed/unavailable back to eligible increments even if the historical target ID matches an older target.
- Installation deletion/suspension/removal hints conservatively invalidate affected current jobs but never restore or schedule work.

## Target, job, attempt, and source-import records

The controller uses the GR-15 `AnalysisTarget` identity from API snapshot fields only:

- installation ID;
- base repository ID;
- head repository ID;
- PR number;
- exact base commit;
- exact head commit;
- fixed controller/analysis profile version.

Target IDs are deterministic operational identifiers. They are not authorization, provenance, authentication, attestation, or proof of source safety.

For a newly eligible generation, the controller creates:

- one immutable target;
- one immutable job in `importing-source`;
- one initial attempt with reason `initial` and state `queued`;
- one credential-free source-import request for GR-15B3.

Required future runner tiers are recorded as an explicit set containing `hardened-container` and `microvm`. The controller emits no worker assignment and does not consider docker-dev, fake, or development-only runners eligible.

## Rerequest policy

Only `check_run.rerequested` is processed. The controller requires a durable Check Run binding for the configured App ID, repository ID, Check Run ID, external ID, target ID, job ID, PR number, and controller generation.

For a known binding, the controller acquires the PR reconciliation lease, obtains a fresh `pull-request-read` token, re-reads current PR state, and creates a new attempt only when the fresh snapshot still maps to the exact current bound target. Historical/stale target rerequests are ignored in v1 and cannot replace the current generation. `requested_action` and unknown Check Runs create no execution.

## Installation lifecycle hints

Installation and installation-repository events are conservative invalidation hints. Deletion, suspension, and explicit repository removal can cancel current affected jobs and increment affected PR generations. Creation, unsuspension, and repository addition are recorded only; a later PR reconciliation must revalidate eligibility before new work exists. No installation-list API is added and no token is minted for lifecycle hints alone.

## Source and worker boundaries

Source-import requests contain numeric repository IDs, pull-request number, bounded route hints, exact commits, target/job/generation IDs, state, and lease metadata. They contain no GitHub token, App private key, webhook secret, API URL, clone/archive URL, branch/ref, PR prose, worker identity, or Check Run credential.

Future source results and worker results are freshness-checked against request ID, target ID, job ID, generation, current PR state, attempt state, and runner tier. Stale source/worker results cannot make superseded work current. Because GR-15B2 emits no assignment, otherwise well-formed hardened-container or microvm worker results remain `unexpected-result` until GR-15C.

## Shutdown and retries

The service processes synchronously with one claimed inbox record at a time, no unbounded in-memory queue, no goroutine pool, no HTTP listener, no metrics/debug/pprof endpoint, no source access, and no background worker scheduler. SIGINT/SIGTERM stops claims and exits after bounded cleanup.

## Optional integration

`make test-github-controller-integration` is gated. It requires explicit test infrastructure and is skipped by default. The intended live harness seeds one GR-15A outbox record, processes it through the controller, verifies current API state, creates one target/job/source request, and performs no source ingestion or execution.
