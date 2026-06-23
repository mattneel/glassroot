# ADR: gVisor production integration shape

## Status

Accepted for technical-spike guidance. Not a production-runner approval.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-14 evaluates whether gVisor runtime monitoring can support a future hardened
Glassroot worker. The spike must prove process-lifecycle trace ingestion from a
controlled trusted fixture while treating all Sentry messages as hostile. It must
not add a production gVisor runner, public pull-request execution, a hardened
capability, signing, authentication, provenance, or an attestation claim.

The tested version is pinned to gVisor `release-20260615.0` at commit
`57efc92f6df8f530b5cc49cc197077f9c3dafe98`. The monitor tool module pins
`gvisor.dev/gvisor v0.0.0-20260613051822-57efc92f6df8` and imports only the
remote wire package. The live fixture requires an operator-supplied runsc binary
with an exact SHA-512 and a dedicated Docker runtime such as
`runsc-glassroot-spike`.

## Decision

Add an isolated GR-14 spike implementation:

- root package `internal/gvisorspike` for prerequisite validation, pod-init
  configuration, fixture container shape, process lifecycle state, typed result
  metadata, and gated integration contracts;
- separate module `tools/gvisormonitor` for bounded remote-sink protocol parsing,
  typed trace conversion, replay tests, and monitor state-machine tests;
- trusted fixture source under `tools/gvisorfixture`;
- documentation and Makefile targets for ordinary unit tests and gated live
  validation.

Runtime monitoring must be configured at sandbox initialization. Setup errors are
not ignored. The remote monitor treats the Sentry as hostile, bounds every packet
and accepted field, records unknown message types as limitations, and treats any
dropped-message count as incomplete observation.

No existing CLI, runner capability, policy, report, evidence, or public workflow
is changed.

## Production integration comparison

### A. Docker runtime integration

A preconfigured Docker runtime is operationally simple and compatible with the
existing Docker Engine boundary. It is useful as the GR-14 spike vehicle because
Docker can select a named runtime for one controlled container. Its drawbacks are
large Docker daemon trust, limited per-job runsc configuration, daemon-level
runtime setup, and less direct ownership of sandbox lifecycle and monitoring
sockets.

### B. containerd shim v2

A `containerd-shim-runsc-v1` worker can use runtime handlers and worker-owned
configuration with a smaller API surface than Docker. It fits a future dedicated
worker better, but Glassroot would need to own image, snapshot, namespace,
cleanup, and runtime-handler operations explicitly.

### C. Direct OCI runsc lifecycle

Direct `runsc create/start/kill/delete` gives the most configuration control and
would make monitor setup and per-sandbox sockets explicit. It also requires
Glassroot to own OCI bundle generation, rootfs preparation, cgroups, namespaces,
image unpacking, cleanup, and many security-sensitive lifecycle details.

### D. Dedicated worker runtime abstraction

A Glassroot worker abstraction above one runtime implementation keeps
controller/publisher code free of runtime-specific details, isolates monitor and
future network broker responsibilities, and allows future coexistence with
Firecracker. The worker must declare exact capabilities and evidence limits and
must fail closed when monitoring or cleanup is incomplete.

## Recommendation

Use the Docker runtime path only for this spike. For production M4, prefer a
dedicated worker runtime abstraction with containerd shim v2 or direct OCI runsc
under review, rather than exposing Docker runtime selection to general run
orchestration. The worker should own per-sandbox monitor sockets, network broker
or deny-only setup, version pinning, resource limits, evidence transport, and
cleanup. Docker may remain a development aid, not the public hostile-code path.

## Consequences

The spike demonstrates monitor protocol parsing and process lifecycle state
handling without changing Glassroot's stable evidence schema. A future production
runner must still design event provenance, drop semantics, monitor lifecycle,
network brokering, filesystem observation, image/rootfs handling, resource and
cleanup guarantees, and independent review before any `hardened-container` claim.

M4 remains incomplete. Passing the controlled fixture is not evidence of escape
resistance and does not authorize public repositories.
