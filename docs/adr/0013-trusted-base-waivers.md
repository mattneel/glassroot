# ADR: Trusted-base waivers and final policy application

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-10A emits immutable unwaived findings from deterministic built-in rules.
Waiver application is a separate trust boundary: it reads repository policy input
from exact revisions, evaluates expiry at an explicit time, inspects proposed
head changes, and must preserve original policy meaning. Combining this with
built-in evaluation would allow trusted-base file access, head proposal handling,
and waiver semantics to contaminate the fixed rule catalog.

## Decision

Glassroot adds `internal/waiver` and GR-10B application code under
`internal/policy`. The only waiver path is `.glassroot/waivers.yaml`. The base
file is optional and is the sole repository waiver authority. Head waiver content
is inspection-only and cannot fill, repair, or modify effective base waiver
application.

Waivers use a strict bounded YAML subset with exact shape:
`apiVersion: glassroot.dev/v1alpha1`, `kind: WaiverSet`, `metadata.name:
default`, and `spec.waivers`. The parser rejects duplicate keys, aliases,
anchors, merge keys, custom tags, complex/non-string keys, multiple documents,
directives, unknown fields, null required values, invalid UTF-8, NUL, excessive
size, excessive depth, excessive nodes, and excessive scalars. A document
containing any invalid waiver applies no waivers.

Each waiver targets exactly one `finding-<sha256>` ID and one matching eligible
rule ID. Wildcards, globs, regexes, scenario/path/package selectors, branch
selectors, rule-only waivers, and permanent waivers are rejected or unsupported.
Eligible rules are `GR-PROC-001`, `GR-FS-001`, `GR-FS-002`, `GR-NET-001`,
`GR-ART-001`, `GR-DET-001`, and `GR-LIMIT-001`. `GR-OBS-001`, failed findings,
configuration governance, and waiver governance are ineligible.

Application receives `evaluatedAt` explicitly. It must be UTC, nonzero, within
the supported range, and has no clock-skew tolerance. A waiver is active when
`evaluatedAt >= issuedAt` and `evaluatedAt < expiresAt`; maximum lifetime is 90
days.

The final application binds the GR-10A evaluation, plan, trusted config, waiver
base/head revisions, and evaluation time. It preserves every original finding and
records a separate effective disposition plus optional waiver metadata. Waiver
owner/reason are retained as hostile metadata and are not authorization.

GR-CONFIG-001 and GR-WAIVER-001 are emitted from a separate governance rule-set
identity:

```text
glassroot.dev/governance-rules/strict/v1alpha1
```

The GR-10A built-in rule-set identity remains:

```text
glassroot.dev/builtin-rules/strict/v1alpha1
```

The final document schema is `glassroot.dev/policy-application/v1alpha1`.
Application digests use
`glassroot.dev/policy-application-json/v1\0 || uint64 length || exact JSON`.
Raw YAML is never embedded. Raw and semantic waiver digests are deterministic
identity data only and are not canonical YAML, signatures, provenance, or
attestations.

## Consequences

Waivers can reduce an eligible finding's effective disposition to `waived`, but
they never delete findings and never change original severity, confidence,
evidence, title, summary, rule identity, or unwaived disposition. Invalid,
expired, not-yet-valid, unused, mismatched, and ineligible waivers remain visible.
Head waiver changes and trusted config changes produce governance findings.

GR-11 can render original findings, effective dispositions, applied waivers,
waiver status records, and governance findings without changing the frozen
application.

## Alternatives considered

- **Apply head waivers:** rejected because head content is lower-trust proposal
  data for the current run.
- **Allow rule-wide or wildcard waivers:** rejected because broad scope can hide
  unrelated findings and weakens reviewability.
- **Treat maintainer metadata as authorization:** rejected; owner/reason are
  display metadata until a future authorization/signature milestone.
- **Use custom policy/OPA/Rego for waivers:** rejected; GR-10B is fixed code over
  exact finding targets only.
- **Delete waived findings:** rejected because original policy judgments must
  remain recoverable.

## Security considerations

A compromised waiver applier can forge, omit, or misclassify final policy
consequences. A compromised clock source can change expiry decisions. A malicious
head can propose waiver changes, but those proposals are inspection-only. A
trusted-base repository administrator can author narrow waivers, but that does
not prove behavior is safe or authorized by any external scheme.

The application performs no rendering, signing, target execution, workspace
access, log/artifact parsing, network access, or sandboxing.

## Validation plan

Validation covers strict YAML parsing, semantic waiver validation, fixed-path
revision loading, base invalidity, head noninterference, expiry boundaries,
cross-input binding, eligibility and ineligible targets, GR-CONFIG-001,
GR-WAIVER-001, frozen application JSON/digest, ownership, golden application
output, fuzz seeds for parser/application behavior, full tests, race tests, vet,
schema checks, govulncheck, and diff/import audits.

## References

- KICKSTART.md GR-10B
- docs/WAIVERS.md
- docs/POLICY.md
- policies/builtin/strict-v1alpha1.md
- docs/THREAT_MODEL.md
- docs/adr/0012-deterministic-builtin-policy.md
- internal/waiver
- internal/policy
