package waiver

const (
	MaxWaiverFileBytes    int64 = 256 << 10
	MaxYAMLDepth                = 24
	MaxYAMLNodes                = 10000
	MaxDiagnostics              = 100
	MaxGeneralScalarBytes       = 4 << 10
	MaxWaivers            int64 = 1000
	MaxOwnerBytes               = 256
	MaxReasonBytes              = 1024
	MaxWaiverLifetimeDays       = 90
)

type Limits struct {
	MaxWaiverFileBytes int64
	MaxYAMLDepth       int
	MaxYAMLNodes       int
	MaxDiagnostics     int
	MaxScalarBytes     int
	MaxWaivers         int64
	MaxOwnerBytes      int
	MaxReasonBytes     int
	MaxLifetimeDays    int
}

func DefaultLimits() Limits {
	return Limits{MaxWaiverFileBytes: MaxWaiverFileBytes, MaxYAMLDepth: MaxYAMLDepth, MaxYAMLNodes: MaxYAMLNodes, MaxDiagnostics: MaxDiagnostics, MaxScalarBytes: MaxGeneralScalarBytes, MaxWaivers: MaxWaivers, MaxOwnerBytes: MaxOwnerBytes, MaxReasonBytes: MaxReasonBytes, MaxLifetimeDays: MaxWaiverLifetimeDays}
}

func validateLimits(l Limits) (Limits, error) {
	if l == (Limits{}) {
		return Limits{}, errCode(CodeInvalidValue, "limits", "zero limits")
	}
	abs := DefaultLimits()
	if l.MaxWaiverFileBytes <= 0 || l.MaxWaiverFileBytes > abs.MaxWaiverFileBytes || l.MaxYAMLDepth <= 0 || l.MaxYAMLDepth > abs.MaxYAMLDepth || l.MaxYAMLNodes <= 0 || l.MaxYAMLNodes > abs.MaxYAMLNodes || l.MaxDiagnostics <= 0 || l.MaxDiagnostics > abs.MaxDiagnostics || l.MaxScalarBytes <= 0 || l.MaxScalarBytes > abs.MaxScalarBytes || l.MaxWaivers <= 0 || l.MaxWaivers > abs.MaxWaivers || l.MaxOwnerBytes <= 0 || l.MaxOwnerBytes > abs.MaxOwnerBytes || l.MaxReasonBytes <= 0 || l.MaxReasonBytes > abs.MaxReasonBytes || l.MaxLifetimeDays <= 0 || l.MaxLifetimeDays > abs.MaxLifetimeDays {
		return Limits{}, errCode(CodeInvalidValue, "limits", "invalid limits")
	}
	return l, nil
}
