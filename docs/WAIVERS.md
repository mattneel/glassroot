# Trusted-base waivers

GR-10B applies narrow waivers after deterministic built-in policy evaluation. A
waiver is repository policy input from the trusted base revision only. It is not
a claim that behavior is safe, not author authorization, and not authentication
or attestation.

## Fixed path and authority

The only waiver file path is:

```text
.glassroot/waivers.yaml
```

Glassroot reads this path through `config.RevisionFileSource` at the exact base
and head commits already bound into the frozen plan. Base content is the only
waiver authority. Head content is inspected and reported, but never applied and
never merged with base. There is no working-tree fallback, alternate path
discovery, pipeline-selected path, environment path, log/artifact source, or
caller-selected waiver path.

The waiver file is optional. Missing base content means no waivers. Invalid or
unsupported base content applies no waivers as a whole and produces governance
findings. Operational read failures fail closed with no policy application.

## Format

The v1alpha1 authoring shape is:

```yaml
apiVersion: glassroot.dev/v1alpha1
kind: WaiverSet
metadata:
  name: default
spec:
  waivers:
    - id: known-network-fixture
      target:
        findingId: finding-<64-lowercase-hex>
        ruleId: GR-NET-001
      owner: mattneel
      reason: Known deterministic fixture behavior pending removal.
      issuedAt: "2026-06-23T00:00:00Z"
      expiresAt: "2026-07-23T00:00:00Z"
```

Every field shown is required. `spec.waivers: []` is valid. Explicit null,
unknown fields, duplicate keys, aliases, anchors, merge keys, custom tags,
complex mapping keys, multiple documents, directives, invalid UTF-8, NUL bytes,
oversized input, excessive nesting, excessive node counts, and excessive scalar
lengths are rejected before semantic validation. Comments, key order, scalar
style, and waiver list order are not semantically meaningful.

Parser ceilings are 256 KiB per file, depth 24, 10,000 YAML nodes, 100
diagnostics, 4 KiB general scalar bytes, and 1,000 waivers.

## Scope and validation

A waiver target names exactly one finding ID and one matching rule ID. There are
no wildcards, globs, regular expressions, multiple finding IDs, rule-only
waivers, scenario/path/package selectors, branch selectors, permanent waivers,
or custom policy expressions.

Eligible rule IDs are:

- `GR-PROC-001`
- `GR-FS-001`
- `GR-FS-002`
- `GR-NET-001`
- `GR-ART-001`
- `GR-DET-001`
- `GR-LIMIT-001`

Ineligible targets include `GR-OBS-001`, `GR-CONFIG-001`, `GR-WAIVER-001`, any
failed finding, nonexistent finding, mismatched rule ID, or malformed finding ID.
Governance findings cannot be waived by the same waiver file.

Waiver IDs are ASCII and match `[a-z][a-z0-9._-]{0,63}`. IDs and targets must be
unique. Owner is required, 1-256 bytes, no controls, and is not interpreted as
authorization. Reason is required, 1-1024 bytes, no controls, and remains hostile
repository-authored display data for GR-11.

## Time and expiry

Policy application receives `evaluatedAt` explicitly from the trusted caller.
Glassroot does not call `time.Now` for waiver expiry. A compromised caller clock
can alter expiry decisions.

`issuedAt` and `expiresAt` are exact UTC RFC3339 strings without fractional
seconds: `YYYY-MM-DDTHH:MM:SSZ`. Offsets are rejected. `issuedAt` must be earlier
than `expiresAt`; maximum lifetime is 90 days.

A waiver is active when `evaluatedAt >= issuedAt` and `evaluatedAt < expiresAt`.
At exactly `expiresAt`, the waiver is expired. No clock-skew tolerance is added.
Expired, not-yet-valid, unused, mismatched, or ineligible waivers do not apply
and remain visible.

## Application result

Waivers annotate findings. They never delete findings and never change original
severity, confidence, title, summary, evidence, rule identity, finding ID, or
unwaived disposition. The final application records a separate effective
disposition. When an active exact eligible waiver applies, the effective
disposition becomes `waived`; the original finding remains recoverable.

The final application also records waiver raw byte digest, semantic waiver-set
digest, base/head waiver states, head change records, waiver status records, and
governance findings. Raw YAML is not embedded.

Application JSON is frozen with compact `encoding/json` and digest domain:

```text
glassroot.dev/policy-application-json/v1\0
|| uint64-big-endian(json-byte-length)
|| exact-json-bytes
```

The digest is deterministic equality data only; it is not signing,
authentication, authorization, provenance, or attestation.

## Rendering responsibility

Waiver owner and reason strings remain untrusted display data. GR-11 must render
waiver metadata, findings, and evidence references safely. A waiver does not
prove behavior is safe, does not authorize an author, and does not hide the
original finding.
