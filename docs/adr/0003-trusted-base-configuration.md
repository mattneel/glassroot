# ADR: Trusted base configuration

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-4 introduced strict parsing and validation for one repository-controlled pipeline document. GR-5 decides which revision may supply that document for a pull request from base `A` to head `B`.

The security invariant from `KICKSTART.md` is that the proposed revision cannot choose how it is inspected. The head revision is hostile proposed content: it may try to increase resources, enable networking, change commands, reduce observation, weaken policy, remove the pipeline, or exploit parser edge cases. Future base and head executions must therefore use configuration loaded only from the trusted base revision.

GR-6 will implement Git-backed revision reading and safe materialization. GR-5 must define the trust boundary using an abstract revision-file source and in-memory tests only.

## Decision

The base revision is the sole repository-level configuration authority for the current request. For `(base=A, head=B)`, Glassroot reads exactly `.glassroot/pipeline.yaml` from `A`, strictly parses and validates it, and returns that validated base pipeline as the only effective configuration.

Head configuration is inspected separately from the same fixed path. It is never merged, selected, or exposed as a selectable effective pipeline. Head fields never fill missing base fields, and there is no `use head config` option, approved-rerun mechanism, waiver loading, or organization/platform policy merging in GR-5.

Base behavior fails closed. A missing, unreadable, unsupported-entry, oversized, syntactically invalid, semantically invalid, or context-cancelled base configuration returns no effective pipeline.

Head behavior distinguishes hostile/invalid content from infrastructure uncertainty. Missing head configuration is a proposed removal. Invalid or oversized head configuration is reported as modified-invalid. Symlinks, gitlinks, directories, and special entries are reported as unsupported-entry-kind and are not followed. Operational inability to inspect head is a typed head-inspection failure; it is not reported as unchanged and does not produce a complete result.

The fixed configuration path is the authoritative constant `.glassroot/pipeline.yaml`. GR-5 does not accept caller-selected paths, `.yml` fallback, recursive discovery, includes, or a path selected by head content.

GR-5 introduces an abstract bounded `RevisionFileSource`. Its contract is to return raw blob bytes for an already-selected immutable commit without clean/smudge filters, text conversion, symlink following, submodule traversal, LFS fetching, checkout state, or working-tree fallback. The loader passes a strict byte limit and defensively rechecks returned bytes. Only regular files are accepted as effective base configuration. The Git adapter is deferred to GR-6 because raw object access, immutable commit resolution, and materialization safety require dedicated implementation and review.

The loader calculates `sha256:<lowercase hex>` digests and byte sizes for successfully read raw configuration files. These digests represent byte identity only. They are not canonical YAML, semantic identity, signatures, or attestations.

Head changes are represented as deterministic typed records with a logical field path, change kind, and separate security-effect classification. Change kinds are `added`, `removed`, `modified`, and `reordered`. Security effects are `privilege-increase`, `privilege-decrease`, `execution-definition-change`, `observation-weakened`, `observation-strengthened`, `policy-change`, `informational`, and `unknown`. Raw `run` contents are not included in change records; run changes carry SHA-256 digests and byte lengths.

When `ValidatedPipeline` gains fields, the comparator inventory and tests must be updated so new semantic fields are not silently omitted.

## Security considerations

This decision preserves the trust boundary that head cannot increase resources, networking, scenarios, collection limits, ignored fields, or policy privileges for the current request. It also prevents lower-trust head configuration from supplying defaults when base is incomplete.

The result does not retain raw base or head YAML bytes and does not expose a validated head pipeline. That reduces the chance that later planning code accidentally treats head content as authoritative.

GR-5 still trusts the future `RevisionFileSource` implementation to enforce raw immutable commit reads. Until GR-6 supplies that Git-backed implementation, this package is not a complete Git security boundary. It also does not introduce a runner, sandbox, host execution, evidence bundle, policy engine, waiver system, or report finding.

## Alternatives considered

- **Use head configuration when valid:** rejected because the proposed revision would choose how it is inspected.
- **Merge base and head fields:** rejected because head could fill omitted values or raise privileges through partial configuration.
- **Fallback to the working tree or built-in permissive defaults:** rejected because failure-closed behavior is safer and reviewable.
- **Expose the validated head pipeline in the result:** rejected to avoid accidental downstream use as effective configuration.
- **Implement Git reading in GR-5:** deferred to GR-6 so Git object loading, symlink/gitlink handling, filters, LFS, and materialization rules can be reviewed separately.

## Consequences

Pull requests that change `.glassroot/pipeline.yaml` will be assessed, but their changes do not affect the current request. A repository with missing or invalid trusted-base configuration cannot plan or run until the trusted base is fixed. Formatting-only changes can be distinguished from semantic changes through raw digest comparison plus semantic comparison.

The comparator must be maintained as the validated configuration model evolves. Reviewers should treat any inventory test failure as a prompt to decide the change kind and security effect for new fields.

## Operational and migration impact

No CLI behavior changes in GR-5. No Git revision resolution, checkout, or materialization is added. Future GR-6 code will implement `RevisionFileSource` for Git and must preserve this contract. Later policy work may intersect trusted base configuration with platform and organization policy, but lower-trust input may only narrow effective permissions.

## Validation plan

Validation uses unit tests for base fail-closed behavior, fixed-path reads, source identity propagation, head assessment states, semantic comparison coverage, deterministic ordering, digest stability, mutation/aliasing safety, no raw run leakage, and no raw YAML retention in successful results. A fuzz target checks that arbitrary head bytes cannot mutate the effective base configuration. Verification also runs module verification, schema checks, normal tests, race tests, vet, the repository-pinned `govulncheck`, and `git diff --check`.

## References

- `KICKSTART.md`
- `docs/CONFIGURATION.md`
- `docs/THREAT_MODEL.md`
- `internal/config`
