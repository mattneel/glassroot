# Glassroot

Glassroot is an open-source security CI system for untrusted software changes.
It compares the behavior of a trusted base revision and a proposed head revision,
then reports observed differences with evidence.

## Status

Glassroot is **pre-alpha**. It is not yet suitable for running hostile or
untrusted workloads. The repository now includes strict evidence verification,
normalization, comparison, built-in policy, trusted-base waiver application, and
safe report rendering for existing bundles, plus the `inspect` CLI. It still does
not include a hardened runner, Docker integration, GitHub App, or user-facing
bundle-creation command.

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

## Development

```bash
make fmt
make test
make verify
```

`make verify` runs formatting, vetting, tests, and a CLI build. The current
pre-alpha inspect path intentionally executes no target repository code.

## Security posture

Do not use this pre-alpha scaffold to run hostile repositories. `inspect` can
read already-created evidence bundles, but it still depends on explicit caller
trust anchors and makes no sandbox, provenance, authentication, attestation, or
safety claim. Future milestones will add user-facing bundle creation and hardened
runners. Until those components exist and are reviewed, Glassroot makes no
hardened-runner security claim.

## License

Glassroot is licensed under the Apache License 2.0. See [LICENSE](LICENSE).
