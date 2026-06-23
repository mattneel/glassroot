package dockerdev

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type ErrorCode string

const (
	CodeAcknowledgementRequired ErrorCode = "acknowledgement-required"
	CodeInvalidRunnerConfig     ErrorCode = "invalid-runner-config"
	CodeInvalidWorkspaceBinding ErrorCode = "invalid-workspace-binding"
	CodeMissingWorkspaceBinding ErrorCode = "missing-workspace-binding"
	CodeExtraWorkspaceBinding   ErrorCode = "extra-workspace-binding"
	CodeDuplicateWorkspace      ErrorCode = "duplicate-workspace"
	CodeWorkspaceOverlap        ErrorCode = "workspace-overlap"
	CodeCapabilityMismatch      ErrorCode = "capability-mismatch"
	CodeContainerCreateFailed   ErrorCode = "container-create-failed"
	CodeContainerConfigMismatch ErrorCode = "container-config-mismatch"
	CodeAttachFailed            ErrorCode = "attach-failed"
	CodeStartFailed             ErrorCode = "start-failed"
	CodeWaitFailed              ErrorCode = "wait-failed"
	CodeInspectFailed           ErrorCode = "inspect-failed"
	CodeOutputFramingInvalid    ErrorCode = "output-framing-invalid"
	CodeOutputSinkFailed        ErrorCode = "output-sink-failed"
	CodeOutputLimit             ErrorCode = "output-limit"
	CodeStopFailed              ErrorCode = "stop-failed"
	CodeKillFailed              ErrorCode = "kill-failed"
	CodeRemoveFailed            ErrorCode = "remove-failed"
	CodeCleanupFailed           ErrorCode = "cleanup-failed"
	CodeInvalidContainerState   ErrorCode = "invalid-container-state"
	CodeAttemptTimeout          ErrorCode = "attempt-timeout"
	CodeContextCancelled        ErrorCode = "context-cancelled"
)

type Error struct {
	Code    ErrorCode
	Stage   string
	Attempt string
	Msg     string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return "dockerdev: <nil>"
	}
	parts := []string{"dockerdev", string(e.Code)}
	if e.Stage != "" {
		parts = append(parts, e.Stage)
	}
	if e.Attempt != "" {
		parts = append(parts, e.Attempt)
	}
	if msg := sanitizeMessage(e.Msg); msg != "" {
		parts = append(parts, msg)
	}
	return strings.Join(parts, ": ")
}
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func errCode(code ErrorCode, stage, attempt, msg string, err error) error {
	return &Error{Code: code, Stage: sanitizeMessage(stage), Attempt: sanitizeMessage(attempt), Msg: msg, Err: err}
}

func sanitizeMessage(s string) string {
	var b strings.Builder
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			b.WriteString(fmt.Sprintf("\\x%02x", s[0]))
			s = s[1:]
			continue
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			b.WriteString(fmt.Sprintf("\\u{%X}", r))
		} else {
			b.WriteRune(r)
		}
		s = s[size:]
	}
	out := b.String()
	if len(out) > 240 {
		return out[:240]
	}
	return out
}
