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
be executed, rendered, or trusted without the later GR-8B verifier and GR-11A
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
still require safe rendering by GR-11A.

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

## Behavioral comparison boundary (GR-9B)

Comparison is another trusted transformation. It accepts only GR-9A
`observe.TraceSet` objects and can create false differences or hide real ones if
implemented incorrectly. It never reads or reopens evidence bundles, executes
target content, renders hostile strings, inspects artifact bytes, parses logs,
or decides policy.

Exact semantic matching is performed before any correlation. Correlation is
typed and conservative: process, filesystem, network, artifact, scenario,
warning, and resource facts use documented anchors, and ambiguous anchor groups
remain explicit instead of being paired arbitrarily. No fuzzy matching,
regular-expression matching, machine-learning correlation, statistical
inference, or intent inference occurs.

Incomplete evidence cannot establish absence. A repetition with incomplete,
not-started, or unknown event coverage is not treated as an observed empty
attempt. One complete repetition is a single sample rather than proof of
determinism. Observation sources remain separate; synthetic observations are not
collapsed with host, sandbox, broker, guest, workload, static-analysis, or model
sources.

Behavioral deltas preserve raw event-stream references so policy and rendering
stages can point back to verified evidence. They also preserve typed
evidence-context facts such as whether evidence is synthetic and whether target
code was executed, even when there are zero ordinary behavior-change records.
Delta record IDs and delta digests provide deterministic equality only. They are
not authentication, signing, attestation, provenance, authorization, risk
scoring, or proof that observations are truthful or safe. A compromised
comparator may forge, omit, or misclassify deltas or evidence context. GR-9B
introduces no findings, severity, confidence, disposition, waiver, rendering,
signing, target execution, or sandbox.

## Built-in policy boundary (GR-10A)

Policy evaluation is a trusted transformation over immutable GR-9B
`compare.FrozenDelta` output. A compromised or incorrect policy engine can create
false findings, omit meaningful behavior, or misclassify coverage and
repeatability. GR-10A therefore validates the complete delta and fails closed on
unknown delta kinds, fact kinds, observation sources, comparison bases, malformed
typed facts, duplicate identifiers, and limit violations.

Repository content cannot define, disable, reorder, or modify rules, and head
configuration cannot select policy behavior. Rules use typed normalized behavior
and occurrence profiles rather than prose, log text, regular expressions,
substring matching, fuzzy matching, shell parsing, machine learning, or artifact
content inspection.

Incomplete evidence is not clean evidence. Synthetic evidence remains explicit
and is not treated as target workload behavior. Severity, confidence, and
disposition are deterministic policy categories, not probabilities, statistical
claims, model confidence, waiver state, or malicious-intent judgments.

Findings preserve delta-record IDs and raw evidence references for later safe
rendering, but findings themselves are derived judgments and do not authenticate
the writer, runner, comparator, repository, or observations. Finding IDs and
policy-evaluation digests provide deterministic equality only; they are not
signatures, provenance, authentication, authorization, or attestations.

GR-10A applies no waivers. Formatting in GR-11A cannot alter the frozen finding
set. A compromised comparator or policy engine can still produce misleading
results. GR-10A introduces no rendering, signing, target execution, workspace
access, evidence-bundle mutation, custom policy language, OPA/Rego/plugin
support, or sandbox claim.

## Trusted waiver and final policy application boundary (GR-10B)

GR-10B is a trusted transformation over the GR-10A `FrozenEvaluation`, exact
`pipeline.FrozenPlan`, trusted base configuration result, exact base/head waiver
revision reads, and explicit `evaluatedAt`. A compromised or incorrect waiver
applier can hide policy consequences, misapply waivers, or create false
governance findings. It therefore cross-binds run ID, plan digest, immutable
base/head commits, effective base pipeline configuration, strict policy profile,
waiver revisions, and evaluation time before returning an application.

Base `.glassroot/waivers.yaml` content is trusted repository policy input, but it
may still be malformed, stale, expired, too broad, duplicated, or semantically
invalid. Invalid base waiver sets apply nothing. Head waiver content is hostile
proposal data and is never applied or merged with base. Waiver owner and reason
are untrusted display strings. Broad waivers are unsupported; every waiver targets
one finding ID and one matching eligible rule ID.

Waiver source reads rely on the GR-6A `RevisionFileSource` contract: exact
immutable revision reads, no symlink following, no gitlink traversal, no LFS
fetching, no working-tree fallback, and bounded raw bytes. Operational inability
to inspect requested waiver revisions fails closed rather than becoming a policy
finding.

Evaluation time is trusted caller input, not trusted wall-clock proof. A
compromised clock source or caller can alter active, expired, and not-yet-valid
waiver decisions. Waivers preserve original findings, severity, confidence,
evidence, and unwaived disposition; only effective disposition is overlaid.
Configuration and waiver governance findings cannot be waived.

Application finding IDs and application digests provide deterministic equality
only. They are not signatures, authorization, authentication, provenance, or
attestations. GR-10B introduces no rendering, signing, target execution,
workspace access, artifact/log parsing, custom policy language, OPA/Rego/plugin
support, or sandbox claim.

## Report composition and rendering boundary (GR-11A)

Reporting is a trusted transformation over a verified bundle, immutable
behavioral delta, and final policy application. It consumes hostile strings from
otherwise verified derived models: logical evidence paths, normalized paths,
network destinations, warning text, owner/reason metadata, identifiers, and
limitations may all be attacker-controlled display data.

Markdown, HTML, ANSI, OSC, terminal hyperlink, clipboard, control-character,
bidi, zero-width, unsafe-link, and mention injection are in scope for rendering.
The GR-11A Markdown and terminal renderers therefore send every dynamic value
through visible escaping. Terminal output contains no ANSI, OSC, terminal
hyperlinks, BEL, carriage return, tab, backspace, C1 controls, bidi controls, or
Unicode format controls. Markdown emits no raw HTML, no tables, no images, and no
untrusted link destinations; evidence paths and endpoints are code values rather
than links.

Report JSON remains machine data. Consumers must not embed JSON into HTML,
Markdown, or terminal output without their own escaping. Safe escaping makes
hostile bytes visible; it does not make evidence strings trustworthy.

Waived findings remain visible with original severity, confidence, disposition,
evidence references, and waiver metadata. Formatting cannot change the frozen
policy facts. Runner tier, synthetic evidence, no-target-execution state,
internal-consistency-only verification, and evidence limitations remain
prominent, and a passed disposition is not described as proof that code is safe.

A compromised renderer can still hide or misrepresent findings. Report and
rendered-output digests provide deterministic equality only; they are not
signatures, authentication, authorization, provenance, attestations, or proof
that observations are truthful. GR-11A introduces no CLI behavior, publishing,
HTML renderer, signing, target execution, network access, workspace access,
evidence-bundle mutation, or sandbox claim.


## Inspect orchestration boundary (GR-11B)

`glassroot inspect` combines several trusted transformations and can create
misleading output if it mis-binds inputs or skips stages. The bundle directory,
bare Git store, base commit, head commit, manifest-integrity mode, and
evaluated-at time are separate explicit caller inputs. The bare Git store is
trusted control-plane metadata, but commit, tree, and blob contents remain
hostile repository data. Full explicit commit IDs prevent symbolic-ref
reinterpretation; they do not authenticate the repository or prove ownership.

Inspect opens evidence only through the GR-8B verifier and opens Git only through
the GR-6A object reader. It does not read a working tree, discover a repository
from the current directory, resolve branches or tags, fetch, clone, checkout,
archive, invoke LFS, initialize submodules, access a network, execute target
content, inspect logs, or inspect artifacts. Internal-consistency-only bundle
verification is an explicit mode and remains visible in the report. Expected
manifest digests are equality inputs, not authentication.

Plan reconstruction binds bundle claims to trusted-base configuration and exact
resolved commit/tree identities. Head configuration and head waivers are
inspection-only and never become effective policy inputs for the current
inspection. Base waivers remain trusted repository policy input but are applied
only through the GR-10B exact-finding rules. The caller-provided evaluated-at
time controls waiver expiry; a compromised clock source or caller can alter
expiry decisions.

Report output is produced only after complete supported reconstruction and GR-11A
composition/rendering. Exit code 0 means only that the effective policy
disposition was `passed`; it is not a safety proof. A compromised Git store,
expected-digest custody process, inspector, host, or clock input can still
produce misleading results. GR-11B introduces no signing, publishing,
authentication, attestation, sandbox, provenance claim, or target execution.

## Deterministic fake-demo boundary (GR-12)

`glassroot demo fake` creates trusted Glassroot-controlled fixture state for an
M2 vertical slice. The fixture source trees are inert repository data and are
never executed. The fake Program is trusted control-plane code compiled into
Glassroot; repository content cannot define synthetic events, modify the Program,
or turn the pipeline `run` string into behavior. Synthetic behavior is not
derived from source execution and must not be described as target-workload
observation, malicious intent, sandbox evidence, provenance, authentication, or a
safety claim.

Strict v1 policy treats complete synthetic evidence or no-target-code execution
as requiring review through a typed `GR-OBS-001` evidence-state finding. This
means the control fixture can demonstrate zero ordinary behavioral deltas without
being treated as a passed real-workload assessment. Report notices remain
presentation metadata and are not policy inputs.

The generated bare Git store is control-plane-created fixture metadata. It is not
copied from caller input, does not use a worktree, and is verified through the
same Git object reader used elsewhere. Commit, tree, blob, and path bytes remain
hostile data for parsers and renderers. Materialization still treats paths and
blobs defensively and is used only to compute source descriptors; temporary
source workspaces are removed before final publication and their paths are not
serialized.

The final output parent is trusted caller state. Private random staging names and
an atomic rename protect against partial publication under normal local
filesystem assumptions. Same-UID mutation, hostile mounts, kernel compromise,
filesystem compromise, and durability guarantees beyond successful sync remain
residual risks. A compromised demo fixture, fake Program, publication code, or
host can still produce misleading synthetic evidence or metadata.

Demo reports and `demo.json` digests provide deterministic equality only. They
are not signatures, authorization, authentication, attestations, provenance, or
proof that observations are truthful. GR-12 introduces no Docker, gVisor,
Firecracker, target execution, network access, signing, publishing, or sandbox
claim.

## Docker development runner boundary (GR-13A)

Docker-dev is the first Glassroot backend that actually executes target
commands. It is restricted to trusted local fixture development and reports
isolation tier `development-only`; ordinary Docker is not a hardened sandbox for
hostile repositories or public pull requests.

The Docker daemon and explicit Unix socket are trusted and highly privileged.
Possession of the socket generally grants powerful control over the daemon host.
The backend never discovers Docker configuration from the environment, Docker
contexts, a working tree, or a default path, and it does not support remote TCP,
TLS, SSH, or npipe endpoints.

Image bytes and image configuration are selected only by an explicit local
immutable digest reference. The backend inspects local image metadata and never
pulls, builds, imports, loads, tags, pushes, searches, or logs into a registry.
Image environment remains immutable-image behavior; the host environment is not
inherited. An immutable digest is not a claim that image behavior is benign.

Each container receives exactly one writable private host workspace bind and no
Docker socket, device, secret, named volume, shared cache, or host namespace. It
runs UID 0 inside the container under dropped capabilities, no-new-privileges,
read-only root filesystem, default seccomp, Docker init, and network mode none.
Network none is Docker enforcement, not an observing broker, and the backend does
not claim comprehensive child-process, filesystem, syscall, or network
observation.

Resource enforcement depends on daemon and kernel configuration and is
preflighted. Docker does not provide a portable exact writable-workspace disk
limit in this design. Output bytes and daemon responses are hostile data.
Cancellation, stop/kill/remove, and output draining can fail under host, kernel,
daemon, or filesystem compromise. Same-UID host mutation, malicious mounts,
container escape, daemon compromise, and kernel compromise remain outside the
backend's guarantee. No public webhook or untrusted execution policy may select
docker-dev.

### GR-13B safe post-run artifact collection

After target execution, the writable workspace is hostile filesystem state. The
target may choose names, contents, permissions, symlinks, hard links, sockets,
FIFOs, devices, directories, and mutation traps. Artifact collection starts only
after the future runner has terminated and reaped the target container by
contract; GR-13B is not filesystem observation during execution.

The collector binds the workspace before execution by retaining an `os.Root` and
checks that the root identity remains stable during collection. `os.Root` is a
traversal-resistant API, not a sandbox. Logical sandbox paths from the trusted
plan never become host paths, and filesystem-selected relative names are not
joined to an absolute host path. Artifact patterns come only from the trusted
effective plan; head configuration cannot alter them for an inspected run.

Collection inventories the complete workspace before reading artifact bytes. It
validates names and paths, rejects `.git` components, rejects filesystem device
boundaries, never follows symlinks, never reads symlink targets, never opens hard
links, FIFOs, sockets, devices, or other special files, and records direct
symlink/special matches as explicit omissions. Stable regular files are opened
through the retained root descriptor, hashed while streamed through a synchronous
sink, and checked for identity, mode, size, link count, mtime, and ctime before
and after reading. A final complete inventory reconciliation detects tested
replacement, rename, resize, relink, and mode-change races.

Incomplete collection is explicit. Oversized matched files become omitted-limit
metadata, matched symlinks and special entries become omissions, and blocked
traversal prevents collection from being complete. Infrastructure failures,
mutation, sink failure, or identity instability return no successful result and
require the enclosing evidence transaction to abort.

A compromised daemon, kernel, filesystem, mount namespace, or same-UID host
process may defeat identity reporting or mutate state outside the collector's
assumptions. GR-13B introduces no execution, CLI behavior, policy decision,
rendering, Docker API import, evidence writer integration, hardened sandbox,
signing, authentication, attestation, or provenance claim.

### GR-13C local docker-dev run orchestration

`glassroot run docker-dev` is the first local path that executes target commands.
It is explicit, local-only, and development-only. The caller supplies a bare Git
store, exact full base/head commit IDs, an explicit Unix Docker socket, run ID,
created-at time, evaluated-at time, and the unsafe-development acknowledgement.
No working tree, symbolic revision, Docker environment, default socket, or target
content can select those inputs.

The Docker daemon and socket are trusted and privileged. Each attempt receives a
fresh private materialized workspace as the single writable bind mount. The
collector binds the workspace before execution, Docker removes the container
before collection starts, and all post-run names, links, modes, contents, and
special files are hostile. Logs, Docker responses, artifact logical paths, and
artifact bytes are hostile evidence data.

Trusted-base configuration is the only execution authority. Head pipeline and
waiver changes are assessed but cannot alter the effective image, shell, run
string, resource limits, artifact patterns, comparison ignores, policy profile,
or waivers. The run path writes evidence, verifies it by expected manifest
digest, and reconstructs the final report through `inspect`; reports are not
trusted from in-memory stage output alone.

Incomplete log or artifact capture is explicit and prevents evidence from being
marked complete. Target failure is data. Docker, collector, evidence, cleanup,
inspection, rendering, publication, cancellation, and stdout failures are
infrastructure failures and produce no successful report output before atomic
publication.

Output publication uses a private sibling staging directory and atomic rename,
but durability still depends on host filesystem semantics. Same-UID mutation,
Docker daemon compromise, container escape, kernel compromise, hostile mounts,
malicious filesystems, cgroup enforcement gaps, output I/O failures, and local
image behavior remain residual risks. No public webhook may invoke docker-dev,
and this path introduces no hardened sandbox, provenance, authentication,
attestation, signing, or safety claim.


## gVisor runtime-monitoring spike boundary (GR-14)

GR-14 is a technical spike, not a production runner. It does not add `glassroot
run gvisor`, does not change any capability to `hardened-container`, and does
not authorize public or hostile pull-request execution.

The controlled fixture is trusted Glassroot-controlled code. The Docker daemon,
dedicated runtime configuration, pinned `runsc` binary, private monitor socket
parent, and host are trusted prerequisites for the live spike. The spike treats
gVisor's Sentry as untrusted: runtime-monitor messages may be malformed, false,
incomplete, duplicated, delayed, or dropped. The monitor runs outside the sandbox
and accepts only bounded lifecycle fields from a private Unix `SOCK_SEQPACKET`
remote endpoint.

Runtime and monitor versions are tightly coupled. The spike pins
`release-20260615.0` and records the exact commit and expected binary hash, but a
version match is not authentication or proof that observations are truthful.
Trace-point selection determines visibility. The spike has no independent
host-side event cross-check, no comprehensive filesystem observer, no external
egress broker, no all-syscall session, and no authenticated evidence transport.
Dropped events make observation incomplete. Unknown trace-point types never
become known process facts.

A compromised Sentry, monitor, Docker daemon, `runsc`, host kernel, runtime
configuration, or control plane can produce misleading results or escape the
spike assumptions. A successful controlled fixture run does not prove hardened
isolation, image safety, host safety, provenance, authentication, attestation, or
public-repository eligibility.

## GitHub App advisory boundary (GR-15)

GR-15 defines a future GitHub App advisory-check boundary without deploying a
webhook receiver, minting credentials, calling GitHub APIs, fetching source,
scheduling workers, publishing Check Runs, or authorizing public PR execution.
Webhook origin is verified through a shared secret over the exact raw body, but
payload repository content remains hostile. Delivery IDs are replay keys, not
authentication by themselves. Deliveries may be duplicated, delayed, missing, or
out of order; all handlers must assume at-least-once processing and idempotent
durable transitions.

Webhook payloads are triggers and hints, not immutable source authority. The
controller must revalidate current PR state through authenticated GitHub API
reads using installation identity, repository numeric ID, PR number, and exact
base/head commit IDs. PR titles, bodies, branch names, comments, labels, sender
identity, clone URLs, and archive URLs cannot select policy, runner,
configuration, execution behavior, or source acquisition.

The GitHub platform, installation identity, and current PR metadata returned by
GitHub APIs are trusted platform inputs for future controller decisions. The
receiver has no App private key. A credential broker is highly trusted and is the
only component that may hold the App private key or mint installation tokens.
Source-read tokens and Checks-write tokens are separate, short lived, and scoped
to the narrowest repository and permissions. Workers receive no GitHub token,
App key, webhook secret, OAuth token, publisher credential, clone URL, or API URL
and cannot publish to GitHub.

The publisher cannot access worker hosts, sandboxes, source stores, evidence
bundles, logs, artifact bytes, or source credentials. It receives only a
validated bounded Check projection and a Checks-write token. Stale worker results
and superseded generations cannot update the current advisory Check Run. A
Check Run conclusion is advisory UI state; v1 maps Glassroot policy outcomes to
neutral and does not mean safe, authenticated, attested, approved, or mergeable.

A compromised receiver, controller, credential broker, source ingester, worker,
or publisher can violate its own boundary. A compromised GitHub installation or
repository administrator can influence trusted-base policy and repository state.
Public execution remains prohibited until a hardened runner and worker boundary
are implemented, runtime-validated, and independently reviewed. GR-15 introduces
no signing, provenance, attestation, authentication, sandbox, safety, webhook
freshness, or exactly-once delivery claim.

## GitHub webhook receiver and inbox (GR-15A)

GR-15A implements only the receiver/inbox boundary. The receiver holds the
GitHub webhook secret and no App private key, installation token, OAuth token,
source credential, publisher credential, or worker credential. It listens on a
private Unix socket behind a deployment-owned TLS reverse proxy. The proxy and
local filesystem are trusted to preserve exact raw request bytes and required
headers; proxy source IP and forwarded identity headers are not authentication.

Webhook HMAC validation proves possession of the shared secret for the exact raw
body. Signed payload content remains hostile. Headers other than the signature
are not independent trust roots and are cross-checked against the typed payload
shape before persistence. Payloads may be duplicated, delayed, missing, or out of
order. Delivery IDs are replay keys, not authentication.

The receiver verifies signatures before JSON parsing, persists only minimal typed
projections and receipts, and never persists raw payloads, signature headers,
secrets, PR title/body, branch names, labels, comments, commit messages, user
names, URLs, patch text, or remote IP. Unsupported signed events/actions may be
recorded as ignored; they do not create controller work.

The SQLite database and state directory are trusted control-plane state. Database
corruption, state-directory compromise, malicious filesystems, network
filesystems, or kernel compromise can forge, drop, reorder, or lose deliveries.
HTTP 202 is issued only after the inbox/outbox transaction commits under the
documented SQLite WAL and synchronous FULL durability contract. Processing
remains at-least-once; no exactly-once delivery, webhook freshness, source
safety, authentication, attestation, or execution eligibility claim is added.

The outbox is a future controller handoff only. GR-15A does not call GitHub APIs,
does not fetch source, does not mint tokens, does not schedule workers directly,
does not publish Check Runs, and does not execute target code. Public PR
execution remains prohibited.

## GitHub App credential broker (GR-15B1)

GR-15B1 introduces a dedicated local credential broker as the only component that
loads the GitHub App private key. The receiver, future controller, future source
ingester, workers, and publisher do not receive private-key bytes. The broker has
no webhook secret, OAuth token, personal access token, source store, worker
capability, Check publisher role, or target-execution path.

Compromise of the broker or its host can mint installation tokens within the App
registration grants. The App registration permission profile is therefore the
outer privilege ceiling and is checked at startup. Each minted token is
downscoped to exactly one repository ID and one read-only purpose: pull-request
metadata reconciliation or source read. Tokens remain powerful short-lived
secrets even when repository-scoped; they are opaque, variable-length values and
are not logged, persisted, placed in IDs, or passed to workers.

The private-key file, broker process, local clock, private Unix socket parent,
and host filesystem are trusted control-plane state. The socket uses Linux peer
UID checks and mode `0600`, but it is not a general security boundary; same-UID
host compromise can request tokens. GitHub API responses are external platform
input and are bounded and validated before use.

GR-15B1 does not reconcile pull requests, fetch repository content, ingest source,
publish checks, schedule or run workers, execute target code, authorize public PR
execution, or introduce any sandbox, provenance, authentication, attestation, or
safety claim.

## GitHub controller reconciliation (GR-15B2)

GR-15B2 adds durable controller reconciliation but no source ingestion, worker
assignment, execution, or publication. The controller holds no App private key
and no webhook secret. It transiently obtains one-repository `pull-request-read`
installation tokens from the GR-15B1 broker and closes each token lease after the
single current-PR read. Tokens are not persisted, logged, placed in IDs, source
requests, jobs, attempts, worker-facing contracts, or errors.

Webhook data remains a hint. Event actions and event-reported base/head SHAs do
not form target authority. The controller revalidates current PR state through
the fixed GitHub REST endpoint `GET /repos/{owner}/{repo}/pulls/{pull_number}`.
GitHub API current state, installation identity, numeric repository IDs, PR
number, and exact base/head commit IDs are the reconciliation inputs. Route
owner/name strings are bounded path hints and are not identity authority.

Per-PR reconciliation leases serialize API snapshots for the same installation,
base repository, and PR number. Controller generations are monotonic and never
decrease. Delayed or out-of-order webhook deliveries cannot restore an older
target; duplicate receiver outbox processing is idempotent by outbox ID and
projection digest. Controller state commits before the receiver outbox is
acknowledged, so a crash may cause reprocessing but not duplicate current
targets, jobs, initial attempts, or source-import requests.

Source-import requests are credential-free durable messages for GR-15B3. They
contain numeric repository IDs, bounded route hints, exact commits, target/job
IDs, and generation; they contain no token, API URL, clone/archive URL, branch,
PR prose, worker identity, Check Run credential, host path, or source content.
Stale source results and stale worker results cannot become current. Fake,
docker-dev, and development-only runner results are rejected; no WorkerAssignment
is emitted in GR-15B2.

Installation lifecycle events are conservative invalidation hints. Deletion,
suspension, and repository removal can cancel affected current jobs and advance
generations; creation, unsuspension, and repository addition cannot authorize or
restore work without a later PR API reconciliation.

The token broker and GitHub API are trusted dependencies. GitHub API responses
remain external platform input and are bounded before use. Controller database
compromise, SQLite/filesystem loss, malicious local filesystems, route renames,
private fork inaccessibility, delayed installation events, clock errors, or lease
expiry races can forge, drop, delay, or misclassify controller work. Public PR
execution remains prohibited, and GR-15B2 introduces no sandbox, provenance,
authentication, attestation, safety, or exactly-once claim.

## GitHub source ingestion (GR-15B3)

GR-15B3 adds exact source ingestion but no worker assignment, target execution,
publication, or public PR authorization. The source ingester holds no App private
key, webhook secret, pull-request-read token, Checks-write token, OAuth token, or
worker credential. It transiently obtains one-repository `source-read`
installation tokens from the GR-15B1 broker for the base repository ID only.
Tokens are not persisted, logged, placed in IDs, Git argv, remote URLs, Git
config files, source-store metadata, controller records, errors, or future worker
contracts. Tokens do exist transiently in the trusted Glassroot process, the Git
child process, Git HTTPS-helper memory, and a fixed child environment variable;
same-UID host compromise can expose them on some systems.

The source ingester uses Git smart HTTP to the fixed GitHub.com base repository
remote. GitHub, TLS roots, Git's smart-HTTP implementation, Git's pack parser,
the trusted Git executable, the broker, the controller store, the source-root
filesystem, the local clock, and the host kernel remain trusted dependencies.
Repository objects, pack contents, trees, blobs, gitlinks, LFS pointer files, and
route owner/name hints are hostile input. Route names are non-authoritative
transport hints; numeric repository IDs and exact controller-authorized commits
remain identity authority.

The PR head ref under `refs/pull/<number>/head` is mutable and is used only as a
transport locator. The fetched head ref must equal the expected head commit
exactly; movement or mismatch fails closed as stale source. No head-repository
token, unauthenticated public fallback, redirect, proxy, archive, checkout,
partial clone, submodule traversal, LFS object fetch, or working tree exists in
GR-15B3.

Source stores are shallow and complete only for the selected revisions. They omit
unrelated history, tags, submodule contents, and LFS object payloads. SourceStoreID
is an opaque descriptor, not authentication, provenance, an attestation, or a
digest of physical pack bytes. A successful import does not prove source content
is benign or safe to execute. The source-root database/filesystem can forge, drop,
corrupt, or withhold stores if compromised, and a quota-backed filesystem remains
required for hard disk-exhaustion defense during pack ingestion. Workers receive
no token and no physical source path. Public PR execution remains prohibited.
