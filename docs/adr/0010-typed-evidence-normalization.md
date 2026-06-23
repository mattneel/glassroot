# ADR: Typed evidence normalization

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-8B verifies existing evidence bundles but the resulting events, logs, artifact names, and strings remain hostile evidence. GR-9 originally combined normalization and base/head comparison. These are different trust boundaries: normalization is a lossy transformation that can hide differences if it is too broad, while comparison classifies differences after normalization.

## Decision

Glassroot splits GR-9 into GR-9A typed evidence normalization and GR-9B behavioral comparison. GR-9A adds `internal/observe` and accepts only a verified `*evidence.Bundle` as its production entry point. It does not accept raw JSONL, arbitrary events, or unverified directories.

The initial normalization profile is `glassroot.dev/normalization-profile/v1alpha1`. It derives from the verified plan and fixed Glassroot behavior. Trusted-base compare-ignore fields are the only ignore authority; event payloads and repository content cannot supply regexes or rewrite rules. Unknown ignore fields fail closed.

Every current observation kind has an explicit handler. Unknown future kinds or sources fail closed until reviewed. Observation provenance is preserved as a semantic field and synthetic observations remain synthetic.

Numeric PIDs are replaced with source-scoped, lineage-aware stable process identities. Process namespaces are separated by attempt and observation source. PID reuse, unresolved parents, and actor-less references remain explicit limitations. The algorithm uses a domain-separated, length-prefixed SHA-256 encoding and does not include raw PID/PPID.

Timestamps are normalized per attempt and observation source. The first timestamp from a source is its origin and later values are integer nanosecond offsets. Global event sequence remains the ordering authority. Cross-source clock synchronization is not inferred. The `event.timestamp` ignore field excludes relative timing from semantic digests but does not remove ordering metadata.

Path normalization is structured and limited to modeled path fields. Workdir and collection-root aliases come from the verified plan. Matching is component-aware, longest-root-wins, and does not call host filesystem APIs. Arbitrary strings, command text, warning prose, logs, URLs, and artifact bytes are not rewritten. Unicode normalization is not performed.

Each normalized fact carries raw evidence references to the verified event-stream digest/path, event ID, sequence, and attempt coordinate. `SemanticDigest` is a cross-run equality key over normalized semantics. `FactID` is run-local provenance identity over plan digest, attempt ID, semantic digest, and event IDs. Neither value is authentication, signing, attestation, provenance, or proof of safe behavior.

Incomplete evidence remains explicit. Transaction validity, execution completeness, evidence completeness, attempt coverage, capture states, observer warnings, omitted artifacts, truncated logs, and synthetic limitations are retained in the trace. Normalized traces are not persisted into GR-8 bundles in this issue.

A narrow reader accessor exposes verified event-stream entry metadata by attempt without exposing host paths, file descriptors, or generic physical-path lookup. A narrow runner helper expands already verified run-plan documents using the same deterministic attempt inventory as GR-7B, without constructing a `FrozenPlan`.

## Security considerations

Normalization is trusted code. A compromised or incorrect normalizer can forge, omit, or over-normalize facts. For that reason, unknown data fails closed, raw evidence references are preserved, and only documented nondeterminism is removed.

Verified bundles remain hostile. Normalized strings are hostile derived data and require safe rendering in GR-11. Semantic digests are equality keys only and do not authenticate the writer, runner, or source.

## Alternatives considered

- **Compare raw events directly:** rejected because PIDs, timestamps, and sandbox root prefixes create known nondeterminism.
- **Persist normalized traces into the evidence bundle now:** rejected; GR-8A/GR-8B format remains stable and persistence can be designed after comparison semantics are proven.
- **Use regular expressions from repositories or events:** rejected because repository-controlled rewrite rules could hide behavior.
- **Global PID correlation across sources:** rejected until an explicit source-correlation mechanism exists.
- **Arbitrary substring path rewriting:** rejected because it can corrupt command text, warnings, logs, and hostile prose.
- **Unicode normalization:** rejected for v1alpha1; byte-distinct evidence remains distinct.

## Consequences

GR-9A introduces derived trace types under `internal/observe` and no model wire-format changes. Existing plan, event, manifest, and reader fixtures remain unchanged. The normalizer is deterministic for identical verified inputs and returns owned data, but it does not compare base and head or produce findings.

## Validation plan

Validation covers verified bundle binding, expected-manifest and internal-consistency modes, explicit kind inventory, source preservation, kind/payload mismatch rejection, PID renumbering, source-scoped process identity, PID reuse and unresolved-parent limitations, source-relative timestamps, path-root normalization, semantic digest exclusions and sensitivity, evidence references, completeness states, deterministic ordering, ownership, golden trace output, and fuzz targets for process traces, path normalization, and fact encoding.

## References

- KICKSTART.md GR-9A
- docs/NORMALIZATION.md
- docs/EVIDENCE_BUNDLE_READER.md
- docs/RUNNER_CONTRACT.md
- docs/THREAT_MODEL.md
- docs/adr/0009-strict-evidence-bundle-verification.md
- internal/observe
