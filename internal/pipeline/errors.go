package pipeline

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type ErrorCode string

const (
	CodeContextCancelled           ErrorCode = "context-cancelled"
	CodeInvalidRunID               ErrorCode = "invalid-run-id"
	CodeInvalidCreatedAt           ErrorCode = "invalid-created-at"
	CodeInvalidTrustedConfig       ErrorCode = "invalid-trusted-config"
	CodeTrustedConfigMismatch      ErrorCode = "trusted-config-mismatch"
	CodeInvalidSourceSnapshot      ErrorCode = "invalid-source-snapshot"
	CodeRevisionMismatch           ErrorCode = "revision-mismatch"
	CodeInvalidObjectFormat        ErrorCode = "invalid-object-format"
	CodeInvalidObjectID            ErrorCode = "invalid-object-id"
	CodeInvalidSourceDigest        ErrorCode = "invalid-source-digest"
	CodeInvalidSourceSummary       ErrorCode = "invalid-source-summary"
	CodeInvalidPlatformConstraints ErrorCode = "invalid-platform-constraints"
	CodePlatformLimitExceeded      ErrorCode = "platform-limit-exceeded"
	CodeUnsupportedNetworkPolicy   ErrorCode = "unsupported-network-policy"
	CodeModelInvariant             ErrorCode = "model-invariant"
	CodePlanTooLarge               ErrorCode = "plan-too-large"
	CodeSerializationFailed        ErrorCode = "serialization-failed"
)

type sentinel ErrorCode

func (s sentinel) Error() string { return string(s) }

const (
	ErrContextCancelled           sentinel = sentinel(CodeContextCancelled)
	ErrInvalidRunID               sentinel = sentinel(CodeInvalidRunID)
	ErrInvalidCreatedAt           sentinel = sentinel(CodeInvalidCreatedAt)
	ErrInvalidTrustedConfig       sentinel = sentinel(CodeInvalidTrustedConfig)
	ErrTrustedConfigMismatch      sentinel = sentinel(CodeTrustedConfigMismatch)
	ErrInvalidSourceSnapshot      sentinel = sentinel(CodeInvalidSourceSnapshot)
	ErrRevisionMismatch           sentinel = sentinel(CodeRevisionMismatch)
	ErrInvalidObjectFormat        sentinel = sentinel(CodeInvalidObjectFormat)
	ErrInvalidObjectID            sentinel = sentinel(CodeInvalidObjectID)
	ErrInvalidSourceDigest        sentinel = sentinel(CodeInvalidSourceDigest)
	ErrInvalidSourceSummary       sentinel = sentinel(CodeInvalidSourceSummary)
	ErrInvalidPlatformConstraints sentinel = sentinel(CodeInvalidPlatformConstraints)
	ErrPlatformLimitExceeded      sentinel = sentinel(CodePlatformLimitExceeded)
	ErrUnsupportedNetworkPolicy   sentinel = sentinel(CodeUnsupportedNetworkPolicy)
	ErrModelInvariant             sentinel = sentinel(CodeModelInvariant)
	ErrPlanTooLarge               sentinel = sentinel(CodePlanTooLarge)
	ErrSerializationFailed        sentinel = sentinel(CodeSerializationFailed)
)

type Error struct {
	Code  ErrorCode
	Stage string
	Path  string
	Err   error
}

func (e *Error) Error() string {
	if e == nil {
		return "pipeline error"
	}
	parts := []string{string(e.Code)}
	if e.Stage != "" {
		parts = append(parts, sanitize(e.Stage, 96))
	}
	if e.Path != "" {
		parts = append(parts, sanitize(e.Path, 192))
	}
	if e.Err != nil {
		parts = append(parts, sanitize(e.Err.Error(), 256))
	}
	return strings.Join(parts, ": ")
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) Is(target error) bool {
	s, ok := target.(sentinel)
	return ok && e != nil && ErrorCode(s) == e.Code
}

func errCode(code ErrorCode, stage, path, msg string, cause error) error {
	if cause == nil && msg != "" {
		cause = errors.New(msg)
	} else if cause != nil && msg != "" {
		cause = fmt.Errorf("%s: %w", msg, cause)
	}
	return &Error{Code: code, Stage: stage, Path: path, Err: cause}
}

func sanitize(s string, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = 256
	}
	var b strings.Builder
	for len(s) > 0 && b.Len() < maxBytes {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			if b.Len()+6 > maxBytes {
				break
			}
			fmt.Fprintf(&b, "\\x%02x", s[0])
			s = s[1:]
			continue
		}
		s = s[size:]
		if r < 0x20 || r == 0x7f {
			if b.Len()+6 > maxBytes {
				break
			}
			fmt.Fprintf(&b, "\\u%04x", r)
			continue
		}
		if b.Len()+size > maxBytes {
			break
		}
		b.WriteRune(r)
	}
	if len(s) > 0 && b.Len()+1 <= maxBytes {
		b.WriteRune('…')
	}
	return b.String()
}
