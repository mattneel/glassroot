package model

import "time"

// CommitRef records a planned or observed revision identity. Values are data
// only; this package does not resolve refs, fetch repositories, or inspect Git.
type CommitRef struct {
	Kind       RevisionKind `json:"kind"`
	Repository string       `json:"repository"`
	Ref        string       `json:"ref"`
	CommitID   string       `json:"commitId"`
	TreeDigest Digest       `json:"treeDigest,omitempty"`
}

// Run is the high-level run record for an independently serialized run document.
type Run struct {
	SchemaVersion SchemaVersion      `json:"schemaVersion"`
	ID            string             `json:"id"`
	CreatedAt     time.Time          `json:"createdAt"`
	Base          CommitRef          `json:"base"`
	Head          CommitRef          `json:"head"`
	PlanDigest    Digest             `json:"planDigest,omitempty"`
	Runner        RunnerCapabilities `json:"runner"`
	Limitations   []Limitation       `json:"limitations"`
}

// RunPlan is the immutable, data-only plan shared by later planner, runner, and
// evidence code. It contains no behavior and performs no validation.
type RunPlan struct {
	SchemaVersion  SchemaVersion      `json:"schemaVersion"`
	ID             string             `json:"id"`
	RunID          string             `json:"runId"`
	CreatedAt      time.Time          `json:"createdAt"`
	Base           CommitRef          `json:"base"`
	Head           CommitRef          `json:"head"`
	Revisions      []RevisionPlan     `json:"revisions"`
	Scenarios      []ScenarioPlan     `json:"scenarios"`
	Runner         RunnerCapabilities `json:"runner"`
	ResourceLimits ResourceLimits     `json:"resourceLimits"`
	NetworkPolicy  NetworkPolicy      `json:"networkPolicy"`
	Environment    []EnvEntry         `json:"environment"`
	Limitations    []Limitation       `json:"limitations"`
}

// RevisionPlan records the materialized facts planned for one revision. It does
// not perform materialization or path access.
type RevisionPlan struct {
	Kind                   RevisionKind `json:"kind"`
	Commit                 CommitRef    `json:"commit"`
	MaterializedTreeDigest Digest       `json:"materializedTreeDigest,omitempty"`
	ScenarioIDs            []string     `json:"scenarioIds"`
}

// RunnerCapabilities records observable runner facts without asserting that a
// runner is generically secure.
type RunnerCapabilities struct {
	Name                      string        `json:"name"`
	Version                   string        `json:"version"`
	IsolationTier             IsolationTier `json:"isolationTier"`
	FreshKernel               bool          `json:"freshKernel"`
	BrokeredNetwork           bool          `json:"brokeredNetwork"`
	ProcessEventCollection    bool          `json:"processEventCollection"`
	FilesystemEventCollection bool          `json:"filesystemEventCollection"`
	SyscallEventCollection    bool          `json:"syscallEventCollection"`
	ArtifactHashing           bool          `json:"artifactHashing"`
	SnapshotSupport           bool          `json:"snapshotSupport"`
}

// IsolationTier describes the kind of isolation a runner reports as a fact.
type IsolationTier string

const (
	IsolationTierFake              IsolationTier = "fake"
	IsolationTierDevelopmentOnly   IsolationTier = "development-only"
	IsolationTierHardenedContainer IsolationTier = "hardened-container"
	IsolationTierMicroVM           IsolationTier = "microvm"
)

// ResourceLimits uses explicit integer units for serialized values.
type ResourceLimits struct {
	TimeoutMillis int64 `json:"timeoutMillis"`
	MemoryBytes   int64 `json:"memoryBytes"`
	DiskBytes     int64 `json:"diskBytes"`
	CPUMillis     int64 `json:"cpuMillis"`
	ProcessCount  int64 `json:"processCount"`
}

// NetworkMode names the requested network posture. Later policy code decides
// whether a request is allowed.
type NetworkMode string

const (
	NetworkModeDeny      NetworkMode = "deny"
	NetworkModeAllowlist NetworkMode = "allowlist"
)

// NetworkPolicy records requested network behavior as data. It does not open,
// inspect, or broker network connections.
type NetworkPolicy struct {
	Mode    NetworkMode        `json:"mode"`
	Allowed []NetworkAllowRule `json:"allowed"`
}

// NetworkAllowRule is an ordered allow-list entry for future policy evaluation.
type NetworkAllowRule struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
}
