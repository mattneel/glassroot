# ADR: Inspect orchestration and CLI

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-11A can compose and safely render a report from a verified bundle, immutable
behavioral delta, and final policy application. Users still need a command that
starts from an existing evidence directory and reconstructs the supported stage
sequence. This command is a trust boundary: it receives host paths, a control
plane Git store, exact revisions, an evaluated-at time, and manifest-integrity
inputs. A mistake could verify the wrong bundle, bind the wrong revisions, apply
head configuration or waivers, or emit a partial report.

## Decision

Glassroot adds `internal/inspect` and the `glassroot inspect` subcommand.
Production inspection accepts only explicit inputs: an absolute evidence
directory, an absolute bare Git directory, exact full base and head commit IDs,
an explicit UTC evaluated-at time, and exactly one manifest-integrity mode. It
accepts no precomputed behavioral delta, policy evaluation, application, or
report as a substitute for reconstruction.

Manifest integrity is explicit. A caller may provide an independently expected
`sha256:<hex>` manifest digest, or may explicitly allow internal-consistency-only
verification. Neither mode is authentication, provenance, signing, or an
attestation.

The Git store is assumed to be control-plane-owned metadata. `inspect` opens it
through GR-6A and uses `ObjectIDSelector` only. Refs, branches, tags, short IDs,
and revision expressions are rejected. After opening the store, commit IDs must
match the repository object format and resolve directly to commits. Resolved
commit/tree/object-format identities must match the verified bundle plan.

`inspect` does not add a generic trusted-plan constructor. It rebuilds the GR-7A
`FrozenPlan` through `pipeline.Build` using the verified run ID and creation
time, source descriptors copied through typed conversion, resolved immutable Git
identities, platform constraints from the verified plan, and trusted-base
configuration loaded through GR-5. The rebuilt plan digest and document must match
the verified bundle plan.

The fixed stage order is strict: request validation, GR-8B bundle verification,
Git open and revision binding, GR-5 trusted config load, GR-7A plan rebuild,
GR-9A normalization, GR-9B comparison, GR-10A policy evaluation, GR-10B waiver
application and governance, GR-11A report composition, bundle close, and selected
GR-11A rendering. A failure stops later stages and returns no report. Coherent
incomplete evidence remains report data rather than an inspection infrastructure
failure.

Trusted-base pipeline configuration is effective. Head configuration is assessed
only. Trusted-base waivers from `.glassroot/waivers.yaml` are the only candidate
waivers; head waivers are inspection-only. The evaluated-at time is explicit and
controls waiver expiry.

The CLI writes only the selected successful report output to stdout. Stderr is
empty on success. Terminal and Markdown use GR-11A renderers; JSON is the exact
compact report JSON with no added newline. Output is fully rendered before the
first stdout write. If stdout fails after writing begins, callers must discard
nonzero output.

Exit codes are fixed: `0` passed report, `2` usage or trusted-base pipeline
configuration input error, `3` verification/reconstruction/infrastructure/render
or output failure, `4` requires-review report, and `5` failed report. Exit `0`
means only that the final effective disposition was `passed`; it does not prove
that code is safe.

Generic JSON digest helper names were avoided. Format-specific helpers name the
bytes and domain they digest, such as run-plan JSON, behavioral-delta JSON, and
policy-application JSON. These helpers compute deterministic equality digests;
they do not validate arbitrary JSON.

## Consequences

The first inspect command is deterministic and fail-closed, but it requires the
caller to supply correct trust anchors: bundle path, bare Git store, exact
commits, expected digest mode, and evaluated-at time. A compromised Git store,
clock input, inspector, host, or expected-digest custody process can still
produce misleading results.

No working-tree fallback, fetch, checkout, archive, LFS, submodule operation,
target execution, log/artifact rendering, evidence mutation, report persistence,
signing, authentication, publishing, sandbox, or provenance claim is introduced.

## Alternatives considered

- **Accept precomputed reports or deltas:** rejected because it would bypass the
  supported verification and binding sequence.
- **Trust the plan merely because it is in the bundle:** rejected; inspect must
  reconstruct it from trusted-base config and explicit immutable revisions.
- **Accept refs or revision expressions:** rejected because symbolic selectors
  can move or reinterpret after request construction.
- **Use head pipeline or head waivers:** rejected because head content is
  lower-trust proposal data for the current inspection.
- **Add an output-file flag:** rejected for GR-11B; stdout-only output keeps file
  writing and persistence out of scope.

## Security considerations

`inspect` combines multiple trusted transformations. A compromised implementation
could mis-bind inputs, skip stages, hide incomplete evidence, apply head waivers,
or emit misleading output. The command therefore requires explicit inputs, exact
stage ordering, narrow error classification, complete output before stdout, and
GR-11A renderers only.

## Validation plan

Validation covers request parsing, path/object-ID/time/integrity validation,
Git revision binding, deterministic plan reconstruction, head noninterference,
CLI output equivalence to direct GR-11A renderers, exit-code classification,
resource cleanup, fuzz seeds for parsing/request/reconstruction inputs, bounded
native fuzzing, full tests, race tests, vet, schema checks, govulncheck, and
import/digest-helper audits.

## References

- KICKSTART.md GR-11B
- docs/INSPECT.md
- docs/REPORTING.md
- docs/WAIVERS.md
- docs/POLICY.md
- docs/PLANNING.md
- docs/CONFIGURATION.md
- docs/GIT_OBJECT_READER.md
- docs/EVIDENCE_BUNDLE_READER.md
- docs/THREAT_MODEL.md
- docs/adr/0014-safe-report-composition-and-rendering.md
- internal/inspect
- cmd/glassroot
