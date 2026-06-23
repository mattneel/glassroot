package model

import "time"

// ObservationEvent is an independently serialized JSONL event. Exactly-one
// payload validation is intentionally deferred to later validation code.
type ObservationEvent struct {
	SchemaVersion   SchemaVersion               `json:"schemaVersion"`
	ID              string                      `json:"id"`
	RunID           string                      `json:"runId"`
	Revision        RevisionKind                `json:"revision"`
	ScenarioID      string                      `json:"scenarioId"`
	SequenceNumber  int64                       `json:"sequenceNumber"`
	ObservedAt      time.Time                   `json:"observedAt"`
	Source          ObservationSource           `json:"source"`
	Kind            ObservationKind             `json:"kind"`
	Process         *ProcessObservation         `json:"process,omitempty"`
	Filesystem      *FilesystemObservation      `json:"filesystem,omitempty"`
	Network         *NetworkObservation         `json:"network,omitempty"`
	Artifact        *ArtifactObservation        `json:"artifact,omitempty"`
	Scenario        *ScenarioObservation        `json:"scenario,omitempty"`
	ObserverWarning *ObserverWarningObservation `json:"observerWarning,omitempty"`
	ResourceLimit   *ResourceLimitObservation   `json:"resourceLimit,omitempty"`
}

// ObservationSource records provenance using Glassroot trust-boundary terms.
type ObservationSource string

const (
	ObservationSourceHostObserved           ObservationSource = "host-observed"
	ObservationSourceNetworkBrokerObserved  ObservationSource = "network-broker-observed"
	ObservationSourceSandboxRuntimeObserved ObservationSource = "sandbox-runtime-observed"
	ObservationSourceGuestAgentReported     ObservationSource = "guest-agent-reported"
	ObservationSourceWorkloadReported       ObservationSource = "workload-reported"
	ObservationSourceStaticAnalysisDerived  ObservationSource = "static-analysis-derived"
	ObservationSourceModelInferred          ObservationSource = "model-inferred"
)

// ObservationKind identifies the category of typed observation payload.
type ObservationKind string

const (
	ObservationKindProcessStart           ObservationKind = "process-start"
	ObservationKindProcessExit            ObservationKind = "process-exit"
	ObservationKindFilesystemCreate       ObservationKind = "filesystem-create"
	ObservationKindFilesystemRead         ObservationKind = "filesystem-read"
	ObservationKindFilesystemWrite        ObservationKind = "filesystem-write"
	ObservationKindFilesystemDelete       ObservationKind = "filesystem-delete"
	ObservationKindFilesystemRename       ObservationKind = "filesystem-rename"
	ObservationKindFilesystemChmod        ObservationKind = "filesystem-chmod"
	ObservationKindDNSQuery               ObservationKind = "dns-query"
	ObservationKindNetworkConnection      ObservationKind = "network-connection"
	ObservationKindArtifactActivity       ObservationKind = "artifact-activity"
	ObservationKindScenarioStarted        ObservationKind = "scenario-started"
	ObservationKindScenarioCompleted      ObservationKind = "scenario-completed"
	ObservationKindObserverWarning        ObservationKind = "observer-warning"
	ObservationKindUnsupportedObservation ObservationKind = "unsupported-observation"
	ObservationKindResourceLimit          ObservationKind = "resource-limit"
)

// ProcessObservation records process lifecycle facts.
type ProcessObservation struct {
	Operation       string     `json:"operation"`
	ProcessID       int64      `json:"processId"`
	ParentProcessID *int64     `json:"parentProcessId,omitempty"`
	ExecutablePath  string     `json:"executablePath"`
	Arguments       []string   `json:"arguments"`
	Environment     []EnvEntry `json:"environment"`
	ExitCode        *int       `json:"exitCode,omitempty"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	ExitedAt        *time.Time `json:"exitedAt,omitempty"`
	DurationMillis  int64      `json:"durationMillis"`
}

// FilesystemObservation records filesystem activity without reading paths.
type FilesystemObservation struct {
	Operation  string `json:"operation"`
	Path       string `json:"path"`
	OldPath    string `json:"oldPath,omitempty"`
	Mode       string `json:"mode,omitempty"`
	Digest     Digest `json:"digest,omitempty"`
	SizeBytes  int64  `json:"sizeBytes"`
	Executable bool   `json:"executable"`
	Truncated  bool   `json:"truncated"`
}

// NetworkObservation records DNS and network activity facts.
type NetworkObservation struct {
	Operation         string   `json:"operation"`
	Protocol          string   `json:"protocol"`
	QueryName         string   `json:"queryName,omitempty"`
	DestinationHost   string   `json:"destinationHost,omitempty"`
	DestinationPort   int      `json:"destinationPort,omitempty"`
	ResolvedAddresses []string `json:"resolvedAddresses"`
	Result            string   `json:"result"`
	DurationMillis    int64    `json:"durationMillis,omitempty"`
}

// ArtifactObservation records declared or observed artifact activity.
type ArtifactObservation struct {
	Operation      string   `json:"operation"`
	ArtifactID     string   `json:"artifactId"`
	Path           string   `json:"path"`
	Digest         Digest   `json:"digest,omitempty"`
	SizeBytes      int64    `json:"sizeBytes"`
	Executable     bool     `json:"executable"`
	SourceEventIDs []string `json:"sourceEventIds"`
}

// ScenarioObservation records scenario lifecycle events.
type ScenarioObservation struct {
	Status         ScenarioStatus `json:"status"`
	Message        string         `json:"message,omitempty"`
	StartedAt      *time.Time     `json:"startedAt,omitempty"`
	CompletedAt    *time.Time     `json:"completedAt,omitempty"`
	DurationMillis int64          `json:"durationMillis"`
}

// ObserverWarningObservation records observer warnings and unsupported gaps.
type ObserverWarningObservation struct {
	Code        string       `json:"code"`
	Message     string       `json:"message"`
	Unsupported bool         `json:"unsupported"`
	Limitations []Limitation `json:"limitations"`
}

// ResourceLimitObservation records a bounded resource-limit event.
type ResourceLimitObservation struct {
	LimitKind     string `json:"limitKind"`
	LimitValue    int64  `json:"limitValue"`
	Unit          string `json:"unit"`
	ObservedValue int64  `json:"observedValue"`
	Exceeded      bool   `json:"exceeded"`
}
