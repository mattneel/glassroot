package observe

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type ErrorCode string

const (
	CodeInvalidLimits               ErrorCode = "invalid-limits"
	CodeNilBundle                   ErrorCode = "nil-bundle"
	CodeBundleClosed                ErrorCode = "bundle-closed"
	CodeBundleReadFailed            ErrorCode = "bundle-read-failed"
	CodeInvalidPlan                 ErrorCode = "invalid-plan"
	CodeInvalidAttemptInventory     ErrorCode = "invalid-attempt-inventory"
	CodeInvalidNormalizationProfile ErrorCode = "invalid-normalization-profile"
	CodeUnsupportedIgnoreField      ErrorCode = "unsupported-ignore-field"
	CodeUnsupportedObservationKind  ErrorCode = "unsupported-observation-kind"
	CodeInvalidObservationSource    ErrorCode = "invalid-observation-source"
	CodeInvalidObservationPayload   ErrorCode = "invalid-observation-payload"
	CodeEventOrder                  ErrorCode = "event-order"
	CodeEvidenceReferenceInvalid    ErrorCode = "evidence-reference-invalid"
	CodeProcessStateInvalid         ErrorCode = "process-state-invalid"
	CodeProcessLimit                ErrorCode = "process-limit"
	CodeTimestampInvalid            ErrorCode = "timestamp-invalid"
	CodePathNormalizationFailed     ErrorCode = "path-normalization-failed"
	CodeFactLimit                   ErrorCode = "fact-limit"
	CodeEvidenceReferenceLimit      ErrorCode = "evidence-reference-limit"
	CodeSemanticEncodingFailed      ErrorCode = "semantic-encoding-failed"
	CodeModelInvariant              ErrorCode = "model-invariant"
	CodeContextCancelled            ErrorCode = "context-cancelled"
	CodeNormalizationTimeout        ErrorCode = "normalization-timeout"
)

type sentinel ErrorCode

func (s sentinel) Error() string { return string(s) }

var (
	ErrInvalidLimits              error = sentinel(CodeInvalidLimits)
	ErrNilBundle                  error = sentinel(CodeNilBundle)
	ErrBundleClosed               error = sentinel(CodeBundleClosed)
	ErrUnsupportedObservationKind error = sentinel(CodeUnsupportedObservationKind)
)

type Error struct {
	Code    ErrorCode
	Stage   string
	Attempt string
	Field   string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return "observe error"
	}
	parts := []string{string(e.Code)}
	for _, p := range []string{e.Stage, e.Attempt, e.Field} {
		if p != "" {
			parts = append(parts, sanitize(p, 192))
		}
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

func errCode(code ErrorCode, stage, attempt, field, msg string, cause error) error {
	if cause == nil && msg != "" {
		cause = errors.New(msg)
	} else if cause != nil && msg != "" {
		cause = fmt.Errorf("%s: %w", msg, cause)
	}
	return &Error{Code: code, Stage: stage, Attempt: attempt, Field: field, Err: cause}
}

func contextErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errCode(CodeNormalizationTimeout, "context", "", "deadline", "normalization deadline exceeded", err)
	}
	return errCode(CodeContextCancelled, "context", "", "cancelled", "context cancelled", err)
}

func sanitize(s string, max int) string {
	if max <= 0 {
		max = 256
	}
	var b strings.Builder
	for len(s) > 0 && b.Len() < max {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			if b.Len()+6 > max {
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
