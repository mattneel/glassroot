# ADR: Deterministic fake-runner demo

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

M2 needs a reproducible vertical slice that creates trusted fixture state,
produces a complete evidence bundle, and verifies the result through the same
inspection path users will exercise. The existing fake runner can produce
synthetic evidence, but exposing it as `glassroot run` would blur the boundary
between fixture generation and workload execution. GR-12 must remain visibly
synthetic and must not introduce a process-capable runner, Docker, network
access, checkout, or target execution.

## Decision

Glassroot adds `internal/demo` and the `glassroot demo fake` subcommand. The
command accepts only a named built-in fixture (`behavior-change` or `control`),
a selected report format, and a new absolute output directory. It accepts no
caller-provided Program, pipeline, waiver file, Git store, revision override,
evaluation time, backend, or unsafe execution flag.

The fixture Program is trusted compiled-in data selected by fixture enum. It is
not parsed from repository content, and the fixture pipeline `run` text remains
inert data. All generated observations keep `synthetic-test-generated`
provenance; the demo does not claim that events occurred in target source code,
that behavior is malicious, or that a control result proves safety.

The demo creates a narrow deterministic SHA-1 bare Git fixture store using a
private loose-object writer. No mutating Git subprocesses are added to production
code. The writer encodes only the reviewed fixture object graph: inert source
files and identical base/head pipeline configuration. There is no waiver file,
no hook, no remote, no worktree, no symlink, no gitlink, and no executable Git
entry. The generated store is then verified by opening it through GR-6A
`gitstore`.

Both exact revisions are materialized only to compute real source descriptors
for planning. Workspaces are closed and removed before publication, and no
materialized path is serialized. Trusted pipeline configuration is loaded from
the exact base commit; head configuration is inspection-only. `pipeline.Build`
constructs the FrozenPlan using fixed run identity, fixed creation time, exact
source descriptors, an empty environment, deny network mode, and strict policy.

Evidence is produced through the real GR-8A writer by executing the deterministic
fake backend with synthetic-test requirements. The bundle is verified through
GR-8B before and after relocation inside demo-owned staging. Final reporting is
reconstructed through `internal/inspect`; the demo does not trust intermediate
in-memory results as a substitute for the supported stage sequence.

The output tree is fixed: `fixture.git/`, `evidence/`, `report.json`,
`report.md`, `report.txt`, and `demo.json`. Report files are exact GR-11A JSON,
Markdown, and terminal renderings. `demo.json` records versioned fixture metadata,
commit/tree IDs, digests, dispositions, relative paths, and selected key evidence
records. It is descriptive metadata, not a trust root.

Publication uses a fresh private sibling staging directory, exclusive file
creation, fsyncs, and an atomic rename to the requested final path. The final path
must not already exist. Pre-publication failures remove staging and workspaces.
A parent-sync failure after rename returns failure and attempts to remove the
published output. No debug preservation option exists.

Exit-code behavior follows the report disposition after successful publication
and stdout write: `0` passed, `4` requires review, `5` failed. Usage/path errors
return `2`; generation, verification, publication, rendering, cleanup, or stdout
failures return `3`. These exit codes are not safety proofs.

## Consequences

M2 has a deterministic local vertical slice that exercises fixture Git reading,
materialization, trusted config, planning, fake evidence generation, evidence
verification, normalization, comparison, policy, waiver application, reporting,
and inspection equivalence without executing target code. The control fixture can
pass under the current strict policy while still carrying fake/synthetic notices;
this is not a safety assessment.

The implementation is intentionally not a general Git repository writer, fixture
plugin system, fake Program parser, custom scenario runner, or workload-capable
backend. Fixture updates require fixture-version review and golden updates.

No Docker, gVisor, Firecracker, image pull, network access, checkout, target
process execution, signing, authentication, attestation, provenance, or sandbox
claim is introduced. GR-13 and GR-14 cover later development and hardened-runner
work.

## Alternatives considered

- **Add `glassroot run`:** rejected because the fake backend cannot satisfy
  workload intent and would obscure that no target code executes.
- **Use Git mutating subprocesses to create fixtures:** rejected to keep
  production process creation confined to existing bounded `internal/gitstore`
  reads and avoid expanding the Git command surface.
- **Embed a prebuilt bare-store archive:** rejected in favor of readable,
  reviewed fixture object encoding and deterministic verification.
- **Allow caller-provided fixture programs or pipelines:** rejected because it
  would create a repository-controlled behavior source and weaken demo
  determinism.
- **Publish reports without inspect reconstruction:** rejected because GR-12 is
  intended to prove the same supported inspect path can reconstruct the report.

## Security considerations

The fixture source is inert and never executed, but a compromised demo fixture or
fake Program could still produce misleading synthetic evidence. Materialization
continues to treat paths and blobs defensively. The final output parent is
trusted caller state; same-UID mutation, hostile mounts, kernel compromise, and
filesystem compromise remain residual risks. Report and metadata digests provide
deterministic equality only and are not signatures, authentication,
attestations, or provenance.

## Validation plan

Validation covers fixture Git identities, materialization cleanup, deterministic
plan construction, fake Program coverage, evidence/log/artifact digests, report
findings and evidence references, direct inspect equivalence, golden metadata and
report files, atomic publication behavior, CLI parsing and exit codes, fuzz seed
execution, full tests, race tests, vet, schema checks, govulncheck, and audits
for no target execution, no network, no host paths, no mutable fixture inputs,
and unchanged upstream goldens.

## References

- KICKSTART.md GR-12
- docs/DEMO.md
- docs/INSPECT.md
- docs/REPORTING.md
- docs/FAKE_RUNNER.md
- docs/EVIDENCE_BUNDLE_FORMAT.md
- docs/THREAT_MODEL.md
- internal/demo
- cmd/glassroot
