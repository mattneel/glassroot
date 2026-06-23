package githubcontrollerstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/mattneel/glassroot/internal/githubapp"
)

func (s *Store) ClaimSourceImports(ctx context.Context, owner string, now time.Time, duration time.Duration, limit int) ([]SourceImportRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validID(owner) || !validTime(now) || duration <= 0 || duration > s.limits.SourceLeaseDuration || limit <= 0 || limit > 100 {
		return nil, errCode(CodeSourceStateInvalid, "source-lease", "source lease inputs rejected", nil)
	}
	expires := now.Add(duration).UTC().Round(0)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, wrap(CodeTransactionFailed, "source-lease", "source lease transaction failed", err)
	}
	defer rollback(tx)
	rows, err := tx.QueryContext(ctx, `SELECT sequence,request_id,target_id,job_id,generation,installation_id,pull_request_number,base_repository_id,base_owner,base_name,base_commit_id,head_repository_id,head_owner,head_name,head_commit_id,state,lease_generation,attempt_count,created_at FROM source_requests WHERE state='pending' OR (state='leased' AND lease_expires_at <= ?) ORDER BY sequence LIMIT ?`, formatTime(now), limit)
	if err != nil {
		return nil, wrap(CodeTransactionFailed, "source-lease", "source claim failed", err)
	}
	type cand struct {
		seq      int64
		req      SourceImportRequest
		prevGen  int64
		attempts int64
	}
	var cands []cand
	for rows.Next() {
		var c cand
		var created string
		var state string
		if err := rows.Scan(&c.seq, &c.req.ID, &c.req.TargetID, &c.req.JobID, &c.req.Generation, &c.req.InstallationID, &c.req.PullRequestNumber, &c.req.Base.RepositoryID, &c.req.Base.Owner, &c.req.Base.Name, &c.req.Base.CommitID, &c.req.Head.RepositoryID, &c.req.Head.Owner, &c.req.Head.Name, &c.req.Head.CommitID, &state, &c.prevGen, &c.attempts, &created); err != nil {
			_ = rows.Close()
			return nil, wrap(CodeRecordInvalid, "source-lease", "source row rejected", err)
		}
		if c.prevGen == math.MaxInt64 || c.attempts == math.MaxInt64 {
			_ = rows.Close()
			return nil, errCode(CodeSourceLeaseStale, "source-lease", "source lease overflow", nil)
		}
		c.req.SchemaVersion = SchemaSourceImportRequestV1Alpha1
		c.req.ControllerProfileVersion = ControllerProfileAdvisoryV1Alpha1
		c.req.State = SourceRequestState(state)
		c.req.CreatedAt = parseTime(created)
		c.req.LeaseOwner = owner
		c.req.LeaseGeneration = c.prevGen + 1
		cands = append(cands, c)
	}
	if err := rows.Close(); err != nil {
		return nil, wrap(CodeTransactionFailed, "source-lease", "source rows close failed", err)
	}
	out := make([]SourceImportRequest, 0, len(cands))
	for _, c := range cands {
		res, err := tx.ExecContext(ctx, `UPDATE source_requests SET state='leased', lease_owner=?, lease_generation=?, lease_expires_at=?, attempt_count=? WHERE sequence=? AND lease_generation=? AND (state='pending' OR (state='leased' AND lease_expires_at <= ?))`, owner, c.req.LeaseGeneration, formatTime(expires), c.attempts+1, c.seq, c.prevGen, formatTime(now))
		if err != nil {
			return nil, wrap(CodeTransactionFailed, "source-lease", "source claim update failed", err)
		}
		n, _ := res.RowsAffected()
		if n == 1 {
			out = append(out, c.req)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, wrap(CodeTransactionFailed, "source-lease", "source claim commit failed", err)
	}
	if out == nil {
		out = []SourceImportRequest{}
	}
	return out, nil
}

func (s *Store) AcknowledgeSourceImport(ctx context.Context, id, owner string, generation int64, when time.Time) error {
	return s.finishSourceLease(ctx, id, owner, generation, when, SourceStateCompleted, "")
}
func (s *Store) ReleaseSourceImport(ctx context.Context, id, owner string, generation int64, when time.Time, failureCode string) error {
	return s.finishSourceLease(ctx, id, owner, generation, when, SourceStatePending, failureCode)
}
func (s *Store) finishSourceLease(ctx context.Context, id, owner string, generation int64, when time.Time, next SourceRequestState, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == "" || !validID(owner) || generation <= 0 || !validTime(when) {
		return errCode(CodeSourceLeaseStale, "source-lease", "source lease identity rejected", nil)
	}
	completedAt := any(nil)
	if next == SourceStateCompleted {
		completedAt = formatTime(when)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE source_requests SET state=?, lease_expires_at=NULL, completed_at=? WHERE request_id=? AND lease_owner=? AND lease_generation=? AND state='leased'`, string(next), completedAt, id, owner, generation)
	if err != nil {
		return wrap(CodeTransactionFailed, "source-lease", "source lease finish failed", err)
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return errCode(CodeSourceLeaseStale, "source-lease", "stale source lease rejected", nil)
	}
	return nil
}

func (s *Store) ApplySourceImportResult(ctx context.Context, r SourceImportResult, owner string, leaseGeneration int64, when time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.RequestID == "" || r.TargetID == "" || r.JobID == "" || r.Generation <= 0 || r.BaseRepositoryID <= 0 || r.HeadRepositoryID <= 0 || !validGitObjectID(r.BaseCommitID) || !validGitObjectID(r.HeadCommitID) || !validID(owner) || leaseGeneration <= 0 || !validTime(when) {
		return errCode(CodeRecordInvalid, "source-result", "source result rejected", nil)
	}
	limitationsJSON := ""
	if !r.Failed {
		var err error
		limitationsJSON, err = validateSuccessfulSourceResult(r)
		if err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap(CodeTransactionFailed, "source-result", "source result tx failed", err)
	}
	defer rollback(tx)
	var targetID, jobID, baseCommit, headCommit, state string
	var gen, baseRepo, headRepo int64
	err = tx.QueryRowContext(ctx, `SELECT target_id,job_id,generation,base_repository_id,head_repository_id,base_commit_id,head_commit_id,state FROM source_requests WHERE request_id=? AND lease_owner=? AND lease_generation=?`, r.RequestID, owner, leaseGeneration).Scan(&targetID, &jobID, &gen, &baseRepo, &headRepo, &baseCommit, &headCommit, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return errCode(CodeSourceLeaseStale, "source-result", "source lease missing", nil)
	}
	if err != nil {
		return wrap(CodeTransactionFailed, "source-result", "source request read failed", err)
	}
	if state != string(SourceStateLeased) || targetID != r.TargetID || jobID != r.JobID || gen != r.Generation || baseRepo != r.BaseRepositoryID || headRepo != r.HeadRepositoryID || baseCommit != r.BaseCommitID || headCommit != r.HeadCommitID {
		return errCode(CodeRecordInvalid, "source-result", "source result identity mismatch", nil)
	}
	nextReq := SourceStateCompleted
	nextJob := githubapp.JobStateAwaitingRunner
	if r.Failed {
		nextReq = SourceStateFailed
		nextJob = githubapp.JobStateFailed
	}
	if _, err := tx.ExecContext(ctx, `UPDATE source_requests SET state=?, completed_at=?, source_store_id=?, source_metadata_digest=?, source_import_profile_version=?, source_object_format=?, source_base_tree_id=?, source_head_tree_id=?, source_limitations_json=?, lease_expires_at=NULL WHERE request_id=?`, string(nextReq), formatTime(when), nullable(r.SourceStoreID), nullable(r.MetadataDigest), nullable(r.ImportProfileVersion), nullable(r.ObjectFormat), nullable(r.BaseTreeID), nullable(r.HeadTreeID), nullable(limitationsJSON), r.RequestID); err != nil {
		return wrap(CodeTransactionFailed, "source-result", "source request update failed", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE jobs SET state=? WHERE job_id=? AND generation=? AND state=?`, string(nextJob), r.JobID, r.Generation, string(githubapp.JobStateImportingSource)); err != nil {
		return wrap(CodeTransactionFailed, "source-result", "job update failed", err)
	}
	if err := tx.Commit(); err != nil {
		return wrap(CodeTransactionFailed, "source-result", "source result commit failed", err)
	}
	return nil
}

func validateSuccessfulSourceResult(r SourceImportResult) (string, error) {
	if !strings.HasPrefix(r.SourceStoreID, "source-") || len(r.SourceStoreID) != len("source-")+64 || !isLowerHex(r.SourceStoreID[len("source-"):], 64) {
		return "", errCode(CodeRecordInvalid, "source-result", "source store id rejected", nil)
	}
	if !strings.HasPrefix(r.MetadataDigest, "sha256:") || len(r.MetadataDigest) != len("sha256:")+64 || !isLowerHex(r.MetadataDigest[len("sha256:"):], 64) {
		return "", errCode(CodeRecordInvalid, "source-result", "metadata digest rejected", nil)
	}
	if r.ImportProfileVersion != SourceImportProfileSmartHTTPShallowV1Alpha1 {
		return "", errCode(CodeRecordInvalid, "source-result", "import profile rejected", nil)
	}
	wantLen := 40
	if len(r.BaseCommitID) == 64 {
		wantLen = 64
	}
	switch r.ObjectFormat {
	case "sha1":
		if wantLen != 40 {
			return "", errCode(CodeRecordInvalid, "source-result", "object format rejected", nil)
		}
	case "sha256":
		if wantLen != 64 {
			return "", errCode(CodeRecordInvalid, "source-result", "object format rejected", nil)
		}
	default:
		return "", errCode(CodeRecordInvalid, "source-result", "object format rejected", nil)
	}
	if !isLowerHex(r.BaseTreeID, wantLen) || !isLowerHex(r.HeadTreeID, wantLen) {
		return "", errCode(CodeRecordInvalid, "source-result", "tree identity rejected", nil)
	}
	if len(r.Limitations) == 0 || len(r.Limitations) > 1000 {
		return "", errCode(CodeRecordInvalid, "source-result", "limitations rejected", nil)
	}
	for _, lim := range r.Limitations {
		if lim == "" || len(lim) > 512 || hasControl(lim) {
			return "", errCode(CodeRecordInvalid, "source-result", "limitations rejected", nil)
		}
		lower := strings.ToLower(lim)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "http://") || strings.Contains(lower, "https://") || strings.Contains(lim, "/") {
			return "", errCode(CodeRecordInvalid, "source-result", "limitations rejected", nil)
		}
	}
	b, err := json.Marshal(r.Limitations)
	if err != nil {
		return "", wrap(CodeRecordInvalid, "source-result", "limitations encode failed", err)
	}
	if len(b) > 65536 {
		return "", errCode(CodeRecordInvalid, "source-result", "limitations rejected", nil)
	}
	return string(b), nil
}

func (s *Store) GetJobState(ctx context.Context, jobID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var state string
	if err := s.db.QueryRowContext(ctx, `SELECT state FROM jobs WHERE job_id=?`, jobID).Scan(&state); err != nil {
		return "", wrap(CodeRecordInvalid, "job", "job missing", err)
	}
	return state, nil
}

func validGitObjectID(s string) bool { return isLowerHex(s, 40) || isLowerHex(s, 64) }

func validRouteHint(s string) bool {
	if s == "" || len(s) > 256 || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == '/' || r == '?' || r == '#' || r == '\\' {
			return false
		}
	}
	return true
}
