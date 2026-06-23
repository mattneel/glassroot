# GitHub App REST API boundary (GR-15B1)

GR-15B1 contains the first Glassroot GitHub REST client. It is intentionally narrow and exists only to authenticate the configured GitHub App, inspect one installation, and mint one repository-scoped read token.

## Fixed origin and headers

The only origin is:

```text
https://api.github.com
```

GitHub Enterprise, arbitrary hosts, user-selected base URLs, redirects, environment proxies, cookies, GraphQL, upload URLs, and repository/source endpoints are unsupported in v1.

Every request sends:

- `Accept: application/vnd.github+json`
- `X-GitHub-Api-Version: 2026-03-10`
- a fixed bounded `User-Agent`
- `Authorization: Bearer <app-jwt>`

The transport uses HTTPS, TLS 1.2 minimum, no proxy-from-environment, no redirect following, bounded dial/TLS/header/overall timeouts, bounded idle pools, and no automatic retry of ambiguous token creation.

## Endpoint inventory

Allowed endpoints are exactly:

| Method | Path | Authentication | Purpose |
| --- | --- | --- | --- |
| GET | `/app` | App JWT | Verify configured App identity and advisory permissions |
| GET | `/app/installations/{installation_id}` | App JWT | Inspect one installation before minting |
| POST | `/app/installations/{installation_id}/access_tokens` | App JWT | Mint one purpose- and repository-scoped token |

No repository, pull-request, contents, Git data, Checks, token revocation, listing, GraphQL, or upload endpoint exists in GR-15B1.

## Request bodies

Installation-token requests are compact deterministic JSON with exactly:

- `repository_ids`: one positive repository numeric ID;
- `permissions`: exactly one purpose mapping (`pull_requests: read` or `contents: read`).

Permission omission is forbidden because GitHub would otherwise grant all App permissions available to the installation. Repository omission is forbidden because GitHub would otherwise grant all repositories available to the installation.

## Response handling

Responses are bounded before allocation. The client rejects unsupported `Content-Encoding`, invalid UTF-8, duplicate JSON members, trailing JSON, oversized bodies, malformed success responses, redirects, and unsupported API-version responses. Unknown additive GitHub fields are ignored after bounded structural preflight. Only typed fields needed for identity, installation, permission, repository scope, token, and expiry validation are retained.

Status handling distinguishes 401/403/404/422/429/5xx and malformed success responses. Rate limits are surfaced; the client does not sleep automatically. Raw response bodies, authorization headers, tokens, account names, and repository names are not exposed in errors.

## Deferred work

GR-15B2 will use `pull-request-read` tokens for controller reconciliation. GR-15B3 will use `source-read` tokens for exact source ingestion into control-plane-created Git stores. GR-15D requires a separate reviewed Checks-write publisher token purpose; it is intentionally absent here.
