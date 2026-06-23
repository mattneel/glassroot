package githubcontrollerstore_test

import (
	"context"
	"testing"

	"github.com/mattneel/glassroot/internal/githubapp"
	"github.com/mattneel/glassroot/internal/githubcontrollerstore"
)

func TestClassifyWorkerResultRejectsDevelopmentRunnerAndUnexpectedCurrentResult(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	scheduled := scheduleEligible(t, store, "outbox-worker", "e", "f")
	result := githubapp.WorkerResult{SchemaVersion: githubapp.SchemaGitHubWorkerResultV1Alpha1, AttemptID: scheduled.AttemptID, JobID: scheduled.JobID, TargetID: scheduled.TargetID, Generation: scheduled.Generation, RunnerTier: githubapp.RunnerTierDockerDev, ReportDigest: "sha256:" + repeat64("a"), ManifestDigest: "sha256:" + repeat64("b"), PolicyApplicationDigest: "sha256:" + repeat64("c"), Limitations: []string{}}
	class, err := store.ClassifyWorkerResult(ctx, result)
	if err != nil {
		t.Fatalf("ClassifyWorkerResult: %v", err)
	}
	if class != githubcontrollerstore.WorkerFreshnessInvalidRunnerTier {
		t.Fatalf("class = %q", class)
	}
	result.RunnerTier = githubapp.RunnerTierHardenedContainer
	class, err = store.ClassifyWorkerResult(ctx, result)
	if err != nil {
		t.Fatalf("ClassifyWorkerResult hardened: %v", err)
	}
	if class != githubcontrollerstore.WorkerFreshnessUnexpectedResult {
		t.Fatalf("class = %q", class)
	}
}

func repeat64(s string) string {
	out := ""
	for len(out) < 64 {
		out += s
	}
	return out[:64]
}
