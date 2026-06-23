package policy

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"sort"

	"github.com/mattneel/glassroot/internal/model"
)

const (
	evaluationJSONDomain = "glassroot.dev/policy-evaluation-json/v1\x00"
	findingIDDomain      = "glassroot.dev/finding-id/v1\x00"
)

type digestWriter interface{ Write([]byte) (int, error) }

func writeU64(w digestWriter, v uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	_, _ = w.Write(b[:])
}
func writeI64(w digestWriter, v int64)           { writeU64(w, uint64(v)) }
func writeString(w digestWriter, s string)       { writeU64(w, uint64(len(s))); _, _ = w.Write([]byte(s)) }
func writeDigest(w digestWriter, d model.Digest) { writeString(w, string(d)) }
func writeStrings(w digestWriter, in []string) {
	vals := append([]string(nil), in...)
	writeU64(w, uint64(len(vals)))
	for _, v := range vals {
		writeString(w, v)
	}
}

func digestBytes(domain string, data []byte) model.Digest {
	h := sha256.New()
	_, _ = h.Write([]byte(domain))
	writeU64(h, uint64(len(data)))
	_, _ = h.Write(data)
	return model.Digest("sha256:" + hex.EncodeToString(h.Sum(nil)))
}

func findingID(profileVersion, ruleSetVersion, ruleID, ruleVersion string, deltaRecordIDs []string, scope string, scenarioIDs []string) (string, error) {
	h := sha256.New()
	_, _ = h.Write([]byte(findingIDDomain))
	writeString(h, profileVersion)
	writeString(h, ruleSetVersion)
	writeString(h, ruleID)
	writeString(h, ruleVersion)
	writeStrings(h, sortedStrings(deltaRecordIDs))
	writeString(h, scope)
	writeStrings(h, sortedStrings(scenarioIDs))
	return "finding-" + hex.EncodeToString(h.Sum(nil)), nil
}

func writeEvidenceRefKey(h hash.Hash, r model.EvidenceRef) {
	writeString(h, string(r.Revision))
	writeString(h, r.ScenarioID)
	writeU64(h, uint64(r.Repetition))
	writeU64(h, r.EventSequence)
	writeStrings(h, r.EventIDs)
	writeDigest(h, r.EventStreamDigest)
	writeString(h, r.EventStreamPath)
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// DigestApplicationJSON returns the GR-10B policy-application JSON digest for
// already frozen application bytes. It is a pure downstream binding helper; it
// does not parse, trust, or repair arbitrary application documents.
func DigestApplicationJSON(data []byte) model.Digest { return digestBytes(applicationJSONDomain, data) }
