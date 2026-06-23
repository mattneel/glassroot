# GitHub inbox store

GR-15A adds a transactional SQLite inbox/outbox for authenticated GitHub webhook
intake. It is a local durability layer for a future controller; it is not a
queue service, source store, worker scheduler, publisher, attestation, or
exactly-once mechanism.

## Dependency

The store uses `modernc.org/sqlite v1.53.0`, a CGO-free `database/sql` SQLite
driver under BSD-3-Clause. Its own documentation describes it as a CGO-free port
and warns that `modernc.org/libc` should match the version required by the
sqlite module. GR-15A pins the module-selected `modernc.org/libc v1.73.4` and
does not override it independently.

New module-graph additions introduced by this dependency set include:

| Module | Version | License |
| --- | --- | --- |
| github.com/dustin/go-humanize | v1.0.1 | MIT |
| github.com/google/pprof | v0.0.0-20250317173921-a4b03ec1a45e | Apache-2.0 |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause |
| github.com/hashicorp/golang-lru/v2 | v2.0.7 | MPL-2.0 |
| github.com/mattn/go-isatty | v0.0.20 | MIT |
| github.com/ncruces/go-strftime | v1.0.0 | MIT |
| github.com/remyoudompheng/bigfft | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause |
| golang.org/x/mod | v0.36.0 | BSD-3-Clause |
| golang.org/x/sync | v0.20.0 | BSD-3-Clause |
| golang.org/x/sys | v0.44.0 | BSD-3-Clause |
| golang.org/x/tools | v0.45.0 | BSD-3-Clause |
| modernc.org/cc/v4 | v4.28.4 | BSD-3-Clause |
| modernc.org/ccgo/v4 | v4.34.4 | BSD-3-Clause |
| modernc.org/fileutil | v1.4.0 | BSD-3-Clause |
| modernc.org/gc/v2 | v2.6.5 | BSD-3-Clause |
| modernc.org/gc/v3 | v3.1.3 | BSD-3-Clause |
| modernc.org/goabi0 | v0.2.0 | BSD-3-Clause |
| modernc.org/libc | v1.73.4 | BSD-3-Clause plus third-party notices |
| modernc.org/mathutil | v1.7.1 | BSD-3-Clause |
| modernc.org/memory | v1.11.0 | BSD-3-Clause plus Go/mmap notices |
| modernc.org/opt | v0.2.0 | BSD-3-Clause |
| modernc.org/sortutil | v1.2.1 | BSD-3-Clause |
| modernc.org/sqlite | v1.53.0 | BSD-3-Clause |
| modernc.org/strutil | v1.2.1 | BSD-3-Clause |
| modernc.org/token | v1.1.0 | BSD-3-Clause |

The modernc generated-code/libc surface is a dependency risk recorded in ADR
0022. No SQLite extensions are enabled, no extension loading is used, and no SQL
is generated from webhook values.

## State directory and database identity

`--state-dir` is trusted control-plane configuration. It must be absolute,
clean, valid UTF-8, control-free, an existing non-symlink directory, mode `0700`,
and owned by the service UID on Linux. The fixed database path is:

```text
<state-dir>/github-webhook.sqlite
```

Webhook data never chooses this name. The database file is regular and mode
`0600`; WAL and SHM files remain inside the trusted state directory. Network
filesystems and hostile mounts are unsupported.

Schema identity: `glassroot.dev/github-inbox-store/v1alpha1`.
The database metadata binds schema identity, schema version, and receiver ID.
Reopening with a different receiver ID fails closed. Newer unknown schema
versions fail closed; GR-15A includes no automatic pruning or repair.

## SQLite settings

The store configures:

- WAL journal mode;
- `synchronous=FULL`;
- foreign keys enabled;
- `trusted_schema=OFF`;
- recursive triggers disabled;
- bounded busy timeout;
- bounded page count / database-size ceiling;
- bounded connection pool.

Startup runs `quick_check` and validates schema metadata. Corruption is not
repaired automatically and a corrupt database is not silently recreated.

## Inbox record

Schema: `glassroot.dev/github-inbox-record/v1alpha1`.

Persisted fields are limited to receiver ID, delivery ID, raw-body SHA-256 digest,
event name, typed action where applicable, projection kind, disposition, matched
secret generation enum, receiver-supplied UTC `receivedAt`, numeric identities
from the minimal projection, event-reported base/head SHAs where projected,
compact typed receipt JSON, compact typed projection JSON, intake fingerprint,
and record-integrity digest.

Deliberately omitted:

- raw body;
- signature header;
- webhook secret;
- all request headers;
- PR title/body;
- branch names;
- labels, comments, commit messages, user names;
- clone/archive/API URLs;
- patch/diff text;
- sender profile;
- remote IP.

The record-integrity digest detects local corruption of the persisted typed
record. It is not authentication or provenance.

## Intake fingerprint and replay

Intake fingerprint domain:

```text
glassroot.dev/github-webhook-intake/v1\0
```

It binds receiver ID, delivery ID, event name, raw body digest, and projection
kind using length-prefixed SHA-256, rendered as `sha256:<hex>`. It excludes
received time, secret generation, optional proxy metadata, and raw body bytes.

Replay behavior:

- first schedulable/cancellation/rerequest/reconciliation delivery inserts an
  inbox row and one pending outbox row in the same transaction;
- first ignored delivery inserts an inbox row and no outbox row;
- same delivery and same fingerprint is a duplicate and inserts no new outbox;
- same delivery ID with a different fingerprint is a conflict and preserves the
  original record;
- a failure between inbox and outbox insert rolls back the transaction;
- no HTTP 202 is returned before transaction commit succeeds.

## Outbox leasing

Outbox schema: `glassroot.dev/github-controller-envelope/v1alpha1`.
Outbox ID domain:

```text
glassroot.dev/github-outbox-record-id/v1\0
```

It binds receiver ID, delivery ID, intake fingerprint, and projection kind,
rendered as `outbox-<64 hex>`. The outbox payload contains only the minimal typed
projection, projection digest, receipt reference, durable sequence number,
created-at time, state, and lease metadata. It contains no token, raw body,
signature, PR prose, GitHub API URL, execution policy, worker assignment, or
Check Run projection.

States:

- `pending`;
- `leased`;
- `acknowledged`.

`ClaimOutbox` leases pending or expired records in durable sequence order and
increments lease generation and attempt count. `AcknowledgeOutbox` and
`ReleaseOutbox` require exact owner and generation; stale owners cannot
acknowledge newer leases. Acknowledged records are never claimed again. There is
no background reaper, dead-letter policy, pruning command, or exactly-once
claim. A controller crash after lease can cause reprocessing after expiry.

## Backup and retention

GR-15A does not delete or vacuum receipt history. Operators must size and back up
the local state directory according to deployment policy. Restoring an old backup
can reintroduce pending outbox work; future controllers must remain idempotent.
