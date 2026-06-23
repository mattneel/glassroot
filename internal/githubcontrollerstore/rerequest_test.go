package githubcontrollerstore_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/githubapp"
	"github.com/mattneel/glassroot/internal/githubcontrollerstore"
)

func TestCurrentCheckRunRerequestCreatesNewAttemptOnly(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	scheduled := scheduleEligible(t, store, "outbox-rereq-source", "a", "b")
	binding := githubcontrollerstore.CheckBinding{AppID: 123, InstallationID: 42, RepositoryID: 101, PullRequestNumber: 7, CheckRunID: 9001, ExternalID: "gr-" + strings.Repeat("c", 64), TargetID: scheduled.TargetID, JobID: scheduled.JobID, ControllerGeneration: scheduled.Generation, PublicationGeneration: 1}
	if err := store.RegisterCheckBinding(ctx, binding); err != nil {
		t.Fatalf("RegisterCheckBinding: %v", err)
	}
	now := time.Date(2026, 6, 23, 12, 6, 0, 0, time.UTC)
	lease, err := store.AcquireReconcileLease(ctx, githubcontrollerstore.PRKey{InstallationID: 42, BaseRepositoryID: 101, PullRequestNumber: 7}, "controller-1", now, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	res, err := store.ApplyCheckRunRerequest(ctx, githubcontrollerstore.RerequestInput{Processed: processedCheck("outbox-rereq", "sha256:"+strings.Repeat("d", 64)), Lease: lease, Projection: githubapp.CheckRunProjection{Action: "rerequested", InstallationID: 42, RepositoryID: 101, CheckRunID: 9001, AppID: 123, ExternalID: binding.ExternalID, HeadSHA: repeat40("b")}, SnapshotEligible: true, SnapshotTargetID: scheduled.TargetID, Now: now.Add(time.Second), ConfiguredAppID: 123})
	if err != nil {
		t.Fatalf("ApplyCheckRunRerequest: %v", err)
	}
	if res.Decision != githubcontrollerstore.DecisionRerequestCreated || res.JobID != scheduled.JobID || res.TargetID != scheduled.TargetID || res.Generation != scheduled.Generation || res.AttemptID == "" {
		t.Fatalf("bad rerequest result: %#v", res)
	}
	count, err := store.CountAttempts(ctx, scheduled.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("attempt count = %d", count)
	}
}

func processedCheck(id, digest string) githubcontrollerstore.ProcessedDelivery {
	p := processed(id, digest)
	p.ProjectionKind = githubapp.ProjectionCheckRun
	return p
}
