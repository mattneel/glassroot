package waiver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type ErrorCode string

const (
	CodeInputTooLarge          ErrorCode = "input-too-large"
	CodeInvalidUTF8            ErrorCode = "invalid-utf8"
	CodeNULByte                ErrorCode = "nul-byte"
	CodeYAMLSyntax             ErrorCode = "yaml-syntax"
	CodeMultipleDocuments      ErrorCode = "multiple-documents"
	CodeDuplicateKey           ErrorCode = "duplicate-key"
	CodeUnknownField           ErrorCode = "unknown-field"
	CodeUnsupportedYAMLFeature ErrorCode = "unsupported-yaml-feature"
	CodeMissingRequiredField   ErrorCode = "missing-required-field"
	CodeInvalidAPIVersion      ErrorCode = "invalid-api-version"
	CodeInvalidKind            ErrorCode = "invalid-kind"
	CodeInvalidValue           ErrorCode = "invalid-value"
	CodeInvalidWaiverID        ErrorCode = "invalid-waiver-id"
	CodeInvalidFindingID       ErrorCode = "invalid-finding-id"
	CodeInvalidRuleID          ErrorCode = "invalid-rule-id"
	CodeDuplicateWaiverID      ErrorCode = "duplicate-waiver-id"
	CodeDuplicateWaiverTarget  ErrorCode = "duplicate-waiver-target"
	CodeOverlyBroadWaiver      ErrorCode = "overly-broad-waiver"
	CodeInvalidOwner           ErrorCode = "invalid-owner"
	CodeInvalidReason          ErrorCode = "invalid-reason"
	CodeInvalidTime            ErrorCode = "invalid-time"
	CodeInvalidLifetime        ErrorCode = "invalid-lifetime"
	CodeUnsupportedEntryKind   ErrorCode = "unsupported-entry-kind"
	CodeBaseReadFailed         ErrorCode = "base-read-failed"
	CodeHeadInspectionFailed   ErrorCode = "head-inspection-failed"
	CodeContextCancelled       ErrorCode = "context-cancelled"
)

type sentinel ErrorCode

func (s sentinel) Error() string { return string(s) }

var ErrContextCancelled error = sentinel(CodeContextCancelled)

type Error struct {
	Code     ErrorCode
	Stage    string
	Identity string
	Err      error
}

func (e *Error) Error() string {
	if e == nil {
		return "waiver error"
	}
	parts := []string{string(e.Code)}
	if e.Stage != "" {
		parts = append(parts, sanitize(e.Stage, 128))
	}
	if e.Identity != "" {
		parts = append(parts, sanitize(e.Identity, 128))
	}
	if e.Err != nil {
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
	s, ok := target.(sentinel)
	return ok && e != nil && ErrorCode(s) == e.Code
}
func errCode(code ErrorCode, stage, msg string) error {
	return &Error{Code: code, Stage: stage, Err: errors.New(msg)}
}
func wrapCode(code ErrorCode, stage, msg string, cause error) error {
	if cause == nil {
		return errCode(code, stage, msg)
	}
	return &Error{Code: code, Stage: stage, Err: fmt.Errorf("%s: %w", msg, cause)}
}
func contextErr(err error) error {
	if err == nil {
		return nil
	}
	return wrapCode(CodeContextCancelled, "context", "context cancelled", err)
}

func sanitize(s string, max int) string {
	var b strings.Builder
	for len(s) > 0 && b.Len() < max {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			if b.Len()+4 > max {
				break
			}
			fmt.Fprintf(&b, "\\x%02x", s[0])
			s = s[1:]
			continue
		}
		s = s[size:]
		if r < 0x20 || r == 0x7f {
			if b.Len()+6 > max {
				break
			}
			fmt.Fprintf(&b, "\\u%04x", r)
			continue
		}
		if b.Len()+size > max {
			break
		}
		b.WriteRune(r)
	}
	if len(s) > 0 && b.Len()+1 <= max {
		b.WriteRune('…')
	}
	return b.String()
}
func classifyContext(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return contextErr(err)
	}
	return nil
}
