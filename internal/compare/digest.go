package compare

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"sort"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/observe"
)

const (
	anchorDomain    = "glassroot.dev/comparison-anchor/v1\x00"
	recordIDDomain  = "glassroot.dev/behavioral-delta-record-id/v1\x00"
	deltaJSONDomain = "glassroot.dev/behavioral-delta-json/v1\x00"
	sequenceDomain  = "glassroot.dev/normalized-fact-sequence/v1\x00"
)

type digestWriter interface{ Write([]byte) (int, error) }

func writeU64(w digestWriter, v uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	_, _ = w.Write(b[:])
}
func writeI64(w digestWriter, v int64) { writeU64(w, uint64(v)) }
func writeBool(w digestWriter, v bool) {
	if v {
		_, _ = w.Write([]byte{1})
	} else {
		_, _ = w.Write([]byte{0})
	}
}
func writeString(w digestWriter, s string)       { writeU64(w, uint64(len(s))); _, _ = w.Write([]byte(s)) }
func writeDigest(w digestWriter, d model.Digest) { writeString(w, string(d)) }

func digestBytes(domain string, data []byte) model.Digest {
	h := sha256.New()
	_, _ = h.Write([]byte(domain))
	writeU64(h, uint64(len(data)))
	_, _ = h.Write(data)
	return model.Digest("sha256:" + hex.EncodeToString(h.Sum(nil)))
}

// DigestBehavioralDeltaJSON returns the GR-9B behavioral-delta JSON digest for
// already frozen delta bytes. It is format-specific and does not parse, trust,
// or repair arbitrary JSON documents.
func DigestBehavioralDeltaJSON(data []byte) model.Digest {
	return digestBytes(deltaJSONDomain, data)
}

func typedAnchorDigest(f observe.Fact) (model.Digest, error) {
	h := sha256.New()
	_, _ = h.Write([]byte(anchorDomain))
	writeString(h, string(f.Kind))
	writeString(h, string(f.Source))
	switch f.Kind {
	case observe.FactKindProcessStart, observe.FactKindProcessExit:
		if f.Process == nil {
			return "", errCode(CodeInvalidFactPayload, "anchor", "", "process", "missing process payload", nil)
		}
		writeString(h, string(f.Process.StableID))
		writeString(h, f.Process.Operation)
	case observe.FactKindFilesystemCreate, observe.FactKindFilesystemRead, observe.FactKindFilesystemWrite, observe.FactKindFilesystemDelete, observe.FactKindFilesystemRename, observe.FactKindFilesystemChmod:
		if f.Filesystem == nil {
			return "", errCode(CodeInvalidFactPayload, "anchor", "", "filesystem", "missing filesystem payload", nil)
		}
		writeString(h, f.Filesystem.Operation)
		writePath(h, f.Filesystem.Path)
	case observe.FactKindDNSQuery, observe.FactKindNetworkConnection:
		if f.Network == nil {
			return "", errCode(CodeInvalidFactPayload, "anchor", "", "network", "missing network payload", nil)
		}
		writeString(h, f.Network.Operation)
		writeString(h, f.Network.Protocol)
		writeString(h, f.Network.QueryName)
		writeString(h, f.Network.DestinationHost)
		writeI64(h, int64(f.Network.DestinationPort))
	case observe.FactKindArtifactActivity:
		if f.Artifact == nil {
			return "", errCode(CodeInvalidFactPayload, "anchor", "", "artifact", "missing artifact payload", nil)
		}
		writePath(h, f.Artifact.Path)
	case observe.FactKindScenarioStarted, observe.FactKindScenarioCompleted:
		if f.Scenario == nil {
			return "", errCode(CodeInvalidFactPayload, "anchor", "", "scenario", "missing scenario payload", nil)
		}
		writeString(h, string(f.Scenario.Status))
	case observe.FactKindObserverWarning, observe.FactKindUnsupportedObservation:
		if f.Warning == nil {
			return "", errCode(CodeInvalidFactPayload, "anchor", "", "warning", "missing warning payload", nil)
		}
		if f.Warning.Code == "" {
			return "", nil
		}
		writeString(h, f.Warning.Code)
	case observe.FactKindResourceLimit:
		if f.Resource == nil {
			return "", errCode(CodeInvalidFactPayload, "anchor", "", "resource", "missing resource payload", nil)
		}
		writeString(h, f.Resource.LimitKind)
	default:
		return "", errCode(CodeUnsupportedFactKind, "anchor", "", string(f.Kind), "unsupported fact kind", nil)
	}
	return model.Digest("sha256:" + hex.EncodeToString(h.Sum(nil))), nil
}

func writePath(w hash.Hash, p observe.NormalizedPath) {
	writeString(w, string(p.Namespace))
	writeU64(w, uint64(p.RootIndex))
	writeString(w, p.Relative)
	switch p.Namespace {
	case observe.PathNamespaceWorkdirRoot, observe.PathNamespaceCollectionRoot:
		writeString(w, "")
	default:
		writeString(w, p.Literal)
	}
	writeString(w, p.Display)
}

func sequenceDigest(seq []model.Digest) model.Digest {
	h := sha256.New()
	_, _ = h.Write([]byte(sequenceDomain))
	writeU64(h, uint64(len(seq)))
	for _, d := range seq {
		writeDigest(h, d)
	}
	return model.Digest("sha256:" + hex.EncodeToString(h.Sum(nil)))
}

func deltaRecordID(profile, normProfile, scenario string, rec model.DeltaRecord) (string, error) {
	h := sha256.New()
	_, _ = h.Write([]byte(recordIDDomain))
	writeString(h, profile)
	writeString(h, normProfile)
	writeString(h, scenario)
	writeString(h, string(rec.Kind))
	writeString(h, rec.FactKind)
	writeString(h, string(rec.Source))
	writeDigest(h, rec.AnchorDigest)
	writeStrings(h, rec.ChangedFields)
	writeDigests(h, rec.BaseSemanticDigests)
	writeDigests(h, rec.HeadSemanticDigests)
	writeOccurrence(h, rec.BaseOccurrence)
	writeOccurrence(h, rec.HeadOccurrence)
	writeString(h, string(rec.Basis))
	return "delta-" + hex.EncodeToString(h.Sum(nil)), nil
}

func writeStrings(w digestWriter, in []string) {
	vals := append([]string(nil), in...)
	writeU64(w, uint64(len(vals)))
	for _, v := range vals {
		writeString(w, v)
	}
}
func writeDigests(w digestWriter, in []model.Digest) {
	vals := append([]model.Digest(nil), in...)
	writeU64(w, uint64(len(vals)))
	for _, v := range vals {
		writeDigest(w, v)
	}
}
func writeOccurrence(w digestWriter, p model.OccurrenceProfile) {
	writeI64(w, p.PlannedRepetitionCount)
	writeI64(w, p.CompleteRepetitionCount)
	writeI64(w, p.IncompleteRepetitionCount)
	writeI64(w, p.MinimumKnownCount)
	writeI64(w, p.MaximumKnownCount)
	writeI64(w, p.TotalKnownCount)
	writeString(w, string(p.Coverage))
	writeString(w, string(p.Repeatability))
	writeU64(w, uint64(len(p.Repetitions)))
	for _, r := range p.Repetitions {
		writeU64(w, uint64(r.Repetition))
		writeString(w, string(r.Coverage))
		writeBool(w, r.CountKnown)
		writeI64(w, r.Count)
	}
}

func sortDigests(in []model.Digest) []model.Digest {
	out := append([]model.Digest(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
