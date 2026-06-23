# ADR: Durable GitHub webhook intake

## Status

Accepted for GR-15A implementation. Not a public-execution approval.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-15 defined GitHub App advisory boundaries as pure contracts. GR-15A is the
first operational component: a bounded webhook receiver and durable inbox/outbox.
It must preserve the GR-15 separation: no GitHub App private key, no installation
token, no GitHub API call, no source ingestion, no worker scheduling from an HTTP
handler, no Check Run publication, and no target execution.

Webhook delivery is at-least-once. A valid HMAC only authenticates the exact raw
body against the shared webhook secret; repository-controlled payload content
remains hostile and may be duplicated, delayed, or out of order. Returning 202
before durable state commits would risk silently dropping accepted deliveries.

## Decision

Add:

- `internal/githubreceiver` for the Unix-socket HTTP intake, secret-file loading,
  raw-body HMAC verification, bounded request handling, fixed responses,
  structured logs, and graceful shutdown;
- `internal/githubinbox` for the SQLite-backed durable inbox/outbox and lease
  state machine;
- `cmd/glassroot-receiver` as a separate service binary.

The receiver listens only on an explicit Unix socket and serves exactly
`POST /webhooks/github`. A separately reviewed TLS reverse proxy is required for
Internet-facing deployment and must preserve raw bytes and headers. The receiver
never trusts source IP or proxy identity headers.

Secrets are loaded from protected files only. Current is required; previous is
optional for rotation. Secret bytes are not trimmed, logged, persisted, or sent to
workers. Previous-secret acceptance is stored only as an enum.

The handler verifies required headers, reads a bounded raw body, verifies
HMAC-SHA256, preflights JSON, builds a minimal projection and receipt, and calls
the store. It returns 202 only after the atomic inbox/outbox transaction commits.
Unsupported signed events/actions are persisted as ignored and create no outbox
work.

The store uses `modernc.org/sqlite v1.53.0` with WAL, `synchronous=FULL`, foreign
keys, `trusted_schema=OFF`, recursive triggers disabled, bounded busy timeout,
and bounded database size. It uses a fixed state-dir database name and schema
identity `glassroot.dev/github-inbox-store/v1alpha1`.

## Dependency decision

`modernc.org/sqlite` was chosen because it is CGO-free and works through the
standard `database/sql` interface. It avoids a CGO SQLite dependency in the
receiver binary. Its risks are the generated translated SQLite/libc surface,
module size, and the documented fragile `modernc.org/libc` coupling; GR-15A pins
`modernc.org/sqlite v1.53.0` and accepts the module-selected
`modernc.org/libc v1.73.4` without independent override.

Rejected alternatives:

- custom filesystem spool: harder to make transactional with outbox leasing and
  replay conflict handling;
- bbolt: single-purpose key/value store would require more custom transaction and
  query structure;
- external PostgreSQL: larger deployment and credential surface for the initial
  local receiver;
- ORM/migration framework: unnecessary dependency and larger trusted computing
  base.

## Security considerations

Raw webhook bodies, signatures, secrets, PR prose, branch names, labels,
comments, commit messages, sender names, URLs, and patch text are never persisted.
Only compact typed receipts and projections are stored. The durable database is
trusted control-plane state; database corruption or filesystem compromise can
forge, drop, or reorder deliveries.

The replay key is receiver ID plus GitHub delivery ID. The intake fingerprint
binds receiver ID, delivery ID, event name, body digest, and projection kind.
Duplicate same fingerprint is idempotent. Conflicting fingerprint preserves the
original and creates no outbox work. This is at-least-once processing, not
exactly-once delivery.

Outbox leases are durable and explicit. Expired leases are reclaimable. A stale
owner/generation cannot acknowledge a newer lease. A future controller must be
idempotent and must revalidate PR state through GitHub before scheduling any
source or worker work.

## Consequences

GR-15A creates a local operational receiver while preserving public-execution
prohibitions. It adds one direct database dependency and no GitHub API client,
credential broker, source fetcher, worker, publisher, deployment manifest, or
TCP listener.

Operational residual risks include reverse-proxy byte preservation, SQLite and
filesystem durability, secret rotation timing, database growth without pruning,
multi-process contention, retention policy, backup restore semantics, and future
controller lease handling.

## Work deferred

GR-15B implements controller reconciliation, credential brokering, and source
ingestion. GR-15C implements the hardened-worker protocol. GR-15D implements the
advisory Check Run publisher. Public PR execution remains prohibited until the
hardened runner and worker boundary are runtime-validated and independently
reviewed.
