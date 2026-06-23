# ADR: Deterministic built-in policy

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-9B produces immutable behavioral deltas from normalized, verified evidence.
GR-10 originally combined built-in policy rules and trusted-base waivers, but
those are separate trust boundaries. Built-in rules are fixed Glassroot code over
a completed delta. Waivers require reading trusted base configuration at an
explicit evaluation time and must never be controlled by head content.

Policy is security-sensitive because it turns derived behavior differences into
findings. It must not execute, render, inspect artifacts, parse logs, or infer
malicious intent. It also must not let repository content define, disable, or
reorder rules.

## Decision

Glassroot splits GR-10 into GR-10A deterministic built-in policy and GR-10B
trusted-base waiver application. GR-10A adds `internal/policy` and accepts only a
non-nil `*compare.FrozenDelta` in its production API. It validates the complete
delta, recomputes the delta JSON digest through a narrow compare helper, and
returns an immutable `FrozenEvaluation` with owned document, compact JSON bytes,
and digest.

The only repository-facing profile is `strict`. Its exact identities are:

- `glassroot.dev/policy-profile/strict/v1alpha1`
- `glassroot.dev/builtin-rules/strict/v1alpha1`
- `glassroot.dev/policy-evaluation/v1alpha1`

The initial emitted rule IDs are `GR-OBS-001`, `GR-PROC-001`, `GR-FS-001`,
`GR-FS-002`, `GR-NET-001`, `GR-ART-001`, `GR-DET-001`, and `GR-LIMIT-001`.
`GR-CONFIG-001` and `GR-WAIVER-001` are reserved and documented for GR-10B but
are not emitted in GR-10A.

Rules are head-positive unless they concern coverage or evidence state.
Removal-only records, count decreases, order-only changes, base-only behavior,
and head repeatability improvements do not produce ordinary behavior findings in
v1alpha1. Incomplete evidence cannot establish absence. Synthetic evidence is
represented explicitly and does not imply target code behaved that way.

Severity, confidence, and disposition are separate. Severity is fixed by rule
and typed condition. Confidence is a deterministic evidence-strength category
from comparison basis plus observation-source caps, not model confidence or
probability. Disposition is `failed` only for incomplete execution/evidence;
other GR-10A findings are `requires-review`; no GR-10A finding is waived.

Finding titles and summaries use fixed templates without hostile interpolation.
Findings retain rule version, delta-record IDs, scenario IDs, side observation
flags, copied evidence references, and limitations. Finding IDs use the domain
`glassroot.dev/finding-id/v1\0` and bind policy/rule identity plus deterministic
scope and delta-record IDs. Evaluation digests use
`glassroot.dev/policy-evaluation-json/v1\0 || uint64 length || exact JSON`.
These identifiers are deterministic equality keys only.

Model changes are additive: `model.Finding` gains optional `ruleVersion` and
`deltaRecordIds` fields. Existing fixtures and schema versions remain valid.

## Security considerations

Policy is trusted code and can create false findings or omit meaningful behavior
if compromised or incorrectly implemented. GR-10A therefore fails closed on
unknown profiles, malformed deltas, unsupported kinds, unsupported bases,
unknown sources, invalid typed payloads, duplicate IDs, and limits.

Rules use typed normalized behavior rather than prose, regular expressions,
substring matching, edit distance, machine learning, shell parsing, or artifact
content inspection. Repository content cannot define or disable rules, and head
configuration cannot select policy behavior.

Findings do not establish malicious intent, authentication, provenance,
attestation, or sandbox guarantees. Severity is not probability. Confidence is
not statistical or model confidence. Evaluation digests and finding IDs provide
repeatability only.

GR-10A applies no waivers. Formatting in GR-11 cannot remove or alter the
frozen finding set.

## Alternatives considered

- **Combine waivers with built-in rules:** rejected because waiver loading needs
  trusted-base file access, expiry time, and separate failure semantics.
- **Use OPA/Rego/CEL or plugins:** rejected for GR-10A because repository or
  extension-controlled rules would expand the trust boundary.
- **Emit policy directly from comparison:** rejected to keep behavior
  classification separate from policy judgments.
- **Interpolate evidence strings into summaries:** rejected because paths,
  endpoints, arguments, and warning messages remain hostile data for GR-11.
- **Treat removals as clean or risky by default:** rejected; v1 policy is
  head-positive and leaves removals visible in the behavioral delta.

## Consequences

GR-10A gives later rendering and policy application a deterministic finding set
with stable rule IDs and raw-evidence traceability. It intentionally may be
conservative: some records produce no findings until a future reviewed rule version
exists.

The policy evaluation is not persisted into evidence bundles in this issue.
GR-10B must preserve original unwaived findings when annotating waivers.
GR-11 must render both deltas and evaluations safely without changing them.

## Operational and migration impact

No CLI behavior, workflows, evidence bundles, reader/writer behavior, runner,
normalizer, or comparator semantics are changed. No dependencies are added.
Existing compatibility fixtures remain valid; a new policy golden fixture records
current GR-10A output.

## Validation plan

Validation covers nil input, profile rejection, delta validation, rule triggers
and exclusions, incomplete and synthetic evidence behavior, confidence/source
caps, finding IDs, frozen JSON and digest, ownership, golden evaluation output,
fuzz seeds for confidence, finding ID encoding, and delta-record evaluation, plus
repository-wide tests, race tests, vet, schema checks, govulncheck, and diff
audits.

## References

- KICKSTART.md GR-10A
- docs/POLICY.md
- docs/COMPARISON.md
- docs/NORMALIZATION.md
- docs/THREAT_MODEL.md
- docs/adr/0011-deterministic-behavioral-comparison.md
- internal/compare
- internal/policy
