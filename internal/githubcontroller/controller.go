package githubcontroller

import (
	"context"
	"time"

	"github.com/mattneel/glassroot/internal/githubapi"
	"github.com/mattneel/glassroot/internal/githubapp"
	"github.com/mattneel/glassroot/internal/githubbroker"
	"github.com/mattneel/glassroot/internal/githubcontrollerstore"
	"github.com/mattneel/glassroot/internal/githubinbox"
)

type Clock interface{ Now() time.Time }
type Broker interface {
	RequestToken(context.Context, githubbroker.TokenRequest) (*githubbroker.TokenLease, error)
}
type PullRequestReader interface {
	GetPullRequest(context.Context, githubapi.TokenUser, githubapi.RepositoryRoute, int64) (githubapi.PullRequestSnapshot, error)
}
type Store interface {
	AcquireReconcileLease(context.Context, githubcontrollerstore.PRKey, string, time.Time, time.Duration) (githubcontrollerstore.ReconcileLease, error)
	ApplyPullRequestSnapshot(context.Context, githubcontrollerstore.PullRequestReconcileInput) (githubcontrollerstore.ReconcileResult, error)
	ApplyCheckRunRerequest(context.Context, githubcontrollerstore.RerequestInput) (githubcontrollerstore.ReconcileResult, error)
	RecordProcessingDecision(context.Context, githubcontrollerstore.ProcessedDelivery, githubcontrollerstore.ProcessingDecision, time.Time) (githubcontrollerstore.ReconcileResult, error)
	LookupCheckBinding(context.Context, int64, int64, int64) (githubcontrollerstore.CheckBinding, error)
	GetCurrentPRState(context.Context, githubcontrollerstore.PRKey) (githubcontrollerstore.CurrentPRState, error)
	ApplyInstallationHint(context.Context, githubcontrollerstore.InstallationHintInput) (githubcontrollerstore.ReconcileResult, error)
}

type Config struct {
	ControllerID string
	Store        Store
	Broker       Broker
	PullRequests PullRequestReader
	Clock        Clock
	Limits       Limits
	AppID        int64
}
type Limits struct {
	InboxLeaseDuration        time.Duration
	APIRequestTimeout         time.Duration
	DeliveryProcessingTimeout time.Duration
}

func DefaultLimits() Limits {
	return Limits{InboxLeaseDuration: 30 * time.Second, APIRequestTimeout: 10 * time.Second, DeliveryProcessingTimeout: 25 * time.Second}
}

type Controller struct {
	id     string
	store  Store
	broker Broker
	prs    PullRequestReader
	clock  Clock
	limits Limits
	appID  int64
}

func New(cfg Config) (*Controller, error) {
	limits := cfg.Limits
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if cfg.ControllerID == "" || cfg.Store == nil || cfg.Broker == nil || cfg.PullRequests == nil || cfg.Clock == nil {
		return nil, errCode(CodeInvalidConfig, "config", "controller configuration rejected", nil)
	}
	return &Controller{id: cfg.ControllerID, store: cfg.Store, broker: cfg.Broker, prs: cfg.PullRequests, clock: cfg.Clock, limits: limits, appID: cfg.AppID}, nil
}
func (c *Controller) ProcessRecord(ctx context.Context, rec githubinbox.LeasedRecord) (githubcontrollerstore.ReconcileResult, error) {
	if c == nil {
		return githubcontrollerstore.ReconcileResult{}, errCode(CodeInvalidConfig, "process", "controller unavailable", nil)
	}
	ctx, cancel := context.WithTimeout(ctx, c.limits.DeliveryProcessingTimeout)
	defer cancel()
	switch rec.ProjectionKind {
	case githubapp.ProjectionPullRequest:
		return c.processPullRequest(ctx, rec)
	case githubapp.ProjectionCheckRun:
		return c.processRerequest(ctx, rec)
	case githubapp.ProjectionInstallation, githubapp.ProjectionInstallationRepositories:
		return c.processInstallation(ctx, rec)
	default:
		return c.recordDecision(ctx, rec, githubcontrollerstore.DecisionLegacyProjectionUnreconcilable)
	}
}

func (c *Controller) processInstallation(ctx context.Context, rec githubinbox.LeasedRecord) (githubcontrollerstore.ReconcileResult, error) {
	p := rec.Projection.Installation
	if p == nil {
		return githubcontrollerstore.ReconcileResult{}, errCode(CodeInvalidProjection, "projection", "installation projection missing", nil)
	}
	processed := githubcontrollerstore.ProcessedDelivery{ReceiverID: rec.ReceiverID, OutboxID: rec.ID, DeliveryID: rec.DeliveryID, ProjectionKind: rec.ProjectionKind, ProjectionDigest: rec.ProjectionDigest, ProcessedAt: c.clock.Now().UTC().Round(0)}
	return c.store.ApplyInstallationHint(ctx, githubcontrollerstore.InstallationHintInput{Processed: processed, Projection: *p, Now: c.clock.Now().UTC().Round(0)})
}
func (c *Controller) processPullRequest(ctx context.Context, rec githubinbox.LeasedRecord) (githubcontrollerstore.ReconcileResult, error) {
	p := rec.Projection.PullRequest
	if p == nil {
		return githubcontrollerstore.ReconcileResult{}, errCode(CodeInvalidProjection, "projection", "pull request projection missing", nil)
	}
	if p.BaseRepositoryOwner == "" || p.BaseRepositoryName == "" {
		return c.recordDecision(ctx, rec, githubcontrollerstore.DecisionLegacyProjectionUnreconcilable)
	}
	key := githubcontrollerstore.PRKey{InstallationID: p.InstallationID, BaseRepositoryID: p.RepositoryID, PullRequestNumber: p.PullRequestNumber}
	now := c.clock.Now().UTC().Round(0)
	lease, err := c.store.AcquireReconcileLease(ctx, key, c.id, now, c.limits.InboxLeaseDuration)
	if err != nil {
		return githubcontrollerstore.ReconcileResult{}, errCode(CodeReconcileLeaseBusy, "lease", "reconcile lease unavailable", err)
	}
	token, err := c.broker.RequestToken(ctx, githubbroker.TokenRequest{SchemaVersion: githubbroker.SchemaTokenRequestV1Alpha1, Purpose: githubbroker.PurposePullRequestRead, InstallationID: p.InstallationID, RepositoryID: p.RepositoryID})
	if err != nil {
		return githubcontrollerstore.ReconcileResult{}, errCode(CodeTokenRequestFailed, "broker", "pull-request-read token failed", err)
	}
	defer token.Close()
	snap, err := c.prs.GetPullRequest(ctx, token, githubapi.RepositoryRoute{Owner: p.BaseRepositoryOwner, Repo: p.BaseRepositoryName, RepositoryID: p.RepositoryID}, p.PullRequestNumber)
	if err != nil {
		return githubcontrollerstore.ReconcileResult{}, errCode(CodePullRequestReadFailed, "api", "pull request read failed", err)
	}
	processed := githubcontrollerstore.ProcessedDelivery{ReceiverID: rec.ReceiverID, OutboxID: rec.ID, DeliveryID: rec.DeliveryID, ProjectionKind: rec.ProjectionKind, ProjectionDigest: rec.ProjectionDigest, ProcessedAt: c.clock.Now().UTC().Round(0)}
	return c.store.ApplyPullRequestSnapshot(ctx, githubcontrollerstore.PullRequestReconcileInput{Processed: processed, Lease: lease, Snapshot: snap, Now: c.clock.Now().UTC().Round(0)})
}
func (c *Controller) processRerequest(ctx context.Context, rec githubinbox.LeasedRecord) (githubcontrollerstore.ReconcileResult, error) {
	p := rec.Projection.CheckRun
	if p == nil {
		return githubcontrollerstore.ReconcileResult{}, errCode(CodeInvalidProjection, "projection", "check_run projection missing", nil)
	}
	binding, err := c.store.LookupCheckBinding(ctx, c.appID, p.RepositoryID, p.CheckRunID)
	if err != nil {
		return c.recordDecision(ctx, rec, githubcontrollerstore.DecisionRerequestIgnored)
	}
	key := githubcontrollerstore.PRKey{InstallationID: binding.InstallationID, BaseRepositoryID: binding.RepositoryID, PullRequestNumber: binding.PullRequestNumber}
	current, err := c.store.GetCurrentPRState(ctx, key)
	if err != nil || current.BaseOwner == "" || current.BaseName == "" {
		return c.recordDecision(ctx, rec, githubcontrollerstore.DecisionRerequestIgnored)
	}
	now := c.clock.Now().UTC().Round(0)
	lease, err := c.store.AcquireReconcileLease(ctx, key, c.id, now, c.limits.InboxLeaseDuration)
	if err != nil {
		return githubcontrollerstore.ReconcileResult{}, errCode(CodeReconcileLeaseBusy, "lease", "reconcile lease unavailable", err)
	}
	token, err := c.broker.RequestToken(ctx, githubbroker.TokenRequest{SchemaVersion: githubbroker.SchemaTokenRequestV1Alpha1, Purpose: githubbroker.PurposePullRequestRead, InstallationID: binding.InstallationID, RepositoryID: binding.RepositoryID})
	if err != nil {
		return githubcontrollerstore.ReconcileResult{}, errCode(CodeTokenRequestFailed, "broker", "pull-request-read token failed", err)
	}
	defer token.Close()
	snap, err := c.prs.GetPullRequest(ctx, token, githubapi.RepositoryRoute{Owner: current.BaseOwner, Repo: current.BaseName, RepositoryID: binding.RepositoryID}, binding.PullRequestNumber)
	if err != nil {
		return githubcontrollerstore.ReconcileResult{}, errCode(CodePullRequestReadFailed, "api", "pull request read failed", err)
	}
	snapshotTargetID, snapshotEligible := rerequestSnapshotTarget(snap, key)
	processed := githubcontrollerstore.ProcessedDelivery{ReceiverID: rec.ReceiverID, OutboxID: rec.ID, DeliveryID: rec.DeliveryID, ProjectionKind: rec.ProjectionKind, ProjectionDigest: rec.ProjectionDigest, ProcessedAt: now}
	return c.store.ApplyCheckRunRerequest(ctx, githubcontrollerstore.RerequestInput{Processed: processed, Lease: lease, Projection: *p, SnapshotEligible: snapshotEligible, SnapshotTargetID: snapshotTargetID, Now: now, ConfiguredAppID: c.appID})
}

func rerequestSnapshotTarget(s githubapi.PullRequestSnapshot, key githubcontrollerstore.PRKey) (string, bool) {
	if s.Number != key.PullRequestNumber || s.Base.RepositoryID != key.BaseRepositoryID || s.State != githubapi.PullRequestStateOpen || s.Draft || !s.Head.Available || s.Head.RepositoryID <= 0 || s.Base.CommitID == "" || s.Head.CommitID == "" {
		return "", false
	}
	target := githubapp.AnalysisTarget{SchemaVersion: githubapp.SchemaGitHubAnalysisTargetV1Alpha1, InstallationID: key.InstallationID, BaseRepositoryID: s.Base.RepositoryID, HeadRepositoryID: s.Head.RepositoryID, PullRequestNumber: s.Number, BaseCommitID: s.Base.CommitID, HeadCommitID: s.Head.CommitID, AnalysisProfileVersion: githubcontrollerstore.ControllerProfileAdvisoryV1Alpha1}
	id, err := target.ID()
	if err != nil {
		return "", false
	}
	return id, true
}

func (c *Controller) ProcessNext(ctx context.Context, inbox *githubinbox.Store) (bool, githubcontrollerstore.ReconcileResult, error) {
	now := c.clock.Now().UTC().Round(0)
	recs, err := inbox.ClaimOutbox(ctx, githubinbox.LeaseOwner(c.id), now, c.limits.InboxLeaseDuration, 1)
	if err != nil {
		return false, githubcontrollerstore.ReconcileResult{}, errCode(CodeInboxClaimFailed, "inbox", "inbox claim failed", err)
	}
	if len(recs) == 0 {
		return false, githubcontrollerstore.ReconcileResult{}, nil
	}
	rec := recs[0]
	res, err := c.ProcessRecord(ctx, rec)
	if err != nil {
		_ = inbox.ReleaseOutbox(ctx, rec.ID, githubinbox.LeaseOwner(c.id), rec.LeaseGeneration, c.clock.Now().UTC().Round(0), string(Diagnostic(err)))
		return true, res, err
	}
	if err := inbox.AcknowledgeOutbox(ctx, rec.ID, githubinbox.LeaseOwner(c.id), rec.LeaseGeneration, c.clock.Now().UTC().Round(0)); err != nil {
		return true, res, errCode(CodeInboxAckFailed, "inbox", "inbox ack failed", err)
	}
	return true, res, nil
}

func (c *Controller) recordDecision(ctx context.Context, rec githubinbox.LeasedRecord, decision githubcontrollerstore.ProcessingDecision) (githubcontrollerstore.ReconcileResult, error) {
	p := githubcontrollerstore.ProcessedDelivery{ReceiverID: rec.ReceiverID, OutboxID: rec.ID, DeliveryID: rec.DeliveryID, ProjectionKind: rec.ProjectionKind, ProjectionDigest: rec.ProjectionDigest, ProcessedAt: c.clock.Now().UTC().Round(0)}
	return c.store.RecordProcessingDecision(ctx, p, decision, c.clock.Now().UTC().Round(0))
}
