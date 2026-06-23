# Deterministic run planning

GR-7A adds the planning stage between trusted configuration loading and future
runner execution. Planning consumes only explicit inputs from earlier trusted
stages and produces inert run-plan data for later components.

## Inputs and authority

Trusted inputs:

- the GR-5 `TrustedLoadResult`, where the effective pipeline is loaded from the
  trusted base revision only;
- explicit base and head source snapshots produced from already materialized
  immutable revisions;
- explicit platform constraints supplied by the control plane;
- caller-supplied run ID and UTC creation time.

Hostile or non-authoritative inputs:

- head revision pipeline content and head assessment details;
- repository paths, blob contents, symlink targets, and LFS pointer text that
  produced only bounded source metadata;
- shell and `run` strings, which remain literal data.

The planner does not read workspaces, inspect files, invoke Git, call a runner,
resolve images, access the network, inherit environment variables, or execute
commands.

## Base-only execution template

The execution template is derived exactly once from the trusted-base validated
pipeline. The same template is copied into base and head revision plans. Head
pipeline content may be reported by GR-5, but it cannot change:

- image or workdir;
- resources or timeouts;
- networking;
- scenario IDs, names, shells, literal `run` strings, or order;
- collection roots, artifact patterns, log limits;
- comparison ignore fields or repetitions;
- policy profile.

A removed, malformed, or modified head pipeline does not replace the base
configuration. If GR-5 cannot complete head inspection, it does not return a
successful trusted result for planning.

## Revision and materialization binding

Each revision plan binds exact immutable source identity:

- revision kind (`base` or `head`);
- full commit ID;
- full tree ID;
- Git object format (`sha1` or `sha256`);
- materialized-tree digest;
- materialization-manifest digest;
- bounded materialization summary;
- bounded source limitations, including skipped gitlinks or detected LFS
  pointer facts.

Symbolic refs, workspace paths, destination parents, file descriptors, `os.Root`
handles, raw file content, raw symlink targets, and command output are not
serialized into the plan.

## Platform admission

Platform constraints are trusted control-plane input. The zero value is invalid.
The planner rejects a pipeline that exceeds a platform ceiling; it does not
silently clamp, widen, or rewrite repository requests. Exact boundary values are
accepted. v1alpha1 requires deny networking.

GR-7A does not merge organization policy or waivers. Later policy-precedence work
may introduce an explicitly reported intersection model.

## Environment and commands

v1alpha1 plans include an explicit empty ordered workload environment array. The
planner does not add `PATH`, `HOME`, `CI`, locale values, credentials, tokens, or
host environment variables.

Scenario shell and `run` fields are preserved literally. The planner does not
trim, expand, interpolate, shell-parse, syntax-check, or execute them. The
legacy `command.argv` field is emitted as an empty array by GR-7A so the future
runner must consume the explicit shell/run fields instead of a host command
constructed during planning.

## Ordering and repetitions

Deterministic future attempt order is:

1. revision order: base, then head;
2. scenario order: trusted-base configuration order;
3. repetition order: 1 through the configured repetition count.

Repetitions are represented explicitly on each scenario and in comparison data.
The planner does not duplicate command documents for every future attempt.

## Frozen plan ownership

`FrozenPlan` owns its normalized model and compact JSON bytes. `Document()` and
`JSON()` return deep copies. Mutating caller-owned inputs or returned values does
not change the frozen plan. This is an ownership guarantee for Go values, not a
general immutability claim.

## JSON and digest

The frozen JSON is compact `encoding/json` output from a normalized model that
uses ordered slices rather than maps. The digest is:

```text
sha256(
  "glassroot.dev/run-plan-json/v1\0" ||
  uint64-big-endian(json-byte-length) ||
  exact-json-bytes
)
```

The output form is `sha256:<64 lowercase hex>`. The digest is tied to this
Glassroot v1 plan encoder. It is useful for integrity comparison and
reproducibility checks only; it is not a signature, authorization token,
attestation, canonical JSON claim, or proof that source is safe.

## Future work

GR-7B will add capability matching and a fake runner that consumes the finalized
plan without reinterpreting commands, limits, or networking. Application-layer
orchestration will associate nonserialized workspace handles with revision plans;
those handles must not enter the wire plan.
