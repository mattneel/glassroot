# GitHub controller store (GR-15B2)

`internal/githubcontrollerstore` is the durable SQLite state store for GR-15B2. It records processed receiver outbox records, per-PR reconciliation leases, current PR generations, immutable targets, jobs, attempts, credential-free source-import requests, Check Run bindings for future publisher rerequests, and stale-result classifications.

It stores no GitHub token, App private key, webhook secret, raw webhook body, raw GitHub API response, PR prose, branch name, clone/archive URL, logs, artifacts, source content, or host path.

## State directory and SQLite contract

The controller state directory is trusted control-plane state. It must be absolute, clean, UTF-8, control-free, existing, non-symlink, owned by the current effective UID on Linux, and mode `0700`. The fixed database path is:

```text
<controller-state-dir>/github-controller.sqlite
```

SQLite is configured with WAL, synchronous FULL, foreign keys, trusted schema disabled, recursive triggers disabled, bounded busy timeout, bounded database size, and a small bounded connection pool. SQL values are parameterized. Startup initializes or verifies metadata, then runs `quick_check`; corrupt or mismatched stores fail closed and are not repaired automatically.

Schema identity:

```text
glassroot.dev/github-controller-store/v1alpha1
```

Controller profile:

```text
glassroot.dev/github-controller-profile/advisory/v1alpha1
```

Metadata binds schema identity/version, controller ID, receiver ID, configured App ID, and controller profile. Reopening with mismatched identity fails closed.

## Processed delivery idempotency

Processed receiver outbox records are keyed by receiver ID and inbox outbox ID. Each record stores the delivery ID, projection kind, projection digest, decision, optional generation/target/job IDs, processed time, and a record-integrity digest.

The same outbox ID and projection digest returns `duplicate-processed` without creating another controller effect. The same outbox ID with a different projection digest is a controller invariant failure. Delivery IDs are not target/job identity inputs.

## PR state and generations

Current PR state is keyed by installation ID, base repository ID, and PR number. It records generation, eligibility, current target/job IDs, bounded base route hints, exact base/head commit IDs, head repository ID, and update time.

Eligibility values are:

- `eligible`
- `draft`
- `closed`
- `source-unavailable`
- `installation-blocked`
- `route-unavailable`

Generations are monotonic signed 63-bit integers. Historical targets/jobs/attempts are retained; no automatic pruning exists in v1.

## Immutable targets, jobs, and attempts

Targets persist the compact typed GR-15 `AnalysisTarget` JSON. Jobs persist the compact typed job JSON plus required future runner tiers. Attempts persist attempt number, reason, state, and creation time.

New eligible generations create one job in `importing-source`, one initial queued attempt, and one source-import request. Supersession marks old current jobs superseded, queued/running attempts cancelled, and pending/leased source requests superseded. Draft/closed/installation-blocked transitions cancel current jobs and source requests.

## Source-import outbox

Source-import requests use schema:

```text
glassroot.dev/github-source-import-request/v1alpha1
```

Request IDs use domain:

```text
glassroot.dev/github-source-import-request-id/v1\0
```

Each request contains target ID, job ID, generation, installation ID, base/head repository IDs, bounded route hints, exact commits, controller profile version, state, sequence, and lease metadata. Requests contain no credentials and no URLs.

Lease states are `pending`, `leased`, `completed`, `failed`, `superseded`, and `cancelled`. Claims are ordered by durable sequence. Expired leases are reclaimable. Acknowledge/release requires exact owner and lease generation. Superseded/cancelled requests are never claimed.

A current successful source result moves the job to `awaiting-runner` and does not emit a worker assignment. A current failed source result marks the job failed. Stale or mismatched source results cannot advance current work.

## Check binding registry

Future GR-15D can register a binding between App ID, installation ID, repository ID, PR number, Check Run ID, external ID, target ID, job ID, controller generation, and publication generation. Conflicting re-registration fails closed. Bindings permit GR-15B2 to validate `check_run.rerequested` events without trusting webhook hints alone.

## Worker-result freshness

The store validates future worker results against known attempt/job/target/generation and runner tier. Fake, docker-dev, and development-only tiers are rejected. Only `hardened-container` and `microvm` shapes can pass the tier gate, but GR-15B2 still returns `unexpected-result` because it emits no worker assignment.

## Crash recovery and semantics

The store is designed for at-least-once input from GR-15A. Controller state commits before receiver outbox acknowledgement. Reprocessing after a crash observes the processed-outbox record and creates no duplicate target, job, initial attempt, or source request. This is not exactly-once processing; durable uniqueness constraints and idempotent transitions are the defense.
