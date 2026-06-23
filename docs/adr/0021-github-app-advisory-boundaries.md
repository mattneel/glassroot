# ADR: GitHub App advisory boundaries

## Status

Accepted for design-spike guidance. Not a deployment or public-execution approval.

## Date

2026-06-23

## Owners

@mattneel

## Context

Glassroot will eventually integrate with GitHub through a GitHub App and advisory
Check Runs. GR-14 and M4 remain runtime-validation pending, and public PR
execution is still prohibited. GR-15 can define pure protocol contracts and
least-privilege boundaries, but it must not deploy a webhook receiver, mint
GitHub credentials, fetch source, schedule workers, call GitHub APIs, or publish
Check Runs.

GitHub webhooks are at-least-once, may be duplicated or delivered out of order,
and contain hostile repository-controlled prose. A valid webhook signature proves
only that GitHub delivered the raw body with the shared secret; it does not make
repository content safe. Check Run conclusions are GitHub UI state, not
Glassroot security attestations.

## Decision

Add `internal/githubapp` with pure bounded contracts for:

- exact initial permission and webhook subscription inventories;
- raw-body HMAC-SHA256 signature verification with current/previous secret
  rotation;
- bounded header parsing and JSON preflight;
- minimal webhook projections;
- delivery receipts and replay/idempotency decisions;
- immutable analysis target, job, attempt, and Check external IDs;
- PR generation/supersession and Check Run rerequest decisions;
- credential-free worker assignment/result contracts;
- publisher-only Check projection contracts;
- explicit delivery/job/attempt/publication state machines.

The initial App permission set is repository Metadata read, Pull requests read,
Contents read, and Checks write. Organization permissions and user authorization
are absent. The initial webhook set is pull_request, check_run, check_suite,
installation, installation_repositories, and ping. Unsupported events/actions
never schedule work.

A future implementation uses one GitHub App registration with separated runtime
components. Tokens are minted only by a credential broker and downscoped by
repository and permission for receiver/controller/source-ingester/publisher
needs. Workers receive no GitHub credential.

All Glassroot policy outcomes map to advisory Check Run conclusion `neutral` in
v1; superseded/cancelled maps to `cancelled`. No annotations, requested actions,
untrusted details URLs, comments, reviews, labels, statuses, workflow dispatch,
merge behavior, signing, or attestation are introduced.

## Security considerations

Webhook verification uses exact raw bytes, `X-Hub-Signature-256`, lowercase
`sha256=<hex>`, bounded secrets, and constant-time comparison. Payloads are
preflighted for duplicate JSON members and bounds before explicit projection.
Unknown GitHub fields are ignored and never drive security decisions.

The webhook payload is only a trigger and hint. The controller must re-read
current PR state through a repository-scoped read token before building an
immutable target. Older events cannot roll back the active generation. Stale
worker results cannot publish as current. Rerequested checks bind to the original
immutable target rather than silently using the current branch.

The publisher consumes only validated projections and a Checks write token. It
cannot access source stores, worker hosts, evidence bundles, logs, artifacts, or
sandbox state. A compromised component can violate its own boundary, so future
implementation must isolate receiver, broker, controller, source ingester,
worker, and publisher privileges.

## Alternatives considered

- **Single monolithic GitHub service.** Rejected because it would mix webhook
  secrets, App private key, source read tokens, execution scheduling, evidence,
  and Checks write authority.
- **Comment or label driven execution.** Rejected because PR prose and labels are
  repository/user-controlled triggers and would add social-engineering and
  authorization ambiguity.
- **Commit statuses instead of Checks.** Rejected for v1 because Checks provide a
  dedicated App-owned advisory surface; Commit statuses are not requested.
- **Blocking success/failure conclusions.** Rejected because Glassroot does not
  yet authorize public execution and v1 Check Runs are advisory only.
- **Using webhook SHAs as final source authority.** Rejected because current PR
  state and accessibility must be revalidated through authenticated API reads.

## Consequences

The design provides deterministic, testable protocol boundaries without exposing
public execution. It adds no dependencies and leaves existing model, evidence,
policy, report, runner, CLI, and workflow behavior unchanged. Future M5 work can
implement receiver, controller, hardened worker protocol, and publisher in
separate reviews.

Residual risks include webhook retention policy, GitHub API ordering and
inaccessible-fork semantics, ambiguous Check Run create recovery, credential
broker compromise, queue semantics, future hardened-runner eligibility, and
operator configuration mistakes.

## Operational and migration impact

No deployment changes occur in GR-15. Future rollout phases:

1. GR-15A: bounded HTTP webhook receiver and durable inbox/outbox.
2. GR-15B: controller, credential broker, source ingestion, and exact PR-state
   reconciliation.
3. GR-15C: hardened worker protocol with no GitHub credentials.
4. GR-15D: advisory Check Run publisher with Checks-only downscoped tokens.

Public PR execution remains disabled until M4 hardened-runner validation and
independent human review are complete.

## Validation plan

Unit tests cover permission inventory, subscription/action matrix, official
HMAC vector, header duplication, JSON duplicate members, projection minimization,
replay decisions, PR supersession, rerequest validation, credential-boundary
serialization, state transitions, Check projection, and deterministic IDs. Fuzz
targets cover signature parsing, JSON preflight, webhook projection, replay,
target encoding, and advisory Check projection.

## References

- GitHub App permissions: <https://docs.github.com/en/apps/creating-github-apps/setting-up-a-github-app/choosing-permissions-for-a-github-app>
- Installation tokens: <https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-an-installation-access-token-for-a-github-app>
- Webhook validation: <https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries>
- Webhook events and headers: <https://docs.github.com/en/webhooks/webhook-events-and-payloads>
- Check Runs REST API: <https://docs.github.com/en/rest/checks/runs>
- REST API versioning: <https://docs.github.com/en/rest/about-the-rest-api/api-versions>
