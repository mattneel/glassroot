# `glassroot inspect`

`glassroot inspect` reconstructs the supported Glassroot analysis pipeline for an
existing evidence bundle. It reads verified evidence, binds the bundle to explicit
immutable Git revisions and trusted-base configuration, then runs normalization,
comparison, built-in policy, trusted-base waiver application, report composition,
and one GR-11A renderer.

It does not create evidence, execute target or bundle content, read a working
tree, render logs or artifacts, fetch from a network, or persist a report.

## Command syntax

```text
glassroot inspect [flags] <absolute-evidence-directory>
```

Required flags:

```text
--git-dir ABSOLUTE_PATH
--base-commit FULL_OBJECT_ID
--head-commit FULL_OBJECT_ID
--evaluated-at YYYY-MM-DDTHH:MM:SSZ
```

Exactly one manifest-integrity mode is required:

```text
--expected-manifest-digest sha256:<64-lowercase-hex>
--allow-internal-consistency-only
```

Optional output format:

```text
--format terminal|markdown|json   # default: terminal
```

The evidence directory and `--git-dir` must be absolute, lexically clean UTF-8
paths without NUL or control characters. Relative paths are rejected; Glassroot
does not call the current directory to make them acceptable and does not discover
a repository from the working tree. The path values are host inputs and are not
serialized into the report.

Commit inputs must be full lowercase object IDs, either 40 or 64 hexadecimal
characters. Refs, branches, tags, `HEAD`, abbreviated IDs, revision expressions,
path syntax, and wildcard syntax are rejected. After the bare store reports its
object format, both IDs must have exactly that object width and must resolve
directly to commits. `inspect` uses no ref selector.

`--evaluated-at` accepts only exact UTC RFC3339 seconds, for example
`2026-06-23T00:00:00Z`. Fractional seconds, timezone offsets, whitespace, local
time defaults, clock-skew tolerance, and implicit current time are not accepted.
The caller-provided time controls waiver expiry decisions and participates in the
final policy-application and report digests.

## Manifest-integrity modes

`--expected-manifest-digest` supplies an independently obtained manifest digest
to the GR-8B verifier. A mismatch is an inspection failure. The expected digest
is an equality input; it is not authentication, provenance, signing, or proof of
who wrote the bundle.

`--allow-internal-consistency-only` explicitly permits strict internal bundle
consistency without an independent expected digest. This weaker mode remains
prominent in the resulting report. It is not silently selected, and it does not
by itself create a policy finding or force a different CLI exit code.

Supplying neither mode, both modes, or a malformed digest is a usage error.

## Trusted bare Git store

`--git-dir` must name the control-plane-owned bare Git store that contains the
exact base and head commits. `inspect` opens Git only through the GR-6A
`internal/gitstore` object reader. It does not clone, fetch, pull, check out,
archive, initialize submodules, invoke LFS, access a network, or read a working
tree. The Git store path is a trust anchor supplied by the caller, but the path
itself is not proof of repository ownership.

Commit, tree, and blob contents remain hostile repository data. Full explicit
commit IDs prevent symbolic-ref reinterpretation; they do not authenticate the
repository.

## Deterministic stage order

`inspect` performs this logical sequence:

1. Validate explicit CLI/request inputs.
2. Open and strictly verify the evidence bundle through GR-8B.
3. Read the verified plan document and plan digest from the bundle APIs.
4. Open the explicit bare Git store through GR-6A.
5. Resolve the explicit base and head full object IDs as commits.
6. Require resolved commit, tree, and object-format identities to match the
   verified bundle plan.
7. Create a revision-file source from the bare store.
8. Load trusted pipeline configuration through GR-5 from the trusted base commit;
   head configuration is assessed but never effective.
9. Rebuild the GR-7A `FrozenPlan` from the verified run ID, creation time,
   source descriptors, platform constraints, trusted-base config, and resolved
   immutable source identities.
10. Require the rebuilt plan digest and document to match the verified bundle
    plan.
11. Normalize the verified bundle through GR-9A.
12. Compare normalized traces through GR-9B.
13. Evaluate the strict built-in policy through GR-10A.
14. Apply trusted-base waivers and governance rules through GR-10B using the
    fixed `.glassroot/waivers.yaml` path and the explicit evaluated-at time.
15. Compose a GR-11A `FrozenReport`.
16. Close the evidence bundle before successful return.
17. Render the selected GR-11A output format.

A failure stops later stages. A coherent incomplete evidence bundle is valid
input and is represented through policy/report data; it is not an infrastructure
failure merely because observations are incomplete. Missing or invalid trusted
base pipeline configuration is a usage/trusted-input error. Invalid trusted-base
waiver content applies no waivers and becomes GR-WAIVER-001 governance output in
the report.

No stage consumes head pipeline configuration or head waivers as authority. Head
pipeline and waiver proposals are inspection-only governance data.

## Output formats

Successful output is written only to stdout. Stderr is empty. The command fully
builds and renders the selected output before writing the first stdout byte.

- `--format terminal` uses the GR-11A ANSI-free terminal renderer and ends with
  exactly one LF.
- `--format markdown` uses the GR-11A safe Markdown renderer and ends with
  exactly one LF.
- `--format json` writes the exact compact `FrozenReport.JSON()` bytes with no
  added newline and no envelope.

The command emits no progress messages, paths, Git directory, raw YAML, raw
events, logs, artifact bytes, or run commands. Terminal output contains no ANSI
or OSC controls. Markdown output contains no raw HTML or attacker-controlled
links. Evidence references are structured data, not host file paths or links.

If stdout fails after writing begins, partial output may be visible. Consumers
must discard output whenever the command exits nonzero.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | A complete report was written and the effective policy disposition is `passed`. This does not prove the change is safe. |
| `2` | Usage or trusted configuration input error: malformed flags, invalid path/digest/object ID/time, invalid flag combination, or missing/unsupported/invalid trusted-base pipeline configuration. |
| `3` | Bundle/Git verification, revision binding, plan reconstruction, stage invariant, timeout/cancellation, bundle close, rendering, or output failure. |
| `4` | A complete report was written and the effective policy disposition is `requires-review`. |
| `5` | A complete report was written and the effective policy disposition is `failed`. |

Exit codes `4` and `5` are returned only after complete successful report output.
Target test failures are report data; they do not directly choose the CLI exit
code outside final policy disposition. Internal-consistency-only mode does not
automatically force `4` or `5`.

Failure diagnostics use a bounded deterministic form on stderr:

```text
glassroot inspect: <error-code>: <bounded-message>
```

Diagnostics contain no ANSI, OSC, stack traces, raw JSON/YAML/events/logs,
artifact bytes, command strings, waiver owner/reason text, endpoints, warning
messages, or host paths.

## Example

```bash
glassroot inspect \
  --git-dir /control/repos/example.git \
  --base-commit 1111111111111111111111111111111111111111 \
  --head-commit 2222222222222222222222222222222222222222 \
  --evaluated-at 2026-06-23T00:00:00Z \
  --expected-manifest-digest sha256:3333333333333333333333333333333333333333333333333333333333333333 \
  --format terminal \
  /absolute/path/to/evidence-bundle
```

Use `--allow-internal-consistency-only` only when the absence of an independent
expected manifest digest is intentional and acceptable to the caller.

## Deferred work

GR-12 will add a user-facing fake-runner demonstration command that creates an
evidence bundle. `inspect` only reads and verifies existing state. Future work may
add GitHub publishing or signing, but this command introduces no publishing,
signing, attestation, authentication, sandbox, or provenance claim.
