package localrun

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type ErrorCode string

const (
	CodeUnsupportedPlatform      ErrorCode = "unsupported-platform"
	CodeInvalidLimits            ErrorCode = "invalid-limits"
	CodeInvalidRequest           ErrorCode = "invalid-request"
	CodeInvalidOutputPath        ErrorCode = "invalid-output-path"
	CodeOutputParentInvalid      ErrorCode = "output-parent-invalid"
	CodeOutputAlreadyExists      ErrorCode = "output-already-exists"
	CodeInvalidGitDir            ErrorCode = "invalid-git-dir"
	CodeInvalidDockerSocket      ErrorCode = "invalid-docker-socket"
	CodeInvalidBaseCommit        ErrorCode = "invalid-base-commit"
	CodeInvalidHeadCommit        ErrorCode = "invalid-head-commit"
	CodeInvalidRunID             ErrorCode = "invalid-run-id"
	CodeInvalidCreatedAt         ErrorCode = "invalid-created-at"
	CodeInvalidEvaluatedAt       ErrorCode = "invalid-evaluated-at"
	CodeAcknowledgementInvalid   ErrorCode = "acknowledgement-invalid"
	CodeStagingCreateFailed      ErrorCode = "staging-create-failed"
	CodeGitOpenFailed            ErrorCode = "git-open-failed"
	CodeRevisionResolveFailed    ErrorCode = "revision-resolve-failed"
	CodeTrustedConfigFailed      ErrorCode = "trusted-config-failed"
	CodeMaterializationFailed    ErrorCode = "materialization-failed"
	CodeSourceSnapshotMismatch   ErrorCode = "source-snapshot-mismatch"
	CodePlanBuildFailed          ErrorCode = "plan-build-failed"
	CodeAttemptLimit             ErrorCode = "attempt-limit"
	CodeCollectorBindFailed      ErrorCode = "collector-bind-failed"
	CodeDockerOpenFailed         ErrorCode = "docker-open-failed"
	CodeImageNotPresent          ErrorCode = "image-not-present"
	CodeRunnerCreateFailed       ErrorCode = "runner-create-failed"
	CodeLogCaptureFailed         ErrorCode = "log-capture-failed"
	CodeExecutionFailed          ErrorCode = "execution-failed"
	CodeArtifactCollectionFailed ErrorCode = "artifact-collection-failed"
	CodeArtifactEvidenceFailed   ErrorCode = "artifact-evidence-failed"
	CodeWorkspaceCleanupFailed   ErrorCode = "workspace-cleanup-failed"
	CodeEvidenceWriteFailed      ErrorCode = "evidence-write-failed"
	CodeEvidenceVerifyFailed     ErrorCode = "evidence-verify-failed"
	CodeEvidenceRelocateFailed   ErrorCode = "evidence-relocate-failed"
	CodeInspectFailed            ErrorCode = "inspect-failed"
	CodeReportRenderFailed       ErrorCode = "report-render-failed"
	CodeMetadataWriteFailed      ErrorCode = "metadata-write-failed"
	CodeSyncFailed               ErrorCode = "sync-failed"
	CodePublishCollision         ErrorCode = "publish-collision"
	CodePublishFailed            ErrorCode = "publish-failed"
	CodeOutputFailed             ErrorCode = "output-failed"
	CodeCleanupFailed            ErrorCode = "cleanup-failed"
	CodeContextCancelled         ErrorCode = "context-cancelled"
	CodeRunTimeout               ErrorCode = "run-timeout"
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
		return "localrun error"
	}
	parts := []string{string(e.Code)}
	if e.Stage != "" {
		parts = append(parts, sanitize(e.Stage, 64))
	}
	if e.Message != "" {
		parts = append(parts, sanitize(e.Message, 256))
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

func errCode(code ErrorCode, stage, msg string, err error) error {
	return &Error{Code: code, Stage: stage, Message: msg, Usage: usageCode(code), Err: err}
}
func usageErr(code ErrorCode, stage, msg string, err error) error {
	return &Error{Code: code, Stage: stage, Message: msg, Usage: true, Err: err}
}
func wrap(code ErrorCode, stage, msg string, err error) error {
	if err == nil {
		return errCode(code, stage, msg, nil)
	}
	if errors.Is(err, context.Canceled) {
		return errCode(CodeContextCancelled, stage, "context cancelled", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errCode(CodeRunTimeout, stage, "context deadline exceeded", err)
	}
	return &Error{Code: code, Stage: stage, Message: msg, Usage: usageCode(code), Err: err}
}

func usageCode(code ErrorCode) bool {
	switch code {
	case CodeInvalidLimits, CodeInvalidRequest, CodeInvalidOutputPath, CodeOutputParentInvalid, CodeOutputAlreadyExists, CodeInvalidGitDir, CodeInvalidDockerSocket, CodeInvalidBaseCommit, CodeInvalidHeadCommit, CodeInvalidRunID, CodeInvalidCreatedAt, CodeInvalidEvaluatedAt, CodeAcknowledgementInvalid, CodeTrustedConfigFailed:
		return true
	default:
		return false
	}
}

func IsUsageError(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Usage
}
func CodeOf(err error) ErrorCode {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return CodeInvalidRequest
}
func Diagnostic(err error) string {
	var e *Error
	if errors.As(err, &e) {
		msg := string(e.Code)
		if e.Message != "" {
			msg += ": " + sanitize(e.Message, 192)
		}
		return msg
	}
	if err == nil {
		return string(CodeInvalidRequest)
	}
	return fmt.Sprintf("%s: %s", CodeInvalidRequest, sanitize(err.Error(), 192))
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
