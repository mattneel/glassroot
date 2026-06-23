# ADR: Deterministic run planning

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-5 made trusted-base configuration the only effective configuration for a pull
request. GR-6A resolves immutable Git revisions, and GR-6B materializes exact
source trees into private workspaces. M1 still needed a deterministic run plan so
future runners do not decide commands, limits, source identities, or trust
semantics themselves.

Planning is security-sensitive because it freezes which inert commands and
limits future execution components will consume. It must not inspect workspaces,
execute target code, or let head configuration influence effective execution
fields.

## Decision

Add `internal/pipeline` as a pure planning package. It builds a `FrozenPlan`
from explicit caller inputs:

- GR-5 trusted load result;
- base/head planner-owned source snapshots;
- trusted platform constraints;
- caller-supplied run ID and UTC creation time.

The planner derives one execution template from the trusted-base pipeline and
copies it for base and head. It binds exact commit IDs, tree IDs, Git object
formats, materialized-tree digests, materialization-manifest digests, source
summaries, and source limitations before freezing the plan.

GR-7 was split into GR-7A and GR-7B so planning can be reviewed independently
from runner behavior. GR-7B will consume a finalized plan and must not reinterpret
or augment commands, limits, or networking.

Additive model changes extend `glassroot.dev/run-plan/v1alpha1` with:

- Git object format and exact tree ID fields;
- trusted configuration provenance;
- execution environment image/workdir data;
- collection, comparison, policy, and platform plan sections;
- explicit scenario shell/run/repetition fields;
- source summary and source limitations;
- CPU count alongside existing resource units.

No existing model field is removed, renamed, or given a new JSON type.

## Security considerations

The trusted-base pipeline remains the sole repository-level execution authority.
Head pipeline values are not merged, selected, or copied into execution fields.
The planner rejects inconsistent base/head commit identities, malformed source
descriptors, unsupported object formats, missing configuration digests, platform
ceiling violations, invalid run IDs, invalid timestamps, and oversized plans.

Platform admission rejects requests that exceed ceilings rather than silently
clamping them. This avoids hidden defaults or undisclosed narrowing. Later
policy-precedence work may introduce an explicit intersection model.

Serialized plans contain shell and `run` strings as inert data. The planner does
not execute, expand, interpolate, parse, or syntax-check them. It does not read
workspaces, contact Git, contact registries, access the network, inherit host
environment variables, or include workspace paths.

The plan digest is domain-separated and reproducible for the current encoder. It
is not a signature, authorization token, attestation, canonical JSON claim, or
proof that source is safe.

## Alternatives considered

- Let the fake runner derive commands from configuration directly. Rejected
  because it would duplicate trust-boundary decisions in runner code.
- Include concrete materializer or gitstore types in the planner API. Rejected to
  avoid coupling planning to infrastructure handles and workspace paths.
- Clamp repository resource requests to platform ceilings. Rejected because it
  would hide the difference between requested and admitted behavior.
- Expand repetitions into concrete attempt jobs. Rejected to avoid duplicating
  large command strings and to keep runner job derivation in GR-7B.
- Generate run IDs or timestamps inside the planner. Rejected because implicit
  clock or randomness would break deterministic planning.

## Consequences

The wire plan is more complete and suitable for later runner consumption, but the
run-plan model remains v1alpha1 and is still evolving. Additive fields require
review of compatibility fixtures and documentation. The current plan still
contains the legacy runner-capabilities object from GR-3; GR-7A leaves it empty
rather than inventing runner facts before GR-7B.

The planner relies on GR-5, GR-6A, and GR-6B contracts for trusted config,
immutable object identities, and materialization descriptors, while defensively
validating consistency and descriptor shape.

## Operational and migration impact

No CLI behavior, workflows, config parser behavior, Git reading behavior, or
materialization behavior changes. Existing v1alpha1 model fixtures continue to
round-trip. New planner-output fixtures exercise the current GR-7A plan encoder
and digest.

Future application orchestration must keep workspace handles outside the wire
plan and associate them with revision plans through trusted in-memory state.
Future runner capability matching must consume the plan without selecting head
configuration or adding host environment defaults.

## Validation plan

Validation includes:

- unit tests for base-only authority and head noninterference;
- source snapshot and trusted-result consistency tests;
- platform admission boundary tests;
- frozen ownership/deep-copy tests;
- deterministic JSON and digest golden tests;
- digest sensitivity tests for execution-affecting fields;
- fuzz seeds for source descriptors, run IDs, digests, and plan building;
- audits that `internal/pipeline` imports no filesystem, process, or network
  packages and calls no clock or random source.

Human review is required because this is a security-sensitive trust-boundary
change; coding-agent approval alone is insufficient.

## References

- KICKSTART.md GR-7A
- docs/THREAT_MODEL.md
- docs/CONFIGURATION.md
- docs/MATERIALIZATION.md
- docs/PLANNING.md
- docs/adr/0001-core-model-schema-versioning.md
- docs/adr/0003-trusted-base-configuration.md
- docs/adr/0004-trusted-git-object-reader.md
- docs/adr/0005-safe-revision-materialization.md
