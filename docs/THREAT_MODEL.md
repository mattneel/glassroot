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

## Runner contract and fake runner boundary (GR-7B)

The runner consumes a finalized `FrozenPlan`. Actual backend capabilities are trusted backend claims and must be matched against trusted caller requirements before any event is emitted. Repository input cannot opt into the fake runner, select a backend, relax capability requirements, or provide a fake Program.

Fake Programs are trusted control-plane or test data. Fake events are synthetic and use the `synthetic-test-generated` provenance label. They are not observations of repository behavior. The event envelope is stamped outside backend control, so a backend cannot forge run ID, revision, scenario ID, repetition, event ID, or sequence number through an event draft.

A compromised backend can still lie about typed payload observations, so future evidence and policy layers must surface runner identity, capabilities, provenance, and limitations. Sink failure creates incomplete evidence and stops execution immediately; cancellation may leave no final lifecycle event. No persistent evidence layer exists until GR-8.

The fake runner has no workspace, network, process, image, package-manager, Git, or secret access. No target execution exists in GR-7B. Future workload-capable runners must bind separate base/head workspaces outside the serialized plan and enforce cleanup, cancellation, observation, and isolation themselves.

## Evidence bundle writer boundary (GR-8A)

The evidence writer consumes a completed frozen plan and runner output. Event
payloads, logs, logical artifact paths, artifact bytes, target outcomes, and
synthetic fake observations remain hostile evidence data. Bundle files must not
be executed, rendered, or trusted without the later GR-8B verifier and GR-11
renderer.

The bundle parent directory is trusted control-plane state. It must not be
repository-controlled, inside a target workspace, writable by the analyzed
workload, replaceable by a hostile same-UID process, or backed by
attacker-controlled mounts, devices, or filesystem behavior. Staging and final
directory names are generated by Glassroot. Repository values, event fields, and
logical artifact paths cannot choose physical bundle paths; artifact object paths
are derived only from raw SHA-256 content digests.

GR-8A writes into a fresh private staging directory and returns the final path
only after manifest-last publication, required file syncs, a staging-to-final
rename, and parent-directory sync succeed. Disk, serialization, write, rename,
or sync failure prevents successful publication and triggers cleanup of private
partial output. Capture limits are explicit: log truncation, artifact omission,
missing attempts, incomplete execution, and sink/cancellation failures are
recorded as incomplete evidence rather than hidden inside a complete bundle.

Actual runner identity and capability facts are persisted outside the frozen
plan, but they remain trusted backend claims. A compromised backend may emit
false observations, and a compromised writer may forge or omit evidence. Payload
and manifest digests support later byte-integrity checks when independently
verified; they do not authenticate the writer, sign evidence, attest truth, or
prove storage was not modified.

Same-UID concurrent mutation, malicious mounts, hostile filesystems,
compromised kernels, compromised storage devices, and compromised control-plane
code remain outside GR-8A's guarantees. GR-8B must open every existing bundle as
hostile data and verify paths, sizes, digests, JSON/JSONL structure, sequence
numbers, and manifest invariants. GR-8A does not introduce a publisher,
comparator, policy engine, renderer, workload-capable runner, workspace access,
or target-code execution.

## Evidence bundle reader boundary (GR-8B)

Existing evidence bundle directories are hostile. Manifest paths, sizes,
schema versions, event coordinates, logs, logical artifact paths, and artifact
bytes are attacker-controlled until the reader verifies them. Structured JSON is
also hostile because parser ambiguity can hide duplicated or case-variant fields.

GR-8B opens one directory through a stable root descriptor, inventories the
complete physical tree before trusting `manifest.json`, and rejects symlinks,
hard links, special files, unsafe paths, unexpected modes, undeclared files, and
unsupported layout entries. Payload files are read through opened descriptors;
identity, link count, mode, size, mtime, and ctime are compared before and after
reads. Mutations detected during verification fail closed.

The strict decoder rejects duplicate JSON members, escaped-equivalent duplicate
names, unknown fields, field-case variants, invalid UTF-8, trailing values, and
non-writer-normalized JSON. JSONL events must have exact LF framing, schema
versions, run/attempt coordinates, global sequences, deterministic event IDs,
and typed payloads. The verifier cross-checks plan digest, execution metadata,
attempt results, log capture states, artifact indexes, digest-derived object
paths, completion flags, and every declared payload size and digest.

Internal digest consistency does not establish provenance, authentication,
signing, attestation, or truthfulness. An independently retained expected
manifest digest can detect manifest substitution, but it still does not
authenticate the writer. A compromised writer or runner may produce internally
consistent false evidence. Verified hostile strings, logs, and artifact bytes
still require safe rendering by GR-11.

The initial reader is Linux-only and depends on ordinary filesystem identity
reporting. Malicious kernels, hostile filesystems, bind mounts that cannot be
distinguished by device/inode facts, sufficiently privileged concurrent
mutation, compromised storage, and compromised control-plane code remain outside
this guarantee. GR-8B does not execute target code, repair bundles, render
reports, compare behavior, decide policy, sign evidence, or provide a sandbox.

## Evidence normalization boundary (GR-9A)

Normalization is a trusted transformation over verified bundles. It can hide
behavioral differences if implemented incorrectly, so GR-9A accepts only a
fully verified `evidence.Bundle`, handles every supported observation kind
explicitly, and fails closed on unknown kinds, unknown sources, malformed typed
payloads, or unsupported compare-ignore fields.

Numeric process IDs, process parent IDs, absolute clocks, and sandbox root
prefixes are treated as nondeterministic evidence coordinates. Process identity
is rebuilt per attempt and observation source; sources are never collapsed into
one PID namespace. Timestamps are converted to per-source relative offsets while
global event sequence remains the ordering authority. Trusted root prefixes are
normalized only in structured path fields. Arbitrary evidence strings, command
text, warnings, logs, DNS names, URLs, and artifact bytes are not rewritten.
Unicode is not normalization-folded.

Raw evidence remains reachable through event-stream digest/path, event ID,
sequence number, and attempt coordinate. Incomplete evidence, truncated logs,
omitted artifacts, observer warnings, unsupported observations, synthetic
evidence, and internal-consistency-only manifest verification remain explicit in
the normalized trace. A normalized trace is derived data, not raw observation.

Semantic digests provide deterministic equality keys for later comparison only.
They are not authentication, signing, attestation, provenance, or proof that
source behavior is safe. A compromised normalizer may forge, omit, or
over-normalize facts. GR-9A introduces no comparison, policy decision,
rendering, signing, target execution, workspace access, or sandbox.
