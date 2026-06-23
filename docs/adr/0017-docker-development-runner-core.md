# ADR: Docker development runner core

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

M3 begins with Glassroot's first backend that actually executes a command.
Docker is useful for local trusted fixture development, but ordinary Docker is
not a hardened security boundary for hostile repositories. The execution surface
must therefore be split: GR-13A implements only the Docker Engine boundary and
attempt runner core, while GR-13B separately reviews user-facing local run
orchestration, evidence persistence, hostile workspace artifact collection, and
unsafe-development CLI acknowledgement.

## Decision

Add `internal/dockerengine` as the only package that imports the official Moby
modules and talks to Docker. It uses `github.com/moby/moby/client v0.5.0` and
`github.com/moby/moby/api v1.55.0`. The client module is pre-v1, so behavior is
contained behind a narrow internal interface and recorder tests. No Docker CLI,
Compose SDK, testcontainers, `github.com/docker/docker`, or `moby/v2` module is
used.

The engine adapter accepts only an explicit absolute Unix socket. It ignores all
Docker environment variables and Docker contexts. It performs bounded preflight:
ping with API negotiation, version and system-info inspection, Linux daemon
requirement, minimum API `1.44`, resource-enforcement checks, seccomp checks,
and socket identity recheck. The daemon and socket are trusted local development
inputs; their metadata is diagnostic and not a security guarantee.

Images are local-only and immutable by reference. The runner requires
`@sha256:<64 lowercase hex>`, inspects only local image metadata, requires the
exact repository digest, rejects non-Linux images and image-declared volumes,
and never pulls, builds, imports, loads, tags, pushes, or searches images.
Container creation overrides image entrypoint, command, and healthcheck.

Add `internal/runner/dockerdev` with fixed capabilities: name `docker-dev`,
version `v1`, isolation tier `development-only`, target execution true,
synthetic evidence false, network-deny enforcement true, and no comprehensive
process, filesystem, syscall, artifact-hashing, snapshot, brokered-network, or
fresh-kernel capabilities. Attempts require an exact unsafe-development
acknowledgement value. The acknowledgement is not authorization.

Each planned attempt must be bound to one trusted private workspace directory.
Workspaces are control-plane inputs, not plan or repository strings. The
container receives exactly one read-write bind mount from that directory to the
planned workdir. No Docker socket, device, named volume, shared cache, Git store,
evidence directory, or host namespace is mounted.

The reviewed container configuration uses network mode `none`, privileged false,
cap-drop `ALL`, no added capabilities, no devices, no host PID/IPC/UTS
namespace, read-only root filesystem, no-new-privileges, default seccomp,
Docker init, stdin closed, TTY false, restart policy none, auto-remove false,
log driver none, bounded tmpfs mounts, bounded `/dev/shm`, and exact resource
limits. GR-13A runs as UID 0 inside the container and records that as a
limitation rather than claiming user-namespace isolation.

The command vector is exactly the planned absolute shell, `-c`, and the literal
planned run string. Glassroot never invokes a host shell, inspects command text,
rewrites commands, inherits host environment variables, or includes the raw run
string in runtime events/errors.

Stdout/stderr streaming is added as a narrow runner-local output sink. The
backend attaches before start, demultiplexes Docker's non-TTY frames, streams raw
bytes synchronously with backpressure, bounds accepted bytes, drains after
truncation, and never falls back to persistent `docker logs`. Output bytes remain
hostile evidence data and are rendered only by later safe rendering layers.

Runtime events are intentionally limited: one development-only observation-gap
warning, one container process start, one process exit, and exact resource-limit
events such as OOM. The backend does not infer child-process, filesystem,
syscall, or network attempts from command text, logs, paths, or container exit.

Cleanup stops/kills as needed and removes only the exact created container. It
uses no global prune and no broad orphan sweep. Cleanup failure prevents a
complete successful outcome.

## Consequences

Glassroot gains a local development runner core without exposing a public run
CLI. Future public or hostile execution policy must reject `development-only`.
GR-13B can bridge stdout/stderr to evidence and handle hostile post-run artifact
collection without expanding the Docker API boundary. GR-14 remains the hardened
backend spike.

The design cannot provide a portable exact disk limit for the writable
workspace. It also does not provide comprehensive observation of child processes,
filesystem writes, syscalls, or network attempts. Docker daemon compromise,
container escape, kernel compromise, malicious bind mounts, rootless/cgroup
misconfiguration, and same-UID host mutation remain residual risks.

## Alternatives considered

- **Expose Docker through `glassroot run` in GR-13A:** rejected to keep the first
  command-execution review separate from CLI trust, evidence persistence, and
  hostile artifact collection.
- **Use the Docker CLI:** rejected to keep production subprocess creation out of
  the runner and confined to existing Git object-reader code.
- **Use environment-selected Docker configuration:** rejected because it would
  allow ambient host state to select a powerful daemon endpoint.
- **Pull missing images:** rejected because registry access and mutable image
  resolution are outside this trusted-local-fixture core.
- **Run non-root in GR-13A:** deferred until copied/chowned workspace behavior is
  explicitly designed.
- **Use Docker logs after attach failure:** rejected because daemon log
  persistence is not required and would create a fallback path with different
  semantics.
- **Add an orphan cleanup sweep:** rejected because it risks deleting containers
  not created by the current operation.

## Security considerations

Docker-dev actually executes target commands. The Docker daemon and socket are
highly privileged trusted local inputs. The container receives one writable host
workspace bind and runs UID 0 inside ordinary Docker with reduced privileges. No
secrets, Docker socket, devices, host namespaces, or named volumes are mounted,
but Docker is still not suitable for hostile public workloads. Output and daemon
responses are hostile data. Resource enforcement depends on daemon/kernel
configuration and is checked before use, but cleanup and enforcement can still
fail under host or daemon compromise.

## Validation plan

Validation covers socket parsing, environment non-discovery, daemon/API/resource
preflight, immutable image checks, exact container configuration, workspace
binding invariants, command argv separation, output demultiplexing/truncation,
sink failure cleanup, lifecycle cleanup, fixed capabilities, requirement
rejection, fuzz seeds, optional real-daemon integration gating, full tests, race
tests, vet, govulncheck, and audits for no Docker CLI, no host shell, no image
pull/build/load, no network daemon endpoint, no Docker socket mount, no global
prune, unchanged existing goldens, and unchanged CLI behavior.

## References

- KICKSTART.md GR-13A
- docs/DOCKER_DEV_RUNNER.md
- docs/RUNNER_CONTRACT.md
- docs/THREAT_MODEL.md
- internal/dockerengine
- internal/runner/dockerdev
