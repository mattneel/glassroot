package monitor

import (
	"errors"

	"github.com/mattneel/glassroot/tools/gvisormonitor/internal/model"
)

type StateMachine struct {
	conns   map[string]map[int64]state
	summary model.Summary
}
type state struct{ created, execed, exited bool }

func NewStateMachine() *StateMachine { return &StateMachine{conns: map[string]map[int64]state{}} }
func (m *StateMachine) Apply(ev model.Event) error {
	if ev.ConnectionID == "" {
		ev.ConnectionID = "default"
	}
	if m.conns[ev.ConnectionID] == nil {
		m.conns[ev.ConnectionID] = map[int64]state{}
		m.summary.ConnectionCount++
	}
	s := m.conns[ev.ConnectionID]
	switch ev.Operation {
	case model.OperationContainerStart:
		if ev.ThreadGroupID != 0 {
			s[ev.ThreadGroupID] = state{created: true}
		}
	case model.OperationClone:
		if ev.ChildThreadGroupID == 0 {
			return errors.New("missing child")
		}
		if st := s[ev.ChildThreadGroupID]; st.created && !st.exited {
			return errors.New("active reuse")
		}
		s[ev.ChildThreadGroupID] = state{created: true}
		m.summary.ProcessCreations++
	case model.OperationExec:
		st := s[ev.ThreadGroupID]
		if !st.created || st.exited {
			return errors.New("exec without process")
		}
		st.execed = true
		s[ev.ThreadGroupID] = st
		m.summary.Execs++
	case model.OperationExit:
		st := s[ev.ThreadGroupID]
		if !st.created || st.exited {
			return errors.New("exit without process")
		}
		st.exited = true
		s[ev.ThreadGroupID] = st
		m.summary.Exits++
	case model.OperationUnsupported:
		return nil
	}
	return nil
}
func (m *StateMachine) Summary() model.Summary {
	out := m.summary
	for _, states := range m.conns {
		for _, st := range states {
			if st.created && !st.exited {
				out.Incomplete++
			}
		}
	}
	out.Complete = out.ConnectionCount > 0 && out.ProcessCreations > 0 && out.Execs > 0 && out.Exits > 0 && out.Incomplete == 0
	return out
}
