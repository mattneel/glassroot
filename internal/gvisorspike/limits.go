package gvisorspike

import "time"

const (
	SchemaVersion        = "glassroot.dev/gvisor-spike/v1alpha1"
	MonitorEventVersion  = "glassroot.dev/gvisor-monitor-event/v1alpha1"
	PinnedRunscRelease   = "release-20260615.0"
	PinnedRunscCommit    = "57efc92f6df8f530b5cc49cc197077f9c3dafe98"
	MonitorModuleVersion = "v0.0.0-20260613051822-57efc92f6df8"
)

type Limits struct {
	MaxPathBytes             int64
	MaxTracePoints           int64
	MaxContextFields         int64
	MaxStringBytes           int64
	MaxProcesses             int64
	MaxLimitations           int64
	MaxConnectionDuration    time.Duration
	MaxFixtureExecution      time.Duration
	MaxMonitorMessages       int64
	MaxMonitorConnections    int64
	MaxMonitorMessageBytes   int64
	MaxTotalBytesPerConn     int64
	MaxMonitorHandshakeBytes int64
}

func DefaultLimits() Limits {
	return Limits{MaxPathBytes: 4096, MaxTracePoints: 64, MaxContextFields: 16, MaxStringBytes: 64 << 10, MaxProcesses: 100000, MaxLimitations: 1000, MaxConnectionDuration: 10 * time.Minute, MaxFixtureExecution: 2 * time.Minute, MaxMonitorMessages: 100000, MaxMonitorConnections: 8, MaxMonitorMessageBytes: 1 << 20, MaxTotalBytesPerConn: 256 << 20, MaxMonitorHandshakeBytes: 64 << 10}
}

func validateLimits(l Limits) (Limits, error) {
	ceil := DefaultLimits()
	if l == (Limits{}) {
		return ceil, nil
	}
	if l.MaxPathBytes <= 0 || l.MaxPathBytes > ceil.MaxPathBytes || l.MaxTracePoints <= 0 || l.MaxTracePoints > ceil.MaxTracePoints || l.MaxContextFields <= 0 || l.MaxContextFields > ceil.MaxContextFields || l.MaxStringBytes <= 0 || l.MaxStringBytes > ceil.MaxStringBytes || l.MaxProcesses <= 0 || l.MaxProcesses > ceil.MaxProcesses || l.MaxLimitations <= 0 || l.MaxLimitations > ceil.MaxLimitations || l.MaxConnectionDuration <= 0 || l.MaxConnectionDuration > ceil.MaxConnectionDuration || l.MaxFixtureExecution <= 0 || l.MaxFixtureExecution > ceil.MaxFixtureExecution || l.MaxMonitorMessages <= 0 || l.MaxMonitorMessages > ceil.MaxMonitorMessages || l.MaxMonitorConnections <= 0 || l.MaxMonitorConnections > ceil.MaxMonitorConnections || l.MaxMonitorMessageBytes <= 0 || l.MaxMonitorMessageBytes > ceil.MaxMonitorMessageBytes || l.MaxTotalBytesPerConn <= 0 || l.MaxTotalBytesPerConn > ceil.MaxTotalBytesPerConn || l.MaxMonitorHandshakeBytes <= 0 || l.MaxMonitorHandshakeBytes > ceil.MaxMonitorHandshakeBytes {
		return Limits{}, errCode(CodeInvalidLimits, "limits", "", "gVisor spike limits are invalid", nil)
	}
	return l, nil
}
