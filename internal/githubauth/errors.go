package githubauth

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

type ErrorCode string

const (
	CodeInvalidAppIdentity        ErrorCode = "invalid-app-identity"
	CodeInvalidPrivateKeyPath     ErrorCode = "invalid-private-key-path"
	CodePrivateKeySymlink         ErrorCode = "private-key-symlink"
	CodePrivateKeyModeInvalid     ErrorCode = "private-key-mode-invalid"
	CodePrivateKeyOwnerInvalid    ErrorCode = "private-key-owner-invalid"
	CodePrivateKeyHardlink        ErrorCode = "private-key-hardlink"
	CodePrivateKeySizeInvalid     ErrorCode = "private-key-size-invalid"
	CodePrivateKeyReadFailed      ErrorCode = "private-key-read-failed"
	CodePrivateKeyFormatInvalid   ErrorCode = "private-key-format-invalid"
	CodePrivateKeyTypeUnsupported ErrorCode = "private-key-type-unsupported"
	CodePrivateKeyStrengthInvalid ErrorCode = "private-key-strength-invalid"
	CodeJWTTimeInvalid            ErrorCode = "jwt-time-invalid"
	CodeJWTSignFailed             ErrorCode = "jwt-sign-failed"
	CodeContextCancelled          ErrorCode = "context-cancelled"
)

type Error struct {
	Code    ErrorCode
	Stage   string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return "githubauth error"
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
		return string(CodeInvalidAppIdentity)
	}
	return string(CodeInvalidAppIdentity)
}
