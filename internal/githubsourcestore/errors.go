package githubsourcestore

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeInvalidSourceRoot      ErrorCode = "invalid-source-root"
	CodeSourceRootModeInvalid  ErrorCode = "source-root-mode-invalid"
	CodeInvalidSourceStoreID   ErrorCode = "invalid-source-store-id"
	CodeStagingCreateFailed    ErrorCode = "staging-create-failed"
	CodeMetadataInvalid        ErrorCode = "metadata-invalid"
	CodeMetadataDigestMismatch ErrorCode = "metadata-digest-mismatch"
	CodeStoreLayoutInvalid     ErrorCode = "store-layout-invalid"
	CodeStorePermissionInvalid ErrorCode = "store-permission-invalid"
	CodeStoreConflict          ErrorCode = "store-conflict"
	CodeStoreCorrupt           ErrorCode = "store-corrupt"
	CodePublishCollision       ErrorCode = "publish-collision"
	CodePublishFailed          ErrorCode = "publish-failed"
	CodeSyncFailed             ErrorCode = "sync-failed"
	CodeCleanupFailed          ErrorCode = "cleanup-failed"
	CodeOpenFailed             ErrorCode = "open-failed"
	CodeContextCancelled       ErrorCode = "context-cancelled"
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
		return "source-store-failed"
	}
	return ""
}
