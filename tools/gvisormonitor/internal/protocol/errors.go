package protocol

import (
	"errors"
	"strings"
	"unicode/utf8"
)

type ErrorCode string

const (
	CodeInvalidSocketPath          ErrorCode = "invalid-socket-path"
	CodeSocketExists               ErrorCode = "socket-exists"
	CodeSocketSymlink              ErrorCode = "socket-symlink"
	CodeSocketCreateFailed         ErrorCode = "socket-create-failed"
	CodeAcceptFailed               ErrorCode = "accept-failed"
	CodeConnectionLimit            ErrorCode = "connection-limit"
	CodeHandshakeInvalid           ErrorCode = "handshake-invalid"
	CodeProtocolVersionUnsupported ErrorCode = "protocol-version-unsupported"
	CodeHeaderInvalid              ErrorCode = "header-invalid"
	CodeMessageTooLarge            ErrorCode = "message-too-large"
	CodeMessageLimit               ErrorCode = "message-limit"
	CodeTotalBytesLimit            ErrorCode = "total-bytes-limit"
	CodeProtobufInvalid            ErrorCode = "protobuf-invalid"
	CodeUnknownMessageType         ErrorCode = "unknown-message-type"
	CodeFieldLimit                 ErrorCode = "field-limit"
	CodeProcessStateInvalid        ErrorCode = "process-state-invalid"
	CodeDroppedEvents              ErrorCode = "dropped-events"
	CodeOutputFailed               ErrorCode = "output-failed"
	CodeContextCancelled           ErrorCode = "context-cancelled"
	CodeMonitorTimeout             ErrorCode = "monitor-timeout"
)

type Error struct {
	Code              ErrorCode
	Stage, Field, Msg string
	Err               error
}

func (e *Error) Error() string {
	if e == nil {
		return "gvisor monitor: <nil>"
	}
	parts := []string{"gvisor monitor", string(e.Code)}
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
			b.WriteString("?")
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
