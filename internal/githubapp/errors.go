package githubapp

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type ErrorCode string

const (
	CodeInvalidLimits               ErrorCode = "invalid-limits"
	CodeBodyTooLarge                ErrorCode = "body-too-large"
	CodeInvalidSecretSet            ErrorCode = "invalid-secret-set"
	CodeMissingSignature            ErrorCode = "missing-signature"
	CodeDuplicateSignatureHeader    ErrorCode = "duplicate-signature-header"
	CodeInvalidSignatureFormat      ErrorCode = "invalid-signature-format"
	CodeSignatureMismatch           ErrorCode = "signature-mismatch"
	CodeMissingRequiredHeader       ErrorCode = "missing-required-header"
	CodeDuplicateRequiredHeader     ErrorCode = "duplicate-required-header"
	CodeInvalidContentType          ErrorCode = "invalid-content-type"
	CodeUnsupportedContentEncoding  ErrorCode = "unsupported-content-encoding"
	CodeInvalidDeliveryID           ErrorCode = "invalid-delivery-id"
	CodeInvalidEventName            ErrorCode = "invalid-event-name"
	CodeInvalidUTF8                 ErrorCode = "invalid-utf8"
	CodeInvalidJSON                 ErrorCode = "invalid-json"
	CodeDuplicateJSONMember         ErrorCode = "duplicate-json-member"
	CodeTrailingJSON                ErrorCode = "trailing-json"
	CodeJSONDepthLimit              ErrorCode = "json-depth-limit"
	CodeJSONTokenLimit              ErrorCode = "json-token-limit"
	CodeProjectionInvalid           ErrorCode = "projection-invalid"
	CodeUnsupportedEvent            ErrorCode = "unsupported-event"
	CodeUnsupportedAction           ErrorCode = "unsupported-action"
	CodeInvalidInstallationID       ErrorCode = "invalid-installation-id"
	CodeInvalidRepositoryID         ErrorCode = "invalid-repository-id"
	CodeInvalidPullRequestNumber    ErrorCode = "invalid-pull-request-number"
	CodeInvalidObjectID             ErrorCode = "invalid-object-id"
	CodeDeliveryConflict            ErrorCode = "delivery-conflict"
	CodeInvalidTarget               ErrorCode = "invalid-target"
	CodeInvalidTargetID             ErrorCode = "invalid-target-id"
	CodeInvalidJob                  ErrorCode = "invalid-job"
	CodeInvalidJobID                ErrorCode = "invalid-job-id"
	CodeInvalidAttempt              ErrorCode = "invalid-attempt"
	CodeInvalidStateTransition      ErrorCode = "invalid-state-transition"
	CodeStaleGeneration             ErrorCode = "stale-generation"
	CodeSupersededJob               ErrorCode = "superseded-job"
	CodeForeignCheckRun             ErrorCode = "foreign-check-run"
	CodeInvalidWorkerAssignment     ErrorCode = "invalid-worker-assignment"
	CodeInvalidWorkerResult         ErrorCode = "invalid-worker-result"
	CodeCredentialBoundaryViolation ErrorCode = "credential-boundary-violation"
	CodeInvalidCheckProjection      ErrorCode = "invalid-check-projection"
	CodeInvalidCheckConclusion      ErrorCode = "invalid-check-conclusion"
	CodePublishInvariant            ErrorCode = "publish-invariant"
	CodeSerializationFailed         ErrorCode = "serialization-failed"
	CodeProtocolDocumentTooLarge    ErrorCode = "protocol-document-too-large"
	CodeContextCancelled            ErrorCode = "context-cancelled"
)

type Error struct {
	Code    ErrorCode
	Stage   string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return "githubapp error"
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
	return &Error{Code: code, Stage: stage, Message: msg, Err: err}
}

func codeOf(err error) ErrorCode {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
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

func deterministicErr(code ErrorCode, stage string) error {
	return errCode(code, stage, fmt.Sprintf("%s rejected", code), nil)
}
