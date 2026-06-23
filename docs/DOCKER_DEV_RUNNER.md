# Docker development runner core

GR-13A adds a Docker Engine adapter and a `docker-dev` runner core for trusted
local development fixtures. It is the first Glassroot backend that actually
starts a target command, and it is intentionally **not** a hardened sandbox.
Ordinary Docker and access to the Docker socket must not be treated as suitable
for hostile repositories, public pull requests, webhooks, or shared worker
queues.

GR-13A exposes no user-facing execution CLI. GR-13B will decide how a local user
explicitly opts into this backend.

## Trust boundary

The backend runs only after trusted caller code constructs an exact unsafe
acknowledgement:

`I understand docker-dev is not a security boundary`

The acknowledgement is fixed Glassroot UI text. It is not authorization and is
not serialized into evidence. Capabilities can be inspected without it, but no
attempt can start without it.

The backend also has to be explicitly allowed by trusted runner requirements.
Its isolation tier is always `development-only`; it never reports hardened
isolation and it never satisfies a public or untrusted execution policy.

## Docker Engine connection

`internal/dockerengine` is the only package that imports Moby modules or talks to
Docker. It accepts only an explicit absolute Unix-domain socket path. It never
uses `DOCKER_HOST`, Docker contexts, TLS configuration, `$HOME` Docker config, a
remote TCP/SSH endpoint, or a default fallback path.

Opening the adapter performs bounded preflight:

- lstat the final socket component and reject symlinks and non-sockets;
- ping the daemon with API negotiation enabled;
- inspect server version and system info;
- require a Linux daemon;
- require a negotiated API at least `1.44`;
- require reported CPU, memory, swap, and PID limit support;
- require seccomp support and reject unconfined seccomp;
- record daemon OS, architecture, engine/API versions, cgroup facts, rootless
  state, and security options as diagnostics only.

Possession of the Docker socket generally grants powerful control over the
Docker daemon host. The socket and daemon are part of the trusted local
development environment.

## Dependencies

GR-13A uses the supported public Moby modules:

- `github.com/moby/moby/client v0.5.0`
- `github.com/moby/moby/api v1.55.0`

The client module is pre-v1. The adapter does not use the Docker CLI, Docker
Compose SDK, testcontainers, or `github.com/docker/docker`.

## Images

The planned image must already be present locally and must be an immutable
reference containing `@sha256:<64 lowercase hex>`. The adapter only inspects the
local image; it never pulls, builds, imports, loads, tags, pushes, searches, or
logs into a registry.

The local image must report the exact requested repository digest, Linux image
metadata, and no image-declared volumes. Container creation overrides image
entrypoint, command, and healthcheck. Image environment remains part of immutable
image behavior; host environment is never inherited.

An immutable digest identifies bytes selected by the trusted caller. It does not
make the image benign.

## Workspace binding

Each planned attempt must have exactly one trusted workspace binding. The host
path is a control-plane input, not repository data, and must be absolute, clean,
private, non-symlink, and unique. Base/head attempts and repetitions cannot share
or overlap writable workspace directories.

The container receives exactly one read-write bind mount:

`attempt workspace -> planned workdir`

No Docker socket, device, Git store, evidence directory, or shared cache is
mounted. GR-13A does not create, traverse, collect artifacts from, or remove the
workspace; cleanup ownership remains with the caller. A writable bind mount is
one reason the backend is development-only.

## Container configuration

Every attempt creates one container. The reviewed configuration includes:

- network disabled and network mode `none`;
- no published ports, exposed-port publication, DNS customization, extra hosts,
  or links;
- privileged mode false;
- all Linux capabilities dropped and none added;
- no devices or device requests;
- no host PID, IPC, or UTS namespace;
- read-only root filesystem;
- `no-new-privileges=true`;
- Docker's default seccomp behavior retained;
- Docker init enabled;
- TTY false and stdin closed;
- restart policy `no` and auto-remove false;
- healthcheck disabled;
- logging driver `none` with live attach streaming;
- bounded tmpfs mounts for `/tmp`, `/run`, and `/var/tmp`;
- bounded `/dev/shm`.

GR-13A explicitly runs as UID 0 inside the container while dropping capabilities
and enabling no-new-privileges. It does not claim rootless workload execution or
user-namespace isolation.

## Command semantics

The container command is exactly:

`<planned absolute shell> -c <literal planned run string>`

Each value is passed as a distinct Docker API argument. Glassroot does not invoke
a host shell, inspect or rewrite the run string, prepend setup commands, source
profiles, inherit host environment variables, or expose the run ID through the
container environment. The raw run string is not included in runtime events or
errors.

## Resources and output

CPU, memory, memory-swap, PID, timeout, tmpfs, `/dev/shm`, stdout, stderr, and
total-output limits are explicit. Unsupported or unverifiable daemon resource
enforcement fails closed during preflight. Docker does not provide a portable
exact workspace disk limit in this design, so GR-13A does not claim disk
bound enforcement.

The backend attaches before start, keeps stdout and stderr distinct, parses
Docker's non-TTY multiplexed frames, and writes raw bytes synchronously to a
per-attempt output sink. It does not decode UTF-8, parse lines, sanitize terminal
controls, or use `docker logs` as a fallback. When output limits are reached, it
records truncation and continues draining without buffering discarded bytes.

## Events and limitations

The backend emits only facts it can establish:

- a `sandbox-runtime-observed` observer-warning event that docker-dev is
  development-only and lacks comprehensive child-process, filesystem, syscall,
  and network observation;
- one container-init process start event after start;
- one process exit event after final state is known;
- a resource-limit event when Docker reports OOM or another exact supported
  resource condition.

It does not invent child-process, filesystem, syscall, or network-broker events,
and it does not infer behavior from command text, paths, or logs.

## Cleanup

On success, target failure, timeout, cancellation, output-sink failure, or
infrastructure failure, the runner terminates as needed and removes only the
exact container it created. It never uses auto-remove, global prune, or an orphan
sweep. Container names and labels are generated by trusted code; labels are not
authentication. Cleanup failure prevents a complete successful result.

## Testing

Ordinary unit tests use recorder/fake-engine calls and do not require Docker or
execute target fixtures. `make test-dockerdev-integration` is explicitly gated
for future real-daemon checks and must use a local already-present immutable
image; it must never pull an image or access a registry.

GR-13B will bridge output into evidence, collect configured artifacts from
hostile post-run workspaces, and expose the local run CLI. GR-14 is the first
hardened-backend spike.
