# ADR: Deterministic behavioral comparison

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-9A produces normalized typed traces from verified evidence. GR-9B must compare base and head behavior without reopening bundles, executing content, rendering hostile strings, or turning derived data into policy findings. Comparison is distinct from normalization and policy: normalization removes documented nondeterminism, comparison classifies differences and coverage, and GR-10 will decide policy.

## Decision

Glassroot adds `internal/compare`. Its production entry point accepts only a non-nil `*observe.TraceSet`; it does not accept raw events, raw JSON, bundle paths, or trace documents supplied as a trust substitute.

The initial comparison profile is `glassroot.dev/comparison-profile/v1alpha1`. It requires normalization profile `glassroot.dev/normalization-profile/v1alpha1` and records exact semantic multiset matching, typed-anchor correlation, repetition occurrence profiling, strict semantic sequence ordering, and the unknown-is-not-absence policy.

The comparator groups attempts by scenario, revision, and repetition. Exact semantic matches by GR-9A `SemanticDigest`, fact kind, and observation source are resolved first as multisets. Occurrence profiles record planned repetitions, per-repetition known counts, complete/incomplete counts, minimum/maximum/total known counts, coverage, and repeatability. Repeatability distinguishes stable, variable, single-sample, and not-assessable states without probabilities.

Incomplete coverage cannot establish absence. Definitive additions and removals require positive evidence on one side and complete relevant coverage on the absence side. Positive observations under incomplete coverage remain visible with coverage-limited basis.

Unmatched facts may be correlated as modifications only through one-to-one typed anchors. Anchor fields are deliberately selected per fact kind: process stable identity and operation; filesystem operation and primary normalized path; network/DNS operation and query/destination identity; artifact logical path; scenario lifecycle phase; warning code; and resource category. Ambiguous anchor groups remain add/remove or count records with an `ambiguous-correlation` limitation.

Changed-field inventories are typed and deterministic. They exclude fact IDs, semantic digests, evidence references, event IDs, sequences, run IDs, absolute timestamps, raw PID/PPID, and normalization-excluded fields. Order-change records are emitted only for complete, repeatably identical per-side sequences with equal multisets and different semantic order.

The output uses existing schema version `glassroot.dev/behavioral-delta/v1alpha1` with additive model fields for comparison profile, normalization profile, plan/manifest binding, manifest verification mode, completeness states, scenario comparisons, summary counts, generic delta kinds, comparison basis, occurrence profiles, typed normalized fact snapshots, and side-specific evidence references. Legacy delta-kind wire values are preserved.

`FrozenDelta` owns an immutable document copy, compact JSON bytes, and a digest over `glassroot.dev/behavioral-delta-json/v1\0 || uint64 length || JSON`. Record IDs use `glassroot.dev/behavioral-delta-record-id/v1\0`. Anchor digests use `glassroot.dev/comparison-anchor/v1\0`. These identifiers are deterministic equality keys only.

## Security considerations

Comparison is trusted code and can create false differences or hide real differences if implemented incorrectly. It therefore fails closed on unsupported profiles, malformed normalized facts, duplicate identities, unsupported sources, malformed semantic digests, invalid evidence references, and limit violations.

The comparator does not execute, render, inspect artifact contents, parse logs, read bundles, assign severity/confidence/disposition, infer malicious intent, sign, attest, or make sandbox/provenance/authentication claims. Behavioral strings remain hostile data for GR-11. Delta IDs and digests are not authentication or proof that observations are truthful.

## Alternatives considered

- **Compare raw events directly:** rejected because GR-9A already defines the safe lossy boundary for PIDs, clocks, and roots.
- **Use fuzzy matching or regex correlation:** rejected because it could hide or invent behavior changes.
- **Treat incomplete repetitions as zero observations:** rejected; unknown is not absence.
- **Use statistical repeatability:** rejected for M1/M2. GR-9B records exact counts and deterministic states only.
- **Emit policy findings directly:** rejected; GR-10 owns policy semantics and reviewable rule IDs.
- **Persist deltas into bundles now:** rejected; GR-8A/GR-8B remain evidence format work, and persistence can be designed later.

## Consequences

GR-9B provides deterministic, policy-usable comparison records while preserving provenance and coverage limits. It may report coverage-limited or ambiguous records instead of definitive additions/removals when evidence cannot establish absence or correlation. Consumers must treat deltas as derived data, not findings or proof.

## Operational and migration impact

The behavioral-delta schema version remains v1alpha1. Model changes are additive: existing fixtures continue to decode, and legacy delta-kind values remain unchanged. No CLI, workflow, evidence-bundle writer/reader, runner, normalizer behavior, or public JSON Schema changes are introduced.

## Validation plan

Validation covers input binding, unsupported profiles, duplicate attempts/facts, exact semantic matching, absence rules, modifications by typed anchors, ambiguous correlation, count changes, repeatability states, strict order comparison, coverage records, evidence references, deterministic record IDs, frozen JSON and digest, ownership, golden behavioral delta output, and fuzz targets for occurrence profiles, typed anchors, and delta-record encoding.

## References

- KICKSTART.md GR-9B
- docs/COMPARISON.md
- docs/NORMALIZATION.md
- docs/EVIDENCE_BUNDLE_READER.md
- docs/THREAT_MODEL.md
- docs/adr/0010-typed-evidence-normalization.md
- internal/compare
