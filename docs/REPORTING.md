# Immutable report composition and safe rendering

GR-11A composes the verified evidence bundle, immutable behavioral delta, and
final policy application into one deterministic report document. Reporting runs
after GR-10B waiver application and before GR-11B CLI orchestration. It is a
presentation boundary only: it does not reopen evidence, inspect logs or
artifacts, mutate bundles, persist reports into bundles, publish results, or
execute target content.

## Versions

The initial identities are:

```text
glassroot.dev/report/v1alpha1
glassroot.dev/report-profile/v1alpha1
glassroot.dev/markdown-renderer/v1alpha1
glassroot.dev/terminal-renderer/v1alpha1
```

A behavior-changing report field-selection or escaping change requires explicit
review. The compact JSON is deterministic for the supported structures, but it
is not described as canonical JSON.

## Inputs and binding

Production report construction accepts only:

- an open GR-8B verified `*evidence.Bundle`;
- a non-nil GR-9B `*compare.FrozenDelta`;
- a non-nil GR-10B `*policy.FrozenApplication`.

The builder obtains owned copies and cross-checks run ID, plan digest, manifest
digest, delta digest, policy application digest, exact schema/profile versions,
base/head immutable identities, completeness state, runner capabilities,
scenario references, delta-record references, evidence-reference attempt
coordinates, and summary counts. A coherent incomplete execution is valid report
data, not a report infrastructure error, but it is displayed prominently.

The current GR-9B delta document does not carry populated base/head commit
objects; GR-11A therefore binds immutable source identity through the verified
plan and policy application, and treats zero delta commit fields as omitted data
rather than as alternate authority.

## Report contents

The report records:

- schema and report profile versions;
- run ID and evaluation time;
- plan, manifest, behavioral-delta, policy-evaluation, and policy-application
  digests;
- manifest verification mode;
- base/head commit, tree, object-format, and materialization digests from the
  plan;
- runner name, version, isolation tier, and capability facts;
- transaction validity, execution/evidence completeness, synthetic state, and
  attempt coverage summaries;
- the complete GR-10B application summary, every applied finding, original and
  effective dispositions, applied waiver metadata, configuration authority, and
  waiver authority;
- every GR-9B scenario comparison and delta record;
- bundle, runner, comparator, application, and reporting limitations.

Every application finding and every delta record appears exactly once. Waived,
failed, configuration-governance, and waiver-governance findings remain visible.
Formatting cannot change severity, confidence, original disposition, effective
disposition, rule identity, finding identity, evidence references, or waiver
state.

The report intentionally omits raw event JSON, log bytes, artifact bytes, raw
waiver YAML, run commands, repository host paths, workspace paths, bundle-parent
paths, bare-store paths, and rendered Markdown/terminal strings. These omissions
avoid turning hostile content into display or filesystem input; the typed delta,
finding, waiver, governance, limitation, and evidence-reference data remains.

## Notices

Report notices use fixed Glassroot text and deterministic ordering. Notices cover
incomplete execution/evidence, synthetic evidence, no target execution, fake or
development-only runners, missing network-deny enforcement, internal-consistency
only manifest verification, applied waivers, governance findings, observer
limitations, and the fixed statement that a passed disposition is not proof of
safety. Notice text never interpolates hostile data.

Fake runners and development-only isolation tiers are not described as hardened
security boundaries. Internal-consistency-only manifest verification is not
called authentication.

## Evidence references

Evidence references are structured data only. Rendering shows revision, scenario,
repetition, event sequence, event IDs, event-stream digest, and logical event
stream path. Logical paths are never opened, joined to host paths, prepended with
`file://`, or made clickable. The references do not authenticate observations;
they identify raw verified evidence for later safe inspection.

## JSON

`FrozenReport` stores compact `encoding/json` bytes with the standard encoder's
HTML-safe string escaping. Required arrays are non-null. JSON consumers remain
responsible for safe display and must not embed report JSON into HTML or terminal
contexts without their own escaping.

The report digest is:

```text
glassroot.dev/report-json/v1\0
|| uint64-big-endian(json-byte-length)
|| exact-json-bytes
```

The digest is a deterministic equality value only. It is not a signature,
authentication, authorization, provenance, attestation, canonical JSON claim, or
proof that observations are truthful or safe.

## Visible text escaping

Markdown and terminal renderers use the same visible-text encoder for dynamic
values. Ordinary printable valid Unicode is preserved without Unicode
normalization. Invalid UTF-8 bytes are displayed as `\xNN`. Backslash and all
C0/C1 controls, NUL, DEL, ESC, BEL, backspace, carriage return, tab, vertical
and form feed, embedded line feed, Unicode line/paragraph separators, bidi
controls, zero-width format controls, BOM, word joiner, and marks are rendered
as visible ASCII escapes such as `\x1B` or `\u{202E}`. No byte or rune is
silently dropped, and each hostile value remains one logical display line.

Escaping makes display effects visible; it does not make the underlying evidence
trustworthy.

## Markdown renderer

Markdown output is fixed CommonMark-compatible Markdown with no raw HTML, no
tables, no untrusted headings, no images, and no links from evidence data. Every
dynamic value is escaped and placed in a robust code span whose delimiter is
longer than any backtick run in the value. Logical evidence paths, artifact
paths, endpoints, URLs, owner/reason text, and findings are displayed as inert
code values rather than link destinations. The renderer emits exactly one final
LF, no CRLF, no BOM, and no renderer-generated timestamp.

## Terminal renderer

Terminal output is plain UTF-8 text. It emits no ANSI, OSC, terminal hyperlink,
color, cursor movement, clipboard command, BEL, carriage return, tab, backspace,
C1 control, bidi control, or format control. Newlines are generated only by the
renderer. It does not inspect terminal width, TTY status, locale, environment, or
current directory.

The terminal format includes every finding, every delta-record ID, delta kind,
fact kind, scenario identity, comparison basis, occurrence state,
evidence-reference count, limitations, notices, runner tier, synthetic state,
internal-consistency-only state, and waiver/original-disposition details.

Rendered-output digests use the same length-prefixed byte contract with domains:

```text
glassroot.dev/report-markdown/v1\0
glassroot.dev/report-terminal/v1\0
```

## Limits and failure behavior

Report construction and rendering use explicit limits for findings, delta
records, evidence references, limitations, notices, display input, escaped
display output, JSON bytes, Markdown bytes, terminal bytes, rendered lines, and
duration. Callers may lower limits but cannot raise them above the absolute
ceilings. Limit failures return no `FrozenReport` or `RenderedOutput`; findings,
deltas, references, and limitations are not silently truncated.

## Future work

GR-11B will add `glassroot inspect` orchestration using only the strict bundle
reader and these renderers. GR-12 will use the report APIs in the end-to-end
fake-runner demonstration. HTML, SARIF, GitHub annotations, publishing, signing,
attestation, authentication, and report persistence are outside GR-11A.
