package gvisorspike

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type ErrorCode string

const (
	CodeUnsupportedPlatform     ErrorCode = "unsupported-platform"
	CodeInvalidLimits           ErrorCode = "invalid-limits"
	CodeInvalidPrerequisite     ErrorCode = "invalid-prerequisite"
	CodeInvalidRunscPath        ErrorCode = "invalid-runsc-path"
	CodeRunscSymlink            ErrorCode = "runsc-symlink"
	CodeRunscHashMismatch       ErrorCode = "runsc-hash-mismatch"
	CodeRunscVersionMismatch    ErrorCode = "runsc-version-mismatch"
	CodeUnsupportedArchitecture ErrorCode = "unsupported-architecture"
	CodeUnsupportedPlatformMode ErrorCode = "unsupported-platform-mode"
	CodeDockerOpenFailed        ErrorCode = "docker-open-failed"
	CodeRuntimeNotConfigured    ErrorCode = "runtime-not-configured"
	CodeImageNotPresent         ErrorCode = "image-not-present"
	CodeFixtureContainerCreate  ErrorCode = "fixture-container-create-failed"
	CodeFixtureContainerStart   ErrorCode = "fixture-container-start-failed"
	CodeFixtureContainerWait    ErrorCode = "fixture-container-wait-failed"
	CodeFixtureContainerRemove  ErrorCode = "fixture-container-remove-failed"
	CodeMonitorStartFailed      ErrorCode = "monitor-start-failed"
	CodeMonitorNotReady         ErrorCode = "monitor-not-ready"
	CodeMonitorIncomplete       ErrorCode = "monitor-incomplete"
	CodeTracepointMissing       ErrorCode = "tracepoint-missing"
	CodeDroppedEvents           ErrorCode = "dropped-events"
	CodeFixtureOutcomeInvalid   ErrorCode = "fixture-outcome-invalid"
	CodeCleanupFailed           ErrorCode = "cleanup-failed"
	CodeContextCancelled        ErrorCode = "context-cancelled"
	CodeSpikeTimeout            ErrorCode = "spike-timeout"
	CodeProcessStateInvalid     ErrorCode = "process-state-invalid"
	CodeInvalidPodInitConfig    ErrorCode = "invalid-pod-init-config"
	CodeInvalidContainerRequest ErrorCode = "invalid-container-request"
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
		return "gvisor spike: <nil>"
	}
	parts := []string{"gvisor spike", string(e.Code)}
	if e.Stage != "" {
		parts = append(parts, e.Stage)
	}
	if e.Field != "" {
		parts = append(parts, e.Field)
	}
	if e.Msg != "" {
		parts = append(parts, sanitize(e.Msg))
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
	var other *Error
	return errors.As(target, &other) && other.Code == e.Code
}

func errCode(code ErrorCode, stage, field, msg string, err error) error {
	return &Error{Code: code, Stage: stage, Field: field, Msg: msg, Err: err}
}

func As(err error, target **Error) bool { return errors.As(err, target) }

func sanitize(in string) string {
	if !utf8.ValidString(in) {
		return "invalid utf-8"
	}
	var b strings.Builder
	for _, r := range in {
		if r < 0x20 || r == 0x7f {
			fmt.Fprintf(&b, "\\u%04X", r)
			continue
		}
		b.WriteRune(r)
		if b.Len() > 512 {
			b.WriteString("...")
			break
		}
	}
	return b.String()
}
