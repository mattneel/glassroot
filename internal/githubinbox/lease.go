package githubinbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"time"
)

func (s *Store) ClaimOutbox(ctx context.Context, owner LeaseOwner, now time.Time, duration time.Duration, limit int) ([]LeasedRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.validateLeaseInputs(owner, now, duration, limit); err != nil {
		return nil, err
	}
	expires := now.UTC().Add(duration)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, wrap(CodeTransactionFailed, "lease", "lease transaction failed", err)
	}
	defer rollback(tx)
	rows, err := tx.QueryContext(ctx, `SELECT sequence,id,receiver_id,delivery_id,projection_kind,projection_json,projection_digest,receipt_json,lease_generation,attempt_count,created_at FROM github_outbox WHERE state='pending' OR (state='leased' AND lease_expires_at <= ?) ORDER BY sequence LIMIT ?`, formatTime(now), limit)
	if err != nil {
		return nil, wrap(CodeTransactionFailed, "lease", "claim query failed", err)
	}
	type candidate struct {
		rec         LeasedRecord
		previousGen int64
	}
	var candidates []candidate
	for rows.Next() {
		var rec LeasedRecord
		var projectionJSON, receiptJSON, created string
		var gen, attempts int64
		if err := rows.Scan(&rec.Sequence, &rec.ID, &rec.ReceiverID, &rec.DeliveryID, &rec.ProjectionKind, &projectionJSON, &rec.ProjectionDigest, &receiptJSON, &gen, &attempts, &created); err != nil {
			_ = rows.Close()
			return nil, wrap(CodeRecordInvalid, "lease", "claim row invalid", err)
		}
		if gen == math.MaxInt64 {
			_ = rows.Close()
			return nil, errCode(CodeLeaseGenerationOverflow, "lease", "lease generation overflow", nil)
		}
		if attempts == math.MaxInt64 {
			_ = rows.Close()
			return nil, errCode(CodeAttemptCountOverflow, "lease", "attempt count overflow", nil)
		}
		if err := json.Unmarshal([]byte(projectionJSON), &rec.Projection); err != nil {
			_ = rows.Close()
			return nil, wrap(CodeRecordInvalid, "lease", "projection decode failed", err)
		}
		if err := json.Unmarshal([]byte(receiptJSON), &rec.Receipt); err != nil {
			_ = rows.Close()
			return nil, wrap(CodeRecordInvalid, "lease", "receipt decode failed", err)
		}
		rec.LeaseOwner = owner
		rec.LeaseGeneration = uint64(gen + 1)
		rec.AttemptCount = uint64(attempts + 1)
		rec.CreatedAt = parseTime(created)
		rec.LeaseExpiresAt = expires
		candidates = append(candidates, candidate{rec: rec, previousGen: gen})
	}
	if err := rows.Close(); err != nil {
		return nil, wrap(CodeTransactionFailed, "lease", "claim rows close failed", err)
	}
	claimed := make([]LeasedRecord, 0, len(candidates))
	for i := range candidates {
		rec := candidates[i].rec
		res, err := tx.ExecContext(ctx, `UPDATE github_outbox SET state='leased', lease_owner=?, lease_generation=?, lease_expires_at=?, attempt_count=? WHERE id=? AND sequence=? AND lease_generation=? AND (state='pending' OR (state='leased' AND lease_expires_at <= ?))`, string(owner), int64(rec.LeaseGeneration), formatTime(expires), int64(rec.AttemptCount), rec.ID, rec.Sequence, candidates[i].previousGen, formatTime(now))
		if err != nil {
			return nil, wrap(CodeTransactionFailed, "lease", "claim update failed", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, wrap(CodeTransactionFailed, "lease", "claim row count failed", err)
		}
		if n == 0 {
			continue
		}
		if n != 1 {
			return nil, errCode(CodeOutboxStateInvalid, "lease", "claim update rejected", nil)
		}
		claimed = append(claimed, rec)
	}
	if err := tx.Commit(); err != nil {
		return nil, wrap(CodeTransactionFailed, "lease", "claim commit failed", err)
	}
	if claimed == nil {
		claimed = []LeasedRecord{}
	}
	return claimed, nil
}

func (s *Store) validateLeaseInputs(owner LeaseOwner, now time.Time, duration time.Duration, limit int) error {
	if owner == "" || len(owner) > s.limits.MaxLeaseOwnerBytes || hasControl(string(owner)) {
		return errCode(CodeLeaseOwnerInvalid, "lease", "lease owner rejected", nil)
	}
	if now.IsZero() || now.Location() != time.UTC {
		return errCode(CodeRecordInvalid, "lease", "lease time rejected", nil)
	}
	if duration <= 0 || duration > time.Duration(s.limits.MaxLeaseDurationSeconds)*time.Second {
		return errCode(CodeLeaseDurationInvalid, "lease", "lease duration rejected", nil)
	}
	if limit <= 0 || limit > s.limits.MaxClaimLimit {
		return errCode(CodeOutboxStateInvalid, "lease", "claim limit rejected", nil)
	}
	return nil
}

func (s *Store) AcknowledgeOutbox(ctx context.Context, id string, owner LeaseOwner, generation uint64, acknowledgedAt time.Time) error {
	return s.finishLease(ctx, id, owner, generation, acknowledgedAt, true, "")
}

func (s *Store) ReleaseOutbox(ctx context.Context, id string, owner LeaseOwner, generation uint64, releasedAt time.Time, failureCode string) error {
	if failureCode != "" && (len(failureCode) > 64 || hasControl(failureCode)) {
		return errCode(CodeOutboxStateInvalid, "lease", "failure code rejected", nil)
	}
	return s.finishLease(ctx, id, owner, generation, releasedAt, false, failureCode)
}

func (s *Store) finishLease(ctx context.Context, id string, owner LeaseOwner, generation uint64, when time.Time, ack bool, failureCode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == "" || owner == "" || generation == 0 || when.IsZero() || when.Location() != time.UTC {
		return errCode(CodeStaleLease, "lease", "lease identity rejected", nil)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap(CodeTransactionFailed, "lease", "lease finish transaction failed", err)
	}
	defer rollback(tx)
	var state, gotOwner string
	var gotGeneration int64
	err = tx.QueryRowContext(ctx, `SELECT state, COALESCE(lease_owner,''), lease_generation FROM github_outbox WHERE id=?`, id).Scan(&state, &gotOwner, &gotGeneration)
	if errors.Is(err, sql.ErrNoRows) {
		return errCode(CodeStaleLease, "lease", "lease record missing", nil)
	}
	if err != nil {
		return wrap(CodeTransactionFailed, "lease", "lease read failed", err)
	}
	if gotOwner != string(owner) || uint64(gotGeneration) != generation {
		return errCode(CodeStaleLease, "lease", "stale lease rejected", nil)
	}
	if ack {
		if state == string(OutboxStateAcknowledged) {
			_ = tx.Commit()
			return nil
		}
		if state != string(OutboxStateLeased) {
			return errCode(CodeOutboxStateInvalid, "lease", "ack state rejected", nil)
		}
		_, err = tx.ExecContext(ctx, `UPDATE github_outbox SET state='acknowledged', acknowledged_at=? WHERE id=?`, formatTime(when), id)
	} else {
		if state != string(OutboxStateLeased) {
			return errCode(CodeOutboxStateInvalid, "lease", "release state rejected", nil)
		}
		_, err = tx.ExecContext(ctx, `UPDATE github_outbox SET state='pending', lease_expires_at=NULL, last_failure_code=? WHERE id=?`, failureCode, id)
	}
	if err != nil {
		return wrap(CodeTransactionFailed, "lease", "lease finish update failed", err)
	}
	if err := tx.Commit(); err != nil {
		return wrap(CodeTransactionFailed, "lease", "lease finish commit failed", err)
	}
	return nil
}
