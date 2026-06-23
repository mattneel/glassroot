# GitHub App permissions and subscriptions

Schema: `glassroot.dev/github-app-permissions/advisory/v1alpha1`.

## Requested repository permissions

| Permission | Access | Component use |
| --- | --- | --- |
| Metadata | read | GitHub-required baseline metadata access. |
| Pull requests | read | Controller revalidates PR number, draft/closed state, and exact base/head metadata. |
| Contents | read | Future source ingestion reads exact objects without clone/fetch from worker code. |
| Checks | write | Publisher creates/updates advisory Check Runs only. |

One GitHub App registration has the union of these permissions. Future
installation tokens must be downscoped by repository and permission for each
component. The publisher should receive Checks-only access when GitHub supports a
narrower token for the operation. Workers receive no GitHub credential.

## Explicitly absent repository permissions

Actions, Administration, Codespaces, Commit statuses, Deployments, Environments,
Issues, Members, Packages, Pages, Repository hooks, Repository projects, Secret
scanning, Secrets, Security events, and Workflows are absent. Organization and
account permissions are absent. User authorization is disabled and no user OAuth
scope is requested.

Pull requests share some GitHub UI concepts with issues, but Glassroot does not
request Issues permission. It does not request Commit statuses because the
initial publisher uses advisory Check Runs. It does not request Actions,
Workflows, or Administration.

A new permission requires version review, ADR update, threat-model update, and
explicit human approval.

## Webhook subscription profile

Schema: `glassroot.dev/github-app-webhooks/advisory/v1alpha1`.

| Event | Action | Decision |
| --- | --- | --- |
| pull_request | opened | schedule after API revalidation |
| pull_request | reopened | schedule after API revalidation |
| pull_request | synchronize | schedule/supersede after API revalidation |
| pull_request | ready_for_review | schedule after API revalidation |
| pull_request | converted_to_draft | cancel/supersede |
| pull_request | closed | cancel/supersede |
| pull_request | other known/unknown actions | deterministic no-op |
| check_run | rerequested | validate durable mapping, then new attempt for exact historical target |
| check_run | other actions | deterministic no-op |
| check_suite | any action | accepted safely, no primary scheduling |
| installation | lifecycle actions | reconciliation only |
| installation_repositories | lifecycle actions | reconciliation only |
| ping | n/a | operational no-op |

Absent initial triggers: push, issue_comment, issues, pull_request_review,
pull_request_review_comment, workflow_run, workflow_job, repository_dispatch,
merge_group, deployment, and status. There are no slash commands, label
triggers, branch-name triggers, Check Run buttons, or custom requested actions in
v1. Merge-queue support is deferred and must not be approximated through push
events.

## Component data access

| Component | May access | Must not access |
| --- | --- | --- |
| Receiver | webhook secret, raw delivery during verification, minimal projection | App private key, installation token, source, worker host, publisher token |
| Credential broker | App private key and token minting logic | workload data, sandbox, worker evidence content |
| Controller | verified delivery, read-token handle, current PR metadata, target/job state | sandbox, publisher token, raw workload logs/artifacts |
| Source ingester | repository-scoped read token, exact repository/commit IDs | Checks token, worker sandbox, target execution |
| Worker | immutable job assignment, source-store identity, evidence-output capability | any GitHub token, App key, webhook secret, publisher credentials |
| Publisher | Checks write token, validated CheckProjection, repository numeric identity | source token, source store, evidence bundle, worker host, sandbox |

Tokens are opaque secrets. Token strings never enter IDs, logs, errors, queue
messages, reports, evidence, or worker assignments.
