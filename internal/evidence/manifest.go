package evidence

import (
	"encoding/json"
	"io"
	"sort"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/runner"
)

func (s *Session) writeResults(completion Completion) error {
	if completion.Execution.PlanDigest != s.planDigest {
		return errCode(CodeResultInvalid, "result", "planDigest", "execution result plan digest mismatch", nil)
	}
	results := map[string]runner.AttemptResult{}
	for _, r := range completion.Execution.Attempts {
		k := attemptKeyString(AttemptKey{Revision: r.Revision, ScenarioID: r.ScenarioID, Repetition: r.Repetition})
		if _, ok := results[k]; ok {
			return errCode(CodeResultInvalid, "result", "attempt", "duplicate attempt result", nil)
		}
		results[k] = r
	}
	if completion.Execution.Complete && len(results) != len(s.attemptOrder) {
		return errCode(CodeResultInvalid, "result", "attempts", "complete execution missing attempt result", nil)
	}
	for _, ak := range s.attemptOrder {
		as := s.attempts[ak]
		r, ok := results[ak]
		if !ok {
			continue
		}
		if r.AcceptedEventCount != as.eventsCount || r.FirstAcceptedSequence != as.firstSeq || r.LastAcceptedSequence != as.lastSeq {
			return attemptErr(CodeResultInvalid, "result", "events", as.attemptID, "attempt result event range does not match persisted events", nil)
		}
		attemptLimitations := cloneLimitations(r.Limitations)
		doc := AttemptResultDocument{SchemaVersion: model.SchemaVersionAttemptResultV1Alpha1, AttemptID: r.AttemptID, Revision: r.Revision, ScenarioID: r.ScenarioID, Repetition: r.Repetition, TargetOutcome: r.Outcome.Status, ExitCode: cloneIntPtr(r.Outcome.ExitCode), DurationMillis: r.Outcome.DurationMillis, FirstAcceptedSequence: r.FirstAcceptedSequence, LastAcceptedSequence: r.LastAcceptedSequence, AcceptedEventCount: r.AcceptedEventCount, Limitations: attemptLimitations}
		data, err := json.Marshal(doc)
		if err != nil {
			return errCode(CodeSerializationFailed, "result", "json", "marshal attempt result", err)
		}
		if err := s.writePayload(as.dir+"/result.json", EntryRoleAttemptResult, "application/json", as, data); err != nil {
			return err
		}
		as.resultState = CaptureStateCaptured
	}
	execDoc := ExecutionDocument{SchemaVersion: model.SchemaVersionExecutionResultV1Alpha1, RunID: s.runID, PlanDigest: s.planDigest, Runner: completion.Execution.Runner, ExecutionComplete: completion.Execution.Complete, EvidenceComplete: !s.evidenceIncomplete && completion.Execution.Complete && !completion.Incomplete, BundleTransactionValid: true, TotalAcceptedEvents: completion.Execution.TotalEmittedEvents, Attempts: []AttemptExecutionSummary{}, Limitations: cloneLimitations(completion.Execution.Limitations), Failure: cloneFailure(completion.Failure)}
	for _, r := range completion.Execution.Attempts {
		attemptLimitations := cloneLimitations(r.Limitations)
		execDoc.Attempts = append(execDoc.Attempts, AttemptExecutionSummary{AttemptID: r.AttemptID, Revision: r.Revision, ScenarioID: r.ScenarioID, Repetition: r.Repetition, TargetOutcome: r.Outcome.Status, ExitCode: cloneIntPtr(r.Outcome.ExitCode), DurationMillis: r.Outcome.DurationMillis, FirstAcceptedSequence: r.FirstAcceptedSequence, LastAcceptedSequence: r.LastAcceptedSequence, AcceptedEventCount: r.AcceptedEventCount, Limitations: attemptLimitations})
	}
	data, err := json.Marshal(execDoc)
	if err != nil {
		return errCode(CodeSerializationFailed, "execution", "json", "marshal execution result", err)
	}
	return s.writePayload("execution.json", EntryRoleExecutionResult, "application/json", nil, data)
}

func (s *Session) buildManifest(completion Completion) (Manifest, error) {
	if completion.Execution.PlanDigest != s.planDigest {
		return Manifest{}, errCode(CodeCompletionInvalid, "completion", "planDigest", "plan digest mismatch", nil)
	}
	evidenceComplete := !s.evidenceIncomplete && completion.Execution.Complete && !completion.Incomplete
	if !completion.Incomplete && !evidenceComplete {
		return Manifest{}, errCode(CodeCompletionInvalid, "completion", "evidence", "complete bundle cannot contain incomplete evidence", nil)
	}
	if completion.Incomplete && completion.Failure == nil {
		return Manifest{}, errCode(CodeCompletionInvalid, "completion", "failure", "incomplete completion requires failure record", nil)
	}
	attempts := make([]AttemptManifest, 0, len(s.attemptOrder))
	for _, ak := range s.attemptOrder {
		as := s.attempts[ak]
		attempts = append(attempts, AttemptManifest{AttemptID: as.attemptID, Ordinal: as.ordinal, Revision: as.key.Revision, ScenarioID: as.key.ScenarioID, Repetition: as.key.Repetition, Directory: as.dir, Events: as.eventsState, Stdout: as.stdoutState, Stderr: as.stderrState, Artifacts: as.artifactsState, Result: as.resultState, FirstEventSequence: as.firstSeq, LastEventSequence: as.lastSeq, AcceptedEventCount: as.eventsCount})
	}
	limitations := cloneLimitations(s.limitations)
	if s.evidenceIncomplete {
		limitations = append(limitations, model.Limitation{ID: "evidence-incomplete", Summary: "one or more evidence captures were truncated or omitted"})
	}
	return Manifest{SchemaVersion: model.SchemaVersionEvidenceManifestV1Alpha1, ID: "bundle-" + s.runID, RunID: s.runID, CreatedAt: s.createdAt, BundleFormatVersion: BundleFormatV1Alpha1, PlanDigest: s.planDigest, ExecutionComplete: completion.Execution.Complete, EvidenceComplete: evidenceComplete, BundleTransactionValid: true, Entries: sortedEntries(s.entries), Artifacts: cloneArtifactRecords(s.artifactRecords), Attempts: attempts, Limitations: limitations, Failure: cloneFailure(completion.Failure)}, nil
}

func normalizeManifest(m Manifest, limits Limits) ([]byte, error) {
	if m.SchemaVersion != model.SchemaVersionEvidenceManifestV1Alpha1 || m.RunID == "" || m.ID == "" || !validDigest(m.PlanDigest) {
		return nil, errCode(CodeManifestInvariant, "manifest", "validate", "invalid manifest identity", nil)
	}
	m.Entries = sortedEntries(m.Entries)
	seen := map[string]struct{}{}
	for _, e := range m.Entries {
		if err := ValidateEvidenceEntryPath(e.Path); err != nil {
			return nil, err
		}
		if _, ok := seen[e.Path]; ok {
			return nil, pathErr(CodeManifestInvariant, "manifest", "entry", e.Path, "duplicate manifest entry", nil)
		}
		seen[e.Path] = struct{}{}
		if !validDigest(e.Digest) || e.SizeBytes < 0 || e.Path == "manifest.json" {
			return nil, pathErr(CodeManifestInvariant, "manifest", "entry", e.Path, "invalid manifest entry", nil)
		}
	}
	sort.SliceStable(m.Artifacts, func(i, j int) bool {
		if m.Artifacts[i].Attempt.Revision != m.Artifacts[j].Attempt.Revision {
			return m.Artifacts[i].Attempt.Revision < m.Artifacts[j].Attempt.Revision
		}
		if m.Artifacts[i].Attempt.ScenarioID != m.Artifacts[j].Attempt.ScenarioID {
			return m.Artifacts[i].Attempt.ScenarioID < m.Artifacts[j].Attempt.ScenarioID
		}
		if m.Artifacts[i].Attempt.Repetition != m.Artifacts[j].Attempt.Repetition {
			return m.Artifacts[i].Attempt.Repetition < m.Artifacts[j].Attempt.Repetition
		}
		return m.Artifacts[i].LogicalPath < m.Artifacts[j].LogicalPath
	})
	if m.Entries == nil {
		m.Entries = []ManifestEntry{}
	}
	if m.Artifacts == nil {
		m.Artifacts = []ArtifactRecord{}
	}
	if m.Attempts == nil {
		m.Attempts = []AttemptManifest{}
	}
	if m.Limitations == nil {
		m.Limitations = []model.Limitation{}
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, errCode(CodeSerializationFailed, "manifest", "json", "marshal manifest", err)
	}
	if int64(len(data)) > limits.MaxManifestBytes {
		return nil, errCode(CodeManifestTooLarge, "manifest", "size", "manifest exceeds size limit", nil)
	}
	return data, nil
}

func (s *Session) writeManifest(data []byte) error {
	f, err := s.openExclusive("manifest.json.tmp")
	if err != nil {
		return err
	}
	if n, err := f.Write(data); err != nil || n != len(data) {
		if err == nil {
			err = ioErrShortWrite()
		}
		_ = f.Close()
		return pathErr(CodeEventWriteFailed, "manifest", "write", "manifest.json.tmp", "write manifest", err)
	}
	if err := syncFile(f, s.writer.hooks); err != nil {
		_ = f.Close()
		return pathErr(CodeSyncFailed, "manifest", "sync", "manifest.json.tmp", "sync manifest", err)
	}
	if err := f.Close(); err != nil {
		return pathErr(CodeEventWriteFailed, "manifest", "close", "manifest.json.tmp", "close manifest", err)
	}
	if err := s.root.Rename("manifest.json.tmp", "manifest.json"); err != nil {
		return pathErr(CodePublishFailed, "manifest", "rename", "manifest.json", "publish manifest", err)
	}
	delete(s.payloads, "manifest.json.tmp")
	return nil
}

func cloneIntPtr(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
func cloneFailure(f *FailureRecord) *FailureRecord {
	if f == nil {
		return nil
	}
	c := *f
	return &c
}
func cloneLimitations(in []model.Limitation) []model.Limitation {
	out := make([]model.Limitation, len(in))
	copy(out, in)
	return out
}
func cloneArtifactRecord(in ArtifactRecord) ArtifactRecord {
	out := in
	out.DeclaredSize = cloneInt64Ptr(in.DeclaredSize)
	out.Limitations = cloneLimitations(in.Limitations)
	return out
}
func cloneArtifactRecords(in []ArtifactRecord) []ArtifactRecord {
	if in == nil {
		return []ArtifactRecord{}
	}
	out := make([]ArtifactRecord, len(in))
	for i := range in {
		out[i] = cloneArtifactRecord(in[i])
	}
	return out
}
func ioErrShortWrite() error { return io.ErrShortWrite }
