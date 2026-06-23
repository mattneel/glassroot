package githubreceiver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type ErrorCode string

const (
	CodeUnsupportedPlatform        ErrorCode = "unsupported-platform"
	CodeInvalidConfig              ErrorCode = "invalid-config"
	CodeInvalidListenerPath        ErrorCode = "invalid-listener-path"
	CodeListenerParentInvalid      ErrorCode = "listener-parent-invalid"
	CodeListenerPathExists         ErrorCode = "listener-path-exists"
	CodeListenerCreateFailed       ErrorCode = "listener-create-failed"
	CodeListenerChanged            ErrorCode = "listener-changed"
	CodeInvalidStateDir            ErrorCode = "invalid-state-dir"
	CodeInvalidReceiverID          ErrorCode = "invalid-receiver-id"
	CodeInvalidSecretPath          ErrorCode = "invalid-secret-path"
	CodeSecretSymlink              ErrorCode = "secret-symlink"
	CodeSecretModeInvalid          ErrorCode = "secret-mode-invalid"
	CodeSecretOwnerInvalid         ErrorCode = "secret-owner-invalid"
	CodeSecretSizeInvalid          ErrorCode = "secret-size-invalid"
	CodeSecretReadFailed           ErrorCode = "secret-read-failed"
	CodeDuplicateSecret            ErrorCode = "duplicate-secret"
	CodeRequestCapacity            ErrorCode = "request-capacity"
	CodeInvalidMethod              ErrorCode = "invalid-method"
	CodeInvalidPath                ErrorCode = "invalid-path"
	CodeQueryNotAllowed            ErrorCode = "query-not-allowed"
	CodeInvalidContentType         ErrorCode = "invalid-content-type"
	CodeUnsupportedContentEncoding ErrorCode = "unsupported-content-encoding"
	CodeBodyTooLarge               ErrorCode = "body-too-large"
	CodeBodyReadFailed             ErrorCode = "body-read-failed"
	CodeSignatureInvalid           ErrorCode = "signature-invalid"
	CodeProjectionInvalid          ErrorCode = "projection-invalid"
	CodeStoreUnavailable           ErrorCode = "store-unavailable"
	CodeIntakeTimeout              ErrorCode = "intake-timeout"
	CodeResponseWriteFailed        ErrorCode = "response-write-failed"
	CodeShutdownFailed             ErrorCode = "shutdown-failed"
	CodeCleanupFailed              ErrorCode = "cleanup-failed"
	CodeContextCancelled           ErrorCode = "context-cancelled"
)

type Error struct {
	Code    ErrorCode
	Stage   string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return "githubreceiver error"
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
		return string(CodeInvalidConfig)
	}
	return string(CodeInvalidConfig)
}
