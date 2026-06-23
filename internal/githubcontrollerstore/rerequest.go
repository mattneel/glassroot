package githubcontrollerstore

import (
	"context"
	"database/sql"
	"errors"
	"math"

	"github.com/mattneel/glassroot/internal/githubapp"
)

func (s *Store) RegisterCheckBinding(ctx context.Context, b CheckBinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b.AppID != s.appID || b.InstallationID <= 0 || b.RepositoryID <= 0 || b.PullRequestNumber <= 0 || b.CheckRunID <= 0 || b.ExternalID == "" || b.TargetID == "" || b.JobID == "" || b.ControllerGeneration <= 0 || b.PublicationGeneration <= 0 {
		return errCode(CodeRecordInvalid, "check-binding", "check binding rejected", nil)
	}
	res, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO check_bindings(app_id,installation_id,repository_id,pull_request_number,check_run_id,external_id,target_id,job_id,controller_generation,publication_generation) VALUES(?,?,?,?,?,?,?,?,?,?)`, b.AppID, b.InstallationID, b.RepositoryID, b.PullRequestNumber, b.CheckRunID, b.ExternalID, b.TargetID, b.JobID, b.ControllerGeneration, b.PublicationGeneration)
	if err != nil {
		return wrap(CodeTransactionFailed, "check-binding", "binding insert failed", err)
	}
	n, _ := res.RowsAffected()
	if n == 1 {
		return nil
	}
	var existing CheckBinding
	err = s.db.QueryRowContext(ctx, `SELECT app_id,installation_id,repository_id,pull_request_number,check_run_id,external_id,target_id,job_id,controller_generation,publication_generation FROM check_bindings WHERE app_id=? AND repository_id=? AND check_run_id=?`, b.AppID, b.RepositoryID, b.CheckRunID).Scan(&existing.AppID, &existing.InstallationID, &existing.RepositoryID, &existing.PullRequestNumber, &existing.CheckRunID, &existing.ExternalID, &existing.TargetID, &existing.JobID, &existing.ControllerGeneration, &existing.PublicationGeneration)
	if err != nil {
		return wrap(CodeTransactionFailed, "check-binding", "binding read failed", err)
	}
	if existing != b {
		return errCode(CodeCheckBindingConflict, "check-binding", "check binding conflict", nil)
	}
	return nil
}

func (s *Store) ApplyCheckRunRerequest(ctx context.Context, in RerequestInput) (ReconcileResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if in.ConfiguredAppID != s.appID || in.Projection.Action != "rerequested" || in.Projection.AppID != s.appID || in.Projection.CheckRunID <= 0 || in.Processed.ProjectionKind != githubapp.ProjectionCheckRun || in.Processed.ReceiverID != s.receiverID || !validDigest(in.Processed.ProjectionDigest) || !validTime(in.Now) || in.Now.After(in.Lease.ExpiresAt) {
		return ReconcileResult{}, errCode(CodeRecordInvalid, "rerequest", "rerequest input rejected", nil)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReconcileResult{}, wrap(CodeTransactionFailed, "rerequest", "rerequest tx failed", err)
	}
	defer rollback(tx)
	if res, ok, err := s.checkProcessed(ctx, tx, in.Processed); err != nil || ok {
		if ok && err == nil {
			if relErr := releaseReconcileLease(ctx, tx, in.Lease); relErr != nil {
				return ReconcileResult{}, relErr
			}
			if commitErr := tx.Commit(); commitErr != nil {
				return ReconcileResult{}, wrap(CodeTransactionFailed, "rerequest", "duplicate commit failed", commitErr)
			}
		}
		return res, err
	}
	var b CheckBinding
	err = tx.QueryRowContext(ctx, `SELECT app_id,installation_id,repository_id,pull_request_number,check_run_id,external_id,target_id,job_id,controller_generation,publication_generation FROM check_bindings WHERE app_id=? AND repository_id=? AND check_run_id=?`, s.appID, in.Projection.RepositoryID, in.Projection.CheckRunID).Scan(&b.AppID, &b.InstallationID, &b.RepositoryID, &b.PullRequestNumber, &b.CheckRunID, &b.ExternalID, &b.TargetID, &b.JobID, &b.ControllerGeneration, &b.PublicationGeneration)
	if errors.Is(err, sql.ErrNoRows) {
		res := ReconcileResult{Decision: DecisionRerequestIgnored}
		if err := insertProcessed(ctx, tx, in.Processed, res); err != nil {
			return ReconcileResult{}, err
		}
		if err := releaseReconcileLease(ctx, tx, in.Lease); err != nil {
			return ReconcileResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return ReconcileResult{}, wrap(CodeTransactionFailed, "rerequest", "ignored commit failed", err)
		}
		return res, nil
	}
	if err != nil {
		return ReconcileResult{}, wrap(CodeTransactionFailed, "rerequest", "binding read failed", err)
	}
	if b.InstallationID != in.Projection.InstallationID || b.ExternalID != in.Projection.ExternalID || b.PullRequestNumber != in.Lease.Key.PullRequestNumber {
		return ReconcileResult{}, errCode(CodeCheckBindingConflict, "rerequest", "foreign check run rejected", nil)
	}
	cur, err := readCurrent(ctx, tx, in.Lease.Key)
	if err != nil {
		return ReconcileResult{}, err
	}
	if cur.Generation != b.ControllerGeneration || cur.CurrentTargetID != b.TargetID || cur.CurrentJobID != b.JobID || cur.Eligibility != PREligibilityEligible || !in.SnapshotEligible || in.SnapshotTargetID != b.TargetID {
		res := ReconcileResult{Decision: DecisionRerequestIgnored, Generation: cur.Generation, TargetID: b.TargetID, JobID: b.JobID}
		if err := insertProcessed(ctx, tx, in.Processed, res); err != nil {
			return ReconcileResult{}, err
		}
		if err := releaseReconcileLease(ctx, tx, in.Lease); err != nil {
			return ReconcileResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return ReconcileResult{}, wrap(CodeTransactionFailed, "rerequest", "stale commit failed", err)
		}
		return res, nil
	}
	var maxNum sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(attempt_number) FROM attempts WHERE job_id=?`, b.JobID).Scan(&maxNum); err != nil {
		return ReconcileResult{}, wrap(CodeTransactionFailed, "rerequest", "attempt count failed", err)
	}
	next := int64(1)
	if maxNum.Valid {
		next = maxNum.Int64 + 1
	}
	if next > s.limits.MaxAttemptsPerJob || next > math.MaxInt64 {
		return ReconcileResult{}, errCode(CodeSourceStateInvalid, "rerequest", "attempt limit reached", nil)
	}
	job := githubapp.AnalysisJob{SchemaVersion: githubapp.SchemaGitHubAnalysisJobV1Alpha1, ID: b.JobID, TargetID: b.TargetID, Generation: b.ControllerGeneration, AnalysisProfileVersion: ControllerProfileAdvisoryV1Alpha1, RequiredRunnerTier: githubapp.RunnerTierHardenedContainer}
	attempt, err := githubapp.NewAnalysisAttempt(job, next, githubapp.AttemptReasonCheckRerequest)
	if err != nil {
		return ReconcileResult{}, wrap(CodeRecordInvalid, "rerequest", "attempt invalid", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO attempts(attempt_id,job_id,target_id,generation,attempt_number,reason,state,created_at) VALUES(?,?,?,?,?,?,?,?)`, attempt.ID, attempt.JobID, attempt.TargetID, attempt.Generation, attempt.AttemptNumber, string(attempt.Reason), string(githubapp.AttemptStateQueued), formatTime(in.Now)); err != nil {
		return ReconcileResult{}, wrap(CodeTransactionFailed, "rerequest", "attempt insert failed", err)
	}
	res := ReconcileResult{Decision: DecisionRerequestCreated, Generation: b.ControllerGeneration, TargetID: b.TargetID, JobID: b.JobID, AttemptID: attempt.ID}
	if err := insertProcessed(ctx, tx, in.Processed, res); err != nil {
		return ReconcileResult{}, err
	}
	if err := releaseReconcileLease(ctx, tx, in.Lease); err != nil {
		return ReconcileResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ReconcileResult{}, wrap(CodeTransactionFailed, "rerequest", "rerequest commit failed", err)
	}
	return res, nil
}

func (s *Store) CountAttempts(ctx context.Context, jobID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM attempts WHERE job_id=?`, jobID).Scan(&n); err != nil {
		return 0, wrap(CodeTransactionFailed, "attempt", "attempt count failed", err)
	}
	return n, nil
}

func (s *Store) LookupCheckBinding(ctx context.Context, appID, repositoryID, checkRunID int64) (CheckBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b CheckBinding
	err := s.db.QueryRowContext(ctx, `SELECT app_id,installation_id,repository_id,pull_request_number,check_run_id,external_id,target_id,job_id,controller_generation,publication_generation FROM check_bindings WHERE app_id=? AND repository_id=? AND check_run_id=?`, appID, repositoryID, checkRunID).Scan(&b.AppID, &b.InstallationID, &b.RepositoryID, &b.PullRequestNumber, &b.CheckRunID, &b.ExternalID, &b.TargetID, &b.JobID, &b.ControllerGeneration, &b.PublicationGeneration)
	if errors.Is(err, sql.ErrNoRows) {
		return b, errCode(CodeCheckBindingConflict, "check-binding", "check binding not found", nil)
	}
	if err != nil {
		return b, wrap(CodeTransactionFailed, "check-binding", "check binding read failed", err)
	}
	return b, nil
}
