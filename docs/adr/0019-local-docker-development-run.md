# ADR: Local Docker development run

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-13A added the explicitly acknowledged development-only Docker runner core.
GR-13B added the post-run artifact collector for hostile workspace state. The
remaining M3 vertical slice needs user-facing local orchestration that combines
immutable Git revisions, trusted-base configuration, per-attempt materialized
workspaces, Docker execution, bounded logs, safe artifacts, evidence writing,
strict verification, inspect reconstruction, report rendering, and atomic output
publication.

Docker-dev is useful for trusted local fixtures but is not a hardened sandbox and
must not be available to public or hostile pull-request execution.

## Decision

Add `glassroot run docker-dev` as an explicit local-only command. The CLI
requires a bare Git store, exact base/head commit IDs, an explicit Unix Docker
socket, run ID, plan creation time, waiver evaluation time, a new absolute output
directory, and the fixed acknowledgement phrase `I understand docker-dev is not a
security boundary`. There are no defaults from the current directory,
environment, Docker contexts, refs, branches, tags, image overrides, working
Trees, or runner fallbacks.

The local run uses fixed platform profile
`glassroot.dev/platform-profile/docker-dev/v1alpha1`. Repository configuration
may not exceed that profile. Trusted pipeline configuration is loaded only from
the base commit; head configuration and head waivers are inspection-only. The
plan is built from exact source snapshots derived by materializing exact Git
commits. No generic trusted-plan constructor is added.

Every attempt receives its own private materialized workspace. The artifact
collector is bound to the workspace before Docker starts, and docker-dev receives
the same workspace identity. Containers are removed before post-run artifact
collection begins. Workspaces are deleted only after container cleanup and
artifact collection.

Runner hooks bridge attempt lifecycle boundaries. `BeforeAttempt` opens bounded
stdout/stderr evidence captures before container start. `AfterAttempt` runs after
container removal and before the terminal scenario-complete event, allowing
post-run artifact drafts to be stamped by the runner-owned event envelope.
Backend or hook infrastructure failures stop later attempts and abort evidence.

Logs are raw evidence bytes and are not decoded or sanitized. Truncation is
metadata, not a synthetic text marker. Artifact results are bridged into
evidence through the GR-13B synchronous sink contract. Stored artifacts become
host-observed post-run artifact events. Omissions and blocked collection become
explicit evidence metadata and observer-warning drafts. Incomplete log or
artifact collection makes evidence incomplete; infrastructure errors return no
report.

After execution, the evidence bundle is committed, strictly verified by expected
manifest digest, relocated to `evidence/`, and verified again. The final report is
reconstructed through `internal/inspect` using the external Git store, exact
commits, evaluation time, and manifest digest. Localrun never constructs a report
from unverified or in-memory-only evidence.

Successful publication writes exactly `run.json`, `evidence/`, `report.json`,
`report.md`, and `report.txt` in a private staging directory, syncs files and
directories, removes private temporary parents, verifies the final tree, and
atomically renames staging to the requested output path. A pre-publication
failure removes staging. A stdout failure after publication leaves the valid
output directory and returns exit 3.

Exit codes follow the existing command model: 0 passed, 2 usage/trusted input, 3
infrastructure/render/output, 4 requires-review, and 5 failed. Exit 0 is not a
safety proof.

## Consequences

GR-13C closes the local development vertical slice without adding host execution,
implicit Docker selection, image pulls, working-tree support, public webhook
integration, signing, authentication, provenance, or hardened-sandbox claims.
The implementation depends on the boundaries established in GR-13A and GR-13B:
only `internal/dockerengine` talks to Docker, and artifact collection remains
separate from execution.

M3 is implementation-complete when ordinary tests pass. M3 runtime validation is
complete only after the gated real-Docker localrun integration suite passes
against a recorded preloaded immutable local image.

## Alternatives considered

- **Add `glassroot run` with implicit Docker defaults:** rejected because socket,
  image, and unsafe-development acknowledgement must be explicit.
- **Use a working tree:** rejected because source authority must be exact Git
  objects and trusted-base configuration.
- **Trust the just-built in-memory report:** rejected; inspect reconstruction is
  required to prove the published evidence can be independently verified.
- **Use Docker logs after execution:** rejected in favor of live bounded capture
  and evidence metadata.
- **Collect artifacts before container removal:** rejected to keep collection as
  post-run hostile-state processing rather than live observation.
- **Publish partial output for debugging:** rejected because failure-closed
  cleanup and atomic publication are part of the security boundary.
