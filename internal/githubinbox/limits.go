package githubinbox

const (
	AbsoluteMaxStateDirBytes        = 4096
	AbsoluteMaxDatabaseBytes        = int64(8 << 30)
	AbsoluteMaxLeaseOwnerBytes      = 128
	AbsoluteMaxClaimLimit           = 256
	AbsoluteMaxLeaseDurationSeconds = 24 * 60 * 60
)

type Limits struct {
	MaxStateDirBytes        int
	MaxDatabaseBytes        int64
	BusyTimeoutMilliseconds int
	MaxOpenConnections      int
	MaxIdleConnections      int
	MaxLeaseOwnerBytes      int
	MaxClaimLimit           int
	MaxLeaseDurationSeconds int
}

func DefaultLimits() Limits {
	return Limits{MaxStateDirBytes: AbsoluteMaxStateDirBytes, MaxDatabaseBytes: AbsoluteMaxDatabaseBytes, BusyTimeoutMilliseconds: 500, MaxOpenConnections: 4, MaxIdleConnections: 4, MaxLeaseOwnerBytes: AbsoluteMaxLeaseOwnerBytes, MaxClaimLimit: AbsoluteMaxClaimLimit, MaxLeaseDurationSeconds: AbsoluteMaxLeaseDurationSeconds}
}

func validateLimits(l Limits) error {
	if l.MaxStateDirBytes <= 0 || l.MaxStateDirBytes > AbsoluteMaxStateDirBytes || l.MaxDatabaseBytes <= 0 || l.MaxDatabaseBytes > AbsoluteMaxDatabaseBytes || l.BusyTimeoutMilliseconds <= 0 || l.BusyTimeoutMilliseconds > 30000 || l.MaxOpenConnections <= 0 || l.MaxOpenConnections > 16 || l.MaxIdleConnections < 0 || l.MaxIdleConnections > l.MaxOpenConnections || l.MaxLeaseOwnerBytes <= 0 || l.MaxLeaseOwnerBytes > AbsoluteMaxLeaseOwnerBytes || l.MaxClaimLimit <= 0 || l.MaxClaimLimit > AbsoluteMaxClaimLimit || l.MaxLeaseDurationSeconds <= 0 || l.MaxLeaseDurationSeconds > AbsoluteMaxLeaseDurationSeconds {
		return errCode(CodeInvalidLimits, "limits", "limits rejected", nil)
	}
	return nil
}
