# Deterministic fake-runner demo

`glassroot demo fake` creates a complete synthetic M2 demonstration. It is a
fixture generator, not a workload runner: it does not execute target or fixture
source, invoke a shell, access a network, pull an image, checkout a working tree,
or inspect artifact contents.

## Command

```bash
glassroot demo fake [flags] <absolute-new-output-directory>
```

Flags:

- `--fixture behavior-change|control` selects the built-in fixture. The default
  is `behavior-change`.
- `--format terminal|markdown|json` selects the report representation written to
  stdout. The default is `terminal`.

The output directory must be an absolute path, its parent must already exist,
and the final directory must not exist. The command never overwrites or merges
with existing files. `--help` prints deterministic usage and performs no file,
Git, evidence, or report operation.

## Output tree

A successful demo publishes exactly:

```text
demo.json
fixture.git/
evidence/
report.json
report.md
report.txt
```

`fixture.git` is a control-plane-created bare Git store. `evidence` is a real
GR-8A evidence bundle that is verified through GR-8B. `report.json` is the exact
GR-11A compact report JSON with no trailing newline; `report.md` and
`report.txt` are the exact GR-11A Markdown and ANSI-free terminal renderings.
`demo.json` is compact metadata with no trailing newline. None of these files
contains absolute host paths, raw YAML, raw events, logs, artifact bytes, or run
commands.

Publication uses a private sibling staging directory and an atomic final rename.
Pre-publication failures remove staging, materialization workspaces, and partial
evidence state. A stdout failure after publication can leave a valid output tree
in place; callers should discard partial stdout whenever the exit code is
nonzero.

## Fixture identities

The fixtures are versioned as:

- `glassroot.dev/fake-demo-fixture/behavior-change/v1alpha1`
- `glassroot.dev/fake-demo-fixture/control/v1alpha1`

The metadata schema is `glassroot.dev/fake-demo/v1alpha1`. Fixed UTC values are
used for run planning and policy evaluation:

- plan `createdAt`: `2026-06-23T00:00:00Z`
- policy `evaluatedAt`: `2026-06-24T00:00:00Z`

Fixture bytes, Git author identity, commit timestamps, pipeline bytes, source
text, fake Program events, log bytes, artifact bytes, and platform constraints
are compiled-in Glassroot data. Wall clock, host name, user name, locale,
current directory, and environment variables do not contribute to serialized
output.

## Trusted Git fixture store

The demo writes a narrow SHA-1 bare Git object store using private fixture code.
It does not run `git init`, `git add`, `git commit`, `hash-object`,
`commit-tree`, `update-ref`, `fast-import`, `clone`, `fetch`, or checkout. The
store contains no hooks, alternates, grafts, remotes, submodules, LFS metadata,
or worktree. It is verified by opening it through `internal/gitstore`.

Each fixture has exact base and head trees containing inert text files:

- `.glassroot/pipeline.yaml`
- `README.md`
- `src/fixture.txt`

The base and head pipeline bytes are identical in GR-12, there is no waiver file,
no symlink, no gitlink, and no executable Git entry. The scenario `run` string is
literal inert data and is never parsed or executed.

## Materialization and planning

The demo materializes the exact base and head commits only to compute real source
descriptors: commit IDs, tree IDs, object format, materialized-tree digest,
materialization-manifest digest, source summary, and limitations. Materialized
workspaces are closed and removed before publication and their paths are not
serialized.

Trusted configuration is loaded from the exact base commit. Head configuration
is inspected but not effective. `pipeline.Build` constructs the real FrozenPlan
with an explicit empty environment, `strict` policy, `deny` network mode, and two
repetitions.

## Fake Program boundary

The fake Program is trusted compiled-in Go data chosen only by the fixture enum.
Repository content cannot define or modify it, and the pipeline `run` field is
not inspected to choose behavior. All fake events retain
`synthetic-test-generated` provenance. Synthetic events are useful for verifying
Glassroot plumbing; they are not observations of target workload behavior and do
not prove malicious intent or safety.

### `behavior-change`

Base repetitions emit stable synthetic lifecycle, process, filesystem, and
ordinary artifact behavior with no network attempt. Head repetitions emit the
corresponding base behavior plus stable head-only synthetic behavior:

- a new child process / executable identity;
- a filesystem write resulting in an executable entry;
- a denied TCP connection attempt to `canary.invalid:443`;
- a new executable artifact;
- a changed ordinary output artifact.

All target outcomes succeed, demonstrating that conventional success is separate
from behavioral equivalence. The current strict policy produces review findings
for the synthetic evidence and applicable process, network, executable file or
artifact, and artifact deltas.

### `control`

The control fixture uses distinct base and head commits but emits semantically
equivalent complete synthetic behavior. It demonstrates that source revision
changes alone do not invent ordinary behavioral findings. It still reports fake
runner, synthetic-evidence, no-target-code-executed, network-deny-not-enforced,
and passed-is-not-proof-of-safety notices; a passed control disposition is not a
safety claim.

## Evidence and reports

Evidence is written through the GR-8A writer, verified before and after relocation
through GR-8B, and then inspected using the same `internal/inspect` path as
`glassroot inspect`. The demo does not manually construct a substitute report.
Reports include evidence references to exact logical `events.jsonl` streams and
event IDs; artifact and log bytes are captured in evidence but are not rendered.

`demo.json` records fixture identity, run ID, fixed timestamps, commit and tree
IDs, plan/manifest/delta/policy/report digests, renderer digests, effective
disposition, expected CLI exit code, relative output paths, and selected key
evidence records. Metadata is descriptive and is not a trust root.

To rerun inspection from a published demo:

```bash
glassroot inspect \
  --git-dir /absolute/path/to/demo/fixture.git \
  --base-commit <baseCommitId from demo.json> \
  --head-commit <headCommitId from demo.json> \
  --evaluated-at 2026-06-24T00:00:00Z \
  --expected-manifest-digest <manifestDigest from demo.json> \
  --format terminal \
  /absolute/path/to/demo/evidence
```

The resulting terminal, Markdown, and JSON bytes must match `report.txt`,
`report.md`, and `report.json` respectively.

## Exit codes

- `0`: output was published, report output was written, and effective disposition
  is `passed`.
- `2`: usage or output-path error.
- `3`: fixture generation, materialization, evidence, inspect, report,
  publication, cleanup, rendering, or stdout failure.
- `4`: output was published, report output was written, and effective disposition
  is `requires-review`.
- `5`: output was published, report output was written, and effective disposition
  is `failed`.

Exit code `0`, `4`, or `5` is not a safety proof. It only reflects the final
policy disposition of the synthetic demo report.

## Platform and deferred work

Demo creation is initially Linux-only because current hardened materialization
and evidence bundle behavior are Linux-focused. Other platforms compile but fail
closed with `unsupported-platform` for demo creation.

Workload-capable execution is deferred. GR-13 will cover a Docker development
runner, and GR-14 will cover a gVisor spike. GR-12 introduces no Docker, gVisor,
Firecracker, network setup, signing, authentication, attestation, provenance, or
sandbox claim.
