package githubcontrollerstore_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/githubapp"
	"github.com/mattneel/glassroot/internal/githubcontrollerstore"
)

func TestInstallationRemovalHintInvalidatesAffectedCurrentJobsOnly(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	affected := scheduleEligible(t, store, "outbox-install-affected", "a", "b")
	unaffected := scheduleEligibleFor(t, store, "outbox-install-unaffected", githubcontrollerstore.PRKey{InstallationID: 42, BaseRepositoryID: 303, PullRequestNumber: 8}, "c", "d")

	res, err := store.ApplyInstallationHint(ctx, githubcontrollerstore.InstallationHintInput{
		Processed: processedInstallation("outbox-install-hint", "sha256:"+strings.Repeat("1", 64)),
		Projection: githubapp.InstallationProjection{
			Action:         "removed",
			InstallationID: 42,
			RepositoryIDs:  []int64{101},
		},
		Now: time.Date(2026, 6, 23, 12, 9, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ApplyInstallationHint: %v", err)
	}
	if res.Decision != githubcontrollerstore.DecisionInstallationBlocked {
		t.Fatalf("bad result: %#v", res)
	}
	affectedState, err := store.GetCurrentPRState(ctx, githubcontrollerstore.PRKey{InstallationID: 42, BaseRepositoryID: 101, PullRequestNumber: 7})
	if err != nil {
		t.Fatal(err)
	}
	if affectedState.Generation != affected.Generation+1 || affectedState.Eligibility != githubcontrollerstore.PREligibilityInstallationBlocked {
		t.Fatalf("affected state not invalidated: %#v", affectedState)
	}
	if jobState, err := store.GetJobState(ctx, affected.JobID); err != nil || jobState != string(githubapp.JobStateCancelled) {
		t.Fatalf("affected job state=%q err=%v", jobState, err)
	}
	unaffectedState, err := store.GetCurrentPRState(ctx, githubcontrollerstore.PRKey{InstallationID: 42, BaseRepositoryID: 303, PullRequestNumber: 8})
	if err != nil {
		t.Fatal(err)
	}
	if unaffectedState.Generation != unaffected.Generation || unaffectedState.Eligibility != githubcontrollerstore.PREligibilityEligible {
		t.Fatalf("unaffected state changed: %#v", unaffectedState)
	}
}

func processedInstallation(id, digest string) githubcontrollerstore.ProcessedDelivery {
	p := processed(id, digest)
	p.ProjectionKind = githubapp.ProjectionInstallation
	return p
}
