# ADR: GitHub App credential broker

## Status

Accepted for GR-15B1 implementation. Not a public-execution approval.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-15B originally combined private-key custody, controller reconciliation, and exact source ingestion. Those are distinct privileged boundaries. The GitHub App private key is the most sensitive GitHub credential because it can sign App JWTs and mint installation tokens within the App registration grants. Combining it with controller and source-ingestion state would increase blast radius and make future least-privilege review harder.

GitHub documentation requires RS256 JWTs for App authentication and permits installation-token downscoping by repository IDs and permissions. GitHub also changed installation-token format expectations in 2026, so Glassroot must treat tokens as opaque variable-length secrets.

## Decision

Split GR-15B into:

- GR-15B1: a dedicated credential broker and fixed GitHub App REST boundary;
- GR-15B2: controller reconciliation and durable job state;
- GR-15B3: exact source ingestion into trusted bare Git stores.

GR-15B1 adds `internal/githubauth`, `internal/githubapi`, `internal/githubbroker`, and `cmd/glassroot-credential-broker`.

`internal/githubauth` loads one protected RSA private-key file and signs compact RS256 JWTs with only `iat`, `exp`, and `iss`. It accepts PKCS#1 RSA and PKCS#8 RSA keys, rejects weak or unsupported keys, and performs filesystem identity and stability checks before returning key custody to the broker process.

`internal/githubapi` is the only new package importing `net/http`. It connects only to `https://api.github.com`, sends REST version `2026-03-10`, follows no redirects, uses no environment proxy, and implements only `GET /app`, `GET /app/installations/{id}`, and `POST /app/installations/{id}/access_tokens`.

`internal/githubbroker` exposes a private Unix stream socket with length-prefixed compact JSON. Linux peer UID must match the broker UID. It mints only `pull-request-read` and `source-read` tokens, each restricted to one repository ID and one permission purpose. Tokens are not persisted, logged, placed in IDs, or exposed except through a `TokenLease` callback.

No token cache is added in v1. A fresh JWT per request is acceptable. Token revocation is deferred; minted tokens remain valid until expiry.

## Security considerations

The broker is the sole private-key holder. The receiver, controller, source ingester, worker, and publisher do not receive the private key in GR-15B1. Workers receive no GitHub credential. The publisher receives no token in this issue.

The App registration permission profile remains the outer privilege ceiling: Checks write, Contents read, Pull requests read, and Metadata read. The broker validates that profile at startup. It rejects unexpected App permissions, suspended or mismatched installations, insufficient installation permissions, broader token responses, repository mismatches, and malformed expiries.

The Unix socket is private local IPC, not a general security boundary. Same-UID host compromise can access it. GitHub API responses are external platform input and remain bounded and validated. Tokens are opaque secrets; token possession is not permission to execute source and App authentication is not proof that repository content is safe.

Go memory zeroization is best effort. Private key and token bytes may exist in runtime copies outside direct control. Errors and logs are designed not to include key, JWT, token, authorization header, socket path, file path, response body, account name, or repository name.

## Alternatives considered

- Put key custody in the future controller: rejected because PR reconciliation and durable job state do not need private-key bytes.
- Let source ingestion mint tokens directly: rejected because source ingestion needs only short-lived Contents-read tokens, not the App key.
- Use `go-github` or a JWT package: rejected to avoid new dependencies and to keep the endpoint/JWT surface auditable.
- Support GitHub Enterprise or arbitrary API origins now: rejected to avoid host/proxy trust expansion.
- Cache installation tokens: rejected in v1 because it complicates lifetime, restart, and revocation behavior without being required.
- Add Checks-write token purpose now: rejected; publication is GR-15D and requires a separate review.

## Consequences

The implementation creates a clear credential custody process and a small REST boundary. It adds no root-module dependencies. Operational deployment must protect the key file, private socket parent, broker host, clock accuracy, and GitHub API reachability. Same-UID compromise or broker compromise can mint tokens within App grants.

GR-15B2 and GR-15B3 must use this broker instead of handling private keys. GR-15D must add a separately reviewed publisher token path. Public PR execution remains prohibited.

## Operational and migration impact

Operators must register the GitHub App with the GR-15 advisory permission profile, create a protected private-key file, start the broker with explicit App ID and client ID, and verify startup identity. Key rotation is performed by registering a new GitHub key, replacing the protected file through trusted deployment, restarting, verifying identity, and revoking the old key.

## Validation plan

Unit tests cover protected key parsing, JWT claims/signing, fixed endpoint requests, App/installation permission checks, token response validation, broker protocol framing, Unix socket behavior, token lease ownership, CLI parsing, fuzz seeds, and secret-leak canaries. `make test-github-broker-integration` optionally verifies live `GET /app`, installation inspection, and one repository-scoped read token when explicit credentials are supplied.

## References

- ADR 0021: GitHub App advisory boundaries
- ADR 0022: Durable GitHub webhook intake
- GitHub Docs: Generating a JSON Web Token for a GitHub App
- GitHub Docs: Generating an installation access token for a GitHub App
- GitHub Docs: REST API endpoints for GitHub Apps
