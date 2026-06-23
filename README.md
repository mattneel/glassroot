# Glassroot

Glassroot is an open-source security CI system for untrusted software changes.
It compares the behavior of a trusted base revision and a proposed head revision,
then reports observed differences with evidence.

## Status

Glassroot is **pre-alpha**. It is not yet suitable for running hostile or
untrusted workloads. The repository now includes strict evidence verification,
normalization, comparison, built-in policy, trusted-base waiver application, safe
report rendering for existing bundles, the `inspect` CLI, a deterministic
fake-runner demo, and an explicitly acknowledged local docker-dev run path for
trusted fixtures. It still does not include a hardened runner, GitHub App, or
public workload execution service.

## Install for development

This repository pins Go with `mise`:

```bash
mise install
mise exec -- go version
```

The expected toolchain is Go 1.26.4.

## Usage

```bash
go run ./cmd/glassroot version
```

The command prints Glassroot build metadata. Build pipelines may override the
`version`, `commit`, and `built` variables with Go linker flags.

Pre-alpha evidence inspection is available for existing verified bundles:

```bash
go run ./cmd/glassroot inspect \
  --git-dir /control/repos/example.git \
  --base-commit 1111111111111111111111111111111111111111 \
  --head-commit 2222222222222222222222222222222222222222 \
  --evaluated-at 2026-06-23T00:00:00Z \
  --expected-manifest-digest sha256:3333333333333333333333333333333333333333333333333333333333333333 \
  --format terminal \
  /absolute/path/to/evidence-bundle
```

`inspect` verifies an existing bundle and renders a report; it does not create
evidence, execute target code, fetch Git data, read a working tree, or prove that
a change is safe. See [docs/INSPECT.md](docs/INSPECT.md).

Pre-alpha synthetic demo output can be generated with the fake runner:

```bash
go run ./cmd/glassroot demo fake \
  --fixture behavior-change \
  --format terminal \
  /absolute/path/to/glassroot-demo
```

The demo creates trusted fixture revisions, synthetic fake-runner evidence, and
reports, then verifies the bundle through `inspect`. It executes nothing, does
not run fixture source, does not access a network, and is an M2 demonstration
only. Synthetic observations are not target-workload observations and are not
suitable for analyzing hostile workloads. See [docs/DEMO.md](docs/DEMO.md).

Pre-alpha local development execution is available only with an explicit unsafe
Docker acknowledgement:

```bash
go run ./cmd/glassroot run docker-dev \
  --git-dir /control/repos/example.git \
  --base-commit 1111111111111111111111111111111111111111 \
  --head-commit 2222222222222222222222222222222222222222 \
  --docker-socket /absolute/path/to/docker.sock \
  --run-id local-dev-001 \
  --created-at 2026-06-23T00:00:00Z \
  --evaluated-at 2026-06-23T00:30:00Z \
  --acknowledge-unsafe-development-runner "I understand docker-dev is not a security boundary" \
  --format terminal \
  /absolute/path/to/glassroot-local-run
```

This path executes target commands in development-only Docker containers, uses
one private materialized workspace per attempt, writes and verifies evidence,
and reconstructs the report through `inspect`. Docker-dev is not hardened, is
not suitable for hostile repositories or public pull requests, never pulls an
image, and does not make a sandbox, provenance, authentication, attestation, or
safety claim. See [docs/LOCAL_RUN.md](docs/LOCAL_RUN.md).

GR-14 includes a gVisor runtime-monitoring technical spike for a controlled
trusted fixture. It is not a production runner, adds no `glassroot run gvisor`
command, and makes no hardened-sandbox claim. See
[docs/GVISOR_SPIKE.md](docs/GVISOR_SPIKE.md).

## Development

```bash
make fmt
make test
make verify
```

`make verify` runs formatting, vetting, tests, and a CLI build. The current
pre-alpha inspect and demo paths intentionally execute no target repository code.
The local docker-dev path does execute target commands, but only after an exact
unsafe-development acknowledgement and only for trusted local fixtures.

## Security posture

Do not use this pre-alpha scaffold to run hostile repositories. `inspect` can
read already-created evidence bundles, `demo fake` can create synthetic demo
fixtures, and `run docker-dev` can execute trusted local fixtures in ordinary
Docker only after explicit acknowledgement. Docker-dev must not be used for
hostile repositories or public pull requests and makes no sandbox, provenance,
authentication, attestation, or safety claim. Future milestones will add
hardened runner investigations. Until those components exist and are reviewed,
Glassroot makes no hardened-runner security claim.

## License

Glassroot is licensed under the Apache License 2.0. See [LICENSE](LICENSE).
