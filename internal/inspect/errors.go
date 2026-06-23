package inspect

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

type ErrorCode string

const (
	CodeInvalidLimits            ErrorCode = "invalid-limits"
	CodeInvalidRequest           ErrorCode = "invalid-request"
	CodeInvalidBundlePath        ErrorCode = "invalid-bundle-path"
	CodeInvalidGitDir            ErrorCode = "invalid-git-dir"
	CodeIntegrityModeRequired    ErrorCode = "integrity-mode-required"
	CodeConflictingIntegrityMode ErrorCode = "conflicting-integrity-mode"
	CodeInvalidManifestDigest    ErrorCode = "invalid-manifest-digest"
	CodeInvalidBaseCommit        ErrorCode = "invalid-base-commit"
	CodeInvalidHeadCommit        ErrorCode = "invalid-head-commit"
	CodeInvalidEvaluatedAt       ErrorCode = "invalid-evaluated-at"
	CodeBundleOpenFailed         ErrorCode = "bundle-open-failed"
	CodeGitOpenFailed            ErrorCode = "git-open-failed"
	CodeRevisionResolveFailed    ErrorCode = "revision-resolve-failed"
	CodeRevisionMismatch         ErrorCode = "revision-mismatch"
	CodeTreeMismatch             ErrorCode = "tree-mismatch"
	CodeObjectFormatMismatch     ErrorCode = "object-format-mismatch"
	CodeTrustedConfigFailed      ErrorCode = "trusted-config-failed"
	CodePlanRebuildFailed        ErrorCode = "plan-rebuild-failed"
	CodePlanMismatch             ErrorCode = "plan-mismatch"
	CodeNormalizationFailed      ErrorCode = "normalization-failed"
	CodeComparisonFailed         ErrorCode = "comparison-failed"
	CodePolicyEvaluationFailed   ErrorCode = "policy-evaluation-failed"
	CodePolicyApplicationFailed  ErrorCode = "policy-application-failed"
	CodeReportBuildFailed        ErrorCode = "report-build-failed"
	CodeBundleCloseFailed        ErrorCode = "bundle-close-failed"
	CodeRenderFailed             ErrorCode = "render-failed"
	CodeOutputFailed             ErrorCode = "output-failed"
	CodeContextCancelled         ErrorCode = "context-cancelled"
	CodeInspectTimeout           ErrorCode = "inspect-timeout"
)

type sentinel ErrorCode

func (s sentinel) Error() string { return string(s) }

var (
	ErrInvalidRequest        error = sentinel(CodeInvalidRequest)
	ErrInvalidBundlePath     error = sentinel(CodeInvalidBundlePath)
	ErrInvalidGitDir         error = sentinel(CodeInvalidGitDir)
	ErrTrustedConfigFailed   error = sentinel(CodeTrustedConfigFailed)
	ErrBundleOpenFailed      error = sentinel(CodeBundleOpenFailed)
	ErrRevisionResolveFailed error = sentinel(CodeRevisionResolveFailed)
)

type Error struct {
	Code    ErrorCode
	Stage   string
	Message string
	Usage   bool
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return "inspect error"
	}
	msg := string(e.Code)
	if e.Stage != "" {
		msg = e.Stage + ": " + msg
	}
	if e.Message != "" {
		msg += ": " + sanitize(e.Message, 256)
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) Is(target error) bool {
	s, ok := target.(sentinel)
	return ok && e != nil && ErrorCode(s) == e.Code
}

func errCode(code ErrorCode, stage, msg string, cause error) error {
	return &Error{Code: code, Stage: sanitize(stage, 64), Message: sanitize(msg, 256), Usage: usageCode(code), Err: cause}
}

func usageErr(code ErrorCode, stage, msg string, cause error) error {
	return &Error{Code: code, Stage: sanitize(stage, 64), Message: sanitize(msg, 256), Usage: true, Err: cause}
}

func wrapStage(code ErrorCode, stage, msg string, cause error) error {
	if cause == nil {
		return errCode(code, stage, msg, nil)
	}
	if errors.Is(cause, context.DeadlineExceeded) || errors.Is(cause, context.Canceled) {
		return contextErr(stage, cause)
	}
	return &Error{Code: code, Stage: sanitize(stage, 64), Message: sanitize(msg, 256), Usage: usageCode(code), Err: cause}
}

func contextErr(stage string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errCode(CodeInspectTimeout, stage, "inspection deadline exceeded", err)
	}
	if errors.Is(err, context.Canceled) {
		return errCode(CodeContextCancelled, stage, "context cancelled", err)
	}
	return errCode(CodeContextCancelled, stage, "context error", err)
}

func usageCode(code ErrorCode) bool {
	switch code {
	case CodeInvalidLimits, CodeInvalidRequest, CodeInvalidBundlePath, CodeInvalidGitDir, CodeIntegrityModeRequired, CodeConflictingIntegrityMode, CodeInvalidManifestDigest, CodeInvalidBaseCommit, CodeInvalidHeadCommit, CodeInvalidEvaluatedAt:
		return true
	default:
		return false
	}
}

func IsUsageError(err error) bool {
	var ie *Error
	return errors.As(err, &ie) && ie.Usage
}

func CodeOf(err error) ErrorCode {
	var ie *Error
	if errors.As(err, &ie) {
		return ie.Code
	}
	return CodeInvalidRequest
}

func Diagnostic(err error) string {
	var ie *Error
	if errors.As(err, &ie) {
		return string(ie.Code) + ": " + sanitize(ie.Message, 256)
	}
	return string(CodeInvalidRequest) + ": " + sanitize("inspection failed", 256)
}

func sanitize(s string, max int) string {
	if max <= 0 {
		max = 256
	}
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "\\xff")
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
		out = out[:max]
	}
	return out
}
