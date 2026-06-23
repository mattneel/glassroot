package githubbroker

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

type ErrorCode string

const (
	CodeUnsupportedPlatform       ErrorCode = "unsupported-platform"
	CodeInvalidLimits             ErrorCode = "invalid-limits"
	CodeInvalidConfig             ErrorCode = "invalid-config"
	CodeInvalidListenerPath       ErrorCode = "invalid-listener-path"
	CodeListenerParentInvalid     ErrorCode = "listener-parent-invalid"
	CodeListenerPathExists        ErrorCode = "listener-path-exists"
	CodeListenerCreateFailed      ErrorCode = "listener-create-failed"
	CodeListenerChanged           ErrorCode = "listener-changed"
	CodePeerCredentialUnavailable ErrorCode = "peer-credential-unavailable"
	CodePeerUIDRejected           ErrorCode = "peer-uid-rejected"
	CodeConnectionLimit           ErrorCode = "connection-limit"
	CodeRequestTimeout            ErrorCode = "request-timeout"
	CodeRequestFrameInvalid       ErrorCode = "request-frame-invalid"
	CodeRequestTooLarge           ErrorCode = "request-too-large"
	CodeUnsupportedSchemaVersion  ErrorCode = "unsupported-schema-version"
	CodeInvalidTokenPurpose       ErrorCode = "invalid-token-purpose"
	CodeInvalidInstallationID     ErrorCode = "invalid-installation-id"
	CodeInvalidRepositoryID       ErrorCode = "invalid-repository-id"
	CodeTokenIssueFailed          ErrorCode = "token-issue-failed"
	CodeResponseWriteFailed       ErrorCode = "response-write-failed"
	CodeClientConnectionFailed    ErrorCode = "client-connection-failed"
	CodeClientResponseInvalid     ErrorCode = "client-response-invalid"
	CodeTokenLeaseClosed          ErrorCode = "token-lease-closed"
	CodeShutdownFailed            ErrorCode = "shutdown-failed"
	CodeCleanupFailed             ErrorCode = "cleanup-failed"
	CodeContextCancelled          ErrorCode = "context-cancelled"
)

type Error struct {
	Code           ErrorCode
	Stage, Message string
	Err            error
}

func (e *Error) Error() string {
	if e == nil {
		return "githubbroker error"
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
func wrap(code ErrorCode, stage, msg string, err error) error { return errCode(code, stage, msg, err) }
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
		return string(e.Code)
	}
	if err == nil {
		return string(CodeInvalidConfig)
	}
	return string(CodeInvalidConfig)
}
