# Evidence bundle reader and verifier

GR-8B opens an existing Glassroot evidence directory as hostile input. It verifies the physical tree, strict JSON/JSONL structure, payload sizes and digests, plan and attempt binding, event ordering, artifact references, and completion-state invariants before returning a `Bundle`.

## Trust boundary

The bundle directory and every byte beneath it are hostile. The reader never executes, renders, repairs, shell-expands, imports, or interprets bundle content beyond typed validation. A verified bundle remains untrusted evidence data.

Initial hardened verification is Linux-only. On other platforms `OpenAndVerify` fails closed with `unsupported-platform`. The Linux reader uses filesystem identity facts from `os.FileInfo.Sys()` to compare device, inode, link count, mode, size, mtime, and ctime before and after reads. These checks detect tested replacement and mutation races; they are not protection from a malicious kernel, hostile filesystem, bind-mount ambiguity, privileged concurrent mutation, or compromised control plane.

## Root opening and inventory

The caller-supplied bundle path must be absolute, clean, valid UTF-8, bounded, and not a symlink at the final component. The reader:

1. `Lstat`s the final path without following it.
2. Opens it with `os.OpenRoot`.
3. Stats `.` through the opened root.
4. Requires the pre-open and opened identities to match.
5. Re-`Lstat`s the original final path and requires it to still name the same directory.

After opening, bundle operations use the opened root. The reader inventories the complete physical tree before trusting `manifest.json`. It rejects symlinks, hard links, FIFOs, sockets, devices, special files, invalid paths, undeclared files, unexpected empty directories, alternate layout casing, executable payloads, and modes other than GR-8A's `0700` directories and `0600` files.

## Strict JSON and JSONL

All structured JSON is preflighted before typed decoding. The strict layer rejects invalid UTF-8, BOMs, raw NUL, duplicate object members at any depth, escaped-equivalent duplicate names, unknown fields, field-case variants, excessive depth/tokens/members/arrays/strings/numbers, trailing JSON values, and non-writer-normalized encodings.

After typed decoding with `DisallowUnknownFields`, the reader marshals the typed value with the same standard encoder contract and requires byte-for-byte equality with the original bytes. This intentionally rejects pretty printing, leading/trailing whitespace, alternate member order, alternate escapes, omitted required empty arrays, and other equivalent but non-writer-normalized JSON.

Event files are strict JSONL: every line is exactly one compact JSON object followed by one LF. CRLF, blank lines, unterminated final lines, oversized lines, duplicate fields, wrong schema versions, wrong attempt coordinates, sequence gaps, duplicate sequences, and event-ID mismatches fail verification.

## Manifest, payloads, and cross-file checks

`manifest.json` must use `glassroot.dev/evidence-manifest/v1alpha1`, must not list itself, and must describe every payload file exactly once. The actual file and directory sets must equal the manifest-derived fixed layout exactly. Every payload is opened through a stable descriptor, size-checked, SHA-256 hashed, and restatted after reading.

The reader recomputes the GR-7A plan digest over exact `plan.json` bytes and requires it to match manifest and execution records. It validates execution metadata, runner capabilities, attempt result coordinates, target outcome consistency, event count/range summaries, log capture states, artifact indexes, digest-derived artifact object paths, and completion flags.

Internal digest consistency is not authentication or provenance. If the caller supplies `WithExpectedManifestDigest`, the computed domain-separated manifest digest must match the independently retained value. Without it, `VerificationSummary` explicitly reports internal-consistency-only verification.

## Path-safe APIs

`Bundle` exposes owned metadata copies and typed streaming APIs:

- `WalkEvents(ctx, visit)` yields owned `model.ObservationEvent` values in verified global order.
- `CopyLog(ctx, attempt, stdout|stderr, dst)` copies raw stored log bytes for a known attempt and stream.
- `CopyArtifact(ctx, attempt, logicalPath, dst)` resolves logical artifact metadata through verified indexes and copies only stored complete objects.

These APIs do not expose `os.Root`, file descriptors, staging/final host paths, generic physical path reads, or logical artifact paths as filesystem paths. Streamed output is provisional until the method returns nil; callers must discard output on error because a later stability or digest check can fail.

## Out of scope

GR-8B does not repair bundles, open archives, parse `.grb`, decompress, encrypt, sign, authenticate, attest, compare behavior, evaluate policy, render reports, access target workspaces, run processes, or provide sandboxing. GR-9 consumes verified event streams for normalization/comparison. GR-11 safely renders hostile evidence.
