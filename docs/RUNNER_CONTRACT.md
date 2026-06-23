# Runner contract

GR-7B defines the boundary between immutable planning and future execution backends. A runner consumes a completed `pipeline.FrozenPlan`; it does not reconstruct configuration, resolve revisions, select a backend, inspect a workspace, or amend commands and limits.

## Attempt expansion

`ExecutePlan` expands a frozen plan deterministically:

1. base revision;
2. head revision;
3. scenarios in trusted-base pipeline order;
4. repetitions from `1` through the planned repetition count.

Each `AttemptRequest` contains only owned, bounded execution facts: plan digest, run ID, plan creation time, deterministic attempt ID, revision kind, exact commit and tree IDs, Git object format, materialization digests, immutable image reference, workdir, explicit workload environment, resources, network policy, scenario ID/name, shell, literal `run` data, scenario timeout, repetition number, and collection settings.

It intentionally excludes workspace paths, destination parents, bare Git paths, mutable refs, head-configuration assessments, raw YAML, file contents, symlink targets, credentials, host environment, and callbacks that can modify the plan.

## Requirements and capabilities

The caller supplies trusted `Requirements`. The zero value is invalid. Requirements state the execution intent (`synthetic-test` or `workload`), allowed isolation tiers, whether target execution is required, whether synthetic evidence is allowed, and exact capability facts required for networking, event collection, artifact hashing, snapshots, and fresh kernels.

Runner capabilities are facts, not marketing labels. They are queried once before event emission and matched failure-closed before the first attempt. There is no backend registry, auto-discovery, or fallback. An unavailable workload backend never falls back to the fake runner.

Actual backend identity and capabilities remain outside `FrozenPlan`; they appear in `ExecutionResult` and later evidence manifests. The legacy `RunPlan.runner` field remains non-authoritative in v1alpha1, and `ExecutePlan` rejects a nonzero value rather than selecting a backend from it.

## Events and sinks

Backends emit `EventDraft` values. Drafts contain timestamp, source, kind, and exactly one typed payload. They cannot set schema version, event ID, run ID, revision, scenario ID, repetition, sequence number, or plan authority.

`ExecutePlan` stamps the authoritative `ObservationEvent` envelope. It assigns:

- `glassroot.dev/observation-event/v1alpha1`;
- deterministic event IDs over `glassroot.dev/observation-event-id/v1\0`, plan digest, run ID, and global sequence;
- run ID, revision kind, scenario ID, repetition, and monotonically increasing global sequence.

`EventSink.Emit` is synchronous. Returning nil means the event was accepted. Returning an error stops execution immediately; there is no retry, no unbounded buffering, and no final lifecycle event after a failed sink. GR-8 will add the persistent bounded sink.


## Attempt stdout/stderr output

GR-13A adds a narrow optional output sink for process-capable backends. The sink
is already bound to one attempt; a backend cannot choose another attempt identity
through it. Writes are synchronous and provide backpressure. Bytes are raw
evidence data: runners do not decode UTF-8, parse lines, sanitize controls, or
render output. Existing callers that do not collect logs use an explicit no-log
adapter rather than an implicit unbounded memory buffer. Evidence persistence is
added by later orchestration layers.

## Outcomes, cancellation, and limits

Target outcomes (`succeeded`, `failed`, `timed-out`, `resource-limited`) are data and are distinct from runner infrastructure errors. A nonzero simulated target failure does not stop later attempts; capability failures, backend errors, sink failures, cancellation, and timeouts do.

`ExecutePlan` honors caller context cancellation, the plan global timeout, and each scenario timeout with derived contexts. Future process-capable backends must terminate and reap workloads when their context ends.

GR-7B limits attempts, events per attempt, total events, event JSON size, capability mismatches, and result limitations. JSON-size checks bound output but are not canonical serialization claims.

## Not isolation

The runner interface is not a sandbox. GR-7B does not bind workspaces, open source trees, pull images, configure networking, collect artifact bytes, persist evidence, or execute target code. Future workload-capable backends must bind trusted workspace handles outside the serialized plan and enforce their own isolation, cleanup, and observation contracts.
