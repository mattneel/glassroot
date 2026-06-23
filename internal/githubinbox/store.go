package githubinbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mattneel/glassroot/internal/githubapp"
	_ "modernc.org/sqlite"
)

const (
	databaseFileName = "github-webhook.sqlite"
	applicationID    = 0x4752315a
	schemaVersion    = 1
)

type Store struct {
	mu         sync.Mutex
	db         *sql.DB
	receiverID string
	limits     Limits
}

func Open(ctx context.Context, cfg Config) (*Store, error) {
	limits := cfg.Limits
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := validateLimits(limits); err != nil {
		return nil, err
	}
	if err := validateReceiverID(cfg.ReceiverID); err != nil {
		return nil, err
	}
	if err := validateStateDir(cfg.StateDir, limits); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(cfg.StateDir, databaseFileName)
	if filepath.Clean(dbPath) != dbPath {
		return nil, errCode(CodeDatabasePathInvalid, "open", "database path rejected", nil)
	}
	if st, err := os.Lstat(dbPath); err == nil {
		if st.Mode()&os.ModeSymlink != 0 {
			return nil, errCode(CodeDatabaseSymlink, "open", "database path rejected", nil)
		}
		if !st.Mode().IsRegular() {
			return nil, errCode(CodeDatabasePathInvalid, "open", "database file rejected", nil)
		}
	}
	dsn := sqliteDSN(dbPath, limits)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, wrap(CodeDatabaseOpenFailed, "open", "database open failed", err)
	}
	db.SetMaxOpenConns(limits.MaxOpenConnections)
	db.SetMaxIdleConns(limits.MaxIdleConnections)
	s := &Store{db: db, receiverID: cfg.ReceiverID, limits: limits}
	if err := s.initialize(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if st, err := os.Stat(dbPath); err == nil {
		if st.Mode().Perm() != 0o600 {
			_ = os.Chmod(dbPath, 0o600)
		}
	}
	return s, nil
}

func sqliteDSN(path string, limits Limits) string {
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("mode", "rwc")
	q.Set("cache", "shared")
	for _, p := range []string{
		"foreign_keys(ON)", "journal_mode(WAL)", "synchronous(FULL)", "trusted_schema(OFF)", "recursive_triggers(OFF)", fmt.Sprintf("busy_timeout(%d)", limits.BusyTimeoutMilliseconds), fmt.Sprintf("max_page_count(%d)", limits.MaxDatabaseBytes/4096),
	} {
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
	if err := s.bindMetadata(ctx); err != nil {
		return err
	}
	if err := s.quickCheck(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Store) execPragmas(ctx context.Context) error {
	pragmas := []string{
		fmt.Sprintf("PRAGMA application_id=%d", applicationID),
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=FULL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA trusted_schema=OFF",
		"PRAGMA recursive_triggers=OFF",
		fmt.Sprintf("PRAGMA busy_timeout=%d", s.limits.BusyTimeoutMilliseconds),
		fmt.Sprintf("PRAGMA max_page_count=%d", s.limits.MaxDatabaseBytes/4096),
	}
	for _, p := range pragmas {
		if _, err := s.db.ExecContext(ctx, p); err != nil {
			return wrap(CodeDatabasePragmasInvalid, "pragmas", "database pragma failed", err)
		}
	}
	return nil
}

func (s *Store) bindMetadata(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wrap(CodeTransactionFailed, "metadata", "metadata transaction failed", err)
	}
	defer rollback(tx)
	pairs := map[string]string{"schema_identity": SchemaInboxStoreV1Alpha1, "receiver_id": s.receiverID, "schema_version": fmt.Sprintf("%d", schemaVersion)}
	for k, v := range pairs {
		var existing string
		err := tx.QueryRowContext(ctx, `SELECT value FROM github_metadata WHERE key = ?`, k).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err := tx.ExecContext(ctx, `INSERT INTO github_metadata(key,value) VALUES(?,?)`, k, v); err != nil {
				return wrap(CodeMigrationFailed, "metadata", "metadata insert failed", err)
			}
			continue
		}
		if err != nil {
			return wrap(CodeDatabaseSchemaInvalid, "metadata", "metadata read failed", err)
		}
		if existing != v {
			if k == "receiver_id" {
				return errCode(CodeDatabaseReceiverMismatch, "metadata", "receiver id mismatch", nil)
			}
			return errCode(CodeDatabaseSchemaInvalid, "metadata", "schema metadata mismatch", nil)
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
		return errCode(CodeDatabaseCorrupt, "quick-check", "quick_check rejected database", nil)
	}
	return nil
}

func (s *Store) Accept(ctx context.Context, d VerifiedDelivery) (AcceptResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateDelivery(s.receiverID, d); err != nil {
		return AcceptResult{}, err
	}
	projectionJSON, err := compactJSON(d.Projection)
	if err != nil {
		return AcceptResult{}, err
	}
	receiptJSON, err := compactJSON(d.Receipt)
	if err != nil {
		return AcceptResult{}, err
	}
	projectionDigest := digestBytes("glassroot.dev/github-projection-json/v1\x00", projectionJSON)
	recordDigest := recordDigest(d, receiptJSON, projectionJSON)
	outboxID := ""
	if d.Disposition == githubapp.DeliveryDispositionEnqueued {
		outboxID = computeOutboxID(d.ReceiverID, d.DeliveryID, d.IntakeFingerprint, string(d.Projection.Kind))
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return AcceptResult{}, wrap(CodeTransactionFailed, "accept", "accept transaction failed", err)
	}
	defer rollback(tx)
	res, err := tx.ExecContext(ctx, insertInboxSQL, d.ReceiverID, d.DeliveryID, d.BodyDigest, d.IntakeFingerprint, d.Event, nullableAction(d.Action), string(d.Projection.Kind), string(d.Disposition), string(d.MatchedSecret), formatTime(d.ReceivedAt), projectionID(d, "installation"), projectionID(d, "repository"), projectionID(d, "pr"), projectionID(d, "check"), projectionSHA(d, "base"), projectionSHA(d, "head"), string(receiptJSON), string(projectionJSON), recordDigest, formatTime(d.ReceivedAt))
	if err != nil {
		return AcceptResult{}, wrap(CodeTransactionFailed, "accept", "inbox insert failed", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		var existingFingerprint string
		if err := tx.QueryRowContext(ctx, `SELECT intake_fingerprint FROM github_inbox WHERE receiver_id=? AND delivery_id=?`, d.ReceiverID, d.DeliveryID).Scan(&existingFingerprint); err != nil {
			return AcceptResult{}, wrap(CodeTransactionFailed, "accept", "existing inbox read failed", err)
		}
		if existingFingerprint == d.IntakeFingerprint {
			if err := tx.Commit(); err != nil {
				return AcceptResult{}, wrap(CodeTransactionFailed, "accept", "duplicate commit failed", err)
			}
			return AcceptResult{Decision: AcceptDuplicateSameDelivery}, nil
		}
		_, _ = tx.ExecContext(ctx, `UPDATE github_inbox SET conflict_count = conflict_count + 1 WHERE receiver_id=? AND delivery_id=?`, d.ReceiverID, d.DeliveryID)
		if err := tx.Commit(); err != nil {
			return AcceptResult{}, wrap(CodeTransactionFailed, "accept", "conflict commit failed", err)
		}
		return AcceptResult{Decision: AcceptDeliveryConflict}, errCode(CodeDeliveryConflict, "accept", "delivery id conflict", nil)
	}
	if d.Disposition == githubapp.DeliveryDispositionEnqueued {
		if _, err := tx.ExecContext(ctx, insertOutboxSQL, outboxID, d.ReceiverID, d.DeliveryID, string(d.Projection.Kind), string(projectionJSON), projectionDigest, string(receiptJSON), formatTime(d.ReceivedAt)); err != nil {
			return AcceptResult{}, wrap(CodeTransactionFailed, "accept", "outbox insert failed", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return AcceptResult{}, wrap(CodeTransactionFailed, "accept", "accept commit failed", err)
	}
	if d.Disposition == githubapp.DeliveryDispositionEnqueued {
		return AcceptResult{Decision: AcceptNewEnqueued, OutboxID: outboxID}, nil
	}
	return AcceptResult{Decision: AcceptNewIgnored}, nil
}

func validateDelivery(receiverID string, d VerifiedDelivery) error {
	if d.ReceiverID != receiverID || d.DeliveryID == "" || d.Event == "" || !validDigest(d.BodyDigest) || !validDigest(d.IntakeFingerprint) || d.Receipt.SchemaVersion != githubapp.SchemaGitHubWebhookReceiptV1Alpha1 || d.Receipt.DeliveryID != d.DeliveryID || d.Receipt.Event != d.Event || d.Receipt.BodyDigest != d.BodyDigest || d.Receipt.ProjectionKind != d.Projection.Kind {
		return errCode(CodeRecordInvalid, "delivery", "delivery record rejected", nil)
	}
	if d.MatchedSecret != githubapp.SecretGenerationCurrent && d.MatchedSecret != githubapp.SecretGenerationPrevious {
		return errCode(CodeRecordInvalid, "delivery", "secret generation rejected", nil)
	}
	if d.ReceivedAt.IsZero() || d.ReceivedAt.Location() != time.UTC {
		return errCode(CodeRecordInvalid, "delivery", "received time rejected", nil)
	}
	if d.Disposition != githubapp.DeliveryDispositionEnqueued && d.Disposition != githubapp.DeliveryDispositionIgnored {
		return errCode(CodeRecordInvalid, "delivery", "disposition rejected", nil)
	}
	return nil
}

func compactJSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, wrap(CodeSerializationFailed, "json", "json serialization failed", err)
	}
	return b, nil
}
func recordDigest(d VerifiedDelivery, receipt, projection []byte) string {
	return digestBytes("glassroot.dev/github-inbox-record/v1\x00", []byte(strings.Join([]string{d.ReceiverID, d.DeliveryID, d.IntakeFingerprint, string(receipt), string(projection)}, "\x00")))
}
func nullableAction(a string) any {
	if a == "" {
		return nil
	}
	return a
}
func formatTime(t time.Time) string { return t.UTC().Round(0).Format(time.RFC3339Nano) }
func parseTime(s string) time.Time  { t, _ := time.Parse(time.RFC3339Nano, s); return t.UTC() }
func rollback(tx *sql.Tx)           { _ = tx.Rollback() }

func projectionID(d VerifiedDelivery, which string) any {
	switch p := d.Projection; which {
	case "installation":
		if p.PullRequest != nil {
			return p.PullRequest.InstallationID
		}
		if p.CheckRun != nil {
			return p.CheckRun.InstallationID
		}
		if p.Installation != nil {
			return p.Installation.InstallationID
		}
	case "repository":
		if p.PullRequest != nil {
			return p.PullRequest.RepositoryID
		}
		if p.CheckRun != nil {
			return p.CheckRun.RepositoryID
		}
	case "pr":
		if p.PullRequest != nil {
			return p.PullRequest.PullRequestNumber
		}
	case "check":
		if p.CheckRun != nil {
			return p.CheckRun.CheckRunID
		}
	}
	return nil
}
func projectionSHA(d VerifiedDelivery, which string) any {
	if d.Projection.PullRequest != nil {
		if which == "base" {
			return d.Projection.PullRequest.BaseSHA
		}
		return d.Projection.PullRequest.HeadSHA
	}
	if d.Projection.CheckRun != nil && which == "head" {
		return d.Projection.CheckRun.HeadSHA
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
