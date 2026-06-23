# Fake runner

The fake runner is a deterministic test double for GR-7B through early evidence and reporting milestones. It exists to exercise runner orchestration and event handling without executing target code.

## Explicit synthetic opt-in

The fake runner is accepted only when trusted caller requirements use the `synthetic-test` intent, explicitly allow synthetic evidence, and explicitly allow the `fake` isolation tier. It is rejected for `workload` intent and there is no fallback from a missing or incompatible workload backend to fake.

Fixed fake capabilities are:

- runner name `fake`;
- runner version `v1`;
- isolation tier `fake`;
- executes target code: false;
- synthetic evidence: true;
- fresh kernel: false;
- brokered network: false;
- network-deny enforcement: false;
- real process, filesystem, syscall, and artifact collection: false;
- snapshot support: false.

The fake runner may emit synthetic process, filesystem, network, artifact, warning, and lifecycle events. Those events do not make the real observation flags true.

## Trusted Program

Fake behavior comes from a trusted in-memory typed `Program` supplied by tests or trusted control-plane code. No YAML, JSON, or repository-supplied Program parser exists. The pipeline, run string, repository files, and workspace cannot select or modify a Program.

A Program is bound to the exact frozen-plan digest. It must provide exactly one script for every planned attempt and no extras. Script input order does not affect execution order.

## Synthetic lifecycle

For each attempt, the fake runner emits:

1. synthetic scenario-start lifecycle event;
2. zero or more trusted Program events;
3. synthetic scenario-complete lifecycle event.

Timestamps are derived from the plan creation time, attempt ordinal, and Program offsets. The fake runner does not sleep, call the wall clock, parse shell syntax, interpolate variables, inspect commands, open files, access a network, start a process, invoke Git, pull images, or read a workspace.

Outcomes simulate target status as data: succeeded, failed, timed-out, or resource-limited. Infrastructure failures such as invalid Program data, sink failure, and cancellation remain runner errors.

## Golden event stream

`internal/runner/fake/testdata/v1alpha1/events.jsonl` records a deterministic synthetic event stream. It exercises base and head attempts, scenario lifecycle, process, filesystem, denied network, artifact activity, synthetic provenance, repetition identity, global sequence, and deterministic event IDs.

The fixture is not evidence that a repository performed those actions. It is synthetic test data for orchestration, future GR-8 persistence, and later comparator/reporting work.

## Limitations

Fake output must not be used for public or security-sensitive workload execution. It has no workspace, process, network, image, or secret access. It cannot satisfy a workload-execution request and makes no sandbox or isolation claim. The first backend capable of executing trusted local fixtures is deferred to GR-13.
