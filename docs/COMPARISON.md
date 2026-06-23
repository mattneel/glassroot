# Deterministic behavioral comparison

GR-9B sits after typed evidence normalization (GR-9A) and before deterministic policy rules (GR-10). It consumes only an `*observe.TraceSet` and produces a frozen `glassroot.dev/behavioral-delta/v1alpha1` document. It does not read evidence bundles, inspect artifact bytes, parse logs, execute target content, render output, assign severity, or infer intent.

A behavioral delta is derived comparison data. Raw evidence remains authoritative and every observed side of a delta retains verified event-stream references.

## Comparison profile

The initial profile is:

`glassroot.dev/comparison-profile/v1alpha1`

It records fixed Glassroot behavior:

- required normalization profile: `glassroot.dev/normalization-profile/v1alpha1`;
- exact semantic multiset matching: `glassroot.dev/exact-semantic-multiset/v1`;
- typed anchor algorithm: `glassroot.dev/comparison-anchor/v1`;
- repetition assessment: `glassroot.dev/repetition-occurrence-profile/v1`;
- strict order assessment: `glassroot.dev/strict-semantic-sequence/v1`;
- absence policy: `glassroot.dev/unknown-is-not-absence/v1`;
- the included normalized fact-kind inventory.

The profile is not repository configuration. It does not accept regexes, fuzzy thresholds, string tolerances, or event-supplied rewrite rules. Behavior changes require explicit review and a new profile version.

## Input binding

The comparator rejects nil traces, unsupported normalization profiles, malformed plan or manifest digests, reordered attempts, duplicate attempts, duplicate fact IDs, unsupported fact kinds, unsupported observation sources, invalid typed payloads, malformed semantic digests, and evidence references that do not identify their own attempt.

A coherent incomplete trace is valid comparison input. Incompleteness is comparison data, not an empty observation.

## Exact semantic matching

Exact `SemanticDigest` matches are resolved before any modification correlation. Facts are treated as multisets by scenario, revision, repetition, fact kind, observation source, and semantic digest. Observation source is semantic: synthetic, host, sandbox, broker, guest, workload, static, and model-derived facts are not collapsed.

Differences in run ID, fact ID, event ID, event sequence, manifest digest, raw PID, absolute timestamp, or evidence reference do not create a behavioral modification after GR-9A normalization.

## Repetitions and occurrence profiles

Each semantic variant receives an occurrence profile containing planned repetitions, per-repetition known counts, complete and incomplete repetition counts, minimum/maximum known counts, total known count, coverage, and repeatability.

Coverage states:

- `complete`: all planned repetitions have complete event coverage;
- `partial`: at least one repetition has complete coverage and at least one does not;
- `none`: no complete repetition establishes a known count.

Repeatability states:

- `stable`: at least two complete repetitions have equal counts and no unknown repetition;
- `variable`: at least two complete repetitions have differing counts;
- `single-sample`: exactly one complete repetition and no unknown repetition;
- `not-assessable`: incomplete coverage or no complete repetitions.

One repetition is never treated as proof of determinism. Unknown count is not zero.

## Absence rules

Unknown is not absence.

A definitive `added` record requires positive head evidence and complete relevant base coverage for the repetitions used to establish absence. A definitive `removed` record requires positive base evidence and complete relevant head coverage. When the absence side is incomplete, the observed side is retained with `coverage-limited` basis rather than reported as an ordinary definitive addition or removal.

Examples:

1. Stable addition: base complete repetitions `[0, 0]`, head complete repetitions `[1, 1]` -> `added`, `complete-observation`.
2. Head nondeterminism: base `[0, 0]`, head `[0, 1]` -> variable occurrence data and a stability/count delta where applicable, not a stable addition.
3. Incomplete base: base `[unknown]`, head `[1]` -> head-only observation with `coverage-limited` basis.
4. Artifact modification: same logical artifact path, base digest A, head digest B -> `modified` with changed field `artifact.digest`.

## Typed modification anchors

After exact matching, unmatched facts may be correlated as modifications only through one-to-one typed anchors. Anchor digests use domain `glassroot.dev/comparison-anchor/v1\0` with length-prefixed binary encoding. There is no fuzzy matching, edit distance, regular expression correlation, machine learning, or arbitrary heuristic pairing.

Per-kind anchor rules:

- Process lifecycle: source, stable process identity, and operation. Executable changes that alter stable identity remain add/remove. Exit code and duration can be modifications for the same stable process.
- Filesystem: source, operation, and primary normalized path. Operation or path identity changes remain add/remove. Digest, mode, size, executable flag, truncation, or related metadata can be modifications.
- DNS/network: source, operation/category, protocol, query or destination host, and destination port. New destinations are additions. Result, denial state, response metadata, or duration can be modifications for the same destination anchor.
- Artifact: source and normalized logical artifact path. Digest, size, executable flag, or operation can be modifications. Path changes remain add/remove.
- Scenario lifecycle: source and lifecycle phase/fact kind. Target outcome, exit code-equivalent status, message, or duration can be modifications.
- Observer warning or unsupported observation: source and stable code. Message-only matching is not a correlation rule.
- Resource limit: source and resource category. Threshold, observed amount, unit, or exceeded state can be modifications.

If an anchor group has multiple unmatched variants on either side, no arbitrary pair is chosen. Records retain add/remove or count differences with an `ambiguous-correlation` limitation.

## Delta kinds and basis

GR-9B emits these generic delta kinds: `added`, `removed`, `modified`, `count-changed`, `order-changed`, `stability-changed`, and `coverage-changed`. It does not encode policy severity, confidence, disposition, waiver, rule ID, risk score, or malicious intent.

Comparison basis is separate from kind: `complete-observation`, `single-sample`, `repetition-variable`, `coverage-limited`, or `ambiguous-correlation`. Basis is deterministic evidence-state classification, not probability.

## Changed fields

Modified records include deterministic logical changed-field names selected by typed code for each fact kind. The list excludes fact IDs, semantic digests, evidence references, event IDs, sequences, run IDs, absolute timestamps, raw PID/PPID, and fields excluded by the normalization profile.

## Strict order changes

Order comparison uses GR-9A fact order and semantic digests, never timestamps. An `order-changed` record may be emitted only when both sides have complete coverage, each side has a repeatably identical sequence across complete repetitions, both sides have the same multiset, and the ordered sequences differ. Variable order or count differences suppress an order-only record.

## Coverage and limitations

Comparison preserves execution/evidence completeness, not-started attempts, missing lifecycle coverage, observer warnings, unsupported observations, log truncation limitations, artifact omission limitations, synthetic-evidence limitations, and internal-consistency-only manifest verification.

Shared limitations remain visible; side-specific limitations may produce `coverage-changed` records. Incomplete comparison data is not reported as clean absence.

## Evidence references

Observed sides carry deduplicated, sorted references with event-stream digest, logical bundle-relative stream path, event ID, event sequence, revision, scenario ID, and repetition. Added records have head references; removed records have base references; modified records have both. Logical stream paths are data, not host paths.

## Record IDs and delta digest

Delta record IDs use domain `glassroot.dev/behavioral-delta-record-id/v1\0`. They bind profile versions, scenario ID, delta kind, fact kind/category, observation source, typed anchor digest, changed fields, semantic variants, occurrence profiles, and comparison basis. They exclude run ID, plan digest, manifest digest, fact IDs, event IDs, sequences, evidence references, physical paths, and absolute timestamps.

Frozen delta JSON is compact `encoding/json` output retained by `FrozenDelta`. The delta digest uses:

`glassroot.dev/behavioral-delta-json/v1\0 || uint64-big-endian(json-byte-length) || exact-json-bytes`

The digest is for deterministic equality of this Glassroot document. It is not a signature, attestation, authentication, authorization token, canonical JSON claim, proof that observations are truthful, or proof that behavior is safe.

## Ordering, limits, and complexity

Records are ordered deterministically by scenario order, delta-kind rank, fact kind, source, anchor digest, and record ID. Slices are non-null. Maps do not contribute unordered wire content.

The comparator uses explicit caller limits bounded by absolute ceilings for scenarios, attempts, facts, semantic variants, anchors, variants per anchor, records, evidence references, limitations, and delta JSON bytes. Exact matching uses map/sort work; modification correlation groups by typed anchors rather than all-pairs fuzzy comparison. Context cancellation and deadlines fail closed without a partial delta.

## Deferred work

GR-10 applies deterministic policy rules to behavioral deltas. GR-11 safely renders hostile strings and evidence references. GR-9B does not persist `delta.json`, mutate bundles, compare raw logs or artifact bytes, sign, attest, call GitHub, or execute workloads.
