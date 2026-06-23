package observe

const (
	MaxAttempts                      = 1280
	MaxFactsPerAttempt               = 10000
	MaxFactsPerTraceSet              = 100000
	MaxEvidenceRefsPerFact           = 16
	MaxEvidenceRefsTotal             = 200000
	MaxProcessGenerationsPerAttempt  = 10000
	MaxActiveProcessesPerAttempt     = 10000
	MaxNormalizedStringBytes         = 64 << 10
	MaxNormalizedArgumentsPerProcess = 4096
	MaxNormalizedArgumentBytes       = 256 << 10
	MaxPathRoots                     = 32
	MaxLimitationsPerAttempt         = 1000
	MaxLimitationsTotal              = 10000
	MaxLimitationCodeBytes           = 128
	MaxLimitationMessageBytes        = 1 << 10
)

type Limits struct {
	MaxAttempts                     int64
	MaxFactsPerAttempt              int64
	MaxFactsPerTraceSet             int64
	MaxEvidenceRefsPerFact          int64
	MaxEvidenceRefsTotal            int64
	MaxProcessGenerationsPerAttempt int64
	MaxActiveProcessesPerAttempt    int64
	MaxNormalizedStringBytes        int64
	MaxNormalizedArguments          int64
	MaxNormalizedArgumentBytes      int64
	MaxPathRoots                    int64
	MaxLimitationsPerAttempt        int64
	MaxLimitationsTotal             int64
}

func DefaultLimits() Limits {
	return Limits{MaxAttempts: MaxAttempts, MaxFactsPerAttempt: MaxFactsPerAttempt, MaxFactsPerTraceSet: MaxFactsPerTraceSet, MaxEvidenceRefsPerFact: MaxEvidenceRefsPerFact, MaxEvidenceRefsTotal: MaxEvidenceRefsTotal, MaxProcessGenerationsPerAttempt: MaxProcessGenerationsPerAttempt, MaxActiveProcessesPerAttempt: MaxActiveProcessesPerAttempt, MaxNormalizedStringBytes: MaxNormalizedStringBytes, MaxNormalizedArguments: MaxNormalizedArgumentsPerProcess, MaxNormalizedArgumentBytes: MaxNormalizedArgumentBytes, MaxPathRoots: MaxPathRoots, MaxLimitationsPerAttempt: MaxLimitationsPerAttempt, MaxLimitationsTotal: MaxLimitationsTotal}
}

func validateLimits(l Limits) (Limits, error) {
	if l == (Limits{}) {
		return Limits{}, errCode(CodeInvalidLimits, "limits", "", "zero", "limits must be explicit", nil)
	}
	checks := []struct {
		v, max int64
		name   string
	}{{l.MaxAttempts, MaxAttempts, "maxAttempts"}, {l.MaxFactsPerAttempt, MaxFactsPerAttempt, "maxFactsPerAttempt"}, {l.MaxFactsPerTraceSet, MaxFactsPerTraceSet, "maxFactsPerTraceSet"}, {l.MaxEvidenceRefsPerFact, MaxEvidenceRefsPerFact, "maxEvidenceRefsPerFact"}, {l.MaxEvidenceRefsTotal, MaxEvidenceRefsTotal, "maxEvidenceRefsTotal"}, {l.MaxProcessGenerationsPerAttempt, MaxProcessGenerationsPerAttempt, "maxProcessGenerations"}, {l.MaxActiveProcessesPerAttempt, MaxActiveProcessesPerAttempt, "maxActiveProcesses"}, {l.MaxNormalizedStringBytes, MaxNormalizedStringBytes, "maxStringBytes"}, {l.MaxNormalizedArguments, MaxNormalizedArgumentsPerProcess, "maxArguments"}, {l.MaxNormalizedArgumentBytes, MaxNormalizedArgumentBytes, "maxArgumentBytes"}, {l.MaxPathRoots, MaxPathRoots, "maxPathRoots"}, {l.MaxLimitationsPerAttempt, MaxLimitationsPerAttempt, "maxAttemptLimitations"}, {l.MaxLimitationsTotal, MaxLimitationsTotal, "maxLimitations"}}
	for _, c := range checks {
		if c.v <= 0 || c.v > int64(c.max) {
			return Limits{}, errCode(CodeInvalidLimits, "limits", "", c.name, "limit is out of bounds", nil)
		}
	}
	return l, nil
}
