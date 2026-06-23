# GitHub App advisory-check design spike

GR-15 is a design spike and pure protocol-contract implementation. It does not
deploy a webhook receiver, register a GitHub App, mint installation tokens, call
GitHub APIs, fetch source, schedule a worker, publish a Check Run, or authorize
public pull-request execution.

Public PR execution remains prohibited until a hardened runner is implemented,
runtime-validated, independently reviewed, and connected through a separately
reviewed worker boundary. A signed webhook is only an authenticated delivery from
GitHub; it does not make repository content safe. An advisory Check Run is not a
security attestation, merge approval, or branch-protection gate.

## Component separation

- **Receiver** holds webhook secret material only. It verifies raw-body HMAC,
  parses bounded headers and JSON, extracts a minimal projection, and durably
  records inbox/outbox state. It has no App private key and performs no GitHub
  API call, source fetch, execution, or publication inline.
- **Controller** consumes verified deliveries, asks a credential broker for a
  repository-scoped read token, re-reads current PR state from GitHub, reconciles
  immutable targets, and emits worker assignments only when an eligible hardened
  runner exists. Webhook SHAs are hints until this revalidation succeeds.
- **Source ingester** is a future read-only component that receives repository
  numeric identity, exact commit IDs, and a read-token handle. It produces a
  control-plane-created bare Git store and never executes repository code.
- **Worker** receives immutable source/store identities, a plan or plan digest,
  runner requirements, limits, and an evidence-output capability. It receives no
  GitHub credential and cannot publish.
- **Publisher** is the only component with Checks write access. It receives a
  validated `CheckProjection` and a downscoped Checks token. It cannot access
  source stores, worker hosts, evidence bundles, logs, artifacts, or sandbox
  state.
- **Credential broker** is the only holder of the GitHub App private key. Tokens
  are opaque, short-lived, repository-scoped, permission-downscoped, and never
  placed in IDs, queue messages, evidence, reports, logs, or worker assignments.

## Webhook intake

The future receiver must preserve duplicate header values, require exactly one
`X-GitHub-Delivery`, `X-GitHub-Event`, `X-Hub-Signature-256`, and
`Content-Type`, and reject duplicate required headers. `Content-Type` must be
`application/json` with only an optional `charset=utf-8` parameter.
`Content-Encoding` must be absent or `identity`. Source IP, User-Agent, PR
sender, branch names, labels, comments, titles, and bodies are not authority.

The signature contract verifies the exact raw request body using HMAC-SHA256 and
requires `sha256=<64 lowercase hex>`. SHA-1-only signatures, uppercase hex,
whitespace, duplicate signatures, malformed prefixes, and extra tokens are
rejected. One current secret and one previous rotation secret are supported.
Previous-secret acceptance is explicit in receipt metadata.

Signed JSON is preflighted before projection: valid UTF-8, no BOM, no raw NUL,
one top-level object, no trailing JSON value, no duplicate object member names at
any depth, and bounded depth/tokens/members/arrays/strings/numbers. Unknown
GitHub fields are ignored after preflight and never influence security values.

## Delivery replay and at-least-once semantics

Glassroot does not claim exactly-once delivery. The replay key is the configured
receiver identity plus `X-GitHub-Delivery`.

- First valid delivery: durable inbox record and downstream outbox publication.
- Same delivery and same body digest: duplicate-same-body; no duplicate job.
- Same delivery and different body digest: delivery-conflict; no job.
- Invalid signatures are never inserted as verified and never enqueued.
- Signed unsupported events may be durably recorded as ignored and acknowledged.

The receiver should return `202 Accepted` only after durable acceptance. Future
storage needs an atomic inbox/outbox transaction, a pending-publication recovery
state, and replay retention. Retention expiry does not authorize stale execution;
controller revalidation and generation checks remain a second defense.

## Payload projection and controller authority

`pull_request` projections retain only action, installation ID, repository
numeric IDs, PR number, event-reported base/head SHAs, draft/closed/merged state,
and head repository numeric ID. `check_run` projections retain action,
installation ID, repository ID, check-run ID, head SHA, App ID, and external ID.
Installation projections retain only installation/repository numeric identities.
PR prose, branch names, user logins, URLs, labels, comments, commit messages,
patch text, and review text are not retained.

A future controller must query current PR state through a repository-scoped read
token and require matching installation, repository ID, and PR number. Closed or
draft PRs do not schedule new execution. Inaccessible or deleted heads produce a
neutral unavailable advisory result rather than broadening credential access or
using unauthenticated URLs.

## Immutable target, job, and attempt identity

`AnalysisTarget` binds installation ID, base repository ID, head repository ID,
PR number, exact base commit, exact head commit, and analysis profile version.
Its ID uses domain-separated, length-prefixed SHA-256 and excludes delivery ID,
time, sender, title/body, branch names, and comments.

Jobs bind target ID, controller generation, profile version, and required runner
tier. Attempts bind job ID, target ID, generation, 1-based attempt number, and a
reason (`initial`, `infrastructure-retry`, or `check-rerequest`). Duplicate
webhooks for the same target do not create another ordinary job. A base/head or
fork-identity change creates a different target and increments the PR generation.

## Supersession and rerequest

The controller maintains a monotonic generation per installation, base
repository, and PR number. `opened`, `reopened`, `ready_for_review`, and
`synchronize` reconcile current PR state and schedule the current immutable
target when eligible. `converted_to_draft` and `closed` cancel queued work and
request best-effort cancellation of running work. A stale worker result may be
retained but cannot publish as current.

Only `check_run` action `rerequested` is processed. The check run must exist in
Glassroot's durable mapping, belong to the configured App ID, match repository
and installation IDs, and match the stored external ID. Rerequest targets the
immutable target originally associated with that Check Run; it does not silently
substitute the PR's current head SHA. Custom requested actions are unsupported.

## Worker and publisher boundaries

Worker assignments may contain only immutable attempt/target/source identities,
plan digest, required hardened runner capabilities, limits, generation, and an
evidence-output capability identifier. They must not contain GitHub tokens,
private keys, webhook secrets, PR prose, comments, labels, clone/API URLs,
publisher credentials, or Check Run credentials. Fake, docker-dev, and
`development-only` tiers are rejected for public PR jobs.

Worker results contain digests, completion state, disposition, summary counts,
runner tier/capability facts, storage references from trusted control-plane
storage, and limitations. Results cannot choose Check Run conclusions or
publisher Markdown. The controller validates target and generation before
creating a publish command.

## Advisory Check Run projection

The fixed Check Run name is `Glassroot advisory`. v1 supports statuses
`queued`, `in_progress`, and `completed`. Every Glassroot policy disposition
(`passed`, `requires-review`, `failed`) and coherent incomplete evidence maps to
GitHub conclusion `neutral`; superseded/cancelled maps to `cancelled`. v1 does
not use `success`, `failure`, `action_required`, or `timed_out` as policy
mappings. The disposition remains visible inside fixed summary fields.

The projection has no annotations, requested actions, images, or untrusted links.
`details_url` is absent unless a later implementation provides a trusted
publisher-owned HTTPS origin. Maintainers must not configure this advisory Check
Run as a blocking safety gate.

Future REST requests must use GitHub App installation authentication, `Accept:
application/vnd.github+json`, and explicit `X-GitHub-Api-Version: 2026-03-10`.
GR-15 adds no HTTP/API implementation.

## Remaining implementation work

GR-15A implements bounded HTTP intake and a durable inbox/outbox without GitHub
API credentials or execution. GR-15B implements controller revalidation,
credential brokering, and source ingestion. GR-15C implements a hardened-worker
protocol. GR-15D implements the advisory Check Run publisher. Public execution
stays disabled until the hardened runner and worker boundary pass independent
human review.
