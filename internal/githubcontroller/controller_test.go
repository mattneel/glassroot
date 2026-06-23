package githubcontroller_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/githubapi"
	"github.com/mattneel/glassroot/internal/githubapp"
	"github.com/mattneel/glassroot/internal/githubbroker"
	"github.com/mattneel/glassroot/internal/githubcontroller"
	"github.com/mattneel/glassroot/internal/githubcontrollerstore"
	"github.com/mattneel/glassroot/internal/githubinbox"
)

func TestControllerProcessesPullRequestUsingBrokerTokenAndAPISnapshot(t *testing.T) {
	ctx := context.Background()
	store := openControllerStore(t)
	defer store.Close()
	broker := &fakeBroker{token: githubbroker.NewTokenLeaseForTest(githubbroker.TokenMetadata{Purpose: githubbroker.PurposePullRequestRead, InstallationID: 42, RepositoryID: 101, ExpiresAt: time.Date(2026, 6, 23, 13, 0, 0, 0, time.UTC)}, []byte("pr-token"))}
	api := &fakePRAPI{snapshot: githubapi.PullRequestSnapshot{SchemaVersion: githubapi.PullRequestSnapshotSchemaV1Alpha1, Number: 7, State: githubapi.PullRequestStateOpen, Base: githubapi.PullRequestEndpoint{RepositoryID: 101, Owner: "owner", Name: "repo", CommitID: strings.Repeat("1", 40), Available: true}, Head: githubapi.PullRequestEndpoint{RepositoryID: 202, Owner: "head", Name: "headrepo", CommitID: strings.Repeat("2", 40), Available: true}}}
	c, err := githubcontroller.New(githubcontroller.Config{ControllerID: "controller-1", Store: store, Broker: broker, PullRequests: api, Clock: fixedClock{time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rec := githubinbox.LeasedRecord{ID: "outbox-1", ReceiverID: "receiver-1", DeliveryID: "123e4567-e89b-12d3-a456-426614174000", ProjectionKind: githubapp.ProjectionPullRequest, ProjectionDigest: "sha256:" + strings.Repeat("a", 64), Projection: githubapp.WebhookProjection{Kind: githubapp.ProjectionPullRequest, PullRequest: &githubapp.PullRequestProjection{Action: "synchronize", InstallationID: 42, RepositoryID: 101, BaseRepositoryOwner: "owner", BaseRepositoryName: "repo", PullRequestNumber: 7, BaseSHA: strings.Repeat("0", 40), HeadSHA: strings.Repeat("9", 40)}}}
	res, err := c.ProcessRecord(ctx, rec)
	if err != nil {
		t.Fatalf("ProcessRecord: %v", err)
	}
	if res.Decision != githubcontrollerstore.DecisionScheduledNewTarget || res.Generation != 1 {
		t.Fatalf("bad result: %#v", res)
	}
	if broker.request.Purpose != githubbroker.PurposePullRequestRead || broker.request.RepositoryID != 101 || broker.request.InstallationID != 42 {
		t.Fatalf("bad token request: %#v", broker.request)
	}
	if api.route.Owner != "owner" || api.route.Repo != "repo" || api.number != 7 || !api.usedToken {
		t.Fatalf("bad API call: %#v number=%d used=%v", api.route, api.number, api.usedToken)
	}
}

func TestControllerTreatsLegacyPullRequestProjectionAsNonExecutionDecision(t *testing.T) {
	ctx := context.Background()
	store := openControllerStore(t)
	defer store.Close()
	broker := &fakeBroker{token: githubbroker.NewTokenLeaseForTest(githubbroker.TokenMetadata{Purpose: githubbroker.PurposePullRequestRead, InstallationID: 42, RepositoryID: 101, ExpiresAt: time.Date(2026, 6, 23, 13, 0, 0, 0, time.UTC)}, []byte("pr-token"))}
	api := &fakePRAPI{}
	c, err := githubcontroller.New(githubcontroller.Config{ControllerID: "controller-1", Store: store, Broker: broker, PullRequests: api, Clock: fixedClock{time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rec := githubinbox.LeasedRecord{ID: "outbox-legacy", ReceiverID: "receiver-1", DeliveryID: "123e4567-e89b-12d3-a456-426614174010", ProjectionKind: githubapp.ProjectionPullRequest, ProjectionDigest: "sha256:" + strings.Repeat("e", 64), Projection: githubapp.WebhookProjection{Kind: githubapp.ProjectionPullRequest, PullRequest: &githubapp.PullRequestProjection{Action: "synchronize", InstallationID: 42, RepositoryID: 101, PullRequestNumber: 7, BaseSHA: strings.Repeat("1", 40), HeadSHA: strings.Repeat("2", 40)}}}
	res, err := c.ProcessRecord(ctx, rec)
	if err != nil {
		t.Fatalf("ProcessRecord: %v", err)
	}
	if res.Decision != githubcontrollerstore.DecisionLegacyProjectionUnreconcilable {
		t.Fatalf("bad result: %#v", res)
	}
	if broker.calls != 0 || api.calls != 0 {
		t.Fatalf("legacy projection called broker/api: broker=%d api=%d", broker.calls, api.calls)
	}
}

func TestControllerRerequestRevalidatesCurrentPullRequestBeforeAttempt(t *testing.T) {
	ctx := context.Background()
	store := openControllerStore(t)
	defer store.Close()
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	lease, err := store.AcquireReconcileLease(ctx, githubcontrollerstore.PRKey{InstallationID: 42, BaseRepositoryID: 101, PullRequestNumber: 7}, "controller-1", now, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	scheduled, err := store.ApplyPullRequestSnapshot(ctx, githubcontrollerstore.PullRequestReconcileInput{
		Processed: githubcontrollerstore.ProcessedDelivery{ReceiverID: "receiver-1", OutboxID: "outbox-seed", DeliveryID: "123e4567-e89b-12d3-a456-426614174000", ProjectionKind: githubapp.ProjectionPullRequest, ProjectionDigest: "sha256:" + strings.Repeat("b", 64), ProcessedAt: now.Add(time.Second)},
		Lease:     lease,
		Snapshot:  githubapi.PullRequestSnapshot{SchemaVersion: githubapi.PullRequestSnapshotSchemaV1Alpha1, Number: 7, State: githubapi.PullRequestStateOpen, Base: githubapi.PullRequestEndpoint{RepositoryID: 101, Owner: "owner", Name: "repo", CommitID: strings.Repeat("1", 40), Available: true}, Head: githubapi.PullRequestEndpoint{RepositoryID: 202, Owner: "head", Name: "headrepo", CommitID: strings.Repeat("2", 40), Available: true}},
		Now:       now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	binding := githubcontrollerstore.CheckBinding{AppID: 123, InstallationID: 42, RepositoryID: 101, PullRequestNumber: 7, CheckRunID: 9001, ExternalID: "gr-" + strings.Repeat("c", 64), TargetID: scheduled.TargetID, JobID: scheduled.JobID, ControllerGeneration: scheduled.Generation, PublicationGeneration: 1}
	if err := store.RegisterCheckBinding(ctx, binding); err != nil {
		t.Fatal(err)
	}
	broker := &fakeBroker{token: githubbroker.NewTokenLeaseForTest(githubbroker.TokenMetadata{Purpose: githubbroker.PurposePullRequestRead, InstallationID: 42, RepositoryID: 101, ExpiresAt: now.Add(time.Hour)}, []byte("pr-token"))}
	api := &fakePRAPI{snapshot: githubapi.PullRequestSnapshot{SchemaVersion: githubapi.PullRequestSnapshotSchemaV1Alpha1, Number: 7, State: githubapi.PullRequestStateOpen, Base: githubapi.PullRequestEndpoint{RepositoryID: 101, Owner: "owner", Name: "repo", CommitID: strings.Repeat("1", 40), Available: true}, Head: githubapi.PullRequestEndpoint{RepositoryID: 202, Owner: "head", Name: "headrepo", CommitID: strings.Repeat("2", 40), Available: true}}}
	c, err := githubcontroller.New(githubcontroller.Config{ControllerID: "controller-1", Store: store, Broker: broker, PullRequests: api, Clock: fixedClock{now.Add(2 * time.Second)}, AppID: 123})
	if err != nil {
		t.Fatal(err)
	}
	rec := githubinbox.LeasedRecord{ID: "outbox-rerequest", ReceiverID: "receiver-1", DeliveryID: "123e4567-e89b-12d3-a456-426614174001", ProjectionKind: githubapp.ProjectionCheckRun, ProjectionDigest: "sha256:" + strings.Repeat("d", 64), Projection: githubapp.WebhookProjection{Kind: githubapp.ProjectionCheckRun, CheckRun: &githubapp.CheckRunProjection{Action: "rerequested", InstallationID: 42, RepositoryID: 101, CheckRunID: 9001, AppID: 123, ExternalID: binding.ExternalID, HeadSHA: strings.Repeat("2", 40)}}}
	res, err := c.ProcessRecord(ctx, rec)
	if err != nil {
		t.Fatalf("ProcessRecord rerequest: %v", err)
	}
	if res.Decision != githubcontrollerstore.DecisionRerequestCreated {
		t.Fatalf("bad result: %#v", res)
	}
	if broker.request.Purpose != githubbroker.PurposePullRequestRead || broker.request.RepositoryID != 101 {
		t.Fatalf("rerequest did not request narrow PR token: %#v", broker.request)
	}
	if api.number != 7 || api.route.Owner != "owner" || api.route.Repo != "repo" || !api.usedToken {
		t.Fatalf("rerequest did not re-read current PR: route=%#v number=%d used=%v", api.route, api.number, api.usedToken)
	}
}

func TestGitHubControllerIntegration(t *testing.T) {
	if os.Getenv("GLASSROOT_GITHUB_CONTROLLER_INTEGRATION") != "1" {
		t.Skip("GLASSROOT_GITHUB_CONTROLLER_INTEGRATION=1 not set; live GitHub controller integration skipped")
	}
	for _, name := range []string{
		"GLASSROOT_GITHUB_CONTROLLER_BROKER_SOCKET",
		"GLASSROOT_GITHUB_CONTROLLER_INSTALLATION_ID",
		"GLASSROOT_GITHUB_CONTROLLER_BASE_REPOSITORY_ID",
		"GLASSROOT_GITHUB_CONTROLLER_REPOSITORY_OWNER",
		"GLASSROOT_GITHUB_CONTROLLER_REPOSITORY_NAME",
		"GLASSROOT_GITHUB_CONTROLLER_PULL_NUMBER",
		"GLASSROOT_GITHUB_CONTROLLER_EXPECTED_BASE_SHA",
		"GLASSROOT_GITHUB_CONTROLLER_EXPECTED_HEAD_SHA",
	} {
		if os.Getenv(name) == "" {
			t.Skip(name + " not set; live GitHub controller integration skipped")
		}
	}
	parsePositive := func(name string) int64 {
		t.Helper()
		v, err := strconv.ParseInt(os.Getenv(name), 10, 64)
		if err != nil || v <= 0 {
			t.Skip(name + " invalid; live GitHub controller integration skipped")
		}
		return v
	}
	ctx := context.Background()
	inboxDir := t.TempDir()
	controllerDir := t.TempDir()
	if err := os.Chmod(inboxDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(controllerDir, 0o700); err != nil {
		t.Fatal(err)
	}
	receiverID := "receiver-1"
	controllerID := "controller-1"
	installationID := parsePositive("GLASSROOT_GITHUB_CONTROLLER_INSTALLATION_ID")
	baseRepoID := parsePositive("GLASSROOT_GITHUB_CONTROLLER_BASE_REPOSITORY_ID")
	prNumber := parsePositive("GLASSROOT_GITHUB_CONTROLLER_PULL_NUMBER")
	owner := os.Getenv("GLASSROOT_GITHUB_CONTROLLER_REPOSITORY_OWNER")
	repo := os.Getenv("GLASSROOT_GITHUB_CONTROLLER_REPOSITORY_NAME")
	expectedBase := os.Getenv("GLASSROOT_GITHUB_CONTROLLER_EXPECTED_BASE_SHA")
	expectedHead := os.Getenv("GLASSROOT_GITHUB_CONTROLLER_EXPECTED_HEAD_SHA")
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)

	inbox, err := githubinbox.Open(ctx, githubinbox.Config{StateDir: inboxDir, ReceiverID: receiverID})
	if err != nil {
		t.Fatal(err)
	}
	defer inbox.Close()
	store, err := githubcontrollerstore.Open(ctx, githubcontrollerstore.Config{StateDir: controllerDir, ControllerID: controllerID, ReceiverID: receiverID, AppID: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	projection := githubapp.WebhookProjection{Kind: githubapp.ProjectionPullRequest, PullRequest: &githubapp.PullRequestProjection{Action: "synchronize", InstallationID: installationID, RepositoryID: baseRepoID, BaseRepositoryOwner: owner, BaseRepositoryName: repo, PullRequestNumber: prNumber, BaseSHA: expectedBase, HeadSHA: expectedHead}}
	bodyDigest := githubapp.DigestRawBody([]byte("gr-15b2-live-controller-fixture"))
	deliveryID := "123e4567-e89b-12d3-a456-426614174999"
	_, err = inbox.Accept(ctx, githubinbox.VerifiedDelivery{ReceiverID: receiverID, DeliveryID: deliveryID, Event: "pull_request", Action: "synchronize", BodyDigest: bodyDigest, MatchedSecret: githubapp.SecretGenerationCurrent, ReceivedAt: now, Projection: projection, Receipt: githubapp.DeliveryReceipt{SchemaVersion: githubapp.SchemaGitHubWebhookReceiptV1Alpha1, ReceiverID: receiverID, DeliveryID: deliveryID, Event: "pull_request", BodyDigest: bodyDigest, MatchedSecret: githubapp.SecretGenerationCurrent, ReceivedAt: now, ProjectionKind: githubapp.ProjectionPullRequest, Disposition: githubapp.DeliveryDispositionEnqueued}, Disposition: githubapp.DeliveryDispositionEnqueued, IntakeFingerprint: githubinbox.ComputeIntakeFingerprint(receiverID, deliveryID, "pull_request", bodyDigest, string(githubapp.ProjectionPullRequest))})
	if err != nil {
		t.Fatal(err)
	}
	broker, err := githubbroker.Dial(ctx, os.Getenv("GLASSROOT_GITHUB_CONTROLLER_BROKER_SOCKET"), githubbroker.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	api, err := githubapi.NewInstallationClient(githubapi.InstallationClientConfig{Limits: githubapi.DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	defer api.CloseIdleConnections()
	ctrl, err := githubcontroller.New(githubcontroller.Config{ControllerID: controllerID, Store: store, Broker: broker, PullRequests: api, Clock: fixedClock{now.Add(time.Second)}})
	if err != nil {
		t.Fatal(err)
	}
	processed, res, err := ctrl.ProcessNext(ctx, inbox)
	if err != nil {
		t.Fatal(err)
	}
	if !processed || res.Decision != githubcontrollerstore.DecisionScheduledNewTarget {
		t.Fatalf("processed=%v result=%#v", processed, res)
	}
	reqs, err := store.ClaimSourceImports(ctx, "source-ingester", now.Add(2*time.Second), time.Minute, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 1 || reqs[0].Base.CommitID != expectedBase || reqs[0].Head.CommitID != expectedHead {
		t.Fatalf("bad source request: %#v", reqs)
	}
}

type fakeBroker struct {
	token   *githubbroker.TokenLease
	request githubbroker.TokenRequest
	calls   int
}

func (f *fakeBroker) RequestToken(ctx context.Context, req githubbroker.TokenRequest) (*githubbroker.TokenLease, error) {
	f.calls++
	f.request = req
	return f.token, nil
}

type fakePRAPI struct {
	snapshot  githubapi.PullRequestSnapshot
	route     githubapi.RepositoryRoute
	number    int64
	usedToken bool
	calls     int
}

func (f *fakePRAPI) GetPullRequest(ctx context.Context, token githubapi.TokenUser, route githubapi.RepositoryRoute, number int64) (githubapi.PullRequestSnapshot, error) {
	f.calls++
	f.route = route
	f.number = number
	_ = token.Use(func([]byte) error { f.usedToken = true; return nil })
	return f.snapshot, nil
}

type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }
func openControllerStore(t *testing.T) *githubcontrollerstore.Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	s, err := githubcontrollerstore.Open(context.Background(), githubcontrollerstore.Config{StateDir: dir, ControllerID: "controller-1", ReceiverID: "receiver-1", AppID: 123})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
