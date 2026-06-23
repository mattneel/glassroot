package gvisorspike

import (
	"strings"
	"testing"
)

func FuzzBuildPodInitConfiguration(f *testing.F) {
	f.Add("/tmp/glassroot/events.sock", "container/start", "sentry/clone", "syscall/execve/enter")
	f.Add("relative", "", "\x00", strings.Repeat("a", 5000))
	for _, p := range DefaultTracePoints() {
		_ = p
	}
	f.Fuzz(func(t *testing.T, endpoint, a, b, c string) {
		inventory := append(DefaultTracePoints(), a, b, c)
		_, _ = BuildPodInitConfiguration(PodInitRequest{Endpoint: endpoint, TraceInventory: inventory})
	})
}

func FuzzApplyProcessLifecycleEvent(f *testing.F) {
	f.Add("sandbox", uint64(1), int64(1), int64(2), string(OperationClone))
	f.Fuzz(func(t *testing.T, conn string, seq uint64, tgid, child int64, op string) {
		tr := NewProcessTracker()
		_ = tr.Apply(MonitorEvent{ConnectionID: conn, Sequence: 1, Operation: OperationContainerStart, ThreadGroupID: 1})
		_ = tr.Apply(MonitorEvent{ConnectionID: conn, Sequence: seq, Operation: Operation(op), ThreadGroupID: tgid, ChildThreadGroupID: child})
	})
}
