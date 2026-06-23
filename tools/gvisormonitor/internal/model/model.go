package model

type Operation string

const (
	OperationContainerStart Operation = "container-start"
	OperationClone          Operation = "clone"
	OperationExec           Operation = "exec"
	OperationExit           Operation = "exit"
	OperationUnsupported    Operation = "unsupported"
)

type Event struct {
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
	ContainerIdentity   string    `json:"containerIdentity,omitempty"`
	Limitations         []string  `json:"limitations"`
}

type Summary struct {
	ConnectionCount  int  `json:"connectionCount"`
	ProcessCreations int  `json:"processCreations"`
	Execs            int  `json:"execs"`
	Exits            int  `json:"exits"`
	Incomplete       int  `json:"incomplete"`
	Complete         bool `json:"complete"`
}
