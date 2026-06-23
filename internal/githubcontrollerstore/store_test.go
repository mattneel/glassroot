package githubcontrollerstore_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/githubapi"
	"github.com/mattneel/glassroot/internal/githubapp"
	"github.com/mattneel/glassroot/internal/githubcontrollerstore"
)

func TestApplyEligibleSnapshotCreatesGenerationJobAttemptAndCredentialFreeSourceRequest(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	lease, err := store.AcquireReconcileLease(ctx, githubcontrollerstore.PRKey{InstallationID: 42, BaseRepositoryID: 101, PullRequestNumber: 7}, "controller-1", now, 30*time.Second)
	if err != nil {
		t.Fatalf("AcquireReconcileLease: %v", err)
	}

	result, err := store.ApplyPullRequestSnapshot(ctx, githubcontrollerstore.PullRequestReconcileInput{
		Processed: processed("outbox-1", "sha256:"+strings.Repeat("a", 64)),
		Lease:     lease,
		Snapshot:  eligibleSnapshot(strings.Repeat("1", 40), strings.Repeat("2", 40)),
		Now:       now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("ApplyPullRequestSnapshot: %v", err)
	}
	if result.Decision != githubcontrollerstore.DecisionScheduledNewTarget || result.Generation != 1 || result.TargetID == "" || result.JobID == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
	req, err := store.GetSourceImportRequest(ctx, result.SourceRequestID)
	if err != nil {
		t.Fatalf("GetSourceImportRequest: %v", err)
	}
	if req.TargetID != result.TargetID || req.JobID != result.JobID || req.Generation != 1 || req.InstallationID != 42 {
		t.Fatalf("bad source request: %#v", req)
	}
	if req.TokenCanaryForTest != "" || strings.Contains(req.Base.Owner, "token") || strings.Contains(req.Head.Owner, "token") {
		t.Fatalf("credential-like field persisted: %#v", req)
	}
	current, err := store.GetCurrentPRState(ctx, githubcontrollerstore.PRKey{InstallationID: 42, BaseRepositoryID: 101, PullRequestNumber: 7})
	if err != nil {
		t.Fatalf("GetCurrentPRState: %v", err)
	}
	if current.Generation != 1 || current.Eligibility != githubcontrollerstore.PREligibilityEligible || current.CurrentTargetID != result.TargetID || current.CurrentJobID != result.JobID {
		t.Fatalf("bad current state: %#v", current)
	}
}

func TestOpenRejectsUnsafeExistingDatabaseFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := dir + string(os.PathSeparator) + "github-controller.sqlite"
	if err := os.WriteFile(dbPath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dbPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := githubcontrollerstore.Open(context.Background(), githubcontrollerstore.Config{StateDir: dir, ControllerID: "controller-1", ReceiverID: "receiver-1", AppID: 123}); err == nil {
		t.Fatalf("existing database with wrong mode accepted")
	}
}

func TestOpenRejectsUnsafeControllerStateDir(t *testing.T) {
	ctx := context.Background()
	rel := "relative-controller-state"
	if _, err := githubcontrollerstore.Open(ctx, githubcontrollerstore.Config{StateDir: rel, ControllerID: "controller-1", ReceiverID: "receiver-1", AppID: 123}); err == nil {
		t.Fatalf("relative state dir accepted")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := githubcontrollerstore.Open(ctx, githubcontrollerstore.Config{StateDir: dir, ControllerID: "controller-1", ReceiverID: "receiver-1", AppID: 123}); err == nil {
		t.Fatalf("world-readable state dir accepted")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := githubcontrollerstore.Open(ctx, githubcontrollerstore.Config{StateDir: dir, ControllerID: "controller-1", ReceiverID: "receiver-1", AppID: 123})
	if err != nil {
		t.Fatalf("open corrected state dir: %v", err)
	}
	_ = store.Close()
}

func openTestStore(t *testing.T) *githubcontrollerstore.Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := githubcontrollerstore.Open(context.Background(), githubcontrollerstore.Config{StateDir: dir, ControllerID: "controller-1", ReceiverID: "receiver-1", AppID: 123})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return store
}

func processed(id, digest string) githubcontrollerstore.ProcessedDelivery {
	return githubcontrollerstore.ProcessedDelivery{ReceiverID: "receiver-1", OutboxID: id, DeliveryID: "123e4567-e89b-12d3-a456-426614174000", ProjectionKind: githubapp.ProjectionPullRequest, ProjectionDigest: digest, ProcessedAt: time.Date(2026, 6, 23, 12, 0, 1, 0, time.UTC)}
}

func eligibleSnapshot(base, head string) githubapi.PullRequestSnapshot {
	return githubapi.PullRequestSnapshot{SchemaVersion: githubapi.PullRequestSnapshotSchemaV1Alpha1, Number: 7, State: githubapi.PullRequestStateOpen, Base: githubapi.PullRequestEndpoint{RepositoryID: 101, Owner: "owner", Name: "repo", CommitID: base, Available: true}, Head: githubapi.PullRequestEndpoint{RepositoryID: 202, Owner: "headowner", Name: "headrepo", CommitID: head, Available: true}}
}
