package githubcontroller

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeInvalidLimits                  ErrorCode = "invalid-limits"
	CodeInvalidConfig                  ErrorCode = "invalid-config"
	CodeInvalidProjection              ErrorCode = "invalid-projection"
	CodeLegacyProjectionUnreconcilable ErrorCode = "legacy-projection-unreconcilable"
	CodeInboxClaimFailed               ErrorCode = "inbox-claim-failed"
	CodeInboxRecordInvalid             ErrorCode = "inbox-record-invalid"
	CodeInboxReleaseFailed             ErrorCode = "inbox-release-failed"
	CodeInboxAckFailed                 ErrorCode = "inbox-ack-failed"
	CodeReconcileLeaseBusy             ErrorCode = "reconcile-lease-busy"
	CodeReconcileLeaseStale            ErrorCode = "reconcile-lease-stale"
	CodeBrokerUnavailable              ErrorCode = "broker-unavailable"
	CodeTokenRequestFailed             ErrorCode = "token-request-failed"
	CodePullRequestReadFailed          ErrorCode = "pull-request-read-failed"
	CodePullRequestNotFound            ErrorCode = "pull-request-not-found"
	CodePullRequestResponseInvalid     ErrorCode = "pull-request-response-invalid"
	CodeRepositoryRouteInvalid         ErrorCode = "repository-route-invalid"
	CodeRepositoryIDMismatch           ErrorCode = "repository-id-mismatch"
	CodePullRequestNumberMismatch      ErrorCode = "pull-request-number-mismatch"
	CodeObjectIDInvalid                ErrorCode = "object-id-invalid"
	CodeTargetInvalid                  ErrorCode = "target-invalid"
	CodeGenerationOverflow             ErrorCode = "generation-overflow"
	CodeAttemptLimit                   ErrorCode = "attempt-limit"
	CodeSourceRequestFailed            ErrorCode = "source-request-failed"
	CodeCheckBindingNotFound           ErrorCode = "check-binding-not-found"
	CodeForeignCheckRun                ErrorCode = "foreign-check-run"
	CodeStaleRerequest                 ErrorCode = "stale-rerequest"
	CodeInstallationBlocked            ErrorCode = "installation-blocked"
	CodeControllerStoreFailed          ErrorCode = "controller-store-failed"
	CodeContextCancelled               ErrorCode = "context-cancelled"
	CodeProcessingTimeout              ErrorCode = "processing-timeout"
)

type ControllerError struct {
	Code       ErrorCode
	Stage, Msg string
	Err        error
}

func (e *ControllerError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Msg)
}
func (e *ControllerError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
func (e *ControllerError) Is(target error) bool {
	var t *ControllerError
	return errors.As(target, &t) && t.Code == e.Code
}
func errCode(code ErrorCode, stage, msg string, err error) error {
	return &ControllerError{Code: code, Stage: stage, Msg: msg, Err: err}
}

func Diagnostic(err error) string {
	var e *ControllerError
	if errors.As(err, &e) {
		return string(e.Code)
	}
	if err != nil {
		return "controller-failed"
	}
	return ""
}
