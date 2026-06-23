package report

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type ErrorCode string

const (
	CodeInvalidLimits             ErrorCode = "invalid-limits"
	CodeNilBundle                 ErrorCode = "nil-bundle"
	CodeBundleClosed              ErrorCode = "bundle-closed"
	CodeNilDelta                  ErrorCode = "nil-delta"
	CodeNilApplication            ErrorCode = "nil-application"
	CodeInvalidBundle             ErrorCode = "invalid-bundle"
	CodeInvalidDelta              ErrorCode = "invalid-delta"
	CodeInvalidApplication        ErrorCode = "invalid-application"
	CodeRunIDMismatch             ErrorCode = "run-id-mismatch"
	CodePlanDigestMismatch        ErrorCode = "plan-digest-mismatch"
	CodeManifestDigestMismatch    ErrorCode = "manifest-digest-mismatch"
	CodeDeltaDigestMismatch       ErrorCode = "delta-digest-mismatch"
	CodeApplicationDigestMismatch ErrorCode = "application-digest-mismatch"
	CodeRevisionMismatch          ErrorCode = "revision-mismatch"
	CodeCompletenessMismatch      ErrorCode = "completeness-mismatch"
	CodeInvalidRunnerCapabilities ErrorCode = "invalid-runner-capabilities"
	CodeDuplicateFindingID        ErrorCode = "duplicate-finding-id"
	CodeDuplicateDeltaRecordID    ErrorCode = "duplicate-delta-record-id"
	CodeMissingDeltaRecord        ErrorCode = "missing-delta-record"
	CodeInvalidEvidenceReference  ErrorCode = "invalid-evidence-reference"
	CodeUnsupportedReportProfile  ErrorCode = "unsupported-report-profile"
	CodeUnsupportedDeltaKind      ErrorCode = "unsupported-delta-kind"
	CodeUnsupportedFactKind       ErrorCode = "unsupported-fact-kind"
	CodeInvalidReportModel        ErrorCode = "invalid-report-model"
	CodeDisplayValueTooLarge      ErrorCode = "display-value-too-large"
	CodeEvidenceReferenceLimit    ErrorCode = "evidence-reference-limit"
	CodeReportLimit               ErrorCode = "report-limit"
	CodeReportTooLarge            ErrorCode = "report-too-large"
	CodeMarkdownTooLarge          ErrorCode = "markdown-too-large"
	CodeTerminalTooLarge          ErrorCode = "terminal-too-large"
	CodeSerializationFailed       ErrorCode = "serialization-failed"
	CodeRenderFailed              ErrorCode = "render-failed"
	CodeContextCancelled          ErrorCode = "context-cancelled"
	CodeReportTimeout             ErrorCode = "report-timeout"
	CodeRenderTimeout             ErrorCode = "render-timeout"
)

type Error struct {
	Code  ErrorCode
	Stage string
	Field string
	Msg   string
	Err   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	parts := []string{"report", string(e.Code)}
	if e.Stage != "" {
		parts = append(parts, sanitizeError(e.Stage))
	}
	if e.Field != "" {
		parts = append(parts, sanitizeError(e.Field))
	}
	if e.Msg != "" {
		parts = append(parts, sanitizeError(e.Msg))
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
	var t *Error
	return errors.As(target, &t) && t.Code == e.Code
}

func errCode(code ErrorCode, stage, field, msg string, err error) error {
	return &Error{Code: code, Stage: stage, Field: field, Msg: msg, Err: err}
}

func contextErr(stage string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		code := CodeRenderTimeout
		if stage == "build" {
			code = CodeReportTimeout
		}
		return errCode(code, stage, "context", "context deadline exceeded", err)
	}
	return errCode(CodeContextCancelled, stage, "context", "context cancelled", err)
}

func sanitizeError(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			_, _ = fmt.Fprintf(&b, "\\u{%04X}", r)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
