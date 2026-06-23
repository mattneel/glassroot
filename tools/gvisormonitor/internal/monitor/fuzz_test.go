package monitor

import (
	"testing"

	"github.com/mattneel/glassroot/tools/gvisormonitor/internal/model"
)

func FuzzApplyProcessLifecycleEvent(f *testing.F) {
	f.Add("sandbox", uint64(1), int64(1), int64(2), string(model.OperationClone))
	f.Fuzz(func(t *testing.T, conn string, seq uint64, tgid, child int64, op string) {
		m := NewStateMachine()
		_ = m.Apply(model.Event{ConnectionID: conn, Sequence: 1, Operation: model.OperationContainerStart, ThreadGroupID: 1})
		_ = m.Apply(model.Event{ConnectionID: conn, Sequence: seq, Operation: model.Operation(op), ThreadGroupID: tgid, ChildThreadGroupID: child})
	})
}
