package githubsource

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeUnsupportedPlatform      ErrorCode = "unsupported-platform"
	CodeInvalidLimits            ErrorCode = "invalid-limits"
	CodeInvalidConfig            ErrorCode = "invalid-config"
	CodeInvalidSourceRequest     ErrorCode = "invalid-source-request"
	CodeInvalidRoute             ErrorCode = "invalid-route"
	CodeInvalidPullRequestNumber ErrorCode = "invalid-pull-request-number"
	CodeInvalidObjectFormat      ErrorCode = "invalid-object-format"
	CodeInvalidObjectID          ErrorCode = "invalid-object-id"
	CodeSourceRequestStale       ErrorCode = "source-request-stale"
	CodeSourceRequestSuperseded  ErrorCode = "source-request-superseded"
	CodeSourceLeaseStale         ErrorCode = "source-lease-stale"
	CodeBrokerUnavailable        ErrorCode = "broker-unavailable"
	CodeSourceTokenFailed        ErrorCode = "source-token-failed"
	CodeTokenScopeInvalid        ErrorCode = "token-scope-invalid"
	CodeGitImportFailed          ErrorCode = "git-import-failed"
	CodeBaseObjectUnavailable    ErrorCode = "base-object-unavailable"
	CodePullRefUnavailable       ErrorCode = "pull-ref-unavailable"
	CodePullRefMismatch          ErrorCode = "pull-ref-mismatch"
	CodeSourceStoreLimit         ErrorCode = "source-store-limit"
	CodeSourceStoreVerifyFailed  ErrorCode = "source-store-verify-failed"
	CodeControllerResultFailed   ErrorCode = "controller-result-failed"
	CodeMaxAttemptsExceeded      ErrorCode = "max-attempts-exceeded"
	CodeContextCancelled         ErrorCode = "context-cancelled"
	CodeImportTimeout            ErrorCode = "import-timeout"
)

type SourceError struct {
	Code       ErrorCode
	Stage, Msg string
	Err        error
}

func (e *SourceError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Msg)
}
func (e *SourceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
func (e *SourceError) Is(target error) bool {
	var t *SourceError
	return errors.As(target, &t) && t.Code == e.Code
}
func ErrCode(code ErrorCode) error { return &SourceError{Code: code} }
func errCode(code ErrorCode, stage, msg string, err error) error {
	return &SourceError{Code: code, Stage: stage, Msg: msg, Err: err}
}
func wrap(code ErrorCode, stage, msg string, err error) error { return errCode(code, stage, msg, err) }

func Diagnostic(err error) string {
	var e *SourceError
	if errors.As(err, &e) {
		return string(e.Code)
	}
	if err != nil {
		return "source-ingestion-failed"
	}
	return ""
}
