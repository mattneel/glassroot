# Deterministic built-in policy

GR-10A evaluates immutable behavioral deltas after GR-9B comparison and before
GR-11 rendering. Policy input is a `compare.FrozenDelta`; the evaluator does not
read evidence bundles, workspaces, logs, artifacts, repository files, or command
text. Findings are derived judgments over typed delta records, not raw
observations and not proof of malicious intent.

## Profile and rule-set identity

The only GR-10A repository-facing profile name is:

```text
strict
```

Its exact implementation identities are:

```text
glassroot.dev/policy-profile/strict/v1alpha1
glassroot.dev/builtin-rules/strict/v1alpha1
glassroot.dev/policy-evaluation/v1alpha1
```

Repository content cannot define, disable, reorder, or modify rules. Head
configuration cannot select policy behavior. GR-10A has no OPA, Rego, CEL,
plugins, templates, regular-expression rules, or custom policy language.

## Rule catalog

Emitted rules in `strict/v1alpha1` are:

- `GR-OBS-001` — Observation coverage incomplete or weakened.
- `GR-PROC-001` — New process or executable.
- `GR-FS-001` — New executable file or artifact.
- `GR-FS-002` — New filesystem access outside configured roots.
- `GR-NET-001` — New or changed network behavior.
- `GR-ART-001` — New or changed artifact.
- `GR-DET-001` — Behavioral repeatability degraded.
- `GR-LIMIT-001` — Resource limit behavior introduced.

Reserved for GR-10B and not emitted in GR-10A:

- `GR-CONFIG-001` — Trusted security configuration changed in head.
- `GR-WAIVER-001` — Waiver added, changed, invalid, or expired.

Rule titles are fixed metadata and never interpolate hostile paths, endpoints,
arguments, warning messages, or artifact names.

## Direction and exclusions

The initial profile is head-positive. It evaluates additions, modifications,
head count increases, coverage-limited positive head observations, degraded
repeatability, and incomplete/synthetic evidence. It generally does not emit
ordinary behavior findings for removal-only records, count decreases, base-only
behavior, head repeatability improvement, or order-only records. These remain in
the behavioral delta for rendering and future policy versions.

Incomplete evidence is never treated as clean behavior. A positive observation
may still trigger a finding when the other side is incomplete, but the finding
must not claim that absence was established on the incomplete side.

## Rule triggers

### GR-OBS-001

Triggers on global incomplete execution/evidence, decreased coverage records,
new observer warnings or unsupported-observation facts, and typed evidence
context indicating synthetic evidence or no target-code execution. Execution or
evidence incompleteness receives severity `high`, confidence `high`, and
disposition `failed`. A complete evaluation whose immutable `BehavioralDelta`
context says `syntheticEvidence=true` or `executesTargetCode=false` receives
exactly one global severity `medium`, confidence `high`, disposition
`requires-review` finding. This can happen even when there are zero ordinary
behavior-change records. Internal-consistency-only manifest verification is
retained as an evaluation limitation but does not by itself emit a finding.

### GR-PROC-001

Triggers on head-positive process-start behavior and process occurrence-count
increases. It uses typed process facts and normalized process identities from the
linked delta. Process removals, count decreases, and changed exit status alone do
not trigger this rule.

### GR-FS-001

Triggers when typed head filesystem or artifact behavior establishes a new or
changed executable entry. Executability is read only from typed executable/mode
fields; filename extensions and artifact bytes are not inspected. Executable
artifact behavior may also trigger `GR-ART-001` because the rules represent
separate concerns.

### GR-FS-002

Triggers only for typed filesystem facts whose normalized path namespace is
`absolute-unmapped` and whose operation is in the reviewed table. Mutation
operations such as create, write, delete, rename, chmod, or permission change are
`high` severity. Read or metadata operations are `medium` severity. Relative,
mapped workdir/collection-root, and opaque-invalid paths do not trigger the rule.
Unknown future filesystem operations fail closed for review.

### GR-NET-001

Triggers on head-positive typed DNS/network behavior: new queries, connection
attempts, destinations, denied attempts, result changes for the same typed
anchor, and count increases. A denied attempt is still behavior. The rule does
not infer data exfiltration and does not parse endpoints from prose.

### GR-ART-001

Triggers on head-positive artifact additions, modifications, count increases, or
changes to digest, size, type/disposition when modeled, or executable state. It
never inspects artifact bytes and does not infer type from file extensions.

### GR-DET-001

Triggers on stability-changed records where head repeatability is less
assessable or more variable than base. It records deterministic repeatability
states only; it emits no probabilities and no malicious-intent assessment.

### GR-LIMIT-001

Triggers on head-positive typed resource-limit behavior. It does not infer
limits from target failure or logs.

## Severity, confidence, and disposition

Severity is the deterministic impact category for the built-in rule. It is not a
probability and does not encode waiver state.

Confidence is deterministic evidence strength, not model confidence or a
statistical probability. The basis table is:

- `complete-observation` -> `high`
- `single-sample` -> `medium`
- `repetition-variable` -> `medium`
- `coverage-limited` -> `low`
- `ambiguous-correlation` -> `low`

Source caps are applied explicitly:

- host, network broker, and sandbox runtime observations: no reduction;
- guest-agent, workload, and static-analysis observations: at most `medium`;
- model-inferred observations: `low`;
- synthetic-test-generated target-behavior observations: `low`.

GR-OBS-001 evidence-state findings may be `high` confidence because the state is
directly represented by typed evidence/comparison context.

The synthetic/no-target GR-OBS-001 decision uses only typed context carried in
the immutable `BehavioralDelta`; renderer notices and limitation prose do not
drive policy. Synthetic evidence can support plumbing tests, but strict v1 does
not treat it as sufficient for target-behavior conclusions.

Disposition is independent. GR-10A uses `failed` only when execution or evidence
is incomplete. Other findings are `requires-review`. An evaluation with no
findings is overall `passed`; any failed finding makes the evaluation overall
`failed`; otherwise findings make it `requires-review`. GR-10A never emits a
`waived` finding and all findings have `waived=false`.

## Finding and evaluation identity

Finding IDs use:

```text
glassroot.dev/finding-id/v1\0
```

The encoding binds the policy profile version, rule-set version, rule ID, rule
version, ordered delta-record IDs or a fixed global scope key, and scenario IDs.
It excludes run ID, plan digest, manifest digest, event IDs, event sequences,
evidence-reference paths, raw evidence references, wall clock, and waiver state.
Finding IDs are deterministic equality keys only.

A frozen policy evaluation uses compact `encoding/json` output and digest domain:

```text
glassroot.dev/policy-evaluation-json/v1\0
|| uint64-big-endian(json-byte-length)
|| exact-json-bytes
```

The digest is not a signature, authentication, authorization, provenance,
attestation, canonical JSON claim, or proof that the policy is correct.

## Traceability and ordering

Every record-scoped finding retains delta-record IDs, scenario IDs, side flags,
limitations, and copied evidence references. Evidence logical paths remain data
only and are never used as filesystem paths. Findings are ordered by disposition,
severity, rule ID, scenario order, first delta-record ID, and finding ID.

## Limits

Policy evaluation has explicit ceilings for delta records, findings, findings
per delta record, delta-record IDs, scenario IDs, evidence references,
limitations, title/summary/rule-id bytes, and evaluation JSON size. Limits may be
lowered by trusted callers but cannot be raised above absolute ceilings. Limit
failures return no frozen evaluation; findings are not silently truncated.

## Waivers and future work

GR-10A does not load or apply waivers. GR-10B loads waivers only from trusted
base state, annotates findings without deleting them, and emits waiver and
configuration governance rules where applicable. GR-11A composes the final
policy application with the behavioral delta and verified bundle, then renders
the frozen report safely. No findings does not prove safety; it only means this
fixed rule set did not emit a finding for the supplied delta.

## Final policy application (GR-10B)

GR-10B applies trusted-base waivers after GR-10A. The production application API
accepts the immutable `FrozenEvaluation`, the exact `pipeline.FrozenPlan`, the
trusted base configuration load result, a `RevisionFileSource`, and explicit
`evaluatedAt`. It rejects mismatched run IDs, plan digests, commits, effective
configuration source, policy profile, evaluation identity, and nonzero legacy
runner facts.

Final application introduces:

```text
glassroot.dev/policy-application/v1alpha1
glassroot.dev/governance-rules/strict/v1alpha1
```

It preserves `glassroot.dev/builtin-rules/strict/v1alpha1` unchanged. Original
GR-10A findings remain present with `waived=false` and original disposition.
Application records a separate effective disposition and optional applied-waiver
metadata.

Overall effective disposition is `failed` if any effective finding is failed,
`requires-review` if no failed finding exists and any effective finding requires
review, and `passed` otherwise, including when every otherwise-review finding is
explicitly waived. Waived findings still count as findings.

### Governance rules

`GR-CONFIG-001` belongs to the governance rule set and reports trusted head
configuration changes. It does not mean the head configuration was applied; the
effective configuration remains trusted base. Valid semantic config changes emit
one finding per `config.ConfigChange`. Privilege increase, observation weakened,
execution definition change, policy change, and unknown effects are high
severity. Privilege decrease and observation strengthened are medium. Informational
changes are low. Invalid, removed, or unsupported head configuration state is
high severity. Confidence is high for the configuration comparison fact.

`GR-WAIVER-001` reports waiver governance issues. Invalid or unsupported trusted
base waiver content, duplicate/broad targets, and invalid lifetime are high and
failed. Expired, not-yet-valid, unused, target mismatch, and ineligible trusted
base waivers are medium and require review. Head waiver additions, removals,
target changes, expiry changes, invalid content, and unsupported entries are
high and require review; owner, reason, and issued-at changes are medium and
require review. Formatting-only semantic equivalence is recorded in metadata and
emits no governance finding.

Neither governance rule can be waived in GR-10B.

See `docs/WAIVERS.md` for the fixed waiver format, expiry boundary, authority
rules, and strict parser limits.
