# ADR: Strict evidence bundle verification

## Status

Accepted.

## Date

2026-06-23

## Owners

@mattneel

## Context

GR-8 was split because writing a fresh bundle and reading an existing bundle have different trust boundaries. GR-8A writes from trusted control-plane state. GR-8B must treat every existing path and byte as hostile, including bundles originally produced by Glassroot.

## Decision

Glassroot adds a strict Linux-only directory-bundle verifier under `internal/evidence`. `OpenAndVerify` opens one existing directory through `os.OpenRoot`, compares filesystem identity before and after opening, inventories the complete physical tree before trusting the manifest, and rejects symlinks, hard links, special files, unsafe paths, undeclared entries, unexpected directories, and modes outside the GR-8A `0700`/`0600` contract.

The reader performs stable descriptor-based payload reads. It compares device, inode, link count, mode, size, mtime, and ctime before and after reading and fails closed on detected mutation. These checks are a race detector for the accepted contract, not a sandbox or protection from malicious kernels, hostile filesystems, bind-mount ambiguity, or privileged concurrent mutation.

Structured JSON is validated by a custom preflight layer before typed decoding. The layer rejects duplicate members, escaped-equivalent duplicates, exact-case violations, unknown fields, invalid UTF-8, trailing values, excessive structure, and then requires exact compact re-encoding. JSONL event streams require LF framing and exact event-envelope invariants.

After physical reconciliation, the reader verifies every manifest entry size and SHA-256 digest, the GR-7A plan digest, execution/result documents, runner capabilities, deterministic attempt ordering, global event sequence and IDs, log capture states, artifact indexes, digest-derived object paths, artifact references, and transaction/execution/evidence completion invariants.

A supplied expected manifest digest is compared against the domain-separated digest over exact manifest bytes. This detects manifest substitution only when the caller retained that digest independently. It is not authentication, signing, attestation, provenance, or proof that observations are true. Without an expected digest, the result explicitly reports internal-consistency-only verification.

The returned `Bundle` owns decoded metadata and an internal root descriptor. Accessors return deep copies. Streaming APIs accept typed attempt/logical-artifact identifiers rather than physical paths and recheck size/digest/stability while reading. No generic physical-path reader, repair path, archive parser, renderer, comparator, or execution behavior is introduced.

## Security considerations

Existing bundles are hostile. Manifest paths and sizes are hostile until independently validated. JSON parser ambiguity is a security boundary, so duplicate members, field-case variants, unknown fields, invalid UTF-8, and non-writer-normalized JSON fail closed.

A compromised runner or writer can produce internally consistent false evidence. Payload and manifest digests only support byte-integrity comparison; they do not authenticate the writer. Verified strings, logs, and artifact bytes still require safe rendering by GR-11.

## Alternatives considered

- **Use `json.Unmarshal` plus `DisallowUnknownFields` only:** rejected because it does not address duplicate-member ambiguity or exact writer normalization.
- **Trust manifest paths before inventory:** rejected because undeclared symlinks, hard links, and special files must be rejected before logical reconciliation.
- **Expose a generic bundle-relative open API:** rejected because future callers should not turn hostile physical paths into authority.
- **Repair or ignore unknown files:** rejected because hostile or corrupted bundles must fail closed.
- **Add archive support now:** rejected; archive parsing and extraction are separate trust boundaries.

## Consequences

The reader currently verifies GR-8A-produced directory bundles exactly, including permissions. Copies of bundles that change modes or add platform metadata files may fail verification. This is intentional for the initial security boundary.

No model changes are required. `go.mod`, `go.sum`, workflows, public pipeline schema, planner golden plan, fake-runner event fixture, and GR-8A manifest fixture remain unchanged.

## Validation plan

Validation covers valid writer-produced bundles, expected manifest digest matching, internal-consistency-only mode, symlink/hard-link/special-file rejection, undeclared/missing entries, strict JSON duplicate/case/unknown/noncanonical rejection, payload size/digest mismatch, event sequence and ID checks, artifact object references, path-safe event/log/artifact streaming, ownership, close behavior, and fuzz targets for strict JSON, event lines, inventory reconciliation, and artifact references.

## References

- KICKSTART.md GR-8B
- docs/EVIDENCE_BUNDLE_FORMAT.md
- docs/EVIDENCE_BUNDLE_READER.md
- docs/THREAT_MODEL.md
- docs/PLANNING.md
- docs/RUNNER_CONTRACT.md
- docs/adr/0008-atomic-evidence-bundle-writer.md
- internal/evidence
