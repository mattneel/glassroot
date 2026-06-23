package githubcontrollerstore

import (
	"context"
	"database/sql"
	"math"

	"github.com/mattneel/glassroot/internal/githubapp"
)

func (s *Store) ApplyInstallationHint(ctx context.Context, in InstallationHintInput) (ReconcileResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if in.Processed.ReceiverID != s.receiverID || in.Processed.OutboxID == "" || !validDigest(in.Processed.ProjectionDigest) || !validTime(in.Processed.ProcessedAt) || !validTime(in.Now) || in.Projection.InstallationID <= 0 {
		return ReconcileResult{}, errCode(CodeRecordInvalid, "installation", "installation hint rejected", nil)
	}
	if in.Processed.ProjectionKind != githubapp.ProjectionInstallation && in.Processed.ProjectionKind != githubapp.ProjectionInstallationRepositories {
		return ReconcileResult{}, errCode(CodeRecordInvalid, "installation", "installation projection kind rejected", nil)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReconcileResult{}, wrap(CodeTransactionFailed, "installation", "installation tx failed", err)
	}
	defer rollback(tx)
	if res, ok, err := s.checkProcessed(ctx, tx, in.Processed); err != nil || ok {
		return res, err
	}
	affected := int64(0)
	if installationHintInvalidates(in.Projection) {
		var err error
		affected, err = s.invalidateInstallationRows(ctx, tx, in)
		if err != nil {
			return ReconcileResult{}, err
		}
	}
	decision := DecisionInstallationHintRecorded
	if affected > 0 {
		decision = DecisionInstallationBlocked
	}
	res := ReconcileResult{Decision: decision}
	if err := insertProcessed(ctx, tx, in.Processed, res); err != nil {
		return ReconcileResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ReconcileResult{}, wrap(CodeTransactionFailed, "installation", "installation commit failed", err)
	}
	return res, nil
}

func installationHintInvalidates(p githubapp.InstallationProjection) bool {
	switch p.Action {
	case "deleted", "suspend", "suspended", "removed", "repositories_removed":
		return true
	default:
		return false
	}
}

func (s *Store) invalidateInstallationRows(ctx context.Context, tx *sql.Tx, in InstallationHintInput) (int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT installation_id,base_repository_id,pull_request_number,generation,current_job_id FROM pull_request_state WHERE installation_id=? AND eligibility=?`, in.Projection.InstallationID, string(PREligibilityEligible))
	if err != nil {
		return 0, wrap(CodeTransactionFailed, "installation", "installation state scan failed", err)
	}
	type row struct {
		key PRKey
		gen int64
		job sql.NullString
	}
	var rowsToBlock []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.key.InstallationID, &r.key.BaseRepositoryID, &r.key.PullRequestNumber, &r.gen, &r.job); err != nil {
			_ = rows.Close()
			return 0, wrap(CodeRecordInvalid, "installation", "installation state row rejected", err)
		}
		if repositoryAffectedByHint(in.Projection, r.key.BaseRepositoryID) {
			rowsToBlock = append(rowsToBlock, r)
		}
	}
	if err := rows.Close(); err != nil {
		return 0, wrap(CodeTransactionFailed, "installation", "installation rows close failed", err)
	}
	for _, r := range rowsToBlock {
		if r.gen == math.MaxInt64 {
			return 0, errCode(CodeGenerationOverflow, "installation", "generation overflow", nil)
		}
		if r.job.Valid {
			if err := cancelCurrentJob(ctx, tx, r.job.String); err != nil {
				return 0, err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE pull_request_state SET generation=?, eligibility=?, current_target_id=NULL, current_job_id=NULL, updated_at=? WHERE installation_id=? AND base_repository_id=? AND pull_request_number=? AND generation=?`, r.gen+1, string(PREligibilityInstallationBlocked), formatTime(in.Now), r.key.InstallationID, r.key.BaseRepositoryID, r.key.PullRequestNumber, r.gen); err != nil {
			return 0, wrap(CodeTransactionFailed, "installation", "installation state update failed", err)
		}
	}
	return int64(len(rowsToBlock)), nil
}

func repositoryAffectedByHint(p githubapp.InstallationProjection, repositoryID int64) bool {
	switch p.Action {
	case "deleted", "suspend", "suspended":
		return true
	}
	for _, id := range p.RepositoryIDs {
		if id == repositoryID {
			return true
		}
	}
	return false
}

func cancelCurrentJob(ctx context.Context, tx *sql.Tx, jobID string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE jobs SET state=? WHERE job_id=? AND state NOT IN ('completed','failed','superseded','cancelled')`, string(githubapp.JobStateCancelled), jobID); err != nil {
		return wrap(CodeTransactionFailed, "installation", "job cancel failed", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE attempts SET state=? WHERE job_id=? AND state NOT IN ('completed','failed','cancelled')`, string(githubapp.AttemptStateCancelled), jobID); err != nil {
		return wrap(CodeTransactionFailed, "installation", "attempt cancel failed", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE source_requests SET state=? WHERE job_id=? AND state IN ('pending','leased')`, string(SourceStateCancelled), jobID); err != nil {
		return wrap(CodeTransactionFailed, "installation", "source cancel failed", err)
	}
	return nil
}
