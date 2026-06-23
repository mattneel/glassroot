# ADR: Atomic evidence bundle writer

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-8 originally combined writing fresh evidence and reading existing evidence. Those are different trust boundaries. Writing starts from trusted control-plane state and bounded runner outputs. Reading must treat every existing path and byte as hostile.

GR-8A therefore defines only the directory format and atomic writer. GR-8B will add strict path-safe opening, duplicate JSON member rejection, digest verification, corruption handling, and hostile directory checks.

## Decision

Glassroot writes a v1alpha1 directory bundle before introducing the future `.grb` archive format. The fixed layout contains `manifest.json`, exact `plan.json`, `execution.json`, attempt-local `result.json` and `events.jsonl`, optional logs and artifact indexes, and content-addressed artifact objects under `objects/sha256/`.

The writer consumes `pipeline.FrozenPlan` and writes `FrozenPlan.JSON()` exactly. The GR-7A plan digest is recorded separately and is not treated as a signature. Actual runner capabilities and execution results are serialized outside the plan so backend selection does not mutate the frozen plan.

The writer creates a fresh private staging directory in a trusted parent. Staging and final names are random and never repository-controlled. Files are created exclusively with `0600`, directories with `0700`, and no symlinks, hard links, sockets, FIFOs, or devices are created.

Publication is initially Linux-only. The writer closes and syncs payloads, writes the manifest last through a temporary name, syncs the staging directory, renames staging to a random final sibling, and syncs the parent. Rename and sync failures fail the transaction and trigger cleanup. This is an atomic namespace-publication and best-effort durability contract, not protection from malicious filesystems, kernels, or storage devices.

Events are compact JSONL and are validated against the GR-7B envelope: schema version, run ID, planned attempt coordinate, sequence, deterministic event ID, source/kind, and typed payload. The writer routes events only to precomputed planned attempts and rejects sequence gaps, duplicate event IDs, unknown attempts, and attempts that move backward.

Logs preserve raw bounded byte prefixes and record truncation out-of-band. Artifacts are streamed, hashed, stored by SHA-256 digest, and deduplicated by content. Over-limit artifacts are omitted with explicit metadata rather than persisted as misleading truncated binary objects.

The manifest distinguishes execution completion, evidence completion, and transaction validity. A transaction-valid incomplete bundle is allowed only with a bounded failure record and explicit capture states. No complete bundle may hide truncation, omission, or failed required evidence.

The manifest digest uses the domain `glassroot.dev/evidence-manifest-json/v1\0`, a big-endian length, and exact manifest bytes. It excludes the manifest's own digest. It is not a signature, attestation, authentication mechanism, canonical JSON claim, or proof that observations are true.

Additive model changes introduce schema-version constants for `execution-result`, `attempt-result`, and `artifact-index`. Existing GR-3 fixtures remain compatible.

## Security considerations

Event payloads, logs, logical artifact paths, artifact bytes, and runner observations remain hostile. A compromised backend can emit false observations, and a compromised writer can forge or omit evidence. GR-8A preserves runner identity, capabilities, provenance, limitations, capture states, sizes, and digests so later stages can reason about those facts.

The trusted parent contract is essential: it must not be controlled by the repository or analyzed workload. Same-UID concurrent mutation, malicious mounts, hostile filesystems, compromised kernels, and compromised control-plane code remain out of scope.

GR-8A does not provide a hostile bundle reader. Existing bundles must be verified by GR-8B before comparison, policy, or rendering.

## Alternatives considered

- **Single writer/reader issue:** rejected because reader verification needs a stricter hostile-input design.
- **`.grb` archive now:** rejected to avoid archive path parsing and compression semantics before the directory format is stable.
- **Write directly into the final directory:** rejected because readers could observe partial bundles.
- **Store logical artifact paths physically:** rejected because logical paths are hostile data.
- **Truncate binary artifacts in place:** rejected because policy/reporting could mistake a prefix for complete content.
- **Include actual runner capabilities in the plan:** rejected because that mutates the frozen plan and digest after backend selection.

## Consequences

GR-8A gives fake-runner and future early milestones a deterministic persistent sink with explicit incompleteness. Later readers must not assume bundles are safe merely because this writer can produce valid bundles; every existing bundle remains hostile input.

## Operational and migration impact

No CLI behavior changes. No dependencies are added. `go.mod`, `go.sum`, workflows, public pipeline schema, config, gitstore, materializer, planner behavior, GR-7A plan fixture, and GR-7B fake event fixture remain unchanged.

## Validation plan

Validation covers exact plan-byte preservation, attempt routing, JSONL framing, complete and incomplete commits, log truncation, artifact omission and deduplication, manifest digest golden output, publication cleanup, fault injection for sync/publish failures, ownership checks, and fuzz targets for entry paths, event lines, logical artifact paths, and manifest normalization.

## References

- KICKSTART.md GR-8A
- docs/THREAT_MODEL.md
- docs/PLANNING.md
- docs/RUNNER_CONTRACT.md
- docs/FAKE_RUNNER.md
- docs/EVIDENCE_BUNDLE_FORMAT.md
- docs/adr/0001-core-model-schema-versioning.md
- docs/adr/0007-runner-contract-and-fake-runner.md
- internal/evidence
