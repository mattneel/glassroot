package gvisorspike

import "sort"

type Operation string

const (
	OperationContainerStart Operation = "container-start"
	OperationClone          Operation = "clone"
	OperationExec           Operation = "exec"
	OperationExit           Operation = "exit"
	OperationUnsupported    Operation = "unsupported"
)

type MonitorEvent struct {
	SchemaVersion       string    `json:"schemaVersion"`
	ConnectionID        string    `json:"connectionId"`
	Sequence            uint64    `json:"sequence"`
	TracePoint          string    `json:"tracePoint"`
	Operation           Operation `json:"operation"`
	TimestampNanos      int64     `json:"timestampNanos,omitempty"`
	ThreadID            int64     `json:"threadId,omitempty"`
	ThreadGroupID       int64     `json:"threadGroupId,omitempty"`
	ChildThreadGroupID  int64     `json:"childThreadGroupId,omitempty"`
	ParentThreadGroupID int64     `json:"parentThreadGroupId,omitempty"`
	ProcessName         string    `json:"processName,omitempty"`
	ExecutablePath      string    `json:"executablePath,omitempty"`
	ExitStatus          *int      `json:"exitStatus,omitempty"`
	Limitations         []string  `json:"limitations"`
}

type processState struct {
	Generation int
	Created    bool
	Execed     bool
	Exited     bool
}

type ProcessTracker struct {
	perConn map[string]map[int64]processState
	started map[string]bool
	summary ProcessSummary
}

type ProcessSummary struct {
	ContainerStarted bool     `json:"containerStarted"`
	ProcessCreations int      `json:"processCreations"`
	Execs            int      `json:"execs"`
	Exits            int      `json:"exits"`
	Incomplete       int      `json:"incomplete"`
	Complete         bool     `json:"complete"`
	Connections      []string `json:"connections"`
}

func NewProcessTracker() *ProcessTracker {
	return &ProcessTracker{perConn: map[string]map[int64]processState{}, started: map[string]bool{}}
}
func (t *ProcessTracker) Apply(ev MonitorEvent) error {
	if ev.ConnectionID == "" {
		ev.ConnectionID = "default"
	}
	if t.perConn[ev.ConnectionID] == nil {
		t.perConn[ev.ConnectionID] = map[int64]processState{}
	}
	states := t.perConn[ev.ConnectionID]
	switch ev.Operation {
	case OperationContainerStart:
		if t.started[ev.ConnectionID] {
			return errCode(CodeProcessStateInvalid, "process", "container-start", "duplicate container start", nil)
		}
		t.started[ev.ConnectionID] = true
		t.summary.ContainerStarted = true
		if ev.ThreadGroupID != 0 {
			states[ev.ThreadGroupID] = processState{Generation: 1, Created: true}
		}
	case OperationClone:
		if ev.ChildThreadGroupID == 0 {
			return errCode(CodeProcessStateInvalid, "process", "clone", "missing child thread group", nil)
		}
		if st, ok := states[ev.ChildThreadGroupID]; ok && !st.Exited {
			return errCode(CodeProcessStateInvalid, "process", "clone", "active thread group reused", nil)
		}
		gen := states[ev.ChildThreadGroupID].Generation + 1
		states[ev.ChildThreadGroupID] = processState{Generation: gen, Created: true}
		t.summary.ProcessCreations++
	case OperationExec:
		st := states[ev.ThreadGroupID]
		if !st.Created || st.Exited {
			return errCode(CodeProcessStateInvalid, "process", "exec", "exec without active process", nil)
		}
		st.Execed = true
		states[ev.ThreadGroupID] = st
		t.summary.Execs++
	case OperationExit:
		st := states[ev.ThreadGroupID]
		if !st.Created || st.Exited {
			return errCode(CodeProcessStateInvalid, "process", "exit", "exit without active process", nil)
		}
		st.Exited = true
		states[ev.ThreadGroupID] = st
		t.summary.Exits++
	case OperationUnsupported:
		return nil
	default:
		return errCode(CodeProcessStateInvalid, "process", "operation", "unknown operation", nil)
	}
	return nil
}
func (t *ProcessTracker) Summary() ProcessSummary {
	out := t.summary
	for conn, states := range t.perConn {
		out.Connections = append(out.Connections, conn)
		for _, st := range states {
			if st.Created && !st.Exited {
				out.Incomplete++
			}
		}
	}
	sort.Strings(out.Connections)
	out.Complete = out.ContainerStarted && out.ProcessCreations > 0 && out.Execs > 0 && out.Exits > 0 && out.Incomplete == 0
	return out
}
