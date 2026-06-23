package githubcontrollerstore

import (
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/githubapi"
	"github.com/mattneel/glassroot/internal/githubapp"
)

func FuzzReconcilePullRequestState(f *testing.F) {
	f.Add("open", false, false, true, int64(202), strings.Repeat("1", 40), strings.Repeat("2", 40))
	f.Add("closed", false, true, false, int64(0), strings.Repeat("1", 40), "")
	f.Fuzz(func(t *testing.T, state string, draft, merged, headAvailable bool, headRepo int64, base, head string) {
		if len(state) > 64 || len(base) > 128 || len(head) > 128 {
			return
		}
		_ = eligibilityFor(githubapi.PullRequestSnapshot{State: githubapi.PullRequestState(state), Draft: draft, Merged: merged, Base: githubapi.PullRequestEndpoint{RepositoryID: 101, CommitID: base, Available: true}, Head: githubapi.PullRequestEndpoint{RepositoryID: headRepo, CommitID: head, Available: headAvailable}})
	})
}

func FuzzApplyControllerGeneration(f *testing.F) {
	f.Add(int64(0), string(PREligibilityClosed), string(PREligibilityEligible), "target-a", "target-b")
	f.Add(int64(42), string(PREligibilityEligible), string(PREligibilityEligible), "target-a", "target-a")
	f.Fuzz(func(t *testing.T, generation int64, oldEligibility, newEligibility, oldTarget, newTarget string) {
		_ = generationWouldChange(generation, PREligibility(oldEligibility), PREligibility(newEligibility), oldTarget, newTarget)
	})
}

func FuzzValidateSourceImportRequest(f *testing.F) {
	f.Add("source-"+strings.Repeat("a", 64), "target-"+strings.Repeat("b", 64), "job-"+strings.Repeat("c", 64), int64(1), "owner", "repo", strings.Repeat("1", 40), "head", "headrepo", strings.Repeat("2", 40))
	f.Add("bad", "bad", "bad", int64(-1), "../owner", "repo/name", "HEAD", "head", "headrepo", "")
	f.Fuzz(func(t *testing.T, id, target, job string, generation int64, baseOwner, baseName, baseCommit, headOwner, headName, headCommit string) {
		if len(id)+len(target)+len(job)+len(baseOwner)+len(baseName)+len(baseCommit)+len(headOwner)+len(headName)+len(headCommit) > 4096 {
			return
		}
		_ = validateSourceImportRequest(SourceImportRequest{SchemaVersion: SchemaSourceImportRequestV1Alpha1, ID: id, TargetID: target, JobID: job, Generation: generation, InstallationID: 42, Base: RouteHint{RepositoryID: 101, Owner: baseOwner, Name: baseName, CommitID: baseCommit}, Head: RouteHint{RepositoryID: 202, Owner: headOwner, Name: headName, CommitID: headCommit}, ControllerProfileVersion: ControllerProfileAdvisoryV1Alpha1, State: SourceStatePending})
	})
}

func FuzzClassifyWorkerResultFreshness(f *testing.F) {
	f.Add("attempt-"+strings.Repeat("a", 64), "job-"+strings.Repeat("b", 64), "target-"+strings.Repeat("c", 64), int64(1), string(githubapp.RunnerTierHardenedContainer))
	f.Add("bad", "bad", "bad", int64(-1), string(githubapp.RunnerTierDockerDev))
	f.Fuzz(func(t *testing.T, attempt, job, target string, generation int64, tier string) {
		if len(attempt)+len(job)+len(target)+len(tier) > 1024 {
			return
		}
		_, _, _ = workerResultBasicFreshness(githubapp.WorkerResult{AttemptID: attempt, JobID: job, TargetID: target, Generation: generation, RunnerTier: githubapp.RunnerTier(tier)})
	})
}

func generationWouldChange(generation int64, oldEligibility, newEligibility PREligibility, oldTarget, newTarget string) bool {
	if generation <= 0 {
		return true
	}
	if oldEligibility != newEligibility {
		return true
	}
	return newEligibility == PREligibilityEligible && oldTarget != newTarget
}
