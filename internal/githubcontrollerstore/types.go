package githubcontrollerstore

import (
	"time"

	"github.com/mattneel/glassroot/internal/githubapi"
	"github.com/mattneel/glassroot/internal/githubapp"
)

const (
	SchemaControllerStoreV1Alpha1     = "glassroot.dev/github-controller-store/v1alpha1"
	ControllerProfileAdvisoryV1Alpha1 = "glassroot.dev/github-controller-profile/advisory/v1alpha1"
	SchemaSourceImportRequestV1Alpha1 = "glassroot.dev/github-source-import-request/v1alpha1"
	DomainSourceImportRequestID       = "glassroot.dev/github-source-import-request-id/v1\x00"
)

type Config struct {
	StateDir, ControllerID, ReceiverID string
	AppID                              int64
	Limits                             Limits
}
type Limits struct {
	MaxPathBytes                           int
	MaxDatabaseBytes                       int64
	BusyTimeoutMilliseconds                int
	MaxOpenConnections, MaxIdleConnections int
	MaxRouteSegmentBytes                   int
	MaxAttemptsPerJob                      int64
	ReconcileLeaseDuration                 time.Duration
	SourceLeaseDuration                    time.Duration
}

func DefaultLimits() Limits {
	return Limits{MaxPathBytes: 4096, MaxDatabaseBytes: 16 << 30, BusyTimeoutMilliseconds: 5000, MaxOpenConnections: 4, MaxIdleConnections: 4, MaxRouteSegmentBytes: 256, MaxAttemptsPerJob: 32, ReconcileLeaseDuration: 30 * time.Second, SourceLeaseDuration: 5 * time.Minute}
}

type PRKey struct{ InstallationID, BaseRepositoryID, PullRequestNumber int64 }
type ReconcileLease struct {
	Key                   PRKey
	Owner                 string
	Generation            int64
	AcquiredAt, ExpiresAt time.Time
}
type ProcessedDelivery struct {
	ReceiverID, OutboxID, DeliveryID string
	ProjectionKind                   githubapp.ProjectionKind
	ProjectionDigest                 string
	ProcessedAt                      time.Time
}
type PREligibility string

const (
	PREligibilityEligible            PREligibility = "eligible"
	PREligibilityDraft               PREligibility = "draft"
	PREligibilityClosed              PREligibility = "closed"
	PREligibilitySourceUnavailable   PREligibility = "source-unavailable"
	PREligibilityInstallationBlocked PREligibility = "installation-blocked"
	PREligibilityRouteUnavailable    PREligibility = "route-unavailable"
)

type ProcessingDecision string

const (
	DecisionScheduledNewTarget             ProcessingDecision = "scheduled-new-target"
	DecisionNoChangeCurrentTarget          ProcessingDecision = "no-change-current-target"
	DecisionMarkedDraft                    ProcessingDecision = "marked-draft"
	DecisionMarkedClosed                   ProcessingDecision = "marked-closed"
	DecisionSourceUnavailable              ProcessingDecision = "source-unavailable"
	DecisionInstallationBlocked            ProcessingDecision = "installation-blocked"
	DecisionRerequestCreated               ProcessingDecision = "rerequest-created"
	DecisionRerequestIgnored               ProcessingDecision = "rerequest-ignored"
	DecisionInstallationHintRecorded       ProcessingDecision = "installation-hint-recorded"
	DecisionDuplicateProcessed             ProcessingDecision = "duplicate-processed"
	DecisionLegacyProjectionUnreconcilable ProcessingDecision = "legacy-projection-unreconcilable"
)

type PullRequestReconcileInput struct {
	Processed ProcessedDelivery
	Lease     ReconcileLease
	Snapshot  githubapi.PullRequestSnapshot
	Now       time.Time
}
type ReconcileResult struct {
	Decision                                    ProcessingDecision
	Generation                                  int64
	TargetID, JobID, AttemptID, SourceRequestID string
	Eligibility                                 PREligibility
}
type CurrentPRState struct {
	Key                           PRKey
	Generation                    int64
	Eligibility                   PREligibility
	CurrentTargetID, CurrentJobID string
	BaseCommitID, HeadCommitID    string
	HeadRepositoryID              int64
	BaseOwner, BaseName           string
}
type RouteHint struct {
	RepositoryID int64  `json:"repositoryId"`
	Owner        string `json:"owner"`
	Name         string `json:"name"`
	CommitID     string `json:"commitId"`
}
type SourceRequestState string

const (
	SourceStatePending    SourceRequestState = "pending"
	SourceStateLeased     SourceRequestState = "leased"
	SourceStateCompleted  SourceRequestState = "completed"
	SourceStateFailed     SourceRequestState = "failed"
	SourceStateSuperseded SourceRequestState = "superseded"
	SourceStateCancelled  SourceRequestState = "cancelled"
)

type SourceImportRequest struct {
	SchemaVersion            string             `json:"schemaVersion"`
	ID                       string             `json:"requestId"`
	TargetID                 string             `json:"targetId"`
	JobID                    string             `json:"jobId"`
	Generation               int64              `json:"generation"`
	InstallationID           int64              `json:"installationId"`
	Base                     RouteHint          `json:"base"`
	Head                     RouteHint          `json:"head"`
	ControllerProfileVersion string             `json:"controllerProfileVersion"`
	State                    SourceRequestState `json:"state"`
	CreatedAt                time.Time          `json:"createdAt"`
	LeaseOwner               string             `json:"-"`
	LeaseGeneration          int64              `json:"-"`
	TokenCanaryForTest       string             `json:"-"`
}
type SourceImportResult struct {
	RequestID, TargetID, JobID         string
	Generation                         int64
	BaseRepositoryID, HeadRepositoryID int64
	BaseCommitID, HeadCommitID         string
	SourceStoreID                      string
	Failed                             bool
}
type CheckBinding struct {
	AppID, InstallationID, RepositoryID, PullRequestNumber, CheckRunID, ControllerGeneration, PublicationGeneration int64
	ExternalID, TargetID, JobID                                                                                     string
}
type WorkerFreshness string

const (
	WorkerFreshnessCurrentEligible   WorkerFreshness = "current-eligible"
	WorkerFreshnessStaleGeneration   WorkerFreshness = "stale-generation"
	WorkerFreshnessSupersededJob     WorkerFreshness = "superseded-job"
	WorkerFreshnessCancelledAttempt  WorkerFreshness = "cancelled-attempt"
	WorkerFreshnessUnknownAttempt    WorkerFreshness = "unknown-attempt"
	WorkerFreshnessUnexpectedResult  WorkerFreshness = "unexpected-result"
	WorkerFreshnessInvalidRunnerTier WorkerFreshness = "invalid-runner-tier"
)

type RerequestInput struct {
	Processed        ProcessedDelivery
	Lease            ReconcileLease
	Projection       githubapp.CheckRunProjection
	SnapshotEligible bool
	SnapshotTargetID string
	Now              time.Time
	ConfiguredAppID  int64
}

type InstallationHintInput struct {
	Processed  ProcessedDelivery
	Projection githubapp.InstallationProjection
	Now        time.Time
}
