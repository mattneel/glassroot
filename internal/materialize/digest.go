package materialize

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"strings"
)

const (
	materializedTreeDomain        = "glassroot.dev/materialized-tree/v1\x00"
	materializationManifestDomain = "glassroot.dev/materialization-manifest/v1\x00"
)

func computeMaterializationDigests(entries []EntryResult) (string, string, error) {
	owned := append([]EntryResult(nil), entries...)
	sort.SliceStable(owned, func(i, j int) bool { return owned[i].Path < owned[j].Path })
	treeHash := sha256.New()
	manifestHash := sha256.New()
	_, _ = treeHash.Write([]byte(materializedTreeDomain))
	_, _ = manifestHash.Write([]byte(materializationManifestDomain))
	for _, entry := range owned {
		if err := validateDigestEntry(entry); err != nil {
			return "", "", err
		}
		if isCreatedDisposition(entry.Disposition) {
			treeEntry := entryForTreeDigest(entry)
			if err := writeDigestRecord(treeHash, treeEntry); err != nil {
				return "", "", err
			}
		}
		if err := writeDigestRecord(manifestHash, entry); err != nil {
			return "", "", err
		}
	}
	return "sha256:" + hex.EncodeToString(treeHash.Sum(nil)), "sha256:" + hex.EncodeToString(manifestHash.Sum(nil)), nil
}

func validateDigestEntry(entry EntryResult) error {
	if entry.Path == "" {
		return errCode(CodeManifestLimit, "digest", "entry", "entry path is required", nil)
	}
	if entry.SizeBytes < 0 || entry.TargetBytes < 0 {
		return errCode(CodeManifestLimit, "digest", "entry", "negative sizes are invalid", nil)
	}
	for _, digest := range []string{entry.ContentDigest, entry.TargetDigest} {
		if digest == "" {
			continue
		}
		if !validSHA256Digest(digest) {
			return errCode(CodeManifestLimit, "digest", "entry", "digest must be sha256 lowercase hex", nil)
		}
	}
	return nil
}

func entryForTreeDigest(entry EntryResult) EntryResult {
	if entry.Disposition == DispositionMaterializedLFSPointer {
		entry.Disposition = DispositionMaterializedFile
		entry.LFSPointer = nil
	}
	return entry
}

func isCreatedDisposition(d Disposition) bool {
	switch d {
	case DispositionMaterializedDirectory, DispositionMaterializedFile, DispositionMaterializedExecutable, DispositionMaterializedSymlink, DispositionMaterializedLFSPointer:
		return true
	default:
		return false
	}
}

func writeDigestRecord(h interface{ Write([]byte) (int, error) }, entry EntryResult) error {
	writeBytes(h, []byte(entry.Path))
	writeBytes(h, []byte(entry.SourceObjectID))
	writeBytes(h, []byte(entry.SourceKind))
	writeBytes(h, []byte(entry.Disposition))
	writeUint64(h, uint64(entry.NormalizedMode))
	writeUint64(h, uint64(entry.SizeBytes))
	writeBytes(h, []byte(entry.ContentDigest))
	writeBytes(h, []byte(entry.TargetDigest))
	writeUint64(h, uint64(entry.TargetBytes))
	if entry.LFSPointer == nil {
		writeBytes(h, nil)
		writeUint64(h, 0)
	} else {
		writeBytes(h, []byte(entry.LFSPointer.OID))
		writeUint64(h, uint64(entry.LFSPointer.Size))
	}
	return nil
}

func writeBytes(h interface{ Write([]byte) (int, error) }, data []byte) {
	writeUint64(h, uint64(len(data)))
	_, _ = h.Write(data)
}

func writeUint64(h interface{ Write([]byte) (int, error) }, value uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], value)
	_, _ = h.Write(b[:])
}

func validSHA256Digest(d string) bool {
	if !strings.HasPrefix(d, "sha256:") || len(d) != len("sha256:")+64 {
		return false
	}
	for _, r := range strings.TrimPrefix(d, "sha256:") {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}

func sha256Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func digestRecordBytes(entry EntryResult) []byte {
	var b bytes.Buffer
	_ = writeDigestRecord(&b, entry)
	return b.Bytes()
}
