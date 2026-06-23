package monitor

import (
	"testing"

	"github.com/mattneel/glassroot/tools/gvisormonitor/internal/model"
)

func TestStateMachineIsScopedPerConnection(t *testing.T) {
	m := NewStateMachine()
	for _, ev := range []model.Event{
		{ConnectionID: "a", Sequence: 1, Operation: model.OperationContainerStart, ThreadGroupID: 1},
		{ConnectionID: "b", Sequence: 1, Operation: model.OperationContainerStart, ThreadGroupID: 1},
		{ConnectionID: "a", Sequence: 2, Operation: model.OperationClone, ThreadGroupID: 1, ChildThreadGroupID: 2},
		{ConnectionID: "a", Sequence: 3, Operation: model.OperationExec, ThreadGroupID: 2, ExecutablePath: "/child"},
		{ConnectionID: "a", Sequence: 4, Operation: model.OperationExit, ThreadGroupID: 2},
	} {
		if err := m.Apply(ev); err != nil {
			t.Fatalf("Apply(%+v) error = %v", ev, err)
		}
	}
	out := m.Summary()
	if out.ConnectionCount != 2 || out.ProcessCreations != 1 || out.Execs != 1 || out.Exits != 1 {
		t.Fatalf("unexpected summary: %+v", out)
	}
}
