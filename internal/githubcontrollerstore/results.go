package githubcontrollerstore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/mattneel/glassroot/internal/githubapp"
)

func (s *Store) ClassifyWorkerResult(ctx context.Context, r githubapp.WorkerResult) (WorkerFreshness, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if basic, ok, err := workerResultBasicFreshness(r); !ok || err != nil {
		return basic, err
	}
	var jobID, targetID, state string
	var generation int64
	err := s.db.QueryRowContext(ctx, `SELECT job_id,target_id,generation,state FROM attempts WHERE attempt_id=?`, r.AttemptID).Scan(&jobID, &targetID, &generation, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkerFreshnessUnknownAttempt, nil
	}
	if err != nil {
		return WorkerFreshnessUnexpectedResult, wrap(CodeTransactionFailed, "worker-result", "attempt read failed", err)
	}
	if jobID != r.JobID || targetID != r.TargetID {
		return WorkerFreshnessUnexpectedResult, nil
	}
	if generation != r.Generation {
		return WorkerFreshnessStaleGeneration, nil
	}
	if state == string(githubapp.AttemptStateCancelled) {
		return WorkerFreshnessCancelledAttempt, nil
	}
	var jobState string
	if err := s.db.QueryRowContext(ctx, `SELECT state FROM jobs WHERE job_id=?`, r.JobID).Scan(&jobState); err != nil {
		return WorkerFreshnessUnexpectedResult, wrap(CodeTransactionFailed, "worker-result", "job read failed", err)
	}
	if jobState == string(githubapp.JobStateSuperseded) {
		return WorkerFreshnessSupersededJob, nil
	}
	return WorkerFreshnessUnexpectedResult, nil
}

func workerResultBasicFreshness(r githubapp.WorkerResult) (WorkerFreshness, bool, error) {
	if r.AttemptID == "" || r.JobID == "" || r.TargetID == "" || r.Generation <= 0 {
		return WorkerFreshnessUnexpectedResult, false, errCode(CodeRecordInvalid, "worker-result", "worker result rejected", nil)
	}
	if r.RunnerTier != githubapp.RunnerTierHardenedContainer && r.RunnerTier != githubapp.RunnerTierMicroVM {
		return WorkerFreshnessInvalidRunnerTier, false, nil
	}
	return "", true, nil
}
