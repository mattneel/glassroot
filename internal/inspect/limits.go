package inspect

import (
	"time"

	"github.com/mattneel/glassroot/internal/compare"
	"github.com/mattneel/glassroot/internal/evidence"
	"github.com/mattneel/glassroot/internal/observe"
	"github.com/mattneel/glassroot/internal/policy"
	"github.com/mattneel/glassroot/internal/report"
)

const MaxInspectDurationAbsolute = 30 * time.Minute

type Limits struct {
	Reader             evidence.ReaderLimits
	Normalize          observe.Limits
	Compare            compare.Limits
	Policy             policy.Limits
	Application        policy.ApplicationLimits
	Report             report.Limits
	MaxInspectDuration time.Duration
}

func DefaultLimits() Limits {
	return Limits{
		Reader:             evidence.DefaultReaderLimits(),
		Normalize:          observe.DefaultLimits(),
		Compare:            compare.DefaultLimits(),
		Policy:             policy.DefaultLimits(),
		Application:        policy.DefaultApplicationLimits(),
		Report:             report.DefaultLimits(),
		MaxInspectDuration: MaxInspectDurationAbsolute,
	}
}

func validateLimits(l Limits) (Limits, error) {
	if l == (Limits{}) {
		return Limits{}, errCode(CodeInvalidLimits, "limits", "zero-value limits are invalid", nil)
	}
	if l.MaxInspectDuration <= 0 || l.MaxInspectDuration > MaxInspectDurationAbsolute {
		return Limits{}, errCode(CodeInvalidLimits, "limits", "inspect duration limit outside allowed range", nil)
	}
	if _, err := observe.New(l.Normalize); err != nil {
		return Limits{}, errCode(CodeInvalidLimits, "limits", "normalization limits invalid", err)
	}
	if _, err := compare.New(l.Compare); err != nil {
		return Limits{}, errCode(CodeInvalidLimits, "limits", "comparison limits invalid", err)
	}
	if _, err := policy.New(l.Policy); err != nil {
		return Limits{}, errCode(CodeInvalidLimits, "limits", "policy limits invalid", err)
	}
	if _, err := policy.NewApplier(l.Application); err != nil {
		return Limits{}, errCode(CodeInvalidLimits, "limits", "application limits invalid", err)
	}
	if _, err := report.New(l.Report); err != nil {
		return Limits{}, errCode(CodeInvalidLimits, "limits", "report limits invalid", err)
	}
	return l, nil
}
