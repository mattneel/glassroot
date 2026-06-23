package githubcontrollerstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/mattneel/glassroot/internal/githubapi"
	"github.com/mattneel/glassroot/internal/githubapp"
	_ "modernc.org/sqlite"
)

const databaseFileName = "github-controller.sqlite"
const applicationID = 0x47523243
const schemaVersion = 1

type Store struct {
	mu                       sync.Mutex
	db                       *sql.DB
	controllerID, receiverID string
	appID                    int64
	limits                   Limits
}

func Open(ctx context.Context, cfg Config) (*Store, error) {
	limits := cfg.Limits
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := validateLimits(limits); err != nil {
		return nil, err
	}
	if !validID(cfg.ControllerID) || !validID(cfg.ReceiverID) || cfg.AppID <= 0 {
		return nil, errCode(CodeDatabaseSchemaInvalid, "open", "configuration rejected", nil)
	}
	if err := validateStateDir(cfg.StateDir, limits); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(cfg.StateDir, databaseFileName)
	if err := ensureDatabaseFile(dbPath); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", sqliteDSN(dbPath, limits))
	if err != nil {
		return nil, wrap(CodeDatabaseOpenFailed, "open", "database open failed", err)
	}
	db.SetMaxOpenConns(limits.MaxOpenConnections)
	db.SetMaxIdleConns(limits.MaxIdleConnections)
	s := &Store{db: db, controllerID: cfg.ControllerID, receiverID: cfg.ReceiverID, appID: cfg.AppID, limits: limits}
	if err := s.initialize(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := validateExistingDatabaseFile(dbPath); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func ensureDatabaseFile(path string) error {
	if filepath.Clean(path) != path {
		return errCode(CodeDatabasePathInvalid, "open", "database path rejected", nil)
	}
	st, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		f, createErr := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if createErr != nil {
			return wrap(CodeDatabaseOpenFailed, "open", "database create failed", createErr)
		}
		if closeErr := f.Close(); closeErr != nil {
			return wrap(CodeDatabaseOpenFailed, "open", "database create close failed", closeErr)
		}
		return validateExistingDatabaseFile(path)
	}
	if err != nil {
		return wrap(CodeDatabasePathInvalid, "open", "database path rejected", err)
	}
	return validateDatabaseFileInfo(st)
}

func validateExistingDatabaseFile(path string) error {
	st, err := os.Lstat(path)
	if err != nil {
		return wrap(CodeDatabasePathInvalid, "open", "database path rejected", err)
	}
	return validateDatabaseFileInfo(st)
}

func validateDatabaseFileInfo(st os.FileInfo) error {
	if st.Mode()&os.ModeSymlink != 0 {
		return errCode(CodeDatabaseSymlink, "open", "database path rejected", nil)
	}
	if !st.Mode().IsRegular() {
		return errCode(CodeDatabasePathInvalid, "open", "database file rejected", nil)
	}
	if runtime.GOOS == "linux" {
		if st.Mode().Perm() != 0o600 {
			return errCode(CodeDatabaseModeInvalid, "open", "database mode rejected", nil)
		}
		if sys, ok := st.Sys().(*syscall.Stat_t); ok {
			if sys.Nlink != 1 {
				return errCode(CodeDatabaseModeInvalid, "open", "database link count rejected", nil)
			}
			if sys.Uid != uint32(os.Geteuid()) {
				return errCode(CodeDatabaseModeInvalid, "open", "database owner rejected", nil)
			}
		}
	}
	return nil
}

func sqliteDSN(path string, limits Limits) string {
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("mode", "rwc")
	q.Set("cache", "shared")
	for _, p := range []string{"foreign_keys(ON)", "journal_mode(WAL)", "synchronous(FULL)", "trusted_schema(OFF)", "recursive_triggers(OFF)", fmt.Sprintf("busy_timeout(%d)", limits.BusyTimeoutMilliseconds), fmt.Sprintf("max_page_count(%d)", limits.MaxDatabaseBytes/4096)} {
		q.Add("_pragma", p)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func (s *Store) initialize(ctx context.Context) error {
	if err := s.execPragmas(ctx); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return wrap(CodeMigrationFailed, "schema", "schema initialization failed", err)
	}
	if err := s.ensureSourceRequestColumns(ctx); err != nil {
		return err
	}
	if err := s.bindMetadata(ctx); err != nil {
		return err
	}
	return s.quickCheck(ctx)
}
func (s *Store) ensureSourceRequestColumns(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(source_requests)`)
	if err != nil {
		return wrap(CodeDatabaseSchemaInvalid, "schema", "source_requests schema inspection failed", err)
	}
	defer rows.Close()
	found := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return wrap(CodeDatabaseSchemaInvalid, "schema", "source_requests schema row rejected", err)
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		return wrap(CodeDatabaseSchemaInvalid, "schema", "source_requests schema rows failed", err)
	}
	columns := []struct {
		name string
		sql  string
	}{
		{"pull_request_number", `ALTER TABLE source_requests ADD COLUMN pull_request_number INTEGER NOT NULL DEFAULT 0`},
		{"source_metadata_digest", `ALTER TABLE source_requests ADD COLUMN source_metadata_digest TEXT`},
		{"source_import_profile_version", `ALTER TABLE source_requests ADD COLUMN source_import_profile_version TEXT`},
		{"source_object_format", `ALTER TABLE source_requests ADD COLUMN source_object_format TEXT`},
		{"source_base_tree_id", `ALTER TABLE source_requests ADD COLUMN source_base_tree_id TEXT`},
		{"source_head_tree_id", `ALTER TABLE source_requests ADD COLUMN source_head_tree_id TEXT`},
		{"source_limitations_json", `ALTER TABLE source_requests ADD COLUMN source_limitations_json TEXT`},
	}
	for _, col := range columns {
		if found[col.name] {
			continue
		}
		if _, err := s.db.ExecContext(ctx, col.sql); err != nil {
			return wrap(CodeMigrationFailed, "schema", "source_requests source-result migration failed", err)
		}
	}
	return nil
}

func (s *Store) execPragmas(ctx context.Context) error {
	ps := []string{fmt.Sprintf("PRAGMA application_id=%d", applicationID), "PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL", "PRAGMA foreign_keys=ON", "PRAGMA trusted_schema=OFF", "PRAGMA recursive_triggers=OFF", fmt.Sprintf("PRAGMA busy_timeout=%d", s.limits.BusyTimeoutMilliseconds), fmt.Sprintf("PRAGMA max_page_count=%d", s.limits.MaxDatabaseBytes/4096)}
	for _, p := range ps {
		if _, err := s.db.ExecContext(ctx, p); err != nil {
			return wrap(CodeDatabasePragmasInvalid, "pragmas", "database pragma failed", err)
		}
	}
	return nil
}
func (s *Store) bindMetadata(ctx context.Context) error {
	pairs := map[string]string{"schema_identity": SchemaControllerStoreV1Alpha1, "schema_version": strconv.Itoa(schemaVersion), "controller_id": s.controllerID, "receiver_id": s.receiverID, "app_id": strconv.FormatInt(s.appID, 10), "controller_profile": ControllerProfileAdvisoryV1Alpha1}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap(CodeTransactionFailed, "metadata", "metadata transaction failed", err)
	}
	defer rollback(tx)
	for k, v := range pairs {
		var ex string
		err := tx.QueryRowContext(ctx, `SELECT value FROM controller_metadata WHERE key=?`, k).Scan(&ex)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err := tx.ExecContext(ctx, `INSERT INTO controller_metadata(key,value) VALUES(?,?)`, k, v); err != nil {
				return wrap(CodeMigrationFailed, "metadata", "metadata insert failed", err)
			}
			continue
		}
		if err != nil {
			return wrap(CodeDatabaseSchemaInvalid, "metadata", "metadata read failed", err)
		}
		if ex != v {
			switch k {
			case "controller_id":
				return errCode(CodeDatabaseControllerMismatch, "metadata", "controller mismatch", nil)
			case "receiver_id":
				return errCode(CodeDatabaseReceiverMismatch, "metadata", "receiver mismatch", nil)
			case "app_id":
				return errCode(CodeDatabaseAppMismatch, "metadata", "app mismatch", nil)
			default:
				return errCode(CodeDatabaseSchemaInvalid, "metadata", "metadata mismatch", nil)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return wrap(CodeTransactionFailed, "metadata", "metadata commit failed", err)
	}
	return nil
}
func (s *Store) quickCheck(ctx context.Context) error {
	var got string
	if err := s.db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&got); err != nil {
		return wrap(CodeDatabaseCorrupt, "quick-check", "quick_check failed", err)
	}
	if got != "ok" {
		return errCode(CodeDatabaseCorrupt, "quick-check", "quick_check rejected", nil)
	}
	return nil
}
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	if err := s.db.Close(); err != nil {
		return wrap(CodeCloseFailed, "close", "database close failed", err)
	}
	return nil
}

func (s *Store) AcquireReconcileLease(ctx context.Context, key PRKey, owner string, now time.Time, duration time.Duration) (ReconcileLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validatePRKey(key); err != nil {
		return ReconcileLease{}, err
	}
	if !validID(owner) || !validTime(now) || duration <= 0 || duration > s.limits.ReconcileLeaseDuration {
		return ReconcileLease{}, errCode(CodeRecordInvalid, "lease", "lease inputs rejected", nil)
	}
	expires := now.Add(duration).UTC().Round(0)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReconcileLease{}, wrap(CodeTransactionFailed, "lease", "lease transaction failed", err)
	}
	defer rollback(tx)
	var gen int64
	var exp string
	err = tx.QueryRowContext(ctx, `SELECT lease_generation,expires_at FROM reconcile_leases WHERE installation_id=? AND base_repository_id=? AND pull_request_number=?`, key.InstallationID, key.BaseRepositoryID, key.PullRequestNumber).Scan(&gen, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		gen = 1
		if _, err := tx.ExecContext(ctx, `INSERT INTO reconcile_leases(installation_id,base_repository_id,pull_request_number,owner,lease_generation,acquired_at,expires_at) VALUES(?,?,?,?,?,?,?)`, key.InstallationID, key.BaseRepositoryID, key.PullRequestNumber, owner, gen, formatTime(now), formatTime(expires)); err != nil {
			return ReconcileLease{}, wrap(CodeTransactionFailed, "lease", "lease insert failed", err)
		}
	} else if err != nil {
		return ReconcileLease{}, wrap(CodeTransactionFailed, "lease", "lease read failed", err)
	} else {
		oldExp := parseTime(exp)
		if oldExp.After(now) {
			return ReconcileLease{}, errCode(CodeReconcileLeaseBusy, "lease", "reconcile lease busy", nil)
		}
		if gen == math.MaxInt64 {
			return ReconcileLease{}, errCode(CodeGenerationOverflow, "lease", "lease generation overflow", nil)
		}
		gen++
		if _, err := tx.ExecContext(ctx, `UPDATE reconcile_leases SET owner=?,lease_generation=?,acquired_at=?,expires_at=? WHERE installation_id=? AND base_repository_id=? AND pull_request_number=?`, owner, gen, formatTime(now), formatTime(expires), key.InstallationID, key.BaseRepositoryID, key.PullRequestNumber); err != nil {
			return ReconcileLease{}, wrap(CodeTransactionFailed, "lease", "lease update failed", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return ReconcileLease{}, wrap(CodeTransactionFailed, "lease", "lease commit failed", err)
	}
	return ReconcileLease{Key: key, Owner: owner, Generation: gen, AcquiredAt: now, ExpiresAt: expires}, nil
}

func (s *Store) ApplyPullRequestSnapshot(ctx context.Context, in PullRequestReconcileInput) (ReconcileResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.validateInput(in); err != nil {
		return ReconcileResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReconcileResult{}, wrap(CodeTransactionFailed, "reconcile", "reconcile transaction failed", err)
	}
	defer rollback(tx)
	if res, ok, err := s.checkProcessed(ctx, tx, in.Processed); err != nil || ok {
		if ok && err == nil {
			if relErr := releaseReconcileLease(ctx, tx, in.Lease); relErr != nil {
				return ReconcileResult{}, relErr
			}
			if commitErr := tx.Commit(); commitErr != nil {
				return ReconcileResult{}, wrap(CodeTransactionFailed, "reconcile", "duplicate commit failed", commitErr)
			}
		}
		return res, err
	}
	current, err := readCurrent(ctx, tx, in.Lease.Key)
	if err != nil {
		return ReconcileResult{}, err
	}
	eligibility := eligibilityFor(in.Snapshot)
	nextGen := current.Generation
	same := false
	targetID, jobID, attemptID, srcID := "", "", "", ""
	decision := DecisionSourceUnavailable
	if eligibility == PREligibilityEligible {
		target := githubapp.AnalysisTarget{SchemaVersion: githubapp.SchemaGitHubAnalysisTargetV1Alpha1, InstallationID: in.Lease.Key.InstallationID, BaseRepositoryID: in.Snapshot.Base.RepositoryID, HeadRepositoryID: in.Snapshot.Head.RepositoryID, PullRequestNumber: in.Snapshot.Number, BaseCommitID: in.Snapshot.Base.CommitID, HeadCommitID: in.Snapshot.Head.CommitID, AnalysisProfileVersion: ControllerProfileAdvisoryV1Alpha1}
		targetID, err = target.ID()
		if err != nil {
			return ReconcileResult{}, wrap(CodeRecordInvalid, "target", "target invalid", err)
		}
		if current.Eligibility == PREligibilityEligible && current.CurrentTargetID == targetID {
			same = true
			nextGen = current.Generation
			jobID = current.CurrentJobID
			decision = DecisionNoChangeCurrentTarget
		} else {
			if current.Generation == math.MaxInt64 {
				return ReconcileResult{}, errCode(CodeGenerationOverflow, "generation", "generation overflow", nil)
			}
			nextGen = current.Generation + 1
			decision = DecisionScheduledNewTarget
			if err := supersedeCurrent(ctx, tx, current); err != nil {
				return ReconcileResult{}, err
			}
			job, err := githubapp.NewAnalysisJob(target, nextGen, ControllerProfileAdvisoryV1Alpha1, githubapp.RunnerTierHardenedContainer)
			if err != nil {
				return ReconcileResult{}, wrap(CodeRecordInvalid, "job", "job invalid", err)
			}
			attempt, err := githubapp.NewAnalysisAttempt(job, 1, githubapp.AttemptReasonInitial)
			if err != nil {
				return ReconcileResult{}, wrap(CodeRecordInvalid, "attempt", "attempt invalid", err)
			}
			jobID = job.ID
			attemptID = attempt.ID
			srcID = sourceRequestID(targetID, jobID, nextGen)
			if err := insertTargetJobAttemptSource(ctx, tx, target, job, attempt, srcID, in.Snapshot, in.Now); err != nil {
				return ReconcileResult{}, err
			}
		}
	} else {
		if current.Generation == 0 || current.Eligibility != eligibility {
			if current.Generation == math.MaxInt64 {
				return ReconcileResult{}, errCode(CodeGenerationOverflow, "generation", "generation overflow", nil)
			}
			nextGen = current.Generation + 1
			if err := supersedeCurrent(ctx, tx, current); err != nil {
				return ReconcileResult{}, err
			}
		}
		switch eligibility {
		case PREligibilityDraft:
			decision = DecisionMarkedDraft
		case PREligibilityClosed:
			decision = DecisionMarkedClosed
		default:
			decision = DecisionSourceUnavailable
		}
	}
	if !same {
		if _, err := tx.ExecContext(ctx, `INSERT INTO pull_request_state(installation_id,base_repository_id,pull_request_number,generation,eligibility,current_target_id,current_job_id,base_owner,base_name,base_commit_id,head_repository_id,head_commit_id,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(installation_id,base_repository_id,pull_request_number) DO UPDATE SET generation=excluded.generation,eligibility=excluded.eligibility,current_target_id=excluded.current_target_id,current_job_id=excluded.current_job_id,base_owner=excluded.base_owner,base_name=excluded.base_name,base_commit_id=excluded.base_commit_id,head_repository_id=excluded.head_repository_id,head_commit_id=excluded.head_commit_id,updated_at=excluded.updated_at`, in.Lease.Key.InstallationID, in.Lease.Key.BaseRepositoryID, in.Lease.Key.PullRequestNumber, nextGen, string(eligibility), nullable(targetID), nullable(jobID), nullable(in.Snapshot.Base.Owner), nullable(in.Snapshot.Base.Name), nullable(in.Snapshot.Base.CommitID), nullableInt(in.Snapshot.Head.RepositoryID), nullable(in.Snapshot.Head.CommitID), formatTime(in.Now)); err != nil {
			return ReconcileResult{}, wrap(CodeTransactionFailed, "reconcile", "state upsert failed", err)
		}
	}
	res := ReconcileResult{Decision: decision, Generation: nextGen, TargetID: targetID, JobID: jobID, AttemptID: attemptID, SourceRequestID: srcID, Eligibility: eligibility}
	if err := insertProcessed(ctx, tx, in.Processed, res); err != nil {
		return ReconcileResult{}, err
	}
	if err := releaseReconcileLease(ctx, tx, in.Lease); err != nil {
		return ReconcileResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ReconcileResult{}, wrap(CodeTransactionFailed, "reconcile", "reconcile commit failed", err)
	}
	return res, nil
}
func (s *Store) validateInput(in PullRequestReconcileInput) error {
	if in.Processed.ReceiverID != s.receiverID || in.Processed.OutboxID == "" || in.Processed.DeliveryID == "" || !validDigest(in.Processed.ProjectionDigest) || in.Processed.ProjectionKind != githubapp.ProjectionPullRequest || !validTime(in.Processed.ProcessedAt) {
		return errCode(CodeRecordInvalid, "processed", "processed delivery rejected", nil)
	}
	if err := validatePRKey(in.Lease.Key); err != nil {
		return err
	}
	if !validID(in.Lease.Owner) || in.Lease.Generation <= 0 || !validTime(in.Now) || in.Now.After(in.Lease.ExpiresAt) {
		return errCode(CodeReconcileLeaseStale, "lease", "reconcile lease stale", nil)
	}
	if in.Snapshot.SchemaVersion != "" && in.Snapshot.SchemaVersion != "glassroot.dev/github-pull-request-snapshot/v1alpha1" {
		return errCode(CodeRecordInvalid, "snapshot", "snapshot schema rejected", nil)
	}
	if in.Snapshot.Number != in.Lease.Key.PullRequestNumber || in.Snapshot.Base.RepositoryID != in.Lease.Key.BaseRepositoryID {
		return errCode(CodeRecordInvalid, "snapshot", "snapshot identity mismatch", nil)
	}
	return nil
}
func (s *Store) checkProcessed(ctx context.Context, tx *sql.Tx, p ProcessedDelivery) (ReconcileResult, bool, error) {
	var digest, decision string
	var gen sql.NullInt64
	var target, job sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT projection_digest,decision,generation,target_id,job_id FROM processed_deliveries WHERE receiver_id=? AND outbox_id=?`, p.ReceiverID, p.OutboxID).Scan(&digest, &decision, &gen, &target, &job)
	if errors.Is(err, sql.ErrNoRows) {
		return ReconcileResult{}, false, nil
	}
	if err != nil {
		return ReconcileResult{}, true, wrap(CodeTransactionFailed, "processed", "processed read failed", err)
	}
	if digest != p.ProjectionDigest {
		return ReconcileResult{}, true, errCode(CodeProcessedDeliveryConflict, "processed", "processed digest conflict", nil)
	}
	res := ReconcileResult{Decision: DecisionDuplicateProcessed}
	if gen.Valid {
		res.Generation = gen.Int64
	}
	if target.Valid {
		res.TargetID = target.String
	}
	if job.Valid {
		res.JobID = job.String
	}
	return res, true, nil
}
func insertProcessed(ctx context.Context, tx *sql.Tx, p ProcessedDelivery, r ReconcileResult) error {
	digest := recordDigest(p, r)
	_, err := tx.ExecContext(ctx, `INSERT INTO processed_deliveries(receiver_id,outbox_id,delivery_id,projection_kind,projection_digest,decision,installation_id,base_repository_id,pull_request_number,generation,target_id,job_id,processed_at,record_digest) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, p.ReceiverID, p.OutboxID, p.DeliveryID, string(p.ProjectionKind), p.ProjectionDigest, string(r.Decision), nil, nil, nil, nullableInt(r.Generation), nullable(r.TargetID), nullable(r.JobID), formatTime(p.ProcessedAt), digest)
	if err != nil {
		return wrap(CodeTransactionFailed, "processed", "processed insert failed", err)
	}
	return nil
}
func readCurrent(ctx context.Context, tx *sql.Tx, key PRKey) (CurrentPRState, error) {
	cur := CurrentPRState{Key: key}
	var elig string
	var target, job, owner, name, base, head sql.NullString
	var gen, headRepo sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT generation,eligibility,current_target_id,current_job_id,base_owner,base_name,base_commit_id,head_repository_id,head_commit_id FROM pull_request_state WHERE installation_id=? AND base_repository_id=? AND pull_request_number=?`, key.InstallationID, key.BaseRepositoryID, key.PullRequestNumber).Scan(&gen, &elig, &target, &job, &owner, &name, &base, &headRepo, &head)
	if errors.Is(err, sql.ErrNoRows) {
		return cur, nil
	}
	if err != nil {
		return cur, wrap(CodeTransactionFailed, "state", "current state read failed", err)
	}
	cur.Generation = gen.Int64
	cur.Eligibility = PREligibility(elig)
	if target.Valid {
		cur.CurrentTargetID = target.String
	}
	if job.Valid {
		cur.CurrentJobID = job.String
	}
	if owner.Valid {
		cur.BaseOwner = owner.String
	}
	if name.Valid {
		cur.BaseName = name.String
	}
	if base.Valid {
		cur.BaseCommitID = base.String
	}
	if head.Valid {
		cur.HeadCommitID = head.String
	}
	if headRepo.Valid {
		cur.HeadRepositoryID = headRepo.Int64
	}
	return cur, nil
}
func (s *Store) GetCurrentPRState(ctx context.Context, key PRKey) (CurrentPRState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CurrentPRState{}, wrap(CodeTransactionFailed, "state", "state tx failed", err)
	}
	defer rollback(tx)
	cur, err := readCurrent(ctx, tx, key)
	if err != nil {
		return cur, err
	}
	_ = tx.Commit()
	if cur.Generation == 0 {
		return cur, sql.ErrNoRows
	}
	return cur, nil
}
func insertTargetJobAttemptSource(ctx context.Context, tx *sql.Tx, target githubapp.AnalysisTarget, job githubapp.AnalysisJob, attempt githubapp.AnalysisAttempt, srcID string, snap githubapi.PullRequestSnapshot, now time.Time) error {
	targetJSON, _ := compactJSON(target)
	jobJSON, _ := compactJSON(job)
	tiers, _ := compactJSON([]githubapp.RunnerTier{githubapp.RunnerTierHardenedContainer, githubapp.RunnerTierMicroVM})
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO targets(target_id,target_json,created_at) VALUES(?,?,?)`, job.TargetID, string(targetJSON), formatTime(now)); err != nil {
		return wrap(CodeTransactionFailed, "target", "target insert failed", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO jobs(job_id,target_id,generation,state,required_tiers_json,job_json,created_at) VALUES(?,?,?,?,?,?,?)`, job.ID, job.TargetID, job.Generation, string(githubapp.JobStateImportingSource), string(tiers), string(jobJSON), formatTime(now)); err != nil {
		return wrap(CodeTransactionFailed, "job", "job insert failed", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO attempts(attempt_id,job_id,target_id,generation,attempt_number,reason,state,created_at) VALUES(?,?,?,?,?,?,?,?)`, attempt.ID, attempt.JobID, attempt.TargetID, attempt.Generation, attempt.AttemptNumber, string(attempt.Reason), string(githubapp.AttemptStateQueued), formatTime(now)); err != nil {
		return wrap(CodeTransactionFailed, "attempt", "attempt insert failed", err)
	}
	req := SourceImportRequest{SchemaVersion: SchemaSourceImportRequestV1Alpha1, ID: srcID, TargetID: job.TargetID, JobID: job.ID, Generation: job.Generation, InstallationID: target.InstallationID, PullRequestNumber: snap.Number, Base: RouteHint{RepositoryID: snap.Base.RepositoryID, Owner: snap.Base.Owner, Name: snap.Base.Name, CommitID: snap.Base.CommitID}, Head: RouteHint{RepositoryID: snap.Head.RepositoryID, Owner: snap.Head.Owner, Name: snap.Head.Name, CommitID: snap.Head.CommitID}, ControllerProfileVersion: ControllerProfileAdvisoryV1Alpha1, State: SourceStatePending, CreatedAt: now}
	return insertSourceRequest(ctx, tx, req)
}
func insertSourceRequest(ctx context.Context, tx *sql.Tx, req SourceImportRequest) error {
	if err := validateSourceImportRequest(req); err != nil {
		return err
	}
	b := req.Base
	h := req.Head
	_, err := tx.ExecContext(ctx, `INSERT INTO source_requests(request_id,target_id,job_id,generation,installation_id,pull_request_number,base_repository_id,base_owner,base_name,base_commit_id,head_repository_id,head_owner,head_name,head_commit_id,state,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, req.ID, req.TargetID, req.JobID, req.Generation, req.InstallationID, req.PullRequestNumber, b.RepositoryID, b.Owner, b.Name, b.CommitID, h.RepositoryID, h.Owner, h.Name, h.CommitID, string(req.State), formatTime(req.CreatedAt))
	if err != nil {
		return wrap(CodeTransactionFailed, "source", "source insert failed", err)
	}
	return nil
}

func validateSourceImportRequest(req SourceImportRequest) error {
	if req.ID == "" || req.TargetID == "" || req.JobID == "" || req.Generation <= 0 || req.InstallationID <= 0 || req.PullRequestNumber <= 0 || req.ControllerProfileVersion != ControllerProfileAdvisoryV1Alpha1 {
		return errCode(CodeRecordInvalid, "source", "source request rejected", nil)
	}
	if req.Base.RepositoryID <= 0 || req.Head.RepositoryID <= 0 || !validRouteHint(req.Base.Owner) || !validRouteHint(req.Base.Name) || !validRouteHint(req.Head.Owner) || !validRouteHint(req.Head.Name) || !validGitObjectID(req.Base.CommitID) || !validGitObjectID(req.Head.CommitID) {
		return errCode(CodeRecordInvalid, "source", "source request identity rejected", nil)
	}
	if req.State != SourceStatePending && req.State != SourceStateLeased && req.State != SourceStateCompleted && req.State != SourceStateFailed && req.State != SourceStateSuperseded && req.State != SourceStateCancelled {
		return errCode(CodeRecordInvalid, "source", "source request state rejected", nil)
	}
	return nil
}
func supersedeCurrent(ctx context.Context, tx *sql.Tx, cur CurrentPRState) error {
	if cur.CurrentJobID != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE jobs SET state=? WHERE job_id=? AND state NOT IN ('completed','failed','superseded','cancelled')`, string(githubapp.JobStateSuperseded), cur.CurrentJobID); err != nil {
			return wrap(CodeTransactionFailed, "job", "job supersede failed", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE attempts SET state=? WHERE job_id=? AND state NOT IN ('completed','failed','cancelled')`, string(githubapp.AttemptStateCancelled), cur.CurrentJobID); err != nil {
			return wrap(CodeTransactionFailed, "attempt", "attempt cancel failed", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE source_requests SET state=? WHERE job_id=? AND state IN ('pending','leased')`, string(SourceStateSuperseded), cur.CurrentJobID); err != nil {
			return wrap(CodeTransactionFailed, "source", "source supersede failed", err)
		}
	}
	return nil
}
func releaseReconcileLease(ctx context.Context, tx *sql.Tx, l ReconcileLease) error {
	res, err := tx.ExecContext(ctx, `DELETE FROM reconcile_leases WHERE installation_id=? AND base_repository_id=? AND pull_request_number=? AND owner=? AND lease_generation=?`, l.Key.InstallationID, l.Key.BaseRepositoryID, l.Key.PullRequestNumber, l.Owner, l.Generation)
	if err != nil {
		return wrap(CodeTransactionFailed, "lease", "lease release failed", err)
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return errCode(CodeReconcileLeaseStale, "lease", "lease release rejected", nil)
	}
	return nil
}
func eligibilityFor(s githubapi.PullRequestSnapshot) PREligibility {
	if s.State == githubapi.PullRequestStateClosed || s.Merged {
		return PREligibilityClosed
	}
	if s.Draft {
		return PREligibilityDraft
	}
	if !s.Head.Available || s.Head.RepositoryID <= 0 || s.Head.CommitID == "" {
		return PREligibilitySourceUnavailable
	}
	return PREligibilityEligible
}

func (s *Store) GetSourceImportRequest(ctx context.Context, id string) (SourceImportRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var r SourceImportRequest
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT request_id,target_id,job_id,generation,installation_id,pull_request_number,base_repository_id,base_owner,base_name,base_commit_id,head_repository_id,head_owner,head_name,head_commit_id,state,created_at FROM source_requests WHERE request_id=?`, id).Scan(&r.ID, &r.TargetID, &r.JobID, &r.Generation, &r.InstallationID, &r.PullRequestNumber, &r.Base.RepositoryID, &r.Base.Owner, &r.Base.Name, &r.Base.CommitID, &r.Head.RepositoryID, &r.Head.Owner, &r.Head.Name, &r.Head.CommitID, &r.State, &created)
	if err != nil {
		return r, wrap(CodeRecordInvalid, "source", "source request missing", err)
	}
	r.SchemaVersion = SchemaSourceImportRequestV1Alpha1
	r.ControllerProfileVersion = ControllerProfileAdvisoryV1Alpha1
	r.CreatedAt = parseTime(created)
	return r, nil
}

func validateLimits(l Limits) error {
	if l.MaxPathBytes <= 0 || l.MaxPathBytes > 4096 || l.MaxDatabaseBytes <= 0 || l.MaxDatabaseBytes > 16<<30 || l.BusyTimeoutMilliseconds <= 0 || l.MaxOpenConnections <= 0 || l.MaxIdleConnections <= 0 || l.MaxRouteSegmentBytes <= 0 || l.MaxRouteSegmentBytes > 256 || l.MaxAttemptsPerJob <= 0 || l.MaxAttemptsPerJob > 32 || l.ReconcileLeaseDuration <= 0 || l.ReconcileLeaseDuration > 30*time.Second || l.SourceLeaseDuration <= 0 || l.SourceLeaseDuration > 5*time.Minute {
		return errCode(CodeDatabaseSchemaInvalid, "limits", "limits rejected", nil)
	}
	return nil
}
func validateStateDir(path string, l Limits) error {
	if path == "" || !utf8.ValidString(path) || !filepath.IsAbs(path) || filepath.Clean(path) != path || len(path) > l.MaxPathBytes || hasControl(path) {
		return errCode(CodeInvalidStateDir, "open", "state dir rejected", nil)
	}
	st, err := os.Lstat(path)
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return errCode(CodeInvalidStateDir, "open", "state dir rejected", err)
	}
	if runtime.GOOS == "linux" {
		if st.Mode().Perm() != 0o700 {
			return errCode(CodeInvalidStateDir, "open", "state dir rejected", nil)
		}
		if sys, ok := st.Sys().(*syscall.Stat_t); ok && sys.Uid != uint32(os.Geteuid()) {
			return errCode(CodeInvalidStateDir, "open", "state dir rejected", nil)
		}
	}
	return nil
}
func validatePRKey(k PRKey) error {
	if k.InstallationID <= 0 || k.BaseRepositoryID <= 0 || k.PullRequestNumber <= 0 {
		return errCode(CodeRecordInvalid, "pr-key", "pr key rejected", nil)
	}
	return nil
}
func validID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for i, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
		if i == 0 && (r < 'a' || r > 'z') {
			return false
		}
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
func validTime(t time.Time) bool { return !t.IsZero() && t.Location() == time.UTC }
func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
func validDigest(s string) bool {
	return len(s) == 71 && strings.HasPrefix(s, "sha256:") && isLowerHex(s[7:], 64)
}
func isLowerHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, r := range s {
		if r < '0' || (r > '9' && r < 'a') || r > 'f' {
			return false
		}
	}
	return true
}
func formatTime(t time.Time) string { return t.UTC().Round(0).Format(time.RFC3339) }
func parseTime(s string) time.Time  { t, _ := time.Parse(time.RFC3339, s); return t.UTC().Round(0) }
func rollback(tx *sql.Tx)           { _ = tx.Rollback() }
func compactJSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, wrap(CodeRecordInvalid, "json", "json encode failed", err)
	}
	return b, nil
}
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func nullableInt(i int64) any {
	if i == 0 {
		return nil
	}
	return i
}
func domainHash(domain string, fields ...string) string {
	h := sha256.New()
	h.Write([]byte(domain))
	for _, f := range fields {
		var lenbuf [8]byte
		binary.BigEndian.PutUint64(lenbuf[:], uint64(len(f)))
		h.Write(lenbuf[:])
		h.Write([]byte(f))
	}
	return hex.EncodeToString(h.Sum(nil))
}
func sourceRequestID(targetID, jobID string, generation int64) string {
	return "source-" + domainHash(DomainSourceImportRequestID, targetID, jobID, strconv.FormatInt(generation, 10))
}
func recordDigest(p ProcessedDelivery, r ReconcileResult) string {
	return "sha256:" + domainHash("glassroot.dev/github-controller-processed/v1\x00", p.ReceiverID, p.OutboxID, p.ProjectionDigest, string(r.Decision), strconv.FormatInt(r.Generation, 10), r.TargetID, r.JobID)
}

func (s *Store) RecordProcessingDecision(ctx context.Context, p ProcessedDelivery, decision ProcessingDecision, when time.Time) (ReconcileResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ReceiverID != s.receiverID || p.OutboxID == "" || !validDigest(p.ProjectionDigest) || !validTime(when) {
		return ReconcileResult{}, errCode(CodeRecordInvalid, "processed", "processed decision rejected", nil)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReconcileResult{}, wrap(CodeTransactionFailed, "processed", "decision tx failed", err)
	}
	defer rollback(tx)
	res := ReconcileResult{Decision: decision}
	if got, ok, err := s.checkProcessed(ctx, tx, p); err != nil || ok {
		return got, err
	}
	p.ProcessedAt = when
	if err := insertProcessed(ctx, tx, p, res); err != nil {
		return ReconcileResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ReconcileResult{}, wrap(CodeTransactionFailed, "processed", "decision commit failed", err)
	}
	return res, nil
}
