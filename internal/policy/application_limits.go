package policy

const (
	MaxApplicationWaivers            int64 = 1000
	MaxApplicationWaiverFileBytes    int64 = 256 << 10
	MaxApplicationWaiverLifetimeDays int64 = 90
	MaxApplicationOwnerBytes         int64 = 256
	MaxApplicationReasonBytes        int64 = 1024
	MaxWaiverChanges                 int64 = 10000
	MaxOriginalFindings              int64 = 100000
	MaxGovernanceFindings            int64 = 20000
	MaxAppliedFindings               int64 = 120000
	MaxAppliedWaiverMetadata         int64 = 1000
	MaxGovernanceReferences          int64 = 20000
	MaxApplicationLimitationsTotal   int64 = 20000
	MaxApplicationJSONBytes          int64 = 64 << 20
)

type ApplicationLimits struct {
	MaxWaivers               int64
	MaxWaiverFileBytes       int64
	MaxWaiverLifetimeDays    int64
	MaxOwnerBytes            int64
	MaxReasonBytes           int64
	MaxWaiverChanges         int64
	MaxOriginalFindings      int64
	MaxGovernanceFindings    int64
	MaxAppliedFindings       int64
	MaxAppliedWaiverMetadata int64
	MaxGovernanceReferences  int64
	MaxLimitationsTotal      int64
	MaxApplicationJSONBytes  int64
}

func DefaultApplicationLimits() ApplicationLimits {
	return ApplicationLimits{MaxWaivers: MaxApplicationWaivers, MaxWaiverFileBytes: MaxApplicationWaiverFileBytes, MaxWaiverLifetimeDays: MaxApplicationWaiverLifetimeDays, MaxOwnerBytes: MaxApplicationOwnerBytes, MaxReasonBytes: MaxApplicationReasonBytes, MaxWaiverChanges: MaxWaiverChanges, MaxOriginalFindings: MaxOriginalFindings, MaxGovernanceFindings: MaxGovernanceFindings, MaxAppliedFindings: MaxAppliedFindings, MaxAppliedWaiverMetadata: MaxAppliedWaiverMetadata, MaxGovernanceReferences: MaxGovernanceReferences, MaxLimitationsTotal: MaxApplicationLimitationsTotal, MaxApplicationJSONBytes: MaxApplicationJSONBytes}
}

func validateApplicationLimits(l ApplicationLimits) (ApplicationLimits, error) {
	if l == (ApplicationLimits{}) {
		return ApplicationLimits{}, errCode(CodeInvalidLimits, "application-limits", "", "", "zero", "limits must be explicit", nil)
	}
	checks := []struct {
		v, max int64
		name   string
	}{{l.MaxWaivers, MaxApplicationWaivers, "maxWaivers"}, {l.MaxWaiverFileBytes, MaxApplicationWaiverFileBytes, "maxWaiverFileBytes"}, {l.MaxWaiverLifetimeDays, MaxApplicationWaiverLifetimeDays, "maxWaiverLifetimeDays"}, {l.MaxOwnerBytes, MaxApplicationOwnerBytes, "maxOwnerBytes"}, {l.MaxReasonBytes, MaxApplicationReasonBytes, "maxReasonBytes"}, {l.MaxWaiverChanges, MaxWaiverChanges, "maxWaiverChanges"}, {l.MaxOriginalFindings, MaxOriginalFindings, "maxOriginalFindings"}, {l.MaxGovernanceFindings, MaxGovernanceFindings, "maxGovernanceFindings"}, {l.MaxAppliedFindings, MaxAppliedFindings, "maxAppliedFindings"}, {l.MaxAppliedWaiverMetadata, MaxAppliedWaiverMetadata, "maxAppliedWaiverMetadata"}, {l.MaxGovernanceReferences, MaxGovernanceReferences, "maxGovernanceReferences"}, {l.MaxLimitationsTotal, MaxApplicationLimitationsTotal, "maxLimitationsTotal"}, {l.MaxApplicationJSONBytes, MaxApplicationJSONBytes, "maxApplicationJSONBytes"}}
	for _, c := range checks {
		if c.v <= 0 || c.v > c.max {
			return ApplicationLimits{}, errCode(CodeInvalidLimits, "application-limits", "", "", c.name, "limit is out of bounds", nil)
		}
	}
	return l, nil
}
