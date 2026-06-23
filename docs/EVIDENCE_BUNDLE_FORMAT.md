# Evidence bundle format

GR-8A defines the first Glassroot evidence bundle writer. The MVP bundle is a directory, not an archive, so later readers can verify fixed paths, sizes, and digests without archive extraction or compression semantics.

## Trust boundary

The writer consumes a completed `pipeline.FrozenPlan` and preserves its exact JSON bytes in `plan.json`. It does not reconstruct or reinterpret the plan. Actual runner identity, capabilities, execution results, events, logs, and artifacts are persisted outside the frozen plan.

The bundle parent is trusted control-plane state. It must not be repository-controlled, must not be inside a target workspace, must not be writable or replaceable by the analyzed workload, and must not contain attacker-controlled mounts, devices, or filesystem behavior. GR-8A does not establish those conditions itself.

Repository, event, log, and artifact data remain hostile. Logical artifact paths are evidence metadata only and never become host filesystem paths.

The GR-8A `Session` API is sequential-only: callers emit events, logs, artifacts,
and commit data in one ordered control flow. It is not a concurrent evidence
collector. Future orchestration may add a separate synchronized fan-in layer.

## v1alpha1 directory layout

```text
manifest.json
plan.json
execution.json
attempts/
  base/
    <scenario-id>/
      repetition-0001/
        result.json
        events.jsonl
        stdout.log
        stderr.log
        artifacts.json
  head/
    <scenario-id>/
      repetition-0001/
        result.json
        events.jsonl
        stdout.log
        stderr.log
        artifacts.json
objects/
  sha256/
    <first-two-hex>/
      <64-lowercase-hex>
```

`stdout.log`, `stderr.log`, and `artifacts.json` are present only when capture was provided. Their absence is represented in `manifest.json` attempt capture states. `result.json` and `events.jsonl` are written for attempts with completed runner results and accepted events.

## JSON and JSONL rules

Every independent JSON document has a `schemaVersion`:

- `manifest.json`: `glassroot.dev/evidence-manifest/v1alpha1`
- `execution.json`: `glassroot.dev/execution-result/v1alpha1`
- `result.json`: `glassroot.dev/attempt-result/v1alpha1`
- `artifacts.json`: `glassroot.dev/artifact-index/v1alpha1`
- event lines: `glassroot.dev/observation-event/v1alpha1`
- `plan.json`: the unchanged GR-7A `glassroot.dev/run-plan/v1alpha1` bytes

Event streams are compact JSONL: exactly one complete JSON object followed by `\n` per accepted event. GR-8A does not implement hostile bundle decoding; duplicate member rejection and strict verification belong to GR-8B.

## Logs and artifacts

Logs are raw byte prefixes. NUL bytes, invalid UTF-8, CRLF, and terminal escapes are preserved exactly. No textual truncation marker is appended. Truncation is represented in manifest metadata and makes `evidenceComplete` false.

Artifacts are streamed into writer-generated private files, hashed, and stored under `objects/sha256/<first-two>/<digest>`. Duplicate bytes may reference the same object. Over-limit artifacts are omitted rather than stored as misleading truncated binary objects; omission is explicit and makes evidence incomplete. GR-13C also records post-run collector omissions for matched symlinks and special files without creating placeholder objects. Artifact records may include source executable/mode metadata established by the collector; evidence object files remain writer-created data files and do not preserve source mode bits.

## Completion states

The manifest and execution result distinguish:

- `executionComplete`: runner execution reached a complete `ExecutionResult`;
- `evidenceComplete`: no required evidence was truncated, omitted, or failed;
- `bundleTransactionValid`: every persisted byte and manifest record is internally consistent for the writer transaction.

A transaction-valid incomplete bundle may be committed only with a stable bounded failure record. A successful target failure can still produce complete evidence; capture or filesystem failures cannot be hidden as successful evidence.

## Digests

Every payload file except `manifest.json` has a SHA-256 digest and size in the manifest. `manifest.json` is not self-listed. The returned manifest digest is:

```text
sha256(
  "glassroot.dev/evidence-manifest-json/v1\0" ||
  uint64-big-endian(manifest-json-byte-length) ||
  exact manifest JSON bytes
)
```

Payload and manifest digests are raw-byte integrity comparison values. They are not signatures, authentication, attestations, canonical JSON claims, or proof that observations are truthful. GR-8B recomputes and verifies them when opening existing bundles as hostile data. An independently retained expected manifest digest can detect manifest substitution, but neither payload nor manifest digest authenticates the writer or proves observations truthful.

## Atomic publication

On Linux, GR-8A publishes by:

1. creating a fresh private sibling staging directory with mode `0700`;
2. writing payload files with exclusive creation and mode `0600`;
3. closing and syncing payload files;
4. writing and syncing a temporary manifest;
5. renaming the manifest to `manifest.json`;
6. syncing the staging directory;
7. renaming staging to a fresh random final sibling name;
8. syncing the trusted parent directory.

The final path is returned only after publication succeeds. Failure removes staging or the private final path when possible. This is atomic namespace publication plus best-effort durability on supported Linux filesystems; it is not protection from a malicious kernel, filesystem, storage device, or hostile same-UID process.

## Out of scope

GR-8A does not read existing bundles, repair corrupt bundles, create archives, compress, encrypt, sign, attest, compare, evaluate policy, render, inspect, open workspaces, execute target code, or introduce a workload-capable runner. GR-8B adds the strict path-safe reader/verifier documented in `docs/EVIDENCE_BUNDLE_READER.md`. GR-9 adds normalization/comparison. GR-11 adds safe rendering.
