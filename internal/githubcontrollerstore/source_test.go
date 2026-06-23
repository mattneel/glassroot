package githubcontrollerstore_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/githubcontrollerstore"
)

func TestSourceImportLeaseAndCurrentResultTransitionsJobAwaitingRunner(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	created := scheduleEligible(t, store, "outbox-source", "3", "4")
	now := time.Date(2026, 6, 23, 12, 2, 0, 0, time.UTC)
	claimed, err := store.ClaimSourceImports(ctx, "source-ingester-1", now, 5*time.Minute, 1)
	if err != nil {
		t.Fatalf("ClaimSourceImports: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != created.SourceRequestID || claimed[0].TokenCanaryForTest != "" {
		t.Fatalf("bad claim: %#v", claimed)
	}
	if err := store.ApplySourceImportResult(ctx, githubcontrollerstore.SourceImportResult{RequestID: created.SourceRequestID, TargetID: created.TargetID, JobID: created.JobID, Generation: created.Generation, BaseRepositoryID: 101, HeadRepositoryID: 202, BaseCommitID: claimed[0].Base.CommitID, HeadCommitID: claimed[0].Head.CommitID, SourceStoreID: "store-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, "source-ingester-1", claimed[0].LeaseGeneration, now.Add(time.Second)); err != nil {
		t.Fatalf("ApplySourceImportResult: %v", err)
	}
	state, err := store.GetJobState(ctx, created.JobID)
	if err != nil {
		t.Fatalf("GetJobState: %v", err)
	}
	if state != "awaiting-runner" {
		t.Fatalf("job state = %q", state)
	}
}

func scheduleEligible(t *testing.T, store *githubcontrollerstore.Store, outboxID, baseHex, headHex string) githubcontrollerstore.ReconcileResult {
	t.Helper()
	return scheduleEligibleFor(t, store, outboxID, githubcontrollerstore.PRKey{InstallationID: 42, BaseRepositoryID: 101, PullRequestNumber: 7}, baseHex, headHex)
}

func scheduleEligibleFor(t *testing.T, store *githubcontrollerstore.Store, outboxID string, key githubcontrollerstore.PRKey, baseHex, headHex string) githubcontrollerstore.ReconcileResult {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	lease, err := store.AcquireReconcileLease(ctx, key, "controller-1", now, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	snap := eligibleSnapshot(repeat40(baseHex), repeat40(headHex))
	snap.Number = key.PullRequestNumber
	snap.Base.RepositoryID = key.BaseRepositoryID
	snap.Base.Owner = "owner"
	snap.Base.Name = "repo"
	if key.BaseRepositoryID != 101 {
		snap.Base.Owner = "owner2"
		snap.Base.Name = "repo2"
	}
	res, err := store.ApplyPullRequestSnapshot(ctx, githubcontrollerstore.PullRequestReconcileInput{Processed: processed(outboxID, "sha256:"+strings.Repeat(baseHex, 64)), Lease: lease, Snapshot: snap, Now: now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func repeat40(s string) string {
	out := ""
	for len(out) < 40 {
		out += s
	}
	return out[:40]
}
