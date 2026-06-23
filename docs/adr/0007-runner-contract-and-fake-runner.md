# ADR: Runner contract and fake runner

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-7A produces a deterministic `FrozenPlan` from trusted-base configuration, explicit platform constraints, and exact immutable source identities. M1 still needs a runner boundary so later backends consume the finished plan instead of deciding commands, limits, revisions, networking, or trust semantics themselves.

The first backend must be fake and deterministic. It exists for tests and future demonstrations only; it must not be confused with workload execution, observation, isolation, or security evidence.

## Decision

Add `internal/runner` with an attempt-level interface. `ExecutePlan` consumes only `*pipeline.FrozenPlan`, expands attempts sequentially in base/head, scenario, repetition order, queries backend capabilities once, matches them against trusted caller `Requirements`, and stamps authoritative observation-event envelopes outside backend control.

Actual runner capabilities remain outside `FrozenPlan`. The legacy `RunPlan.runner` field is treated as non-authoritative; `ExecutePlan` rejects a nonzero value instead of using it for backend selection. Removing or redesigning that field is deferred to a future schema version.

Requirements distinguish `synthetic-test` from `workload` intent. Capability matching is exact and failure-closed; allowed isolation tiers are explicit, no ordinal tier ordering is assumed, and there is no backend registry, dynamic discovery, or fallback. Fake is accepted only for explicit synthetic-test requirements and rejected for workload intent.

Backends emit `EventDraft` values containing a timestamp, source, kind, and one typed payload. They cannot set schema version, event ID, run ID, revision, scenario ID, repetition, sequence, or plan digest authority. `ExecutePlan` assigns a global sequence and deterministic event ID using the domain `glassroot.dev/observation-event-id/v1\0`, the plan digest, run ID, and sequence with length-prefixed encoding.

`EventSink.Emit` is synchronous. A sink error stops execution immediately, is wrapped as `sink-failed`, and no retry or final lifecycle event is emitted after failure.

Add `internal/runner/fake`, a standard-library-only deterministic backend driven by a trusted in-memory typed `Program`. The Program is bound to the exact plan digest and must contain exactly one script per planned attempt. The fake runner emits scenario lifecycle events plus trusted Program events with `synthetic-test-generated` provenance and deterministic timestamps derived from plan creation time, attempt ordinal, and Program offsets. It does not inspect run strings or repository contents.

Additive model changes introduce `ObservationEvent.repetition`, observation source `synthetic-test-generated`, and runner facts `executesTargetCode`, `syntheticEvidence`, and `enforcesNetworkDeny`.

## Security considerations

The runner contract preserves the GR-7A plan authority: a backend cannot alter commands, image, resources, network policy, collection settings, source identities, source digests, scenario order, or repetition counts. Capability mismatch occurs before event emission.

Fake capabilities truthfully report no target execution, no network enforcement, and no real process/filesystem/syscall/artifact observation. Fake output carries limitations saying no target code was executed and all observations are synthetic. The fake runner has no workspace, process, shell, Git, Docker, image, package-manager, network, or secret access.

A compromised future backend can still lie in typed payloads. Evidence and policy layers must retain runner identity, capabilities, provenance, and limitations. A sink failure means evidence is incomplete. GR-7B does not persist evidence; GR-8 will define the bounded persistent sink.

## Alternatives considered

- **Let runners consume model.RunPlan directly:** rejected because `FrozenPlan` owns the deterministic JSON/digest and copy contract.
- **Let backends stamp complete events:** rejected because it would let backend drafts forge run IDs, sequences, and event IDs.
- **Use fake as a fallback:** rejected because fallback would violate fail-closed workload execution semantics.
- **Insert actual capabilities into the frozen plan:** rejected because backend selection happens after planning and must not mutate the plan or digest.
- **Parallel attempt execution in GR-7B:** rejected to keep ordering and state ownership simple for the first runner boundary.

## Consequences

Future workload-capable backends must implement the attempt-level contract and return truthful capability facts. Application orchestration must bind nonserialized workspace handles to backend instances outside the plan. Evidence persistence, bundle manifests, comparator behavior, policy findings, and report rendering remain future work.

The run-plan v1alpha1 model remains compatible through additive optional fields. Event fixtures and ADR 0001 document the new repetition and synthetic-source fields.

## Operational and migration impact

No CLI behavior changes. No Go dependencies, workflows, schema files, config parser behavior, Git reading behavior, materialization behavior, or GR-7A golden plan/digest are changed. `make verify` covers ordinary runner tests, and `make test-runner-fuzz-seeds` executes runner fuzz seeds.

## Validation plan

Validation covers capability matching, fake rejection for workload intent, attempt expansion order, frozen-plan noninterference, event envelope stamping, event ID golden output, sink failure, cancellation, fake Program coverage, target outcomes versus infrastructure errors, ownership/deep-copy behavior, deterministic golden JSONL output, and fuzz seeds for capabilities, event drafts, and Program attempt keys.

Human review is required because this defines a security-sensitive runner boundary; coding-agent approval alone is insufficient.

## References

- KICKSTART.md GR-7B
- docs/THREAT_MODEL.md
- docs/PLANNING.md
- docs/RUNNER_CONTRACT.md
- docs/FAKE_RUNNER.md
- docs/adr/0001-core-model-schema-versioning.md
- docs/adr/0006-deterministic-run-planning.md
- internal/pipeline
- internal/runner
