package githubapi

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

type ErrorCode string

const (
	CodeAPIUnavailable                     ErrorCode = "api-unavailable"
	CodeAPITimeout                         ErrorCode = "api-timeout"
	CodeAPITLSFailed                       ErrorCode = "api-tls-failed"
	CodeAPIRedirectRejected                ErrorCode = "api-redirect-rejected"
	CodeAPIVersionUnsupported              ErrorCode = "api-version-unsupported"
	CodeAPIRateLimited                     ErrorCode = "api-rate-limited"
	CodeAppIdentityMismatch                ErrorCode = "app-identity-mismatch"
	CodeAppPermissionsMismatch             ErrorCode = "app-permissions-mismatch"
	CodeInstallationNotFound               ErrorCode = "installation-not-found"
	CodeInstallationMismatch               ErrorCode = "installation-mismatch"
	CodeInstallationSuspended              ErrorCode = "installation-suspended"
	CodeInstallationPermissionInsufficient ErrorCode = "installation-permission-insufficient"
	CodeTokenRequestRejected               ErrorCode = "token-request-rejected"
	CodeTokenResponseInvalid               ErrorCode = "token-response-invalid"
	CodeTokenScopeMismatch                 ErrorCode = "token-scope-mismatch"
	CodeTokenExpiryInvalid                 ErrorCode = "token-expiry-invalid"
	CodeResponseTooLarge                   ErrorCode = "response-too-large"
	CodeResponseInvalid                    ErrorCode = "response-invalid"
	CodeContextCancelled                   ErrorCode = "context-cancelled"
)

type Error struct {
	Code           ErrorCode
	Stage, Message string
	Err            error
}

func (e *Error) Error() string {
	if e == nil {
		return "githubapi error"
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
		return string(CodeAPIUnavailable)
	}
	return string(CodeAPIUnavailable)
}
