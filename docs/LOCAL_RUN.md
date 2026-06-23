# Local docker-dev run

`glassroot run docker-dev` is the pre-alpha local development path that creates a
new evidence bundle by running an exact trusted-base plan in development-only
Docker containers. It is for explicitly trusted local fixtures only. Ordinary
Docker is not a hardened sandbox and must not be used for hostile repositories,
public pull requests, webhooks, controllers, or worker queues.

## Command syntax

```bash
glassroot run docker-dev [flags] <absolute-new-output-directory>
```

Required flags:

- `--git-dir ABSOLUTE_BARE_GIT_DIRECTORY`
- `--base-commit FULL_OBJECT_ID`
- `--head-commit FULL_OBJECT_ID`
- `--docker-socket ABSOLUTE_UNIX_SOCKET`
- `--run-id RUN_ID`
- `--created-at YYYY-MM-DDTHH:MM:SSZ`
- `--evaluated-at YYYY-MM-DDTHH:MM:SSZ`
- `--acknowledge-unsafe-development-runner "I understand docker-dev is not a security boundary"`

Optional flags:

- `--format terminal|markdown|json` (default `terminal`)

The output directory must be absolute, clean, new, and under a trusted existing
parent. The command never overwrites an existing path and has no keep-partial
option. Paths are not made absolute from the current directory.

Commit inputs must be full lowercase SHA-1 or SHA-256 object IDs matching the
opened bare Git store. Branches, tags, refs, abbreviated IDs, and revision
expressions are rejected. The command never reads a working tree and never
clones, fetches, pulls, checks out, archives, runs submodules, or invokes LFS.

The Docker socket is an explicit absolute Unix socket. There is no default socket
and no environment-based Docker discovery. The Docker CLI is not invoked. Images
must be immutable digest references already present in the local daemon; the run
path never pulls, builds, imports, loads, searches, or authenticates to a
registry.

## Fixed platform profile

GR-13C uses `glassroot.dev/platform-profile/docker-dev/v1alpha1`. The profile is
conservative and local-only: CPU 16, memory 64 GiB, disk admission 64 GiB,
processes 4096, global timeout 2 hours, scenario timeout 1 hour, scenarios 64,
repetitions 10, filesystem roots 16, artifacts 64, artifact size 1 GiB, log bytes
100 MiB per stream, and network mode `deny`. Repository configuration may narrow
but not widen these constraints. Docker does not provide a portable exact
workspace disk limit; that limitation is recorded rather than treated as a
security guarantee.

## Stage order

A successful run:

1. validates explicit CLI/request inputs;
2. creates a private sibling staging root;
3. opens the explicit bare Git store;
4. resolves exact base and head commits;
5. loads trusted pipeline configuration from the base commit only;
6. materializes base and head to derive source descriptors;
7. builds a `FrozenPlan`;
8. expands deterministic attempts;
9. materializes one private workspace per attempt;
10. binds an artifact collector to each workspace before execution;
11. opens and preflights the explicit Docker Engine;
12. verifies the immutable image is present locally;
13. runs attempts sequentially with docker-dev;
14. writes bounded stdout/stderr into the evidence session;
15. collects post-run artifacts after container removal;
16. commits and verifies the evidence bundle;
17. relocates the bundle to `evidence/` and verifies again;
18. reconstructs the result through `glassroot inspect`;
19. renders JSON, Markdown, and terminal reports;
20. writes metadata and reports; and
21. atomically renames staging to the requested output directory.

Head pipeline and waiver changes are inspection-only. Effective execution,
artifact patterns, resource limits, and waivers come from trusted-base inputs.

## Output tree

A successful output directory contains exactly:

```text
run.json
evidence/
report.json
report.md
report.txt
```

Directories are mode `0700`; files outside the evidence bundle are mode `0600`.
`report.json` is exact compact GR-11A report JSON with no trailing newline.
`report.md` and `report.txt` are exact GR-11A renderings with one final newline.
No raw logs or artifact bytes are copied outside `evidence/`. No Docker socket,
Git-store, workspace, staging, output-parent, raw YAML, raw command, container ID,
or host path appears in `run.json` or reports.

`run.json` uses schema `glassroot.dev/local-run/v1alpha1` and profile
`glassroot.dev/local-run-profile/docker-dev/v1alpha1`. It records the explicit
run/time inputs, base/head identities, immutable image, plan and report digests,
manifest digest, policy/report digests, runner capability facts, completeness,
expected exit code, daemon metadata excluding the socket path, relative output
paths, counts, and limitations. It is descriptive metadata, not a trust root.

## Logs and artifacts

Log bytes are captured as raw stdout/stderr evidence. Glassroot does not decode,
line-parse, sanitize, or render raw log content. Truncation is represented in
evidence metadata and makes evidence incomplete; no textual truncation marker is
inserted.

Post-run artifact collection starts only after the docker-dev container has
terminated, been reaped, and removed. Collectors are bound before execution and
read the hostile post-run workspace through the GR-13B collector. Stable regular
files are streamed into evidence. Oversized artifacts, matched symlinks, matched
special files, and blocked descendant collection are explicit omissions or
warnings. Collector infrastructure errors abort the evidence transaction.

Post-run artifact events are host-observed facts about stable collection only;
they do not imply live filesystem observation, process attribution, or intent.

## Exit codes

- `0`: output published and effective disposition is `passed`.
- `2`: usage, invalid trusted input, output-path error, or invalid trusted-base
  pipeline configuration.
- `3`: Git, materialization, Docker, collector, evidence, inspection, rendering,
  publication, cleanup, cancellation, timeout, or stdout failure.
- `4`: output published and report written, disposition `requires-review`.
- `5`: output published and report written, disposition `failed`.

A nonzero target command is report data; it does not directly choose the CLI exit
code outside policy disposition. Exit code 0 is not proof that a change is safe.
A stdout failure after publication may leave a valid output directory; consumers
must discard partial stdout when the process exits nonzero.

## Reproducing the report

Use values from `run.json` to run `glassroot inspect` over the published bundle:

```bash
glassroot inspect \
  --git-dir /absolute/control.git \
  --base-commit <baseCommitId> \
  --head-commit <headCommitId> \
  --evaluated-at <evaluatedAt> \
  --expected-manifest-digest <manifestDigest> \
  --format terminal \
  /absolute/output/evidence
```

The inspect output should match the stored report for the selected format and
does not execute target content.

## Integration testing

Ordinary unit tests do not require Docker. `make test-localrun-integration` is a
gated real-Docker target. It requires explicit opt-in and a preloaded immutable
local image such as `GLASSROOT_DOCKERDEV_IMAGE=repo/name@sha256:...`. The test
must never pull an image or contact a registry.

## Security notes

The exact acknowledgement text is required because docker-dev executes target
commands inside ordinary Docker containers with one writable private host
workspace bind. The Docker daemon and socket are trusted and privileged. The
container runs UID 0 inside Docker with reduced privileges and network mode none,
but this is not hardened isolation. Same-UID host mutation, daemon compromise,
container escape, kernel compromise, hostile mounts, malicious filesystems,
cgroup enforcement gaps, and local image behavior remain residual risks. GR-14
will spike gVisor as the next hardened-backend investigation.
