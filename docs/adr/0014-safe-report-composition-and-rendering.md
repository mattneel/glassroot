# ADR: Safe report composition and rendering

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-8B verifies hostile evidence bundles, GR-9B compares normalized behavior, and
GR-10B applies deterministic policy plus trusted-base waivers. Users still need a
single report and human-readable renderers, but rendering is itself a security
boundary: paths, endpoints, warning strings, owner/reason text, identifiers, and
normalized behavior may be attacker-controlled. GR-11 is split so the immutable
report model and safe renderers can be reviewed before `glassroot inspect` wires
them to CLI inputs and exit codes.

## Decision

Glassroot adds `internal/report`. Production report construction accepts only an
open verified `*evidence.Bundle`, a GR-9B `*compare.FrozenDelta`, and a GR-10B
`*policy.FrozenApplication`. It does not accept arbitrary decoded documents as
substitutes, does not walk event streams, does not copy logs or artifacts, does
not reopen bundle paths, and does not execute or render raw evidence content.

The report schema is:

```text
glassroot.dev/report/v1alpha1
```

The report profile is:

```text
glassroot.dev/report-profile/v1alpha1
```

The builder cross-binds run ID, plan digest, manifest digest, behavioral-delta
digest, policy-application digest, schema/profile versions, runner capability
facts, completeness state, base/head immutable identities, delta-record IDs,
scenario IDs, evidence references, and summary counts. It returns no partial
report after a binding or model-invariant failure. The current comparator output
omits populated base/head commit objects, so source identity is bound through the
verified plan and policy application until a future comparator model carries that
same data.

The report is self-contained structured data. It preserves every application
finding, original and effective dispositions, every applied waiver, configuration
and waiver-governance metadata, every behavioral delta record, occurrence and
coverage profiles, limitations, and evidence references. It omits raw event JSON,
logs, artifacts, raw waiver YAML, run commands, workspace paths, bundle host
paths, and rendered Markdown/terminal text.

Report notices are typed fixed records for incomplete evidence, incomplete
execution, synthetic evidence, no target execution, fake/development-only runner,
network deny not enforced, internal-consistency-only manifest verification,
applied waivers, governance findings, observer limitations, and the statement
that a passed disposition is not proof of safety. Notice text does not
interpolate hostile data.

Frozen report JSON uses compact `encoding/json` and the digest contract:

```text
glassroot.dev/report-json/v1\0
|| uint64-big-endian(json-byte-length)
|| exact-json-bytes
```

This is not canonical JSON, signing, authentication, authorization, provenance,
or attestation.

Two fixed renderers are introduced:

```text
glassroot.dev/markdown-renderer/v1alpha1
glassroot.dev/terminal-renderer/v1alpha1
```

Both use a shared visible-text encoder. It preserves ordinary printable valid
Unicode without normalization, converts invalid UTF-8 to `\xNN`, escapes
backslash, C0/C1 controls, DEL, ESC, BEL, backspace, carriage return, tab,
embedded line feed, Unicode line/paragraph separators, bidi controls, zero-width
format controls, BOM, word joiner, and related format controls. It exposes the
presence of hostile bytes or scalars rather than stripping them.

Markdown output is fixed CommonMark-compatible Markdown. It emits no raw HTML,
no tables, no untrusted headings, no untrusted links, no images, and no user
provided templates. Dynamic values are escaped and placed inside code spans with
a delimiter longer than any backtick run in the escaped value.

Terminal output is plain UTF-8 text. It emits no ANSI, OSC, hyperlink, color,
cursor movement, clipboard command, BEL, carriage return, tab, backspace, C1
control, bidi control, or format control. Rendering is independent of TTY,
terminal width, locale, current directory, environment variables, wall clock, and
randomness.

Output limits are fail-closed. If JSON, Markdown, terminal bytes, rendered lines,
evidence references, or escaped display values exceed limits, no complete output
is returned and nothing is silently truncated.

## Consequences

Reports can be rendered without changing frozen policy facts. Waived findings
remain visible with original severity, confidence, and disposition. Failed and
governance findings remain visible. Escaping reduces Markdown and terminal
injection risk but does not make evidence strings trustworthy.

JSON consumers remain responsible for their own display escaping. Future report
fields require explicit renderer review so they are not omitted or unsafe by
default. Markdown parser extensions and terminal emulator behavior remain
residual risks, so the renderer avoids raw HTML, links, and terminal controls
instead of relying on downstream configuration.

No CLI behavior, HTML, SARIF, GitHub annotations, report persistence, signing,
attestation, authentication, publishing, artifact/log rendering, target
workspace access, network access, process execution, Docker, gVisor, Firecracker,
or sandbox/provenance claim is introduced. GR-11B will add `glassroot inspect`;
GR-12 will use the report APIs in the fake-runner demonstration. GitHub
publishing remains future work.
