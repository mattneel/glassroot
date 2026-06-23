package policy

const (
	MaxDeltaRecords             = 100000
	MaxFindings                 = 100000
	MaxFindingsPerDeltaRecord   = 8
	MaxDeltaRecordIDsPerFinding = 128
	MaxScenarioIDsPerFinding    = 64
	MaxEvidenceRefsPerFinding   = 1024
	MaxEvidenceRefsTotal        = 200000
	MaxLimitationsPerFinding    = 1000
	MaxLimitationsTotal         = 10000
	MaxTitleBytes               = 256
	MaxSummaryBytes             = 1024
	MaxRuleIDBytes              = 128
	MaxRuleVersionBytes         = 128
	MaxEvaluationJSONBytes      = 64 << 20
)

type Limits struct {
	MaxDeltaRecords             int64
	MaxFindings                 int64
	MaxFindingsPerDeltaRecord   int64
	MaxDeltaRecordIDsPerFinding int64
	MaxScenarioIDsPerFinding    int64
	MaxEvidenceRefsPerFinding   int64
	MaxEvidenceRefsTotal        int64
	MaxLimitationsPerFinding    int64
	MaxLimitationsTotal         int64
	MaxTitleBytes               int64
	MaxSummaryBytes             int64
	MaxRuleIDBytes              int64
	MaxRuleVersionBytes         int64
	MaxEvaluationJSONBytes      int64
}

func DefaultLimits() Limits {
	return Limits{MaxDeltaRecords: MaxDeltaRecords, MaxFindings: MaxFindings, MaxFindingsPerDeltaRecord: MaxFindingsPerDeltaRecord, MaxDeltaRecordIDsPerFinding: MaxDeltaRecordIDsPerFinding, MaxScenarioIDsPerFinding: MaxScenarioIDsPerFinding, MaxEvidenceRefsPerFinding: MaxEvidenceRefsPerFinding, MaxEvidenceRefsTotal: MaxEvidenceRefsTotal, MaxLimitationsPerFinding: MaxLimitationsPerFinding, MaxLimitationsTotal: MaxLimitationsTotal, MaxTitleBytes: MaxTitleBytes, MaxSummaryBytes: MaxSummaryBytes, MaxRuleIDBytes: MaxRuleIDBytes, MaxRuleVersionBytes: MaxRuleVersionBytes, MaxEvaluationJSONBytes: MaxEvaluationJSONBytes}
}

func validateLimits(l Limits) (Limits, error) {
	if l == (Limits{}) {
		return Limits{}, errCode(CodeInvalidLimits, "limits", "", "", "zero", "limits must be explicit", nil)
	}
	checks := []struct {
		v, max int64
		name   string
	}{
		{l.MaxDeltaRecords, MaxDeltaRecords, "maxDeltaRecords"}, {l.MaxFindings, MaxFindings, "maxFindings"}, {l.MaxFindingsPerDeltaRecord, MaxFindingsPerDeltaRecord, "maxFindingsPerDeltaRecord"}, {l.MaxDeltaRecordIDsPerFinding, MaxDeltaRecordIDsPerFinding, "maxDeltaRecordIdsPerFinding"}, {l.MaxScenarioIDsPerFinding, MaxScenarioIDsPerFinding, "maxScenarioIdsPerFinding"}, {l.MaxEvidenceRefsPerFinding, MaxEvidenceRefsPerFinding, "maxEvidenceRefsPerFinding"}, {l.MaxEvidenceRefsTotal, MaxEvidenceRefsTotal, "maxEvidenceRefsTotal"}, {l.MaxLimitationsPerFinding, MaxLimitationsPerFinding, "maxLimitationsPerFinding"}, {l.MaxLimitationsTotal, MaxLimitationsTotal, "maxLimitationsTotal"}, {l.MaxTitleBytes, MaxTitleBytes, "maxTitleBytes"}, {l.MaxSummaryBytes, MaxSummaryBytes, "maxSummaryBytes"}, {l.MaxRuleIDBytes, MaxRuleIDBytes, "maxRuleIdBytes"}, {l.MaxRuleVersionBytes, MaxRuleVersionBytes, "maxRuleVersionBytes"}, {l.MaxEvaluationJSONBytes, MaxEvaluationJSONBytes, "maxEvaluationJsonBytes"},
	}
	for _, c := range checks {
		if c.v <= 0 || c.v > c.max {
			return Limits{}, errCode(CodeInvalidLimits, "limits", "", "", c.name, "limit is out of bounds", nil)
		}
	}
	return l, nil
}
