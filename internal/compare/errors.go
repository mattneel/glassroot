package compare

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type ErrorCode string

const (
	CodeInvalidLimits                   ErrorCode = "invalid-limits"
	CodeNilTraceSet                     ErrorCode = "nil-trace-set"
	CodeInvalidTraceSet                 ErrorCode = "invalid-trace-set"
	CodeUnsupportedNormalizationProfile ErrorCode = "unsupported-normalization-profile"
	CodeInvalidComparisonProfile        ErrorCode = "invalid-comparison-profile"
	CodeInvalidAttemptInventory         ErrorCode = "invalid-attempt-inventory"
	CodeDuplicateAttempt                ErrorCode = "duplicate-attempt"
	CodeDuplicateFactID                 ErrorCode = "duplicate-fact-id"
	CodeInvalidSemanticDigest           ErrorCode = "invalid-semantic-digest"
	CodeUnsupportedFactKind             ErrorCode = "unsupported-fact-kind"
	CodeInvalidObservationSource        ErrorCode = "invalid-observation-source"
	CodeInvalidFactPayload              ErrorCode = "invalid-fact-payload"
	CodeInvalidOccurrenceProfile        ErrorCode = "invalid-occurrence-profile"
	CodeOccurrenceOverflow              ErrorCode = "occurrence-overflow"
	CodeEvidenceReferenceInvalid        ErrorCode = "evidence-reference-invalid"
	CodeEvidenceReferenceLimit          ErrorCode = "evidence-reference-limit"
	CodeAnchorEncodingFailed            ErrorCode = "anchor-encoding-failed"
	CodeChangedFieldInventory           ErrorCode = "changed-field-inventory"
	CodeComparisonLimit                 ErrorCode = "comparison-limit"
	CodeDeltaLimit                      ErrorCode = "delta-limit"
	CodeModelInvariant                  ErrorCode = "model-invariant"
	CodeSerializationFailed             ErrorCode = "serialization-failed"
	CodeDeltaTooLarge                   ErrorCode = "delta-too-large"
	CodeContextCancelled                ErrorCode = "context-cancelled"
	CodeComparisonTimeout               ErrorCode = "comparison-timeout"
)

type sentinel ErrorCode

func (s sentinel) Error() string { return string(s) }

var (
	ErrInvalidLimits error = sentinel(CodeInvalidLimits)
	ErrNilTraceSet   error = sentinel(CodeNilTraceSet)
)

type Error struct {
	Code                   ErrorCode
	Stage, Scenario, Field string
	Err                    error
}

func (e *Error) Error() string {
	if e == nil {
		return "compare error"
	}
	parts := []string{string(e.Code)}
	for _, p := range []string{e.Stage, e.Scenario, e.Field} {
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

func errCode(code ErrorCode, stage, scenario, field, msg string, cause error) error {
	if cause == nil && msg != "" {
		cause = errors.New(msg)
	} else if cause != nil && msg != "" {
		cause = fmt.Errorf("%s: %w", msg, cause)
	}
	return &Error{Code: code, Stage: stage, Scenario: scenario, Field: field, Err: cause}
}
func contextErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errCode(CodeComparisonTimeout, "context", "", "deadline", "comparison deadline exceeded", err)
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
