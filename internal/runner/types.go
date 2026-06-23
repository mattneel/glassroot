package runner

import (
	"context"
	"time"

	"github.com/mattneel/glassroot/internal/model"
)

const (
	MaxAttempts               = 1280
	MaxEventsPerAttempt       = 10000
	MaxEventsPerExecution     = 100000
	MaxEventJSONBytes         = 256 << 10
	MaxRunnerNameBytes        = 128
	MaxRunnerVersionBytes     = 128
	MaxCapabilityMismatches   = 64
	MaxProgramLimitations     = 1000
	MaxResultLimitations      = 1000
	MaxLimitationCodeBytes    = 128
	MaxLimitationMessageBytes = 1 << 10
	MaxAttemptIDBytes         = 512
)

type Runner interface {
	Capabilities(context.Context) (model.RunnerCapabilities, error)
	RunAttempt(context.Context, AttemptRequest, DraftSink) (AttemptOutcome, error)
}

type PlanAwareRunner interface {
	ValidatePlan(context.Context, model.Digest, []AttemptRequest, Limits) error
}

type DraftSink interface {
	Emit(context.Context, EventDraft) error
}

type EventSink interface {
	Emit(context.Context, model.ObservationEvent) error
}

type ExecutionIntent string

const (
	ExecutionIntentSyntheticTest ExecutionIntent = "synthetic-test"
	ExecutionIntentWorkload      ExecutionIntent = "workload"
)

type Requirements struct {
	Intent                         ExecutionIntent
	AllowedIsolationTiers          []model.IsolationTier
	TargetExecutionRequired        bool
	SyntheticEvidenceAllowed       bool
	NetworkDenyEnforcementRequired bool
	BrokeredNetworkRequired        bool
	ProcessEventsRequired          bool
	FilesystemEventsRequired       bool
	SyscallEventsRequired          bool
	ArtifactHashingRequired        bool
	SnapshotSupportRequired        bool
	FreshKernelRequired            bool
}

type Limits struct {
	MaxAttempts             int64
	MaxEventsPerAttempt     int64
	MaxEventsPerExecution   int64
	MaxEventJSONBytes       int64
	MaxCapabilityMismatches int64
	MaxResultLimitations    int64
}

func DefaultLimits() Limits {
	return Limits{
		MaxAttempts:             MaxAttempts,
		MaxEventsPerAttempt:     MaxEventsPerAttempt,
		MaxEventsPerExecution:   MaxEventsPerExecution,
		MaxEventJSONBytes:       MaxEventJSONBytes,
		MaxCapabilityMismatches: MaxCapabilityMismatches,
		MaxResultLimitations:    MaxResultLimitations,
	}
}

func SyntheticTestRequirements() Requirements {
	return Requirements{
		Intent:                   ExecutionIntentSyntheticTest,
		AllowedIsolationTiers:    []model.IsolationTier{model.IsolationTierFake},
		SyntheticEvidenceAllowed: true,
	}
}

func WorkloadRequirements(tiers []model.IsolationTier) Requirements {
	return Requirements{
		Intent:                         ExecutionIntentWorkload,
		AllowedIsolationTiers:          append([]model.IsolationTier(nil), tiers...),
		TargetExecutionRequired:        true,
		NetworkDenyEnforcementRequired: true,
	}
}

type AttemptRequest struct {
	PlanDigest                    model.Digest
	RunID                         string
	PlanCreatedAt                 time.Time
	AttemptID                     string
	GlobalOrdinal                 uint64
	Revision                      model.RevisionKind
	CommitID                      string
	TreeID                        string
	ObjectFormat                  model.GitObjectFormat
	MaterializedTreeDigest        model.Digest
	MaterializationManifestDigest model.Digest
	Image                         string
	Workdir                       string
	Environment                   []model.EnvEntry
	ResourceLimits                model.ResourceLimits
	NetworkPolicy                 model.NetworkPolicy
	ScenarioID                    string
	ScenarioName                  string
	Shell                         string
	Run                           string
	ScenarioTimeoutMillis         int64
	Repetition                    uint32
	Collection                    CollectionSettings
}

type CollectionSettings struct {
	FilesystemRoots      []string
	FilesystemContents   string
	Artifacts            []model.ExpectedArtifactSpec
	LogMaxBytesPerStream int64
}

type EventDraft struct {
	ObservedAt      time.Time
	Source          model.ObservationSource
	Kind            model.ObservationKind
	Process         *model.ProcessObservation
	Filesystem      *model.FilesystemObservation
	Network         *model.NetworkObservation
	Artifact        *model.ArtifactObservation
	Scenario        *model.ScenarioObservation
	ObserverWarning *model.ObserverWarningObservation
	ResourceLimit   *model.ResourceLimitObservation
}

type AttemptStatus string

const (
	AttemptStatusSucceeded       AttemptStatus = "succeeded"
	AttemptStatusFailed          AttemptStatus = "failed"
	AttemptStatusTimedOut        AttemptStatus = "timed-out"
	AttemptStatusResourceLimited AttemptStatus = "resource-limited"
)

type AttemptOutcome struct {
	Status         AttemptStatus
	ExitCode       *int
	DurationMillis int64
	Limitations    []model.Limitation
}

type ExecutionResult struct {
	PlanDigest         model.Digest
	Runner             model.RunnerCapabilities
	Complete           bool
	Attempts           []AttemptResult
	TotalEmittedEvents uint64
	Limitations        []model.Limitation
}

type AttemptResult struct {
	AttemptID             string
	Revision              model.RevisionKind
	ScenarioID            string
	Repetition            uint32
	Outcome               AttemptOutcome
	FirstAcceptedSequence uint64
	LastAcceptedSequence  uint64
	AcceptedEventCount    uint64
	Limitations           []model.Limitation
}

type CapabilityMismatchCode string

const (
	MismatchExecutionIntent        CapabilityMismatchCode = "execution-intent"
	MismatchIsolationTier          CapabilityMismatchCode = "isolation-tier"
	MismatchTargetExecution        CapabilityMismatchCode = "target-execution"
	MismatchSyntheticEvidence      CapabilityMismatchCode = "synthetic-evidence"
	MismatchNetworkDenyEnforcement CapabilityMismatchCode = "network-deny-enforcement"
	MismatchBrokeredNetwork        CapabilityMismatchCode = "brokered-network"
	MismatchProcessEvents          CapabilityMismatchCode = "process-events"
	MismatchFilesystemEvents       CapabilityMismatchCode = "filesystem-events"
	MismatchSyscallEvents          CapabilityMismatchCode = "syscall-events"
	MismatchArtifactHashing        CapabilityMismatchCode = "artifact-hashing"
	MismatchSnapshotSupport        CapabilityMismatchCode = "snapshot-support"
	MismatchFreshKernel            CapabilityMismatchCode = "fresh-kernel"
)

type CapabilityMismatch struct {
	Code     CapabilityMismatchCode
	Required string
	Actual   string
}
