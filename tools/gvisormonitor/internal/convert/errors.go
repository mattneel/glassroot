package convert

import (
	"errors"
	"strings"
)

type ErrorCode string

const (
	CodeFieldLimit      ErrorCode = "field-limit"
	CodeProtobufInvalid ErrorCode = "protobuf-invalid"
)

type Error struct {
	Code              ErrorCode
	Stage, Field, Msg string
	Err               error
}

func (e *Error) Error() string {
	if e == nil {
		return "gvisor convert: <nil>"
	}
	return strings.Join([]string{"gvisor convert", string(e.Code), e.Stage, e.Field, e.Msg}, ": ")
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
func errCode(c ErrorCode, s, f, m string, e error) error {
	return &Error{Code: c, Stage: s, Field: f, Msg: m, Err: e}
}
