package runner

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type ErrorCode string

const (
	CodeContextCancelled          ErrorCode = "context-cancelled"
	CodeRunTimeout                ErrorCode = "run-timeout"
	CodeAttemptTimeout            ErrorCode = "attempt-timeout"
	CodeInvalidPlan               ErrorCode = "invalid-plan"
	CodeInvalidPlanRunnerField    ErrorCode = "invalid-plan-runner-field"
	CodeInvalidRequirements       ErrorCode = "invalid-requirements"
	CodeCapabilitiesFailed        ErrorCode = "capabilities-failed"
	CodeInvalidCapabilities       ErrorCode = "invalid-capabilities"
	CodeCapabilityMismatch        ErrorCode = "capability-mismatch"
	CodeSyntheticRunnerNotAllowed ErrorCode = "synthetic-runner-not-allowed"
	CodeInvalidLimits             ErrorCode = "invalid-limits"
	CodeInvalidAttempt            ErrorCode = "invalid-attempt"
	CodeInvalidProgram            ErrorCode = "invalid-program"
	CodeProgramPlanMismatch       ErrorCode = "program-plan-mismatch"
	CodeMissingAttemptScript      ErrorCode = "missing-attempt-script"
	CodeDuplicateAttemptScript    ErrorCode = "duplicate-attempt-script"
	CodeExtraAttemptScript        ErrorCode = "extra-attempt-script"
	CodeInvalidEventDraft         ErrorCode = "invalid-event-draft"
	CodeEventTooLarge             ErrorCode = "event-too-large"
	CodeEventLimit                ErrorCode = "event-limit"
	CodeSinkFailed                ErrorCode = "sink-failed"
	CodeBackendFailed             ErrorCode = "backend-failed"
	CodeInvalidOutcome            ErrorCode = "invalid-outcome"
	CodeResultInvariant           ErrorCode = "result-invariant"
	CodeSerializationFailed       ErrorCode = "serialization-failed"
)

type sentinel ErrorCode

func (s sentinel) Error() string { return string(s) }

const (
	ErrContextCancelled          sentinel = sentinel(CodeContextCancelled)
	ErrRunTimeout                sentinel = sentinel(CodeRunTimeout)
	ErrAttemptTimeout            sentinel = sentinel(CodeAttemptTimeout)
	ErrInvalidPlan               sentinel = sentinel(CodeInvalidPlan)
	ErrInvalidPlanRunnerField    sentinel = sentinel(CodeInvalidPlanRunnerField)
	ErrInvalidRequirements       sentinel = sentinel(CodeInvalidRequirements)
	ErrCapabilitiesFailed        sentinel = sentinel(CodeCapabilitiesFailed)
	ErrInvalidCapabilities       sentinel = sentinel(CodeInvalidCapabilities)
	ErrCapabilityMismatch        sentinel = sentinel(CodeCapabilityMismatch)
	ErrSyntheticRunnerNotAllowed sentinel = sentinel(CodeSyntheticRunnerNotAllowed)
	ErrInvalidLimits             sentinel = sentinel(CodeInvalidLimits)
	ErrInvalidAttempt            sentinel = sentinel(CodeInvalidAttempt)
	ErrInvalidProgram            sentinel = sentinel(CodeInvalidProgram)
	ErrProgramPlanMismatch       sentinel = sentinel(CodeProgramPlanMismatch)
	ErrMissingAttemptScript      sentinel = sentinel(CodeMissingAttemptScript)
	ErrDuplicateAttemptScript    sentinel = sentinel(CodeDuplicateAttemptScript)
	ErrExtraAttemptScript        sentinel = sentinel(CodeExtraAttemptScript)
	ErrInvalidEventDraft         sentinel = sentinel(CodeInvalidEventDraft)
	ErrEventTooLarge             sentinel = sentinel(CodeEventTooLarge)
	ErrEventLimit                sentinel = sentinel(CodeEventLimit)
	ErrSinkFailed                sentinel = sentinel(CodeSinkFailed)
	ErrBackendFailed             sentinel = sentinel(CodeBackendFailed)
	ErrInvalidOutcome            sentinel = sentinel(CodeInvalidOutcome)
	ErrResultInvariant           sentinel = sentinel(CodeResultInvariant)
	ErrSerializationFailed       sentinel = sentinel(CodeSerializationFailed)
)

type Error struct {
	Code    ErrorCode
	Stage   string
	Attempt string
	Path    string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return "runner error"
	}
	parts := []string{string(e.Code)}
	if e.Stage != "" {
		parts = append(parts, sanitize(e.Stage, 96))
	}
	if e.Attempt != "" {
		parts = append(parts, sanitize(e.Attempt, MaxAttemptIDBytes))
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

func errCode(code ErrorCode, stage, attempt, path, msg string, cause error) error {
	if cause == nil && msg != "" {
		cause = errors.New(msg)
	} else if cause != nil && msg != "" {
		cause = fmt.Errorf("%s: %w", msg, cause)
	}
	return &Error{Code: code, Stage: stage, Attempt: attempt, Path: path, Err: cause}
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
