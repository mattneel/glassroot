package demo

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type ErrorCode string

const (
	CodeUnsupportedPlatform          ErrorCode = "unsupported-platform"
	CodeInvalidLimits                ErrorCode = "invalid-limits"
	CodeInvalidRequest               ErrorCode = "invalid-request"
	CodeInvalidFixture               ErrorCode = "invalid-fixture"
	CodeInvalidOutputPath            ErrorCode = "invalid-output-path"
	CodeOutputParentInvalid          ErrorCode = "output-parent-invalid"
	CodeOutputAlreadyExists          ErrorCode = "output-already-exists"
	CodeStagingCreateFailed          ErrorCode = "staging-create-failed"
	CodeFixtureStoreFailed           ErrorCode = "fixture-store-failed"
	CodeFixtureObjectInvalid         ErrorCode = "fixture-object-invalid"
	CodeFixtureGitOpenFailed         ErrorCode = "fixture-git-open-failed"
	CodeFixtureRevisionFailed        ErrorCode = "fixture-revision-failed"
	CodeTrustedConfigFailed          ErrorCode = "trusted-config-failed"
	CodeMaterializationFailed        ErrorCode = "materialization-failed"
	CodeMaterializationCleanupFailed ErrorCode = "materialization-cleanup-failed"
	CodePlanBuildFailed              ErrorCode = "plan-build-failed"
	CodeFakeProgramInvalid           ErrorCode = "fake-program-invalid"
	CodeFakeExecutionFailed          ErrorCode = "fake-execution-failed"
	CodeEvidenceWriteFailed          ErrorCode = "evidence-write-failed"
	CodeEvidenceVerifyFailed         ErrorCode = "evidence-verify-failed"
	CodeEvidenceRelocateFailed       ErrorCode = "evidence-relocate-failed"
	CodeInspectFailed                ErrorCode = "inspect-failed"
	CodeReportRenderFailed           ErrorCode = "report-render-failed"
	CodeMetadataInvalid              ErrorCode = "metadata-invalid"
	CodeMetadataWriteFailed          ErrorCode = "metadata-write-failed"
	CodeSyncFailed                   ErrorCode = "sync-failed"
	CodePublishCollision             ErrorCode = "publish-collision"
	CodePublishFailed                ErrorCode = "publish-failed"
	CodeCleanupFailed                ErrorCode = "cleanup-failed"
	CodeContextCancelled             ErrorCode = "context-cancelled"
	CodeDemoTimeout                  ErrorCode = "demo-timeout"
	CodeOutputFailed                 ErrorCode = "output-failed"
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
		return "demo error"
	}
	msg := string(e.Code)
	if e.Stage != "" {
		msg += ": " + e.Stage
	}
	if e.Message != "" {
		msg += ": " + sanitize(e.Message, 256)
	}
	if e.Err != nil {
		msg += ": " + sanitize(e.Err.Error(), 256)
	}
	return msg
}
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && e != nil && e.Code == t.Code
}

func errCode(code ErrorCode, stage, msg string, err error) error {
	return &Error{Code: code, Stage: stage, Message: msg, Err: err}
}
func usageErr(code ErrorCode, stage, msg string, err error) error {
	return &Error{Code: code, Stage: stage, Message: msg, Err: err, Usage: true}
}
func wrap(code ErrorCode, stage, msg string, err error) error {
	if err == nil {
		return errCode(code, stage, msg, nil)
	}
	if errors.Is(err, context.Canceled) {
		return errCode(CodeContextCancelled, stage, "context cancelled", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errCode(CodeDemoTimeout, stage, "context deadline exceeded", err)
	}
	return &Error{Code: code, Stage: stage, Message: msg, Err: err}
}
func IsUsageError(err error) bool { var e *Error; return errors.As(err, &e) && e.Usage }
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
func sanitize(s string, limit int) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			b.WriteString("?")
			continue
		}
		b.WriteRune(r)
		if b.Len() >= limit {
			break
		}
	}
	return b.String()
}
