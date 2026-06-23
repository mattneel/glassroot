# ADR: Core model schema versioning

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-3 introduces the first executable-source architecture change after the repository scaffold: a data-only core model package under `internal/model`. KICKSTART.md requires versioned Go structures serialized as JSON, newline-delimited JSON for future event streams, explicit evidence provenance, separate severity/confidence/disposition fields, standard-library-only model code, and compatibility fixtures.

This decision affects serialized run plans, observation events, scenario results, evidence manifests, behavioral deltas, findings, and reports. The package must not execute, inspect, materialize, parse, or validate target repositories. It must not introduce a runner, target-code execution path, policy engine, evidence bundle reader/writer, JSON Schema generation, or sandbox security claim.

The initial JSON handling also needs honest limits. The standard `encoding/json` package is useful for ordinary typed structures, but its documented compatibility behaviors are not sufficient by themselves for security-sensitive bundle ingestion.

## Decision

Glassroot's initial core model uses plain exported Go structures and the standard `encoding/json` package. Production code in `internal/model` imports only the Go standard library and imports no other Glassroot package.

Top-level wire documents and independently serialized events carry a required `schemaVersion` JSON field with no `omitempty` tag. The initial schema-version naming convention is:

```text
glassroot.dev/<kind>/v1alpha1
```

The initial top-level schema versions are:

- `glassroot.dev/run/v1alpha1`
- `glassroot.dev/run-plan/v1alpha1`
- `glassroot.dev/observation-event/v1alpha1`
- `glassroot.dev/scenario-result/v1alpha1`
- `glassroot.dev/behavioral-delta/v1alpha1`
- `glassroot.dev/evidence-manifest/v1alpha1`
- `glassroot.dev/finding/v1alpha1`
- `glassroot.dev/report/v1alpha1`

Nested value objects inherit the enclosing document version and do not repeat `schemaVersion` unless they are intentionally designed to be serialized independently. For example, `EvidenceRef`, `RunnerCapabilities`, `ResourceLimits`, and observation payloads inherit their parent document version. `ScenarioResult` and `Finding` include `schemaVersion` because they are independently serialized wire documents even when a later report embeds them.

The model uses explicit integer wire units such as `timeoutMillis`, `durationMillis`, `memoryBytes`, `diskBytes`, `sizeBytes`, `cpuMillis`, and `processCount`. It uses `time.Time` for timestamps and pointers for optional timestamps. Optional exit codes use `*int` so an unobserved exit code remains distinguishable from observed exit code zero.

The model uses named string enum types for schema versions, revision kinds, isolation tiers, observation sources and kinds, scenario statuses, delta kinds, severity, confidence, and disposition. It avoids arbitrary untyped JSON in the model: no observation payload is represented by `any`, `interface{}`, `map[string]any`, or `json.RawMessage`. Ordered slices are preferred over maps where future hashing, evidence comparison, or report reproducibility could otherwise depend on map iteration.

The model does not implement custom `MarshalJSON` or `UnmarshalJSON` methods. Missing schema versions remain observable to future validation code instead of being silently defaulted during marshal or unmarshal.

Committed compatibility fixtures under `internal/model/testdata/v1alpha1/` define the initial compatibility contract. Tests decode each fixture into its expected Go type, assert exact schema versions and representative security-relevant fields, marshal and decode again, compare semantic Go values, and confirm emitted top-level documents include `schemaVersion`. The tests do not require byte-for-byte JSON equality because field indentation and formatting are not the compatibility contract.

The initial compatibility policy is:

- adding a genuinely optional field may remain within v1alpha1;
- removing or renaming a field is incompatible;
- changing a field's JSON type is incompatible;
- changing the meaning or units of a field is incompatible;
- changing an enum wire value is incompatible;
- incompatible changes require a new schema version;
- compatibility fixtures must not be casually rewritten to match a breaking implementation change.

Because this is `v1alpha1`, evolution is expected. Schema changes still need explicit review so consumers can distinguish compatible additions from breaking wire-format changes.

This issue does not introduce a hardened untrusted-input decoder. Future readers for evidence bundles, event streams, reports, or repository-supplied documents must validate schema versions and apply explicit hostile-input parsing rules before using decoded values.

The model does not claim that `encoding/json` output is canonical JSON. Content-addressing, canonicalization, signing, and attestation formats are deferred.

Experimental `encoding/json/v2` is not adopted here. The pinned Go 1.26.4 toolchain used for this repository does not provide a buildable `encoding/json/v2` package in the local standard-library source tree, and adopting an experimental JSON API would expand the compatibility surface before Glassroot has a strict hostile-input reader design. A future ADR may revisit JSON libraries or parser settings when GR-8 defines bundle decoding semantics.

## Security considerations

This decision creates data structures only. It does not parse repository pipelines, execute target code, materialize revisions, inspect files, run Git, create event sinks, read or write evidence bundles, evaluate policy, render reports, call GitHub, or introduce a runner.

The JSON structures preserve several security-relevant facts for later components:

- base/head revision identity;
- explicit runner capability facts rather than a generic secure/not-secure assertion;
- machine-readable isolation tier values;
- evidence provenance values such as `host-observed`, `network-broker-observed`, `sandbox-runtime-observed`, `guest-agent-reported`, `workload-reported`, `static-analysis-derived`, and `model-inferred`;
- separate severity, confidence, and disposition fields;
- explicit limitations and evidence references.

Known `encoding/json` concerns that future hostile-input readers must handle include duplicate member names, case-insensitive field matching, unknown fields, invalid UTF-8 handling, and trailing input. In particular, future decoders should decide when to reject duplicate object members, use strict field matching or explicit unknown-field rejection, reject or preserve invalid UTF-8 according to the evidence format, and verify that decoders consume exactly one complete JSON value without trailing input.

The current tests prove fixture round-trip compatibility with ordinary typed structures. They do not prove that arbitrary hostile JSON documents are safe to ingest. Do not use this ADR to justify accepting untrusted evidence bundles without the stricter GR-8 reader semantics.

## Alternatives considered

- **Delay model types until JSON Schema generation.** Rejected because GR-3 needs a small typed compatibility surface before GR-4 adds schema generation and strict pipeline parsing.
- **Use maps or arbitrary JSON payloads for observation events.** Rejected because untyped payloads hide trust-boundary decisions and make future validation, policy, and fixture review harder.
- **Add custom JSON marshal/unmarshal methods now.** Rejected because defaulting or validation in custom methods could hide missing schema versions and blur the boundary between data structures and future hostile-input readers.
- **Adopt experimental `encoding/json/v2`.** Rejected because it is not a stable, buildable dependency in the pinned local toolchain and would require a separate parser-compatibility decision.
- **Generate public JSON schemas under `api/v1alpha1` now.** Rejected because JSON Schema generation belongs to GR-4.

## Consequences

The repository gains a versioned, reviewable model package and compatibility fixtures that future milestones can build on. Schema changes become explicit and testable, while GR-3 remains data-only.

The main tradeoff is that `encoding/json` defaults are permissive. That is acceptable for this issue only because GR-3 is not introducing a hostile-input decoder. GR-8 must define stricter evidence bundle decoding rules before Glassroot ingests security-sensitive bundles.

The fixtures become compatibility artifacts. Updating them should be deliberate and reviewed, not an automatic way to make tests match breaking changes.

## Operational and migration impact

There is no deployment or runtime migration. No CLI behavior changes, no dependencies are added, and no evidence bundle format is read or written by production code.

Future incompatible changes require adding a new schema-version constant and new fixtures rather than rewriting the `v1alpha1` contract in place. Compatible optional additions may stay within `v1alpha1` when they do not change existing field meanings or units.

## Validation plan

Validation for GR-3 includes:

- `go test ./internal/model -count=1` for fixture compatibility, enum wire values, optional field semantics, large integer round-trips, and ADR policy coverage;
- repository-wide `make verify`, race tests, `go test ./... -count=1`, and `go vet ./...`;
- govulncheck using the repository-pinned command;
- audits confirming production imports are standard-library-only and that `internal/model` imports no other Glassroot package;
- audits confirming no dependency changes, CLI behavior changes, generated schemas, runners, target-code execution paths, filesystem/network/process integrations, or external serialization dependencies were introduced.

## References

- KICKSTART.md, especially sections 2.5, 4, 6, 9, 10, 13, and 16.
- `docs/adr/0000-template.md`.
- Go `encoding/json` package documentation and security considerations in the pinned Go 1.26.4 toolchain.

### GR-7A additive run-plan fields

GR-7A keeps the `glassroot.dev/run-plan/v1alpha1` schema-version value and adds
optional run-plan fields for deterministic planning: Git object format, exact
tree ID, trusted configuration provenance, execution environment image/workdir,
collection, comparison, policy, platform ceilings, explicit scenario shell/run
and repetitions, source summaries, and source limitations. These are additive
fields only. Existing v1alpha1 compatibility fixtures are preserved, while a
separate planner golden fixture records the current GR-7A output.

The plan digest is computed outside the run-plan document and is not stored in
the hashed model. It is tied to the GR-7A compact `encoding/json` output and does
not make a canonical JSON or attestation claim.
