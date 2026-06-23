# GitHub source ingestion (GR-15B3)

GR-15B3 adds a narrow source-ingester service that consumes credential-free GR-15B2 source-import requests and imports exact Git objects into control-plane-created bare stores. It performs no planning, materialization, worker assignment, target execution, Check Run publication, or public PR authorization.

## Service CLI

```text
glassroot-source-ingester version

glassroot-source-ingester serve \
  --controller-state-dir ABSOLUTE_DIRECTORY \
  --receiver-id RECEIVER_ID \
  --controller-id CONTROLLER_ID \
  --app-id POSITIVE_INTEGER \
  --source-root ABSOLUTE_DIRECTORY \
  --source-ingester-id SOURCE_INGESTER_ID \
  --credential-broker-unix ABSOLUTE_SOCKET_PATH \
  --git-executable ABSOLUTE_GIT_PATH
```

There are no defaults, environment-variable equivalents, token flags, repository flags, branch/ref flags, GitHub host flags, proxy flags, worker flags, publisher flags, or current-directory fallbacks.

## Source request authority

The durable GR-15B2 source request is the authority. It contains target ID, job ID, generation, installation ID, pull-request number, base/head repository numeric IDs, bounded base route hints, and exact base/head commit IDs. It contains no token, URL, branch name, PR prose, worker identity, source path, or Check Run credential.

Route owner/name values are transport hints only. Numeric repository IDs and exact commit IDs remain identity authority.

## Credential and network behavior

For each current leased request, the service asks the GR-15B1 credential broker for exactly one token:

- purpose: `source-read`;
- installation ID: from the source request;
- repository ID: base repository ID from the source request.

It never asks for pull-request-read, Checks-write, a head-repository token, a combined token, or an all-repository token. It never falls back to unauthenticated public fetch.

The only source transport is Git smart HTTP to:

```text
https://github.com/<base-owner>/<base-repository>.git
```

No GitHub Enterprise host, SSH transport, `git://`, `file://`, `ext`, local path, archive, tarball, zipball, patch, or diff import exists in v1. Redirects and proxies are disabled.

## Fetch strategy

The importer initializes a fresh bare repository and fetches exactly:

```text
+<expected-base-commit>:refs/glassroot/base
+refs/pull/<pull-request-number>/head:refs/glassroot/head
```

`refs/pull/<number>/head` is only a transport locator. After fetch, Glassroot requires `refs/glassroot/head` to equal the exact controller-authorized head commit. A moved pull-request ref fails closed as stale source. No merge ref, branch name, tags, submodules, or LFS objects are fetched.

## Git command and environment boundary

Production source import uses typed Git command construction only:

- `git init --bare --object-format=<sha1|sha256> --template <empty-template> <repository.git>`;
- `git fetch --depth=1 --no-tags --no-write-fetch-head --no-recurse-submodules --no-auto-maintenance --no-write-commit-graph <remote> <exact-refspecs>`;
- existing GR-6A read/verification commands.

The absolute configured Git executable is used. No shell, PATH-selected Git, repository-supplied executable, alias, clone, pull, checkout, worktree, archive, submodule, LFS, push, commit, merge, reset, gc, repack, prune, credential helper, or repository-defined command is used.

The child environment is allowlisted with fixed locale/timezone, `/dev/null` global/system Git config, disabled prompts, disabled optional locks, protocol v2, LFS smudge skipped, and no inherited proxy, SSH, askpass, trace, or SSL-disable variables.

The installation token is converted inside `TokenLease.Use` into an HTTP Basic `Authorization` header and placed only in `GLASSROOT_GIT_AUTHORIZATION_HEADER`. Git receives it through URL-scoped `--config-env=...extraHeader=GLASSROOT_GIT_AUTHORIZATION_HEADER`. The token is not placed in Git argv, remote URL, Git config files, refs, source metadata, controller records, logs, errors, IDs, or worker-facing contracts. The token and derived buffers are best-effort overwritten after use.

Residual risk: the token exists transiently in Glassroot process memory, Git process memory, Git HTTPS-helper memory, and the trusted child environment. Same-UID host compromise may inspect these on some systems. This is a control-plane deployment limitation, not permission to pass credentials to workers.

## Shallow-store semantics

The import profile is:

```text
glassroot.dev/github-source-import/smart-http-shallow/v1alpha1
```

It uses `--depth=1` to import the complete tree/blob closure for the selected base and head commits while omitting unrelated history. Git partial clone, promisor remotes, lazy fetching, alternates, filters, tags, submodule traversal, and LFS object fetching are unsupported. LFS pointer files remain ordinary Git blobs; gitlinks remain tree entries only.

A shallow store is sufficient for exact revision materialization and object reads for the selected commits. It is not a complete repository-history source and must not be used for arbitrary history analysis.

## Failure and retry behavior

Retryable failures include transient broker unavailability, transport unavailability, database busy errors, and timeout before final publication. Terminal failures include invalid requests, invalid route hints, unsupported object format, missing exact base commit, missing or moved PR head ref, object identity mismatch, unsafe Git metadata, source-store limit breaches, token-scope mismatch, corrupt existing store, or maximum attempts exhausted.

Every retry uses a fresh staging directory and a fresh token unless a valid published store already exists. Git stderr is not used for security-sensitive classification.

## Optional live integration

`make test-github-source-ingester-integration` is gated. It requires explicit test-only broker, installation, repository, PR, commit, and Git executable inputs. A skipped integration is visible and is not runtime validation. The integration performs no materialization, execution, image pull, sandbox run, or publication.

## Non-production status

Successful ingestion does not prove source content is benign. A commit ID is not authentication or provenance. A control-plane-created store is not safe to execute. Public PR execution remains prohibited until GR-15C/GR-15D and hardened runtime validation/review are complete.
