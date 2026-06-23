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
	if len(claimed) != 1 || claimed[0].ID != created.SourceRequestID {
		t.Fatalf("bad claim: %#v", claimed)
	}
	if err := store.ApplySourceImportResult(ctx, validSourceImportResult(created, claimed[0]), "source-ingester-1", claimed[0].LeaseGeneration, now.Add(time.Second)); err != nil {
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

func TestApplySourceImportResultRejectsMalformedMetadataDigest(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	created := scheduleEligible(t, store, "outbox-bad-source-result", "7", "8")
	now := time.Date(2026, 6, 23, 12, 3, 0, 0, time.UTC)
	claimed, err := store.ClaimSourceImports(ctx, "source-ingester-1", now, 5*time.Minute, 1)
	if err != nil {
		t.Fatalf("ClaimSourceImports: %v", err)
	}
	result := validSourceImportResult(created, claimed[0])
	result.MetadataDigest = "sha256:ABC"
	if err := store.ApplySourceImportResult(ctx, result, "source-ingester-1", claimed[0].LeaseGeneration, now.Add(time.Second)); err == nil {
		t.Fatalf("malformed metadata digest accepted")
	}
}

func validSourceImportResult(created githubcontrollerstore.ReconcileResult, req githubcontrollerstore.SourceImportRequest) githubcontrollerstore.SourceImportResult {
	return githubcontrollerstore.SourceImportResult{
		RequestID:            created.SourceRequestID,
		TargetID:             created.TargetID,
		JobID:                created.JobID,
		Generation:           created.Generation,
		BaseRepositoryID:     req.Base.RepositoryID,
		HeadRepositoryID:     req.Head.RepositoryID,
		BaseCommitID:         req.Base.CommitID,
		HeadCommitID:         req.Head.CommitID,
		SourceStoreID:        "source-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		MetadataDigest:       "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		ImportProfileVersion: githubcontrollerstore.SourceImportProfileSmartHTTPShallowV1Alpha1,
		ObjectFormat:         "sha1",
		BaseTreeID:           repeat40("c"),
		HeadTreeID:           repeat40("d"),
		Limitations:          []string{"history outside selected shallow commits not imported", "tags not imported"},
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
