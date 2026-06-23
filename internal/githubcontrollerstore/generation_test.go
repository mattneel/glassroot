package githubcontrollerstore_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/githubapp"
	"github.com/mattneel/glassroot/internal/githubcontrollerstore"
)

func TestDuplicateProcessedOutboxIsIdempotentAndReleasesLease(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	first := scheduleEligible(t, store, "outbox-dupe", "5", "6")
	now := time.Date(2026, 6, 23, 12, 4, 0, 0, time.UTC)
	lease, err := store.AcquireReconcileLease(ctx, githubcontrollerstore.PRKey{InstallationID: 42, BaseRepositoryID: 101, PullRequestNumber: 7}, "controller-1", now, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	res, err := store.ApplyPullRequestSnapshot(ctx, githubcontrollerstore.PullRequestReconcileInput{Processed: processed("outbox-dupe", "sha256:"+strings.Repeat("5", 64)), Lease: lease, Snapshot: eligibleSnapshot(repeat40("5"), repeat40("6")), Now: now.Add(time.Second)})
	if err != nil {
		t.Fatalf("duplicate apply: %v", err)
	}
	if res.Decision != githubcontrollerstore.DecisionDuplicateProcessed || res.TargetID != first.TargetID || res.Generation != first.Generation {
		t.Fatalf("bad duplicate result: %#v", res)
	}
	if _, err := store.AcquireReconcileLease(ctx, githubcontrollerstore.PRKey{InstallationID: 42, BaseRepositoryID: 101, PullRequestNumber: 7}, "controller-1", now.Add(2*time.Second), 30*time.Second); err != nil {
		t.Fatalf("lease was not released after duplicate: %v", err)
	}
}

func TestHeadChangeSupersedesOldJobAndCreatesNewGeneration(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	first := scheduleEligible(t, store, "outbox-head1", "7", "8")
	now := time.Date(2026, 6, 23, 12, 5, 0, 0, time.UTC)
	lease, err := store.AcquireReconcileLease(ctx, githubcontrollerstore.PRKey{InstallationID: 42, BaseRepositoryID: 101, PullRequestNumber: 7}, "controller-1", now, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.ApplyPullRequestSnapshot(ctx, githubcontrollerstore.PullRequestReconcileInput{Processed: processed("outbox-head2", "sha256:"+strings.Repeat("9", 64)), Lease: lease, Snapshot: eligibleSnapshot(repeat40("7"), repeat40("9")), Now: now.Add(time.Second)})
	if err != nil {
		t.Fatalf("head change: %v", err)
	}
	if second.Decision != githubcontrollerstore.DecisionScheduledNewTarget || second.Generation != first.Generation+1 || second.TargetID == first.TargetID {
		t.Fatalf("bad head-change result: %#v first %#v", second, first)
	}
	oldState, err := store.GetJobState(ctx, first.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if oldState != string(githubapp.JobStateSuperseded) {
		t.Fatalf("old job state = %q", oldState)
	}
}
