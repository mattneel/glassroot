# Built-in policy catalog: strict/v1alpha1

This document describes the fixed strict policy catalogs. Production code does not
parse this file; it is not executable policy text.

## Emitted rules

### GR-OBS-001 — Observation coverage incomplete or weakened

- Version: `v1alpha1`
- Default severity: medium, except incomplete execution/evidence is high.
- Disposition: failed for incomplete execution/evidence, otherwise requires-review.
- Confidence: high for evidence-state facts; otherwise follows the deterministic basis/source table.
- Triggers: incomplete execution/evidence, head coverage decrease, added or modified observer warnings or unsupported observations, and synthetic evidence.
- Non-triggers: internal-consistency-only manifest verification by itself.
- Evidence requirements: delta completeness state, coverage records, warning facts, or synthetic source facts.
- Limitations: missing behavior is not treated as absent.

### GR-PROC-001 — New process or executable

- Version: `v1alpha1`
- Default severity: medium.
- Disposition: requires-review.
- Triggers: added process-start facts, coverage-limited head process-start observations, and process count increases.
- Non-triggers: removals, count decreases, and changed exit status alone.
- Evidence requirements: typed process facts and delta evidence references.
- Limitations: normalized process identity is not proof that two real processes are identical.

### GR-FS-001 — New executable file or artifact

- Version: `v1alpha1`
- Default severity: high.
- Disposition: requires-review.
- Triggers: new executable filesystem/artifact facts, non-executable to executable changes, and executable count increases.
- Non-triggers: executable removal and filename extension alone.
- Evidence requirements: typed executable fields.
- Limitations: file or artifact bytes are not inspected. This rule may overlap with GR-ART-001 for executable artifacts.

### GR-FS-002 — New filesystem access outside configured roots

- Version: `v1alpha1`
- Default severity: high for mutation, medium for read/metadata.
- Disposition: requires-review.
- Triggers: head-positive filesystem facts with `absolute-unmapped` normalized path namespace and a reviewed operation.
- Non-triggers: workdir-root, collection-root, relative, and opaque-invalid paths.
- Evidence requirements: typed normalized filesystem paths and operations.
- Limitations: no usernames, home-directory lists, regexes, or prose scanning are used.

### GR-NET-001 — New or changed network behavior

- Version: `v1alpha1`
- Default severity: high.
- Disposition: requires-review.
- Triggers: new or changed DNS/network facts, denied connection attempts, new destinations, result changes, and count increases.
- Non-triggers: removals and count decreases.
- Evidence requirements: typed network/DNS fields.
- Limitations: does not infer exfiltration or intent.

### GR-ART-001 — New or changed artifact

- Version: `v1alpha1`
- Default severity: medium.
- Disposition: requires-review.
- Triggers: artifact additions, modifications, count increases, digest/size/executable/disposition changes when modeled.
- Non-triggers: removal-only artifact behavior and filename-extension inference.
- Evidence requirements: typed artifact facts.
- Limitations: artifact bytes are not inspected.

### GR-DET-001 — Behavioral repeatability degraded

- Version: `v1alpha1`
- Default severity: medium.
- Disposition: requires-review.
- Triggers: stability-changed records where head repeatability is worse or less assessable.
- Non-triggers: head repeatability improvement, equal variability, and order-only changes.
- Evidence requirements: comparator occurrence and repeatability profiles.
- Limitations: no probabilities or statistical claims are produced.

### GR-LIMIT-001 — Resource limit behavior introduced

- Version: `v1alpha1`
- Default severity: high.
- Disposition: requires-review.
- Triggers: new, modified, coverage-limited, or increased typed resource-limit facts in head.
- Non-triggers: removals and target failure without a resource-limit fact.
- Evidence requirements: typed resource facts.
- Limitations: logs are not parsed for limit inference.

## Governance-stage emitted rules

These rules are emitted by GR-10B in the separate rule-set identity
`glassroot.dev/governance-rules/strict/v1alpha1`. They do not alter the GR-10A
built-in rule-set identity `glassroot.dev/builtin-rules/strict/v1alpha1`.

### GR-CONFIG-001 — Trusted security configuration changed in head

- Version: `v1`.
- Default severity: high for privilege increase, observation weakened, execution definition change, policy change, unknown effect, invalid head config, removed head config, and unsupported head entry; medium for privilege decrease or observation strengthened; low for informational changes.
- Disposition: requires-review.
- Confidence: high for the deterministic config comparison state.
- Triggers: valid semantic `ConfigChange` records and file-level invalid/removed/unsupported head configuration states.
- Non-triggers: unchanged head config and byte-changed but semantically equivalent config.
- Evidence requirements: trusted config authority metadata, base/head raw digests, and structured config-change references.
- Limitations: effective configuration remains trusted base; hostile changed values are not interpolated. This governance finding cannot be waived.

### GR-WAIVER-001 — Waiver governance issue

- Version: `v1`.
- Default severity/disposition: high/failed for invalid trusted-base waiver documents, unsupported trusted-base entries, duplicate or overly broad targets, and invalid lifetime; medium/requires-review for expired, not-yet-valid, unused, target-mismatched, or ineligible base waivers; high/requires-review for head additions, removals, target or expiry changes, invalid head content, and unsupported head entries; medium/requires-review for head owner, reason, or issued-at changes.
- Confidence: high for deterministic waiver parsing, lifecycle, and head-assessment facts.
- Triggers: trusted-base waiver authority issues and head waiver proposals.
- Non-triggers: an active exact applied waiver by itself, or formatting-only semantic equivalence in head.
- Evidence requirements: fixed waiver path metadata, base/head raw and semantic waiver digests, structured waiver IDs/change kinds/status records.
- Limitations: owner and reason are untrusted display data; head proposals are never applied. Governance findings cannot be waived.

Lack of findings does not prove safety. Findings do not establish malicious
intent. A waived finding remains present and recoverable; waiver application records effective disposition separately.
