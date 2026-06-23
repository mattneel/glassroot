package dockerengine

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type ErrorCode string

const (
	CodeUnsupportedPlatform            ErrorCode = "unsupported-platform"
	CodeInvalidSocketPath              ErrorCode = "invalid-socket-path"
	CodeSocketSymlink                  ErrorCode = "socket-symlink"
	CodeSocketNotUnix                  ErrorCode = "socket-not-unix"
	CodeSocketChanged                  ErrorCode = "socket-changed"
	CodeDaemonUnreachable              ErrorCode = "daemon-unreachable"
	CodeDaemonTimeout                  ErrorCode = "daemon-timeout"
	CodeUnsupportedDaemonOS            ErrorCode = "unsupported-daemon-os"
	CodeUnsupportedAPIVersion          ErrorCode = "unsupported-api-version"
	CodeDaemonCapabilityUnsupported    ErrorCode = "daemon-capability-unsupported"
	CodeResourceEnforcementUnavailable ErrorCode = "resource-enforcement-unavailable"
	CodeSeccompUnavailable             ErrorCode = "seccomp-unavailable"
	CodeImageReferenceInvalid          ErrorCode = "image-reference-invalid"
	CodeImageNotPresent                ErrorCode = "image-not-present"
	CodeImageDigestMismatch            ErrorCode = "image-digest-mismatch"
	CodeImageOSMismatch                ErrorCode = "image-os-mismatch"
	CodeImageVolumeUnsupported         ErrorCode = "image-volume-unsupported"
	CodeEngineResponseInvalid          ErrorCode = "engine-response-invalid"
	CodeEngineOutputTooLarge           ErrorCode = "engine-output-too-large"
	CodeContextCancelled               ErrorCode = "context-cancelled"
	CodeCloseFailed                    ErrorCode = "close-failed"
)

type Error struct {
	Code  ErrorCode
	Stage string
	Op    string
	Msg   string
	Err   error
}

func (e *Error) Error() string {
	if e == nil {
		return "dockerengine: <nil>"
	}
	parts := []string{"dockerengine", string(e.Code)}
	if e.Stage != "" {
		parts = append(parts, e.Stage)
	}
	if e.Op != "" {
		parts = append(parts, e.Op)
	}
	msg := sanitizeMessage(e.Msg)
	if msg != "" {
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

func errCode(code ErrorCode, stage, op, msg string, err error) error {
	return &Error{Code: code, Stage: sanitizeMessage(stage), Op: sanitizeMessage(op), Msg: msg, Err: err}
}

func sanitizeMessage(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			b.WriteString("\\x")
			b.WriteString(fmt.Sprintf("%02x", s[0]))
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
