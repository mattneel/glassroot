package compare

const (
	MaxScenarios                = 64
	MaxAttempts                 = 1280
	MaxFactsPerAttempt          = 10000
	MaxFactsTotal               = 100000
	MaxDistinctSemanticVariants = 100000
	MaxDistinctAnchors          = 100000
	MaxVariantsPerAnchor        = 1024
	MaxDeltaRecords             = 100000
	MaxChangedFieldsPerRecord   = 128
	MaxEvidenceRefsPerRecord    = 1024
	MaxEvidenceRefsTotal        = 200000
	MaxLimitationsPerRecord     = 1000
	MaxLimitationsTotal         = 10000
	MaxDeltaJSONBytes           = 64 << 20
)

type Limits struct {
	MaxScenarios                int64
	MaxAttempts                 int64
	MaxFactsPerAttempt          int64
	MaxFactsTotal               int64
	MaxDistinctSemanticVariants int64
	MaxDistinctAnchors          int64
	MaxVariantsPerAnchor        int64
	MaxDeltaRecords             int64
	MaxChangedFieldsPerRecord   int64
	MaxEvidenceRefsPerRecord    int64
	MaxEvidenceRefsTotal        int64
	MaxLimitationsPerRecord     int64
	MaxLimitationsTotal         int64
	MaxDeltaJSONBytes           int64
}

func DefaultLimits() Limits {
	return Limits{MaxScenarios: MaxScenarios, MaxAttempts: MaxAttempts, MaxFactsPerAttempt: MaxFactsPerAttempt, MaxFactsTotal: MaxFactsTotal, MaxDistinctSemanticVariants: MaxDistinctSemanticVariants, MaxDistinctAnchors: MaxDistinctAnchors, MaxVariantsPerAnchor: MaxVariantsPerAnchor, MaxDeltaRecords: MaxDeltaRecords, MaxChangedFieldsPerRecord: MaxChangedFieldsPerRecord, MaxEvidenceRefsPerRecord: MaxEvidenceRefsPerRecord, MaxEvidenceRefsTotal: MaxEvidenceRefsTotal, MaxLimitationsPerRecord: MaxLimitationsPerRecord, MaxLimitationsTotal: MaxLimitationsTotal, MaxDeltaJSONBytes: MaxDeltaJSONBytes}
}

func validateLimits(l Limits) (Limits, error) {
	if l == (Limits{}) {
		return Limits{}, errCode(CodeInvalidLimits, "limits", "", "zero", "limits must be explicit", nil)
	}
	checks := []struct {
		v, max int64
		name   string
	}{{l.MaxScenarios, MaxScenarios, "maxScenarios"}, {l.MaxAttempts, MaxAttempts, "maxAttempts"}, {l.MaxFactsPerAttempt, MaxFactsPerAttempt, "maxFactsPerAttempt"}, {l.MaxFactsTotal, MaxFactsTotal, "maxFactsTotal"}, {l.MaxDistinctSemanticVariants, MaxDistinctSemanticVariants, "maxVariants"}, {l.MaxDistinctAnchors, MaxDistinctAnchors, "maxAnchors"}, {l.MaxVariantsPerAnchor, MaxVariantsPerAnchor, "maxVariantsPerAnchor"}, {l.MaxDeltaRecords, MaxDeltaRecords, "maxDeltaRecords"}, {l.MaxChangedFieldsPerRecord, MaxChangedFieldsPerRecord, "maxChangedFields"}, {l.MaxEvidenceRefsPerRecord, MaxEvidenceRefsPerRecord, "maxEvidenceRefsPerRecord"}, {l.MaxEvidenceRefsTotal, MaxEvidenceRefsTotal, "maxEvidenceRefsTotal"}, {l.MaxLimitationsPerRecord, MaxLimitationsPerRecord, "maxLimitationsPerRecord"}, {l.MaxLimitationsTotal, MaxLimitationsTotal, "maxLimitationsTotal"}, {l.MaxDeltaJSONBytes, MaxDeltaJSONBytes, "maxDeltaJsonBytes"}}
	for _, c := range checks {
		if c.v <= 0 || c.v > c.max {
			return Limits{}, errCode(CodeInvalidLimits, "limits", "", c.name, "limit is out of bounds", nil)
		}
	}
	return l, nil
}
