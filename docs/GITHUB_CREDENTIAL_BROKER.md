# GitHub App credential broker (GR-15B1)

GR-15B1 adds a local credential broker for future GitHub App controller/source-ingestion work. It is the only Glassroot component in this phase that loads the GitHub App private key. It does not consume webhook outbox work, reconcile pull requests, fetch source, schedule workers, publish Check Runs, or execute target code.

The broker is a credential boundary, not a source-safety boundary. A valid App JWT or installation token does not make repository content safe and does not authorize public PR execution.

## Service CLI

```text
glassroot-credential-broker version

glassroot-credential-broker serve \
  --listen-unix ABSOLUTE_SOCKET_PATH \
  --private-key-file ABSOLUTE_PEM_PATH \
  --app-id POSITIVE_INTEGER \
  --app-client-id CLIENT_ID
```

There are no defaults, environment-variable equivalents, TCP listener flags, GitHub Enterprise host flags, proxy flags, private-key value flags, installation-ID startup flags, repository-ID startup flags, publisher mode, source-ingestion mode, or controller mode. Help performs no file, network, or socket operation.

## Private-key custody

The private-key path is trusted control-plane configuration. The file must be absolute, clean, valid UTF-8, control-free, no more than 4096 bytes, existing, regular, and not a final-component symlink. On Linux it must be owned by the broker effective UID, have link count one, and mode exactly `0400` or `0600`.

The loader performs pre-open `Lstat`, open, descriptor `Stat`, `os.SameFile` comparison, bounded read, post-read descriptor `Stat`, post-open `Lstat`, and stability checks for identity, mode, size, mtime, and ctime where available. Accepted key encodings are PKCS#1 RSA private key and PKCS#8 containing an RSA private key. Encrypted PEM, EC, Ed25519, DSA, certificates, public keys, multiple PEM blocks, trailing non-whitespace data, and RSA keys outside 2048-8192 bits are rejected.

Rotation is operational: register a new key in GitHub, replace the protected file through trusted deployment, restart the broker, verify startup identity, then revoke the old key. GR-15B1 intentionally has no hot reload or previous-key fallback. The implementation best-effort overwrites PEM and token buffers, but Go cannot guarantee complete private-key or token zeroization.

## App JWT contract

JWTs are compact RS256 values with deterministic header and claim structures:

- header: `alg=RS256`, `typ=JWT`;
- claims: `iat` = trusted time minus 60 seconds, `exp` = trusted time plus 9 minutes, `iss` = configured GitHub App client ID;
- no `jti`, repository, installation, or other claims.

A fresh JWT is acceptable for each broker operation. JWTs are sent only as `Authorization: Bearer` to the fixed App endpoints. JWT values are not persisted or logged.

## App identity and token purposes

Startup calls `GET /app` and requires the configured App ID and client ID. The App registration permission profile must match the GR-15 advisory profile:

| Permission | Access |
| --- | --- |
| Checks | write |
| Contents | read |
| Pull requests | read |
| Metadata | read |

Unexpected permissions or missing required permissions fail startup. The broker then mints only these purpose-scoped tokens:

| Purpose | Requested installation-token permission | Repository scope |
| --- | --- | --- |
| `pull-request-read` | `pull_requests: read` | exactly one repository ID |
| `source-read` | `contents: read` | exactly one repository ID |

The broker never omits `repository_ids` or `permissions`, never requests repository names, never requests all repositories, never combines purposes, never asks for Checks write, and never retries with broader permissions after a narrow request fails. Installation preflight rejects missing, mismatched, suspended, or permission-insufficient installations.

Installation tokens are opaque variable-length secrets. The broker does not assume a prefix, exact length, JWT structure, or token claims. Returned tokens must be nonempty, bounded, control-free, scoped to the requested repository, scoped to the requested permission plus optional metadata read, and expire later than request time but no more than 65 minutes later with at least five minutes remaining.

## Local Unix protocol

The broker listens only on an explicit private Unix stream socket. The parent directory must be trusted, mode `0700`, owned by the broker UID, and the socket is created mode `0600`. The socket is removed on shutdown only if it is still the exact socket created by the process.

Linux peer credentials are checked with `SO_PEERCRED`; the peer UID must equal the broker effective UID. Peer PID is not authorization and is not recorded. Same-UID compromise defeats this local boundary.

Protocol frames use a 32-bit big-endian length followed by compact JSON. There is one request and one response per connection.

Request schema: `glassroot.dev/github-token-request/v1alpha1`

```json
{"schemaVersion":"glassroot.dev/github-token-request/v1alpha1","purpose":"source-read","installationId":123,"repositoryId":456}
```

Success response schema: `glassroot.dev/github-token-response/v1alpha1`. It contains the token only in the `token` field plus bounded metadata. Error responses contain only a stable code and fixed message. Invalid protocol requests do not call GitHub.

## TokenLease ownership

Client responses become pointer-owned `TokenLease` values. Token bytes are unexported and supplied only through `Use(func([]byte) error)`, which passes an owned temporary copy and best-effort overwrites it after the callback. `Close` best-effort overwrites internal bytes and is idempotent. Use after close fails. Callers must not retain callback bytes; future controller/source-ingestion code that retains them violates the contract.

## Logging and shutdown

Structured logs may include fixed component/operation, token purpose, decision, stable error code, and bounded duration. They must not include private keys, JWTs, tokens, authorization headers, private-key paths, socket paths, GitHub response bodies, repository names, or account names.

Shutdown handles SIGINT/SIGTERM, stops accepting connections, allows bounded in-flight requests to complete, closes idle GitHub HTTP connections, closes/removes the owned socket, and best-effort clears active secrets.

## Optional integration

`make test-github-broker-integration` is gated by explicit test-only environment variables for App ID, client ID, private-key path, installation ID, and repository ID. It calls only `GET /app`, installation inspection, and one exact repository-scoped read token request. It prints and persists no token. A minted token remains valid until expiry; GR-15B1 has no revocation flow.
