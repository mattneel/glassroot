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
