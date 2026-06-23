# ADR: Pipeline format and validation

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-4 introduces the first parser for repository-controlled Glassroot content: `.glassroot/pipeline.yaml`. This content may be supplied by an untrusted pull request, so parsing must be bounded, deterministic, and separate from any later execution, materialization, planning, comparison, policy, or runner work. The `run` field is especially sensitive: it is command text, but in GR-4 it remains opaque data.

GR-3 established versioned, data-only Go model structures. GR-4 defines a separate configuration syntax and normalized validation result without adding YAML tags or parsing behavior to `internal/model`.

## Decision

Glassroot v1alpha1 uses a CI-style YAML authoring format because it is familiar for repository-local pipeline configuration and readable in pull requests. Glassroot deliberately accepts only a strict subset of YAML:

- exactly one non-empty document;
- no `%YAML` or `%TAG` directives;
- no aliases, anchors, merge keys, explicit/custom tags, or complex mapping keys;
- scalar string mapping keys only;
- duplicate keys and unknown fields rejected;
- explicit `null` does not satisfy required values;
- bounded input size, depth, node count, scalar size, run size, and diagnostic count.

The production YAML dependency is `go.yaml.in/yaml/v4` pinned to `v4.0.0-rc.6`. This is the official YAML organization module. The selected version is a prerelease because, as of implementation, every tagged v4 release is an `rc` prerelease. GR-4 records that dependency risk and keeps parsing behind strict structural inspection and tests.

Parser options use the v4 Load API with explicit known-field, unique-key, and depth/alias limiting behavior. A custom YAML limit callback rejects aliases on first alias observation; Glassroot does not use `limit.AliasNone()` because that disables alias checking. A bounded structural `yaml.Node` inspection enforces the accepted YAML subset and captures source positions. Manual typed decoding then produces syntax structs that keep omitted fields distinguishable from explicit zero or null. Successful validation does not retain `yaml.Node` values.

Semantic validation is separate from parsing and planning. The public directional API is:

- `Parse(data []byte) (Document, error)`;
- `Validate(doc Document) (ValidatedPipeline, error)`;
- `ParseAndValidate(data []byte) (ValidatedPipeline, error)`.

Units are intentionally narrow. Sizes accept only positive base-10 integers with `B`, `KiB`, `MiB`, `GiB`, or `TiB` and normalize to integer bytes with overflow checks. Durations accept only positive base-10 integers with `ms`, `s`, `m`, or `h` and normalize to integer milliseconds. Compound durations, fractions, signs, whitespace, day units, and decimal SI byte units are rejected.

Paths are untrusted lexical POSIX strings. GR-4 uses package `path`, not `filepath`, so validation does not depend on the developer host operating system. Validation rejects relative paths, control characters, NUL bytes, backslashes, traversal segments, and paths whose cleaned representation changes meaning. Artifact globs are syntax-checked, may use the documented `**` form, and are never expanded.

Scenario `run` strings are retained literally. GR-4 does not execute, expand, interpolate, source, template, shell-parse, or environment-expand them. The initial shell whitelist is limited to `/bin/sh`, `/bin/bash`, and `/usr/bin/bash`; existence checks are deferred to runner work.

Networking is deny-only in v1alpha1: `network.mode` must be `deny` and `network.allow` must be an explicitly empty array. Brokered allow rules require a later design that specifies hostname, DNS, IP, protocol, redirect, and rebinding semantics.

A hand-maintained Draft 2020-12 JSON Schema is committed at `api/v1alpha1/pipeline.schema.json` with `$id` `https://glassroot.dev/schemas/v1alpha1/pipeline.schema.json`. It uses no remote `$ref` and no custom vocabulary. Tests compile the schema without network retrieval and validate representative instances. JSON Schema supplements but does not replace Go validation because it cannot express all YAML-subset rules, scenario timeout cross-field checks, scenario-ID or artifact-path uniqueness by object property, every path-cleanliness rule, numeric bounds hidden inside unit strings, or semantic overflow checks.

`glassroot validate [--file PATH]` reads only the selected file, bounds the read before allocating complete input, and returns stable exit codes: `0` for valid configuration, `2` for usage/missing/parse/validation failure, and `3` for unexpected I/O or internal failure. It does not use policy exit codes.

Security-sensitive defaults are not silently supplied. All fields in the v1alpha1 shape are explicit and required unless later ADRs change the format.

Incompatible configuration changes require a future schema version. Removing or renaming a field, changing a JSON/YAML type, changing units or meaning, or changing an enum wire value is incompatible. Adding a genuinely optional field may remain within v1alpha1 only when review confirms that omitting it is safe and unambiguous.

## Security considerations

This change reduces parser attack surface but is not a sandbox. It introduces parsing and validation only. It does not load trusted base/head configurations, construct a run plan, execute commands, materialize source, expand globs, pull images, open target paths, create runners, write evidence, evaluate policy, or claim isolation.

Diagnostics are structured and bounded with stable codes, logical paths, source positions where available, and sanitized messages. They do not include Go stack traces, ANSI styling, source lines, or complete untrusted scalar values.

Known residual dependency concerns:

- `go.yaml.in/yaml/v4 v4.0.0-rc.6` is a prerelease, so API and behavior may still change before v4.0.0 final.
- YAML parsers are historically complex; Glassroot mitigates this with input size limits, parser depth callbacks, alias rejection, structural node inspection, unique-key rejection, fuzzing, and adversarial fixtures.
- JSON Schema tests use `github.com/santhosh-tekuri/jsonschema/v6 v6.0.2` as a test-only compiler, with no production imports and no remote schema references.

Human review remains required for this security-sensitive parser. Coding-agent approval alone is insufficient.

## Alternatives considered

- **JSON-only configuration:** simpler to parse strictly, but less suitable for CI-style authoring and comments. Rejected for initial repository-local pipeline ergonomics.
- **Full YAML:** more expressive, but aliases, tags, merges, directives, and complex keys are unnecessary and increase attack surface. Rejected.
- **Generated JSON Schema from Go structs:** would risk treating implementation details as the public contract and would add another dependency. Rejected for GR-4.
- **Defaulting omitted values:** convenient, but defaults for security-sensitive fields can hide intent and make review harder. Rejected.
- **Network allow rules now:** useful eventually, but unsafe without precise broker semantics. Deferred.

## Consequences

The format is intentionally small and strict. Contributors must write explicit pipeline files and update both Go validation and the public schema when the contract changes. Some valid YAML documents are rejected by design. The separation between parsing, validation, planning, and execution keeps future trust-boundary work reviewable.

## Operational and migration impact

Pre-alpha users can validate `.glassroot/pipeline.yaml` locally with `glassroot validate`. Existing repositories without this file receive a missing-configuration exit code from `validate`. No existing runner behavior changes because no runner exists in GR-4.

Future work:

- GR-5 defines trusted base/head configuration loading and comparison rules.
- GR-6 defines safe source/path materialization protections.
- Later runner work resolves images, checks shell availability inside the selected environment, executes `run` only inside a selected sandbox, and applies platform policy ceilings.

## Validation plan

Validation uses unit tests for strict YAML parsing, semantic validation, diagnostic behavior, schema compilation, schema parity, CLI exit codes, and fuzz seed execution. A bounded native fuzz run exercises `FuzzParseAndValidate`. Verification also runs `go mod verify`, `make verify`, `make test-race`, `make schema-check`, `go vet ./...`, the repository-pinned `govulncheck`, and `git diff --check`.

## References

- `KICKSTART.md`
- `docs/CONFIGURATION.md`
- `api/v1alpha1/pipeline.schema.json`
- `internal/config`
