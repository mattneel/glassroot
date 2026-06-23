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
