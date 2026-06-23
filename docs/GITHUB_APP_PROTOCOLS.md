# GitHub App protocol contracts

GR-15 defines pure logical messages only. There is no HTTP listener, database,
queue, GitHub client, token minting, source fetch, worker execution, or Check Run
publication in this issue.

## VerifiedDelivery

Schema: `glassroot.dev/github-webhook-receipt/v1alpha1`.
Producer: receiver. Consumer: durable inbox/outbox and controller.
Trust: authenticated delivery metadata, not repository safety.
Idempotency key: receiver identity plus `X-GitHub-Delivery`.
Bounds: body digest only, no raw body; bounded header values; explicit matched
secret generation; explicit receivedAt supplied by the receiver.
Retry: duplicate same body is no-op; duplicate different body is conflict.
Forbidden: secrets, signatures, raw JSON, PR prose, URLs, source data.
Terminal states: enqueued, ignored, rejected.

## AnalysisTarget

Schema: `glassroot.dev/github-analysis-target/v1alpha1`.
Producer: controller after API revalidation. Consumer: controller/job planner.
Trust: immutable identity for current decision, not authorization or provenance.
Identity: `target-<sha256>` over length-prefixed installation ID, base repo ID,
head repo ID, PR number, exact base commit, exact head commit, and profile
version.
Forbidden: delivery ID, received time, branch names, sender, title/body,
comments, labels, URLs, credentials.

## AnalysisJob

Schema: `glassroot.dev/github-analysis-job/v1alpha1`.
Producer: controller. Consumer: worker scheduler.
Identity: `job-<sha256>` over target ID, generation, profile version, and
required runner tier. Public PR jobs require `hardened-container` or stronger.
Retry: same immutable target and generation is idempotent; new target increments
generation. Superseded/cancelled jobs cannot publish as current.

## AnalysisAttempt

Schema: `glassroot.dev/github-analysis-attempt/v1alpha1`.
Producer: controller. Consumer: scheduler/worker.
Identity: `attempt-<sha256>` over job ID, target ID, generation, 1-based attempt
number, and reason. Reasons: initial, infrastructure-retry, check-rerequest.
Retry: a retry creates a new attempt instead of mutating a prior attempt.
Bounds: attempts per target are capped.

## WorkerAssignment

Schema: `glassroot.dev/github-worker-assignment/v1alpha1`.
Producer: controller. Consumer: hardened worker.
May contain: attempt ID, target ID, source-store identity, exact base/head
commits, plan digest, required runner tier, execution/evidence limits,
evidence-output capability identifier, deadline, generation, limitations.
Forbidden: GitHub installation token, App private key, webhook secret, PR prose,
comments, labels, sender identity, clone URL, API URL, publisher credential,
Check Run ID unless separately reviewed.
Terminal states: completed, failed, cancelled, lease-expired.

## WorkerResult

Schema: `glassroot.dev/github-worker-result/v1alpha1`.
Producer: worker. Consumer: controller.
May contain: attempt/job/target/generation, completion state, report digest,
manifest digest, policy-application digest, effective disposition, bounded
summary counts, runner tier/capability facts, trusted storage references, and
limitations.
Forbidden: raw logs, raw artifacts, raw events, PR prose, host paths, worker
local URLs, GitHub credentials. WorkerResult cannot select a Check Run
conclusion or arbitrary publisher Markdown.

## PublishCommand

Schema: `glassroot.dev/github-publish-command/v1alpha1`.
Producer: controller after target/generation validation. Consumer: publisher.
May contain: repository numeric ID, head SHA, attempt ID, target ID, generation,
and a validated CheckProjection.
Forbidden: source token, evidence bundle, raw findings/logs/artifacts, worker
host data, source store path, App private key, webhook secret.
Idempotency: publication mapping binds attempt ID, repository ID, Check Run ID,
external ID, generation, and last projection digest. Ambiguous create recovery
must reconcile exact head SHA, fixed name, and exact external ID.

## PublisherReceipt

Producer: publisher. Consumer: controller durable publication state.
May contain: repository ID, attempt ID, Check Run ID, external ID, publication
generation, and projection digest. It is not evidence, provenance, or an
attestation.

## State machines

Delivery: received → verified → persisted → enqueued, or ignored/rejected.
Analysis job: queued → importing-source → planning → awaiting-runner → running →
validating-report → ready-to-publish → completed, with failed/superseded/
cancelled terminal alternatives.
Worker attempt: queued → leased → running → completed/failed/cancelled, with
lease-expired allowing controller-policy retry.
Check publication: absent → create-pending → queued → in-progress →
completion-pending → completed, with cancelled/ambiguous/failed alternatives.
Invalid backward transitions fail closed. A stale generation cannot publish.
