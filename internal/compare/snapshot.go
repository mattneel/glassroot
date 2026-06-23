package compare

import (
	"encoding/json"
	"sort"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/observe"
)

func snapshots(facts []observe.Fact) []model.DeltaFactSnapshot {
	out := make([]model.DeltaFactSnapshot, 0, len(facts))
	for _, f := range facts {
		out = append(out, snapshot(f))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SemanticDigest != out[j].SemanticDigest {
			return out[i].SemanticDigest < out[j].SemanticDigest
		}
		return out[i].ID < out[j].ID
	})
	return out
}
func snapshot(f observe.Fact) model.DeltaFactSnapshot {
	s := model.DeltaFactSnapshot{ID: string(f.ID), SemanticDigest: f.SemanticDigest, Kind: string(f.Kind), Source: f.Source, Evidence: evidenceRefs([]observe.Fact{f}), Limitations: sortLimitations(f.Limitations)}
	if f.Process != nil {
		s.Process = &model.DeltaProcessFact{Operation: f.Process.Operation, StableID: string(f.Process.StableID), ParentStableID: string(f.Process.ParentStableID), ParentRelation: f.Process.ParentRelation, Executable: pathSnap(f.Process.Executable), Arguments: cloneStrings(f.Process.Arguments), Environment: cloneEnv(f.Process.Environment), ExitCode: cloneInt(f.Process.ExitCode), DurationMillis: f.Process.DurationMillis}
	}
	if f.Filesystem != nil {
		fs := &model.DeltaFilesystemFact{Operation: f.Filesystem.Operation, Path: pathSnap(f.Filesystem.Path), Mode: f.Filesystem.Mode, Digest: f.Filesystem.Digest, SizeBytes: f.Filesystem.SizeBytes, Executable: f.Filesystem.Executable, Truncated: f.Filesystem.Truncated}
		if f.Filesystem.OldPath != nil {
			op := pathSnap(*f.Filesystem.OldPath)
			fs.OldPath = &op
		}
		s.Filesystem = fs
	}
	if f.Network != nil {
		s.Network = &model.DeltaNetworkFact{Operation: f.Network.Operation, Protocol: f.Network.Protocol, QueryName: f.Network.QueryName, DestinationHost: f.Network.DestinationHost, DestinationPort: f.Network.DestinationPort, ResolvedAddresses: cloneStrings(f.Network.ResolvedAddresses), Result: f.Network.Result, DurationMillis: f.Network.DurationMillis}
	}
	if f.Artifact != nil {
		s.Artifact = &model.DeltaArtifactFact{Operation: f.Artifact.Operation, ArtifactID: f.Artifact.ArtifactID, Path: pathSnap(f.Artifact.Path), Digest: f.Artifact.Digest, SizeBytes: f.Artifact.SizeBytes, Executable: f.Artifact.Executable, SourceEventIDs: cloneStrings(f.Artifact.SourceEventIDs)}
	}
	if f.Scenario != nil {
		s.Scenario = &model.DeltaScenarioFact{Status: f.Scenario.Status, Message: f.Scenario.Message, DurationMillis: f.Scenario.DurationMillis}
	}
	if f.Warning != nil {
		s.Warning = &model.DeltaWarningFact{Code: f.Warning.Code, Message: f.Warning.Message, Unsupported: f.Warning.Unsupported, Limitations: sortLimitations(f.Warning.Limitations)}
	}
	if f.Resource != nil {
		s.Resource = &model.DeltaResourceFact{LimitKind: f.Resource.LimitKind, LimitValue: f.Resource.LimitValue, Unit: f.Resource.Unit, ObservedValue: f.Resource.ObservedValue, Exceeded: f.Resource.Exceeded}
	}
	return s
}
func pathSnap(p observe.NormalizedPath) model.DeltaNormalizedPath {
	return model.DeltaNormalizedPath{Namespace: string(p.Namespace), RootIndex: p.RootIndex, Relative: p.Relative, Literal: p.Literal, Display: p.Display}
}

func evidenceRefs(facts []observe.Fact) []model.EvidenceRef {
	out := []model.EvidenceRef{}
	for _, f := range facts {
		for _, r := range f.Evidence {
			path := r.EventStreamPath
			out = append(out, model.EvidenceRef{Digest: r.EventStreamDigest, EventStreamDigest: r.EventStreamDigest, EventStreamPath: r.EventStreamPath, EventIDs: []string{r.EventID}, BundlePath: &path, EventSequence: r.EventSequence, Revision: r.Revision, ScenarioID: r.ScenarioID, Repetition: r.Repetition})
		}
	}
	return dedupeEvidence(out)
}
func dedupeEvidence(in []model.EvidenceRef) []model.EvidenceRef {
	seen := map[string]model.EvidenceRef{}
	for _, r := range in {
		key := string(r.Revision) + "\x00" + r.ScenarioID + "\x00" + itoa(r.Repetition) + "\x00" + itoa64(r.EventSequence) + "\x00"
		if len(r.EventIDs) > 0 {
			key += r.EventIDs[0]
		}
		seen[key] = r
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]model.EvidenceRef, 0, len(keys))
	for _, k := range keys {
		out = append(out, seen[k])
	}
	return out
}

func cloneDelta(in model.BehavioralDelta) model.BehavioralDelta {
	var out model.BehavioralDelta
	b, _ := json.Marshal(in)
	_ = json.Unmarshal(b, &out)
	if out.ScenarioIDs == nil {
		out.ScenarioIDs = []string{}
	}
	if out.Records == nil {
		out.Records = []model.DeltaRecord{}
	}
	if out.Limitations == nil {
		out.Limitations = []model.Limitation{}
	}
	if out.ScenarioComparisons == nil {
		out.ScenarioComparisons = []model.ScenarioComparison{}
	}
	return out
}
func sortRecords(in []model.DeltaRecord) []model.DeltaRecord {
	out := append([]model.DeltaRecord(nil), in...)
	rank := func(k model.DeltaKind) int {
		switch k {
		case model.DeltaKindCoverageChanged:
			return 0
		case model.DeltaKindStabilityChanged:
			return 1
		case model.DeltaKindAdded:
			return 2
		case model.DeltaKindRemoved:
			return 3
		case model.DeltaKindModified:
			return 4
		case model.DeltaKindCountChanged:
			return 5
		case model.DeltaKindOrderChanged:
			return 6
		default:
			return 9
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if firstScenario(out[i].ScenarioIDs) != firstScenario(out[j].ScenarioIDs) {
			return firstScenario(out[i].ScenarioIDs) < firstScenario(out[j].ScenarioIDs)
		}
		if rank(out[i].Kind) != rank(out[j].Kind) {
			return rank(out[i].Kind) < rank(out[j].Kind)
		}
		if out[i].FactKind != out[j].FactKind {
			return out[i].FactKind < out[j].FactKind
		}
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		if out[i].AnchorDigest != out[j].AnchorDigest {
			return out[i].AnchorDigest < out[j].AnchorDigest
		}
		return out[i].ID < out[j].ID
	})
	return out
}
func sortLimitations(in []model.Limitation) []model.Limitation {
	if len(in) == 0 {
		return []model.Limitation{}
	}
	out := append([]model.Limitation(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		if out[i].Summary != out[j].Summary {
			return out[i].Summary < out[j].Summary
		}
		return out[i].Details < out[j].Details
	})
	return out
}
func cloneStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
func cloneEnv(in []model.EnvEntry) []model.EnvEntry {
	if in == nil {
		return []model.EnvEntry{}
	}
	out := make([]model.EnvEntry, len(in))
	copy(out, in)
	return out
}
func cloneInt(in *int) *int {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}
func itoa(v uint32) string {
	if v == 0 {
		return "0"
	}
	b := []byte{}
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}
func itoa64(v uint64) string {
	if v == 0 {
		return "0"
	}
	b := []byte{}
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}
