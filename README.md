# Glassroot

Glassroot is an open-source security CI system for untrusted software changes.
It compares the behavior of a trusted base revision and a proposed head revision,
then reports observed differences with evidence.

## Status

Glassroot is **pre-alpha**. It is not yet suitable for running hostile or
untrusted workloads. This repository currently contains only the initial local
scaffold and a version-reporting CLI command. It does not include a runner,
sandbox, Docker integration, GitHub App, policy engine, or target-code execution
path.

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

## Development

```bash
make fmt
make test
make verify
```

`make verify` runs formatting, vetting, tests, and a CLI build. The initial
scaffold intentionally executes no target repository code.

## Security posture

Do not use this pre-alpha scaffold to inspect hostile repositories. Future
Glassroot milestones will add deterministic evidence collection, comparison,
policy evaluation, and isolated runners. Until those components exist and are
reviewed, Glassroot makes no sandbox or hardened-runner security claim.

## License

Glassroot is licensed under the Apache License 2.0. See [LICENSE](LICENSE).
