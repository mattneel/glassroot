package evidence

import (
	"io"
	"time"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/runner"
)

const BundleFormatV1Alpha1 = "directory-v1alpha1"

type State string

const (
	StateActive     State = "active"
	StateFailed     State = "failed"
	StateCommitting State = "committing"
	StateCommitted  State = "committed"
	StateAborted    State = "aborted"
)

type LogStream string

const (
	LogStreamStdout LogStream = "stdout"
	LogStreamStderr LogStream = "stderr"
)

type CaptureState string

const (
	CaptureStateNotProvided   CaptureState = "not-provided"
	CaptureStateCapturedEmpty CaptureState = "captured-empty"
	CaptureStateCaptured      CaptureState = "captured"
	CaptureStateTruncated     CaptureState = "truncated"
	CaptureStateFailed        CaptureState = "failed"
	CaptureStateOmittedLimit  CaptureState = "omitted-limit"
)

type EntryRole string

const (
	EntryRolePlan            EntryRole = "plan"
	EntryRoleExecutionResult EntryRole = "execution-result"
	EntryRoleAttemptResult   EntryRole = "attempt-result"
	EntryRoleEvents          EntryRole = "events"
	EntryRoleStdout          EntryRole = "stdout"
	EntryRoleStderr          EntryRole = "stderr"
	EntryRoleArtifactIndex   EntryRole = "artifact-index"
	EntryRoleArtifactObject  EntryRole = "artifact-object"
)

type ArtifactDisposition string

const (
	ArtifactDispositionStored       ArtifactDisposition = "stored"
	ArtifactDispositionOmittedLimit ArtifactDisposition = "omitted-limit"
	ArtifactDispositionFailed       ArtifactDisposition = "failed"
)

type FailureCategory string

const (
	FailureCategoryCaptureLimit FailureCategory = "capture-limit"
	FailureCategoryCancellation FailureCategory = "cancellation"
	FailureCategoryRunner       FailureCategory = "runner-failure"
	FailureCategorySink         FailureCategory = "sink-failure"
	FailureCategoryFilesystem   FailureCategory = "filesystem-failure"
)

type AttemptKey struct {
	Revision   model.RevisionKind `json:"revision"`
	ScenarioID string             `json:"scenarioId"`
	Repetition uint32             `json:"repetition"`
}

type FailureRecord struct {
	Code      string          `json:"code"`
	Stage     string          `json:"stage"`
	AttemptID string          `json:"attemptId,omitempty"`
	Message   string          `json:"message"`
	Category  FailureCategory `json:"category"`
}

type Completion struct {
	Execution  runner.ExecutionResult
	Failure    *FailureRecord
	Incomplete bool
}

func Complete(result runner.ExecutionResult) Completion { return Completion{Execution: result} }
func Incomplete(result runner.ExecutionResult, failure FailureRecord) Completion {
	f := failure
	return Completion{Execution: result, Failure: &f, Incomplete: true}
}

type BundleResult struct {
	Path           string
	ManifestDigest model.Digest
	ManifestBytes  []byte
	EntryCount     int
	TotalBytes     int64
}

type Manifest struct {
	SchemaVersion          model.SchemaVersion `json:"schemaVersion"`
	ID                     string              `json:"id"`
	RunID                  string              `json:"runId"`
	CreatedAt              time.Time           `json:"createdAt"`
	BundleFormatVersion    string              `json:"bundleFormatVersion"`
	PlanDigest             model.Digest        `json:"planDigest"`
	ExecutionComplete      bool                `json:"executionComplete"`
	EvidenceComplete       bool                `json:"evidenceComplete"`
	BundleTransactionValid bool                `json:"bundleTransactionValid"`
	Entries                []ManifestEntry     `json:"entries"`
	Artifacts              []ArtifactRecord    `json:"artifacts"`
	Attempts               []AttemptManifest   `json:"attempts"`
	Limitations            []model.Limitation  `json:"limitations"`
	Failure                *FailureRecord      `json:"failure,omitempty"`
	ManifestDigest         model.Digest        `json:"-"`
}

type ManifestEntry struct {
	Path                 string             `json:"path"`
	Role                 EntryRole          `json:"role"`
	MediaType            string             `json:"mediaType"`
	Digest               model.Digest       `json:"digest"`
	SizeBytes            int64              `json:"sizeBytes"`
	Revision             model.RevisionKind `json:"revision,omitempty"`
	ScenarioID           string             `json:"scenarioId,omitempty"`
	Repetition           uint32             `json:"repetition,omitempty"`
	CaptureState         CaptureState       `json:"captureState,omitempty"`
	Truncated            bool               `json:"truncated,omitempty"`
	Omitted              bool               `json:"omitted,omitempty"`
	ObservedBytes        int64              `json:"observedBytes,omitempty"`
	ObservedBytesAtLeast int64              `json:"observedBytesAtLeast,omitempty"`
}

type AttemptManifest struct {
	AttemptID          string             `json:"attemptId"`
	Ordinal            uint64             `json:"ordinal"`
	Revision           model.RevisionKind `json:"revision"`
	ScenarioID         string             `json:"scenarioId"`
	Repetition         uint32             `json:"repetition"`
	Directory          string             `json:"directory"`
	Events             CaptureState       `json:"events"`
	Stdout             CaptureState       `json:"stdout"`
	Stderr             CaptureState       `json:"stderr"`
	Artifacts          CaptureState       `json:"artifacts"`
	Result             CaptureState       `json:"result"`
	FirstEventSequence uint64             `json:"firstEventSequence,omitempty"`
	LastEventSequence  uint64             `json:"lastEventSequence,omitempty"`
	AcceptedEventCount uint64             `json:"acceptedEventCount"`
}

type ArtifactRecord struct {
	LogicalPath     string              `json:"logicalPath"`
	Attempt         AttemptKey          `json:"attempt"`
	Disposition     ArtifactDisposition `json:"disposition"`
	Digest          model.Digest        `json:"digest,omitempty"`
	StoredSizeBytes int64               `json:"storedSizeBytes,omitempty"`
	DeclaredSize    *int64              `json:"declaredSizeBytes,omitempty"`
	ObservedAtLeast int64               `json:"observedBytesAtLeast,omitempty"`
	ObjectPath      string              `json:"objectPath,omitempty"`
	MediaType       string              `json:"mediaType,omitempty"`
	Limitations     []model.Limitation  `json:"limitations"`
}

type ArtifactInput struct {
	Attempt      AttemptKey
	LogicalPath  string
	DeclaredSize *int64
	MaxBytes     int64
	Reader       io.Reader
	MediaType    string
}

type ArtifactCaptureResult = ArtifactRecord

type ExecutionDocument struct {
	SchemaVersion          model.SchemaVersion       `json:"schemaVersion"`
	RunID                  string                    `json:"runId"`
	PlanDigest             model.Digest              `json:"planDigest"`
	Runner                 model.RunnerCapabilities  `json:"runner"`
	ExecutionComplete      bool                      `json:"executionComplete"`
	EvidenceComplete       bool                      `json:"evidenceComplete"`
	BundleTransactionValid bool                      `json:"bundleTransactionValid"`
	TotalAcceptedEvents    uint64                    `json:"totalAcceptedEvents"`
	Attempts               []AttemptExecutionSummary `json:"attempts"`
	Limitations            []model.Limitation        `json:"limitations"`
	Failure                *FailureRecord            `json:"failure,omitempty"`
}

type AttemptExecutionSummary struct {
	AttemptID             string               `json:"attemptId"`
	Revision              model.RevisionKind   `json:"revision"`
	ScenarioID            string               `json:"scenarioId"`
	Repetition            uint32               `json:"repetition"`
	TargetOutcome         runner.AttemptStatus `json:"targetOutcome"`
	ExitCode              *int                 `json:"exitCode,omitempty"`
	DurationMillis        int64                `json:"durationMillis"`
	FirstAcceptedSequence uint64               `json:"firstAcceptedSequence"`
	LastAcceptedSequence  uint64               `json:"lastAcceptedSequence"`
	AcceptedEventCount    uint64               `json:"acceptedEventCount"`
	Limitations           []model.Limitation   `json:"limitations"`
}

type AttemptResultDocument struct {
	SchemaVersion         model.SchemaVersion  `json:"schemaVersion"`
	AttemptID             string               `json:"attemptId"`
	Revision              model.RevisionKind   `json:"revision"`
	ScenarioID            string               `json:"scenarioId"`
	Repetition            uint32               `json:"repetition"`
	TargetOutcome         runner.AttemptStatus `json:"targetOutcome"`
	ExitCode              *int                 `json:"exitCode,omitempty"`
	DurationMillis        int64                `json:"durationMillis"`
	FirstAcceptedSequence uint64               `json:"firstAcceptedSequence"`
	LastAcceptedSequence  uint64               `json:"lastAcceptedSequence"`
	AcceptedEventCount    uint64               `json:"acceptedEventCount"`
	Limitations           []model.Limitation   `json:"limitations"`
}

type ArtifactIndexDocument struct {
	SchemaVersion model.SchemaVersion `json:"schemaVersion"`
	Attempt       AttemptKey          `json:"attempt"`
	Artifacts     []ArtifactRecord    `json:"artifacts"`
}
