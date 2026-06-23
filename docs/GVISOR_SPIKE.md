# gVisor runtime-monitoring spike (GR-14)

GR-14 is a technical spike, not a production runner. It adds an isolated monitor
module, a root-module spike model, trusted fixture source, replay tests, and a
gated integration contract for proving gVisor runtime-monitoring behavior in an
operator-prepared environment.

It does **not** add `glassroot run gvisor`, does not change runner capabilities
to `hardened-container`, does not authorize public pull-request execution, and
does not make a sandbox, provenance, authentication, attestation, or escape-
resistance claim.

## Pinned release

The spike is pinned to the official gVisor tag:

- release: `release-20260615.0`
- commit: `57efc92f6df8f530b5cc49cc197077f9c3dafe98`
- monitor module version: `gvisor.dev/gvisor v0.0.0-20260613051822-57efc92f6df8`
- remote protocol version: `1`

The live integration requires an operator-supplied `runsc` binary matching that
release and an exact lowercase SHA-512 supplied by the operator. The repository
never downloads, installs, or updates `runsc`.

## Prerequisites for the gated live spike

The live spike is gated by `make test-gvisor-spike-integration`. Ordinary
`make verify` does not require gVisor, Docker, KVM, root privileges, a preloaded
image, or a configured runtime.

The gated test requires explicit inputs:

```text
GLASSROOT_GVISOR_SPIKE=1
GLASSROOT_GVISOR_RUNSC=/absolute/path/runsc
GLASSROOT_GVISOR_RUNSC_SHA512=<128-lowercase-hex>
GLASSROOT_GVISOR_RELEASE=release-20260615.0
GLASSROOT_GVISOR_PLATFORM=kvm|systrap
GLASSROOT_GVISOR_RUNTIME=runsc-glassroot-spike
GLASSROOT_GVISOR_IMAGE=<name>@sha256:<64-lowercase-hex>
GLASSROOT_DOCKER_SOCKET=/absolute/path/docker.sock
GLASSROOT_GVISOR_MONITOR_SOCKET=/trusted/private/path/events.sock
```

The Docker runtime must be configured by the operator before the test. Glassroot
must not run `runsc install`, edit `daemon.json`, restart Docker, configure
containerd, pull an image, or contact a registry.

## Dedicated runtime and pod-init configuration

The intended Docker runtime name is `runsc-glassroot-spike`; it must not be the
Docker default runtime. Runtime-monitoring is configured at sandbox
initialization through a `Default` trace session using the `remote` sink. Setup
errors are not ignored.

The monitored socket is a private Unix `SOCK_SEQPACKET` endpoint under a trusted
0700 parent. It is created before container creation, is not mounted into the
fixture, and is removed on every path. The socket path is never serialized in
spike results.

Enabled trace-point inventory for this spike:

- `container/start`
- `sentry/clone`
- `syscall/clone/enter`
- `syscall/execve/enter`
- `sentry/execve`
- `sentry/exit_notify_parent`
- `sentry/task_exit`

Enabled context fields are limited to time, thread ID, thread-group ID,
container ID, process name, and parent thread-group ID. The spike does not enable
environment, credentials, file contents, or all-syscall capture by default.

## Monitor trust boundary

The monitor treats gVisor's Sentry as hostile. Remote messages may be malformed,
false, incomplete, duplicated, delayed, or dropped. The monitor enforces explicit
bounds for handshake size, header size, packet size, strings, arguments,
messages, connections, and duration. Unknown message types become limitations,
not process facts. Nonzero dropped-message counts make observation incomplete.

The monitor module is a separate Go module under `tools/gvisormonitor` so gVisor
internal APIs and transitive dependencies do not enter the root module. The
module imports the official pinned gVisor remote `wire` package for the protocol
header constants and uses bounded protowire decoding for the lifecycle fields it
accepts.

## Trusted fixture

Trusted fixture source lives under:

- `tools/gvisorfixture/parent`
- `tools/gvisorfixture/child`

The parent starts the exact child executable, waits for it, writes fixed bounded
stdout, and exits deterministically. The child writes fixed bounded stdout and
exits. The fixture uses no network, shell, randomness, current time, repository
content, package manager, secrets, or exploit payload. Ordinary unit tests build
but do not execute the fixture.

The live integration requires an already-present immutable local image containing
only this trusted fixture and minimal required files. Glassroot does not build,
pull, import, or load that image automatically.

## Controlled container configuration

The live fixture container must use the dedicated gVisor runtime, exact immutable
image, overridden entrypoint, network mode none, no published ports, privileged
false, all capabilities dropped, no host namespaces, read-only root filesystem,
no-new-privileges, TTY false, stdin closed, restart policy none, no devices, no
Docker socket mount, no host filesystem bind, and bounded CPU, memory, PIDs,
timeout, stdout, and stderr.

`runsc do` is not accepted for the primary live spike because it bypasses the
OCI/Docker runtime lifecycle that the production runner must eventually control.

## Observation claims and gaps

A successful spike may claim only that the configured gVisor runtime was selected
for the controlled fixture, the monitor connection was established before the
fixture ran, named trace points were decoded, zero drops were reported, and the
controlled container was removed.

It does not claim complete syscall, process, filesystem, or network coverage. It
has no external network broker, no filesystem observer, no independent host-side
cross-check, no authenticated evidence transport, no signed plan, and no
production evidence schema changes. A compromised Sentry, monitor, Docker daemon,
runsc binary, host kernel, or control plane can mislead or escape the spike.

## Current status

The root spike model and monitor module have unit, replay, and fuzz-seed tests.
The live gVisor fixture remains runtime-validation pending until an operator
provides the pinned `runsc`, dedicated Docker runtime, private monitor socket,
and immutable local fixture image.
