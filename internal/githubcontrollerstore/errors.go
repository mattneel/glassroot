package githubcontrollerstore

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeInvalidStateDir            ErrorCode = "invalid-state-dir"
	CodeDatabasePathInvalid        ErrorCode = "database-path-invalid"
	CodeDatabaseSymlink            ErrorCode = "database-symlink"
	CodeDatabaseModeInvalid        ErrorCode = "database-mode-invalid"
	CodeDatabaseOpenFailed         ErrorCode = "database-open-failed"
	CodeDatabasePragmasInvalid     ErrorCode = "database-pragmas-invalid"
	CodeDatabaseSchemaNewer        ErrorCode = "database-schema-newer"
	CodeDatabaseSchemaInvalid      ErrorCode = "database-schema-invalid"
	CodeDatabaseControllerMismatch ErrorCode = "database-controller-mismatch"
	CodeDatabaseReceiverMismatch   ErrorCode = "database-receiver-mismatch"
	CodeDatabaseAppMismatch        ErrorCode = "database-app-mismatch"
	CodeDatabaseCorrupt            ErrorCode = "database-corrupt"
	CodeDatabaseBusy               ErrorCode = "database-busy"
	CodeDatabaseFull               ErrorCode = "database-full"
	CodeMigrationFailed            ErrorCode = "migration-failed"
	CodeRecordInvalid              ErrorCode = "record-invalid"
	CodeRecordDigestMismatch       ErrorCode = "record-digest-mismatch"
	CodeTransactionFailed          ErrorCode = "transaction-failed"
	CodeProcessedDeliveryConflict  ErrorCode = "processed-delivery-conflict"
	CodeReconcileLeaseBusy         ErrorCode = "reconcile-lease-busy"
	CodeReconcileLeaseStale        ErrorCode = "reconcile-lease-stale"
	CodeGenerationOverflow         ErrorCode = "generation-overflow"
	CodeDuplicateTarget            ErrorCode = "duplicate-target"
	CodeDuplicateJob               ErrorCode = "duplicate-job"
	CodeDuplicateAttempt           ErrorCode = "duplicate-attempt"
	CodeSourceStateInvalid         ErrorCode = "source-state-invalid"
	CodeSourceLeaseStale           ErrorCode = "source-lease-stale"
	CodeCheckBindingConflict       ErrorCode = "check-binding-conflict"
	CodeWorkerResultStale          ErrorCode = "worker-result-stale"
	CodeContextCancelled           ErrorCode = "context-cancelled"
	CodeCloseFailed                ErrorCode = "close-failed"
)

type StoreError struct {
	Code       ErrorCode
	Stage, Msg string
	Err        error
}

func (e *StoreError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Msg)
}
func (e *StoreError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
func (e *StoreError) Is(target error) bool {
	var t *StoreError
	return errors.As(target, &t) && t.Code == e.Code
}
func ErrCode(code ErrorCode) error { return &StoreError{Code: code} }
func errCode(code ErrorCode, stage, msg string, err error) error {
	return &StoreError{Code: code, Stage: stage, Msg: msg, Err: err}
}
func wrap(code ErrorCode, stage, msg string, err error) error { return errCode(code, stage, msg, err) }

func Diagnostic(err error) string {
	var e *StoreError
	if errors.As(err, &e) {
		return string(e.Code)
	}
	if err != nil {
		return "controller-store-failed"
	}
	return ""
}
