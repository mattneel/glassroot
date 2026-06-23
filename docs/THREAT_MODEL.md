# Glassroot threat model

Glassroot is pre-alpha. This document records the repository-configuration trust boundary introduced by GR-5 and must stay aligned with the security invariants in `KICKSTART.md`, especially invariants 2, 3, and 14.

## Configuration attacker model

For a pull request from trusted base revision `A` to proposed head revision `B`, the attacker is assumed to control the entire head revision. In particular, the attacker may:

- modify, remove, enlarge, invalidate, or change the type of `.glassroot/pipeline.yaml`;
- try to increase CPU, memory, disk, process, timeout, artifact, or log limits;
- try to enable or loosen networking;
- add, remove, reorder, or change scenarios and `run` strings;
- reduce observation by removing collection roots, changing artifact/log collection, adding ignored comparison fields, or lowering repetitions;
- weaken policy values or introduce unknown fields;
- use comments, YAML syntax features, duplicate keys, aliases, tags, control characters, or malformed input to confuse diagnostics or reviewers.

The attacker is also assumed to understand Glassroot's public source code and tests.

## Trusted base configuration rule

Only `.glassroot/pipeline.yaml` from the trusted base revision is authoritative for the current request. Head configuration is analysis-only.

Glassroot therefore:

- loads the effective pipeline only from base `A`;
- inspects the same fixed path in head `B` only to assess proposed configuration changes;
- never merges base and head configuration;
- never uses a head field to fill a missing base field;
- never falls back to the working tree, checked-out branch, or a built-in permissive configuration;
- never lets head choose the configuration path;
- treats missing or invalid base configuration as a fail-closed condition with no effective pipeline;
- treats inability to inspect head as incomplete analysis, not success;
- reports invalid, removed, unsupported, or semantically changed head configuration without applying it.

This implements the invariant that the proposed revision cannot choose how it is inspected. It also supports the lower-trust-layer rule: repository configuration may be narrowed later by platform or organization policy, but lower-trust input cannot increase privileges for the current request.

## Source contract and deferred Git enforcement

GR-5 trusts its `RevisionFileSource` abstraction to return raw repository blob content for an already-selected immutable commit. The source contract excludes clean/smudge filters, text conversion, symlink following, submodule traversal, Git LFS fetching, checkout state, and working-tree fallback.

GR-5 does not implement or prove that contract against Git. Exact commit resolution, raw Git object reading, path safety, and materialization protections belong to GR-6.

## Unknown is not safe

Head inspection failures are not treated as unchanged configuration. Unsupported entries, invalid documents, oversized files, and operational read failures remain explicit outcomes. Future planning must not silently continue as if analysis were complete when the head configuration could not be inspected.

## Out of scope for GR-5

GR-5 does not introduce Git integration, source checkout, source materialization, waivers, platform policy merging, runners, evidence I/O, policy findings, report rendering, target-code execution, or a sandbox. A malicious administrator who controls the trusted base branch or future organization policy remains initially out of scope.

## Git object-reader trust boundary

GR-6A reads Git objects from a bare object store created and owned by the Glassroot control plane. The Git directory itself is not supplied directly by the target repository. By contract, its config, hooks directory, refs storage, and object-store layout are control-plane metadata, and the analyzed workload cannot write to it during a repository operation.

Repository content remains hostile. Commit, tree, blob, tag, ref, and path data may be influenced by the target contributor and must be parsed as untrusted data. Exact commit object IDs replace symbolic refs before inspection; after resolution, tree and blob reads use immutable object IDs rather than re-reading refs.

Arbitrary attacker-provided `.git` directories are unsupported. Concurrent object-store mutation, ingestion, cloning, and fetch into the trusted store are outside the GR-6A contract. Alternates, HTTP alternates, grafts, partial clones, promisor remotes, promisor pack markers, unsafe repository extensions, and worktree indirection are rejected.

Git subprocesses in this component run with a fixed command inventory, an absolute Git executable, bounded stdout and stderr, and a sanitized environment. The component grants Git no network permission by command policy: it does not clone, fetch, invoke remote helpers, use credentials, or perform lazy fetches. Hooks, filters, text conversion, Git LFS, archive attributes, and submodule traversal are not invoked. Tracked Git LFS pointer files are raw blob contents; gitlinks are reported as gitlink entries and not traversed.

Raw Git object parsing and Git subprocess behavior remain attack surfaces. GR-6B is responsible for traversal-resistant filesystem writes and host-platform materialization checks. GR-6A does not create a target workspace, run a workload, add a runner, or provide a sandbox.

## Materialization trust boundary

GR-6B writes one already-resolved Git tree into a fresh private workspace. The source revision is an immutable commit/tree identity from the trusted Git object reader; symbolic refs are not resolved again during materialization.

Git tree paths, blob bytes, symlink targets, modes, declared sizes, Gitlinks, and LFS pointer files remain hostile. The destination parent is trusted control-plane state. It must not be writable or replaceable by the analyzed workload, must not contain attacker-controlled mounts or device nodes, and must not be located under the source bare Git store.

Materialization happens before any target workload starts. Every repository-selected path is validated as a relative POSIX path and passed through one `os.Root`; repository content is never appended to an absolute host path. `os.Root` is a traversal-resistant filesystem API, not a sandbox. It does not protect against a malicious kernel, compromised materializer user, attacker-controlled mount namespace, hostile backing filesystem, or same-UID process racing the generated workspace.

The workspace is newly created with mode `0700`. The materializer preflights the complete tree before writing, creates directories first, creates files with `O_EXCL`, normalizes file modes through open file descriptors, and creates validated symlinks last. No repository-derived filesystem operation occurs after symlink creation begins.

Gitlinks are reported and omitted. Git LFS objects are not fetched; pointer files are materialized as their raw tracked blob bytes and may be annotated as pointers. Hooks, filters, text conversion, archive extraction, checkout, submodule initialization, network access, and target-code execution are not part of GR-6B.

Partial output is destroyed on failure. A successful workspace still contains untrusted source data and must not be executed directly on the host. Future sandbox runners are responsible for executing workloads inside an explicit isolation boundary and must not treat configuration trust or materialization as sufficient isolation.

## Deterministic planning boundary (GR-7A)

The planner trusts GR-5's trusted-base configuration result and treats head
configuration as hostile, non-authoritative proposed content. Head assessment
metadata may explain what changed, but it cannot provide commands, resources,
networking, collection rules, comparison rules, policy profile, or defaults for
the effective run plan.

The planner also trusts source descriptors produced by the GR-6B materializer,
while validating that those descriptors are internally consistent and match the
trusted base/head commit identities. Platform constraints are trusted
control-plane input. A compromised planner or compromised platform-policy source
is inside the trusted computing base.

Planning never reads workspaces or repository files. Serialized plans contain
shell and `run` strings as inert data, contain no host workspace paths or
secrets, and include an explicit empty workload environment for v1alpha1. Exact
commit IDs, tree IDs, object formats, and materialization digests bind the plan
to immutable source identities so later symbolic-ref changes cannot reinterpret
what was planned.

Plan digests support reproducibility and integrity comparison for the current
Glassroot plan encoder only. They are not authentication, signatures,
authorization tokens, attestations, sandbox claims, or proof that source is safe.
GR-7B and later runners must consume the finalized plan without reinterpreting or
augmenting commands, limits, networking, or base/head trust semantics. No
workload execution exists in GR-7A.
