package githubinbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type ErrorCode string

const (
	CodeInvalidLimits            ErrorCode = "invalid-limits"
	CodeInvalidStateDir          ErrorCode = "invalid-state-dir"
	CodeDatabasePathInvalid      ErrorCode = "database-path-invalid"
	CodeDatabaseSymlink          ErrorCode = "database-symlink"
	CodeDatabaseModeInvalid      ErrorCode = "database-mode-invalid"
	CodeDatabaseOpenFailed       ErrorCode = "database-open-failed"
	CodeDatabasePragmasInvalid   ErrorCode = "database-pragmas-invalid"
	CodeDatabaseSchemaNewer      ErrorCode = "database-schema-newer"
	CodeDatabaseSchemaInvalid    ErrorCode = "database-schema-invalid"
	CodeDatabaseReceiverMismatch ErrorCode = "database-receiver-mismatch"
	CodeDatabaseCorrupt          ErrorCode = "database-corrupt"
	CodeDatabaseBusy             ErrorCode = "database-busy"
	CodeDatabaseFull             ErrorCode = "database-full"
	CodeMigrationFailed          ErrorCode = "migration-failed"
	CodeRecordInvalid            ErrorCode = "record-invalid"
	CodeRecordDigestMismatch     ErrorCode = "record-digest-mismatch"
	CodeTransactionFailed        ErrorCode = "transaction-failed"
	CodeDeliveryConflict         ErrorCode = "delivery-conflict"
	CodeOutboxStateInvalid       ErrorCode = "outbox-state-invalid"
	CodeLeaseOwnerInvalid        ErrorCode = "lease-owner-invalid"
	CodeLeaseDurationInvalid     ErrorCode = "lease-duration-invalid"
	CodeStaleLease               ErrorCode = "stale-lease"
	CodeLeaseGenerationOverflow  ErrorCode = "lease-generation-overflow"
	CodeAttemptCountOverflow     ErrorCode = "attempt-count-overflow"
	CodeSerializationFailed      ErrorCode = "serialization-failed"
	CodeContextCancelled         ErrorCode = "context-cancelled"
	CodeCloseFailed              ErrorCode = "close-failed"
)

type Error struct {
	Code    ErrorCode
	Stage   string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return "githubinbox error"
	}
	parts := []string{string(e.Code)}
	if e.Stage != "" {
		parts = append(parts, sanitize(e.Stage, 64))
	}
	if e.Message != "" {
		parts = append(parts, sanitize(e.Message, 192))
	}
	return strings.Join(parts, ": ")
}
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && e != nil && t.Code != "" && e.Code == t.Code
}
func ErrCode(code ErrorCode) error { return &Error{Code: code} }

func errCode(code ErrorCode, stage, msg string, err error) error {
	if errors.Is(err, context.Canceled) {
		return &Error{Code: CodeContextCancelled, Stage: stage, Message: "context cancelled", Err: err}
	}
	return &Error{Code: code, Stage: stage, Message: msg, Err: err}
}

func wrap(code ErrorCode, stage, msg string, err error) error {
	if err == nil {
		return errCode(code, stage, msg, nil)
	}
	return &Error{Code: code, Stage: stage, Message: msg, Err: err}
}

func sanitize(s string, max int) string {
	if max <= 0 {
		max = 192
	}
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "?")
	}
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= max {
			break
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			b.WriteByte('?')
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if len(out) > max {
		return out[:max]
	}
	return out
}

func Diagnostic(err error) string {
	var e *Error
	if errors.As(err, &e) {
		if e.Message != "" {
			return fmt.Sprintf("%s: %s", e.Code, sanitize(e.Message, 192))
		}
		return string(e.Code)
	}
	if err == nil {
		return string(CodeRecordInvalid)
	}
	return string(CodeRecordInvalid)
}
