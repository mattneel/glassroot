package observe

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"sort"

	"github.com/mattneel/glassroot/internal/model"
)

const (
	semanticDigestDomain = "glassroot.dev/normalized-observation/v1\x00"
	factIDDomain         = "glassroot.dev/normalized-fact-id/v1\x00"
	processIDDomain      = "glassroot.dev/normalized-process-id/v1\x00"
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

func semanticDigest(profile NormalizationProfile, fact Fact) (model.Digest, error) {
	h := sha256.New()
	_, _ = h.Write([]byte(semanticDigestDomain))
	writeString(h, profile.Version)
	writeString(h, string(fact.Kind))
	writeString(h, string(fact.Source))
	if fact.Timing.IncludedInSemanticDigest {
		writeI64(h, fact.Timing.SourceRelativeNanos)
		writeBool(h, fact.Timing.ClockRegression)
	}
	switch fact.Kind {
	case FactKindProcessStart, FactKindProcessExit:
		if fact.Process == nil {
			return "", errCode(CodeInvalidObservationPayload, "digest", "", "process", "missing process payload", nil)
		}
		writeProcessFact(h, *fact.Process)
	case FactKindFilesystemCreate, FactKindFilesystemRead, FactKindFilesystemWrite, FactKindFilesystemDelete, FactKindFilesystemRename, FactKindFilesystemChmod:
		if fact.Filesystem == nil {
			return "", errCode(CodeInvalidObservationPayload, "digest", "", "filesystem", "missing filesystem payload", nil)
		}
		writeFilesystemFact(h, *fact.Filesystem)
	case FactKindDNSQuery, FactKindNetworkConnection:
		if fact.Network == nil {
			return "", errCode(CodeInvalidObservationPayload, "digest", "", "network", "missing network payload", nil)
		}
		writeNetworkFact(h, *fact.Network)
	case FactKindArtifactActivity:
		if fact.Artifact == nil {
			return "", errCode(CodeInvalidObservationPayload, "digest", "", "artifact", "missing artifact payload", nil)
		}
		writeArtifactFact(h, *fact.Artifact)
	case FactKindScenarioStarted, FactKindScenarioCompleted:
		if fact.Scenario == nil {
			return "", errCode(CodeInvalidObservationPayload, "digest", "", "scenario", "missing scenario payload", nil)
		}
		writeScenarioFact(h, *fact.Scenario)
	case FactKindObserverWarning, FactKindUnsupportedObservation:
		if fact.Warning == nil {
			return "", errCode(CodeInvalidObservationPayload, "digest", "", "warning", "missing warning payload", nil)
		}
		writeWarningFact(h, *fact.Warning)
	case FactKindResourceLimit:
		if fact.Resource == nil {
			return "", errCode(CodeInvalidObservationPayload, "digest", "", "resource", "missing resource payload", nil)
		}
		writeResourceFact(h, *fact.Resource)
	default:
		return "", errCode(CodeUnsupportedObservationKind, "digest", "", string(fact.Kind), "unsupported fact kind", nil)
	}
	return model.Digest("sha256:" + hex.EncodeToString(h.Sum(nil))), nil
}

func factID(planDigest model.Digest, attemptID string, semantic model.Digest, eventIDs []string) FactID {
	h := sha256.New()
	_, _ = h.Write([]byte(factIDDomain))
	writeDigest(h, planDigest)
	writeString(h, attemptID)
	writeDigest(h, semantic)
	ids := append([]string(nil), eventIDs...)
	writeU64(h, uint64(len(ids)))
	for _, id := range ids {
		writeString(h, id)
	}
	return FactID("fact-" + hex.EncodeToString(h.Sum(nil)))
}
func processID(parts ...string) ProcessID {
	h := sha256.New()
	_, _ = h.Write([]byte(processIDDomain))
	for _, p := range parts {
		writeString(h, p)
	}
	return ProcessID("proc-" + hex.EncodeToString(h.Sum(nil)))
}

func writePath(w hash.Hash, p NormalizedPath) {
	writeString(w, string(p.Namespace))
	writeU64(w, uint64(p.RootIndex))
	writeString(w, p.Relative)
	switch p.Namespace {
	case PathNamespaceWorkdirRoot, PathNamespaceCollectionRoot:
		writeString(w, "")
	default:
		writeString(w, p.Literal)
	}
	writeString(w, p.Display)
}
func writeLimitations(w hash.Hash, l []model.Limitation) {
	writeU64(w, uint64(len(l)))
	for _, x := range l {
		writeString(w, x.ID)
		writeString(w, x.Summary)
		writeString(w, x.Details)
	}
}
func writeEnv(w hash.Hash, env []model.EnvEntry) {
	writeU64(w, uint64(len(env)))
	for _, e := range env {
		writeString(w, e.Name)
		writeString(w, e.Value)
	}
}
func writeStrings(w hash.Hash, s []string) {
	writeU64(w, uint64(len(s)))
	for _, v := range s {
		writeString(w, v)
	}
}
func writeProcessFact(w hash.Hash, p ProcessFact) {
	writeString(w, p.Operation)
	writeString(w, string(p.StableID))
	writeString(w, string(p.ParentStableID))
	writeString(w, p.ParentRelation)
	writePath(w, p.Executable)
	writeStrings(w, p.Arguments)
	writeEnv(w, p.Environment)
	if p.ExitCode != nil {
		writeBool(w, true)
		writeI64(w, int64(*p.ExitCode))
	} else {
		writeBool(w, false)
	}
	writeI64(w, p.DurationMillis)
}
func writeFilesystemFact(w hash.Hash, f FilesystemFact) {
	writeString(w, f.Operation)
	writePath(w, f.Path)
	if f.OldPath != nil {
		writeBool(w, true)
		writePath(w, *f.OldPath)
	} else {
		writeBool(w, false)
	}
	writeString(w, f.Mode)
	writeDigest(w, f.Digest)
	writeI64(w, f.SizeBytes)
	writeBool(w, f.Executable)
	writeBool(w, f.Truncated)
}
func writeNetworkFact(w hash.Hash, n NetworkFact) {
	writeString(w, n.Operation)
	writeString(w, n.Protocol)
	writeString(w, n.QueryName)
	writeString(w, n.DestinationHost)
	writeI64(w, int64(n.DestinationPort))
	writeStrings(w, n.ResolvedAddresses)
	writeString(w, n.Result)
	writeI64(w, n.DurationMillis)
}
func writeArtifactFact(w hash.Hash, a ArtifactFact) {
	writeString(w, a.Operation)
	writeString(w, a.ArtifactID)
	writePath(w, a.Path)
	writeDigest(w, a.Digest)
	writeI64(w, a.SizeBytes)
	writeBool(w, a.Executable)
	writeStrings(w, a.SourceEventIDs)
}
func writeScenarioFact(w hash.Hash, s ScenarioFact) {
	writeString(w, string(s.Status))
	writeString(w, s.Message)
	writeI64(w, s.DurationMillis)
}
func writeWarningFact(w hash.Hash, wf WarningFact) {
	writeString(w, wf.Code)
	writeString(w, wf.Message)
	writeBool(w, wf.Unsupported)
	lim := append([]model.Limitation(nil), wf.Limitations...)
	sort.SliceStable(lim, func(i, j int) bool { return lim[i].ID < lim[j].ID })
	writeLimitations(w, lim)
}
func writeResourceFact(w hash.Hash, r ResourceFact) {
	writeString(w, r.LimitKind)
	writeI64(w, r.LimitValue)
	writeString(w, r.Unit)
	writeI64(w, r.ObservedValue)
	writeBool(w, r.Exceeded)
}
