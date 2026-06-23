package evidence

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type ErrorCode string

const (
	CodeUnsupportedPlatform    ErrorCode = "unsupported-platform"
	CodeInvalidParent          ErrorCode = "invalid-parent"
	CodeParentNotDirectory     ErrorCode = "parent-not-directory"
	CodeParentSymlink          ErrorCode = "parent-symlink"
	CodeInvalidPlan            ErrorCode = "invalid-plan"
	CodeInvalidLimits          ErrorCode = "invalid-limits"
	CodeInvalidSessionState    ErrorCode = "invalid-session-state"
	CodeStagingCreateFailed    ErrorCode = "staging-create-failed"
	CodeStagingOpenFailed      ErrorCode = "staging-open-failed"
	CodeInvalidAttempt         ErrorCode = "invalid-attempt"
	CodeInvalidEntryPath       ErrorCode = "invalid-entry-path"
	CodeDestinationEntryExists ErrorCode = "destination-entry-exists"
	CodeInvalidEvent           ErrorCode = "invalid-event"
	CodeEventOrder             ErrorCode = "event-order"
	CodeDuplicateEventID       ErrorCode = "duplicate-event-id"
	CodeEventTooLarge          ErrorCode = "event-too-large"
	CodeEventLimit             ErrorCode = "event-limit"
	CodeEventWriteFailed       ErrorCode = "event-write-failed"
	CodeInvalidLogStream       ErrorCode = "invalid-log-stream"
	CodeDuplicateLogCapture    ErrorCode = "duplicate-log-capture"
	CodeLogWriteFailed         ErrorCode = "log-write-failed"
	CodeLogLimit               ErrorCode = "log-limit"
	CodeInvalidArtifact        ErrorCode = "invalid-artifact"
	CodeDuplicateArtifact      ErrorCode = "duplicate-artifact"
	CodeArtifactLimit          ErrorCode = "artifact-limit"
	CodeArtifactWriteFailed    ErrorCode = "artifact-write-failed"
	CodeResultInvalid          ErrorCode = "result-invalid"
	CodeCompletionInvalid      ErrorCode = "completion-invalid"
	CodeSerializationFailed    ErrorCode = "serialization-failed"
	CodeManifestInvariant      ErrorCode = "manifest-invariant"
	CodeManifestTooLarge       ErrorCode = "manifest-too-large"
	CodeSyncFailed             ErrorCode = "sync-failed"
	CodePublishCollision       ErrorCode = "publish-collision"
	CodePublishFailed          ErrorCode = "publish-failed"
	CodeContextCancelled       ErrorCode = "context-cancelled"
	CodeWriteTimeout           ErrorCode = "write-timeout"
	CodeCleanupFailed          ErrorCode = "cleanup-failed"
)

func (c ErrorCode) Error() string { return string(c) }

var (
	ErrUnsupportedPlatform error = CodeUnsupportedPlatform
	ErrInvalidParent       error = CodeInvalidParent
	ErrInvalidPlan         error = CodeInvalidPlan
	ErrInvalidSessionState error = CodeInvalidSessionState
	ErrInvalidAttempt      error = CodeInvalidAttempt
	ErrInvalidEvent        error = CodeInvalidEvent
	ErrEventOrder          error = CodeEventOrder
	ErrCompletionInvalid   error = CodeCompletionInvalid
	ErrSyncFailed          error = CodeSyncFailed
	ErrCleanupFailed       error = CodeCleanupFailed
)

type Error struct {
	Code    ErrorCode
	Stage   string
	Op      string
	Attempt string
	Path    string
	Msg     string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return "evidence error"
	}
	parts := []string{string(e.Code)}
	for _, part := range []string{e.Stage, e.Op, e.Attempt, e.Path} {
		if part != "" {
			parts = append(parts, sanitize(part, 160))
		}
	}
	if e.Msg != "" {
		parts = append(parts, sanitize(e.Msg, 256))
	} else if e.Err != nil {
		parts = append(parts, sanitize(e.Err.Error(), 256))
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
	code, ok := target.(ErrorCode)
	return ok && e != nil && e.Code == code
}

func errCode(code ErrorCode, stage, op, msg string, err error) error {
	return &Error{Code: code, Stage: stage, Op: op, Msg: msg, Err: err}
}
func pathErr(code ErrorCode, stage, op, path, msg string, err error) error {
	return &Error{Code: code, Stage: stage, Op: op, Path: path, Msg: msg, Err: err}
}
func attemptErr(code ErrorCode, stage, op, attempt, msg string, err error) error {
	return &Error{Code: code, Stage: stage, Op: op, Attempt: attempt, Msg: msg, Err: err}
}

func contextErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errCode(CodeWriteTimeout, "context", "deadline", "evidence write deadline exceeded", err)
	}
	return errCode(CodeContextCancelled, "context", "cancelled", "context cancelled", err)
}

func sanitize(s string, max int) string {
	if max <= 0 {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= max {
			break
		}
		if r == '\n' || r == '\t' || (r >= 0x20 && r != 0x7f) {
			b.WriteRune(r)
		} else {
			b.WriteString(fmt.Sprintf("\\x%02x", r))
		}
	}
	if len(s) > b.Len() {
		b.WriteString("...")
	}
	return b.String()
}
