# Typed evidence normalization

GR-9A sits after strict evidence-bundle verification (GR-8B) and before behavioral comparison (GR-9B). It consumes only an open, verified `*evidence.Bundle` and converts verified observation events into deterministic typed facts. It does not read raw bundle paths, inspect artifacts or logs, execute target content, render evidence, compare base/head behavior, or persist normalized output into the bundle.

Raw evidence remains authoritative. Normalized facts are derived data. Every fact retains a verified event-stream reference containing the logical bundle path, SHA-256 event-stream digest, event ID, global sequence, revision, scenario ID, and repetition. Normalized facts are not raw observations.

## Profile

The initial profile is:

`glassroot.dev/normalization-profile/v1alpha1`

The profile records:

- the profile version;
- trusted-base compare-ignore fields;
- process identity algorithm `glassroot.dev/normalized-process-id/v1`;
- timestamp algorithm `glassroot.dev/source-relative-timestamp/v1`;
- path-root algorithm `glassroot.dev/rooted-posix-path/v1`;
- ordered trusted root aliases derived from the verified plan.

The profile derives from the verified run plan and fixed Glassroot behavior. Head configuration, event payloads, repository files, and bundle content cannot supply rewrite rules. Supported ignore fields are exactly `event.timestamp` and `process.pid`; unknown ignore fields fail closed.

## Event coverage and provenance

Every current `model.ObservationKind` has an explicit handler: process lifecycle, filesystem activity, DNS/network activity, artifact activity, scenario lifecycle, observer warnings, unsupported observations, and resource limits. Unknown kinds fail closed rather than being grouped into an "other" bucket.

Observation source is preserved as a semantic field. Sources such as `synthetic-test-generated`, `host-observed`, `sandbox-runtime-observed`, `network-broker-observed`, and `workload-reported` are not collapsed or relabeled.

## Process identity

Numeric PIDs and PPIDs are not cross-run identities and are not retained in normalized process payloads. The normalizer builds source-scoped, lineage-aware identities per attempt and observation source:

1. process events are consumed in verified sequence order;
2. a process-start opens a PID generation;
3. active PID reuse records a limitation;
4. reuse after exit creates a new generation;
5. a child start links to the active parent generation when available;
6. missing parents and actor-less exits remain visible with explicit unresolved markers;
7. a stable `proc-<sha256>` ID is computed from source, parent identity/marker, normalized executable attributes, and an occurrence index for indistinguishable siblings.

The `process.pid` ignore field only excludes raw numeric PID/PPID. It does not erase process existence, parent-child relationships, executables, lifecycle events, or exit data.

## Timestamp normalization

Absolute event timestamps are never cross-run equality keys. For each attempt and observation source, the first timestamp establishes a source-local origin. Later events record integer nanosecond offsets from that origin. The global event sequence remains authoritative for ordering; clocks from different sources are never synchronized or used to reorder events. Clock regressions become limitations.

When trusted-base comparison ignores `event.timestamp`, source-relative timing is retained as metadata but excluded from semantic fact digests. Without that ignore field, the relative offset participates exactly, with no tolerance.

## Path normalization

Only explicitly typed path fields are normalized: process executable paths, filesystem paths, and artifact logical paths that model sandbox paths. The normalizer never rewrites arbitrary command strings, warning messages, log text, DNS names, URLs, or artifact bytes.

Path normalization is lexical POSIX reasoning using trusted roots from the verified plan. The workdir receives a dedicated alias; collection roots keep deterministic plan order after exact duplicates are skipped. Overlapping roots use the longest whole-component match; ties prefer workdir and then lower configured root index. `/workspace2` is not under `/workspace`.

Paths are represented structurally with a namespace (`workdir-root`, `collection-root`, `absolute-unmapped`, `relative`, or `opaque-invalid`), root index, relative component, original literal, and deterministic display token. Unicode is not normalized; byte-distinct Unicode remains distinct. Invalid or ambiguous paths remain visible with explicit state instead of being cleaned into acceptance.

## Fact identities and digests

Each fact has two identifiers:

- `SemanticDigest`: `sha256:<hex>` over `glassroot.dev/normalized-observation/v1\0` plus a binary length-prefixed encoding of the profile version, fact kind, observation source, normalized typed payload, and timing only when included by profile.
- `FactID`: `fact-<hex>` over `glassroot.dev/normalized-fact-id/v1\0` plus plan digest, attempt ID, semantic digest, and raw event IDs.

Semantic digests exclude revision kind, attempt ordinal, run ID, event ID, event sequence, manifest digest, event-stream digest, raw PID/PPID, absolute timestamp, physical bundle path, and provenance metadata. They are equality keys for GR-9B only; they are not authentication, provenance, signatures, attestations, or proof that behavior is safe.

## Completeness

The trace preserves transaction-valid bundle metadata, execution completeness, evidence completeness, attempt coverage, capture states, limitations, observer warnings, and synthetic evidence limitations. An attempt with no events is not treated as evidence of no behavior. Truncated logs and omitted artifacts remain limitations even though GR-9A does not parse log or artifact bytes.

When a bundle was verified without an independently supplied expected manifest digest, normalization may proceed but the trace records `internal-consistency-only` verification and makes no provenance claim.

## Limits and ownership

Normalization uses explicit caller limits bounded by absolute ceilings for attempts, facts, evidence references, process generations, active processes, normalized strings, path roots, and limitations. Context cancellation and deadlines fail closed without returning a partial trace.

Returned trace data is owned and copied by accessors. The normalizer does not retain raw event streams, logs, artifacts, bundle callbacks, file descriptors, host paths, or mutable caller input.

## Deferred work

GR-9B compares normalized base/head attempts and repetitions. GR-10 converts deltas into policy findings. GR-11 safely renders hostile evidence and derived facts. Hostname correlation, cross-source process correlation, richer path namespaces, and persisted normalized traces are future explicit design work.
