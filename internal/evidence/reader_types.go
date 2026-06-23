package evidence

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/mattneel/glassroot/internal/model"
)

type VerificationMode string

const (
	VerificationModeInternalConsistencyOnly VerificationMode = "internal-consistency-only"
	VerificationModeExpectedManifestDigest  VerificationMode = "expected-manifest-digest"
)

type VerificationSummary struct {
	Mode                           VerificationMode `json:"mode"`
	ManifestDigest                 model.Digest     `json:"manifestDigest"`
	ExpectedManifestDigestSupplied bool             `json:"expectedManifestDigestSupplied"`
	ExpectedManifestDigestMatched  bool             `json:"expectedManifestDigestMatched"`
	InternallyConsistent           bool             `json:"internallyConsistent"`
	Limitations                    []model.Limitation
}

type VerifiedAttempt struct {
	AttemptID          string
	Ordinal            uint64
	Revision           model.RevisionKind
	ScenarioID         string
	Repetition         uint32
	Directory          string
	Events             CaptureState
	Stdout             CaptureState
	Stderr             CaptureState
	Artifacts          CaptureState
	Result             CaptureState
	FirstEventSequence uint64
	LastEventSequence  uint64
	AcceptedEventCount uint64
}

type VerifiedEntryReference struct {
	Path         string
	Digest       model.Digest
	SizeBytes    int64
	Role         EntryRole
	CaptureState CaptureState
	Attempt      AttemptKey
}

type CopyResult struct {
	Bytes        int64
	Digest       model.Digest
	CaptureState CaptureState
	NotStored    bool
	Disposition  ArtifactDisposition
}

type Bundle struct {
	mu     sync.Mutex
	root   *os.Root
	closed bool

	limits         ReaderLimits
	manifest       Manifest
	manifestDigest model.Digest
	plan           model.RunPlan
	execution      ExecutionDocument
	attempts       []VerifiedAttempt
	verification   VerificationSummary

	entries         map[string]ManifestEntry
	attemptsByKey   map[string]VerifiedAttempt
	eventsByAttempt map[string]ManifestEntry
	logs            map[string]ManifestEntry
	artifactRecords map[string]ArtifactRecord
	artifactObjects map[model.Digest]ManifestEntry
	artifactIndexes map[string]ArtifactIndexDocument
	eventOrder      []string
	planDigest      model.Digest
	runID           string
}

func (b *Bundle) Manifest() Manifest {
	if b == nil {
		return Manifest{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return cloneManifest(b.manifest)
}
func (b *Bundle) ManifestDigest() model.Digest {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.manifestDigest
}
func (b *Bundle) Plan() model.RunPlan {
	if b == nil {
		return model.RunPlan{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return cloneRunPlan(b.plan)
}
func (b *Bundle) Execution() ExecutionDocument {
	if b == nil {
		return ExecutionDocument{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return cloneExecutionDocument(b.execution)
}
func (b *Bundle) Attempts() []VerifiedAttempt {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]VerifiedAttempt, len(b.attempts))
	copy(out, b.attempts)
	return out
}
func (b *Bundle) Verification() VerificationSummary {
	if b == nil {
		return VerificationSummary{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.verification
	out.Limitations = cloneLimitations(out.Limitations)
	return out
}
func (b *Bundle) EventStreamReference(attempt AttemptKey) (VerifiedEntryReference, error) {
	if b == nil {
		return VerifiedEntryReference{}, errCode(CodeInvalidSessionState, "event-reference", "open", "bundle is nil", nil)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.ensureOpen("event-reference"); err != nil {
		return VerifiedEntryReference{}, err
	}
	entry, ok := b.eventsByAttempt[attemptKeyString(attempt)]
	if !ok {
		return VerifiedEntryReference{}, errCode(CodeInvalidAttempt, "event-reference", "attempt", "unknown event stream attempt", nil)
	}
	return VerifiedEntryReference{Path: entry.Path, Digest: entry.Digest, SizeBytes: entry.SizeBytes, Role: entry.Role, CaptureState: entry.CaptureState, Attempt: attempt}, nil
}

func (b *Bundle) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	if b.root != nil {
		err := b.root.Close()
		b.root = nil
		if err != nil {
			return errCode(CodeCloseFailed, "reader", "close", "close bundle root", err)
		}
	}
	return nil
}
func (b *Bundle) ensureOpen(stage string) error {
	if b == nil {
		return errCode(CodeInvalidSessionState, stage, "open", "bundle is nil", nil)
	}
	if b.closed || b.root == nil {
		return errCode(CodeInvalidSessionState, stage, "open", "bundle is closed", nil)
	}
	return nil
}

func cloneManifest(in Manifest) Manifest {
	out := in
	out.Entries = cloneManifestEntries(in.Entries)
	out.Artifacts = cloneArtifactRecords(in.Artifacts)
	out.Attempts = cloneAttemptManifests(in.Attempts)
	out.Limitations = cloneLimitations(in.Limitations)
	out.Failure = cloneFailure(in.Failure)
	return out
}
func cloneManifestEntries(in []ManifestEntry) []ManifestEntry {
	if in == nil {
		return []ManifestEntry{}
	}
	out := make([]ManifestEntry, len(in))
	copy(out, in)
	return out
}
func cloneAttemptManifests(in []AttemptManifest) []AttemptManifest {
	if in == nil {
		return []AttemptManifest{}
	}
	out := make([]AttemptManifest, len(in))
	copy(out, in)
	return out
}
func cloneRunPlan(in model.RunPlan) model.RunPlan {
	var out model.RunPlan
	data, _ := json.Marshal(in)
	_ = json.Unmarshal(data, &out)
	return out
}
func cloneExecutionDocument(in ExecutionDocument) ExecutionDocument {
	out := in
	out.Attempts = cloneAttemptExecutionSummaries(in.Attempts)
	out.Limitations = cloneLimitations(in.Limitations)
	out.Failure = cloneFailure(in.Failure)
	return out
}
func cloneAttemptExecutionSummaries(in []AttemptExecutionSummary) []AttemptExecutionSummary {
	if in == nil {
		return []AttemptExecutionSummary{}
	}
	out := make([]AttemptExecutionSummary, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].ExitCode = cloneIntPtr(in[i].ExitCode)
		out[i].Limitations = cloneLimitations(in[i].Limitations)
	}
	return out
}
func cloneArtifactIndex(in ArtifactIndexDocument) ArtifactIndexDocument {
	out := in
	out.Artifacts = cloneArtifactRecords(in.Artifacts)
	return out
}
