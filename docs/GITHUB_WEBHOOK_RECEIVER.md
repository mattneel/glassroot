# GitHub webhook receiver

GR-15A adds `glassroot-receiver`, a local intake service for future GitHub App
webhooks. It does not deploy a public endpoint, hold a GitHub App private key,
mint installation tokens, call GitHub APIs, fetch source, schedule a worker
inline, publish Check Runs, or execute target code. Public PR execution remains
prohibited.

## Command

```text
glassroot-receiver version

glassroot-receiver serve \
  --listen-unix /absolute/private/github.sock \
  --state-dir /absolute/private/state \
  --receiver-id receiver-1 \
  --current-secret-file /absolute/private/current.secret \
  [--previous-secret-file /absolute/private/previous.secret]
```

There are no defaults for the socket, state directory, or secrets. Secret values
are never accepted on the command line and there are no environment-variable
equivalents. `serve --help` performs no file, socket, or database operation.

## Endpoint and deployment boundary

The service listens only on a caller-supplied Unix stream socket and serves
exactly:

```text
POST /webhooks/github
```

All other paths return fixed `404`; non-POST methods return fixed `405` with
`Allow: POST`. Query strings are rejected. GR-15A intentionally adds no health,
metrics, debug, pprof, admin, TCP, or TLS endpoint.

Internet-facing deployments require a separately reviewed TLS reverse proxy. The
proxy must preserve the exact raw request body and required headers. The receiver
does not trust source IP, `Forwarded`, `X-Forwarded-For`, `X-Real-IP`,
User-Agent, or proxy identity headers. IP allowlisting can be a deployment layer,
but it is not authentication.

The socket parent must be a trusted local directory with mode `0700`, owned by
the service UID on Linux. The created socket is mode `0600`. Shutdown removes the
socket only when the path still identifies the exact socket created by this
process; replaced paths and unrelated siblings are not removed.

## Secret files and rotation

The receiver loads only webhook-secret material:

- current secret: required;
- previous secret: optional rotation overlap.

Each secret path must be absolute, clean, valid UTF-8, control-free, and at most
4096 bytes. The file must be a non-symlink regular file, link count one on
Linux, owned by the service UID on Linux, mode exactly `0400` or `0600`, and
between 32 and 256 bytes. Secret bytes are consumed exactly; trailing newlines
are part of the secret and are not trimmed. The receiver copies secret bytes into
owned memory and best-effort overwrites them on shutdown, but Go cannot guarantee
complete zeroization of every runtime copy.

Rotation procedure:

1. deploy with new current secret and old previous secret;
2. update GitHub's configured webhook secret;
3. verify deliveries signed by the new current secret;
4. redeploy without the previous secret.

The durable receipt records only `current` or `previous`; secret bytes and raw
signature headers are never persisted.

## Request validation order

The handler applies this logical sequence:

1. exact method;
2. exact path and empty query;
3. active-request limit;
4. required header multiplicity and syntax;
5. `Content-Type: application/json` with optional `charset=utf-8` only;
6. absent or identity `Content-Encoding`, and no trailers;
7. bounded raw body read under 4 MiB;
8. HMAC-SHA256 verification over those exact raw bytes;
9. bounded GR-15 JSON preflight;
10. minimal typed projection;
11. delivery receipt and intake fingerprint construction;
12. atomic inbox/outbox transaction;
13. fixed response.

JSON parsing happens only after signature verification. Invalid signatures never
touch durable storage. Unsupported but signed events/actions may be recorded as
ignored and acknowledged, but they do not create outbox work.

## HTTP bounds and responses

Default bounds:

| Setting | Value |
| --- | --- |
| Max header bytes | 32 KiB |
| Max webhook body bytes | 4 MiB |
| Max active requests | 64 |
| Max active connections | 128 |
| Read header timeout | 2 seconds |
| Read timeout | 8 seconds |
| Per-request intake timeout | 8 seconds |
| Write timeout | 2 seconds |
| Idle timeout | 30 seconds |
| Shutdown timeout | 9 seconds |

Responses are fixed plain text with `Cache-Control: no-store` and
`X-Content-Type-Options: nosniff`.

| Status | Meaning |
| --- | --- |
| 202 | newly persisted/enqueued, newly persisted/ignored, or duplicate same delivery/fingerprint; returned only after durable commit |
| 400 | malformed headers, malformed JSON, invalid projection, event/payload mismatch, trailers, or ambiguous request format |
| 401 | missing or invalid webhook signature |
| 404 | wrong path or query string |
| 405 | wrong method |
| 409 | same receiver/delivery ID with conflicting authenticated intake fingerprint |
| 413 | body exceeds the receiver limit |
| 415 | unsupported media type or content encoding |
| 503 | request capacity, database unavailable/busy, transaction failure, intake timeout, or durability failure |

Response bodies never include delivery IDs, event names, repository identities,
parser details, SQL details, filesystem paths, signatures, secrets, or payload
content.

## Logging and shutdown

Operational logs are structured and bounded. Allowed fields are component,
operation, HTTP status, decision, duration, and a stable error code. Logs go to
stderr and do not include request bodies, signatures, secrets, full headers, PR
prose, branch names, URLs, SQL, database paths, socket paths, or stack traces for
expected errors.

`glassroot-receiver serve` handles SIGINT/SIGTERM by stopping new accepts,
allowing bounded in-flight requests to finish, shutting down the HTTP server,
closing the listener, closing SQLite, best-effort overwriting secrets, and
removing only the owned socket.

## Non-claims

HMAC validation proves only possession of the shared webhook secret for the raw
body delivered to the receiver. Signed payload content remains hostile and can be
duplicated, delayed, or out of order. The receiver does not authorize execution,
select policy or runners, fetch source, or approve a pull request.
