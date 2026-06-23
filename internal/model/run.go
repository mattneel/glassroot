package model

import "time"

// CommitRef records a planned or observed revision identity. Values are data
// only; this package does not resolve refs, fetch repositories, or inspect Git.
type CommitRef struct {
	Kind         RevisionKind    `json:"kind"`
	Repository   string          `json:"repository"`
	Ref          string          `json:"ref"`
	CommitID     string          `json:"commitId"`
	ObjectFormat GitObjectFormat `json:"objectFormat,omitempty"`
	TreeID       string          `json:"treeId,omitempty"`
	TreeDigest   Digest          `json:"treeDigest,omitempty"`
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
	SchemaVersion        SchemaVersion       `json:"schemaVersion"`
	ID                   string              `json:"id"`
	RunID                string              `json:"runId"`
	CreatedAt            time.Time           `json:"createdAt"`
	PipelineName         string              `json:"pipelineName,omitempty"`
	Configuration        *RunPlanConfig      `json:"configuration,omitempty"`
	ExecutionEnvironment *RunPlanEnvironment `json:"executionEnvironment,omitempty"`
	Base                 CommitRef           `json:"base"`
	Head                 CommitRef           `json:"head"`
	Revisions            []RevisionPlan      `json:"revisions"`
	Scenarios            []ScenarioPlan      `json:"scenarios"`
	Runner               RunnerCapabilities  `json:"runner"`
	ResourceLimits       ResourceLimits      `json:"resourceLimits"`
	NetworkPolicy        NetworkPolicy       `json:"networkPolicy"`
	Environment          []EnvEntry          `json:"environment"`
	Collection           *CollectionPlan     `json:"collection,omitempty"`
	Comparison           *ComparisonPlan     `json:"comparison,omitempty"`
	Policy               *PolicyPlan         `json:"policy,omitempty"`
	Platform             *RunPlanPlatform    `json:"platform,omitempty"`
	Limitations          []Limitation        `json:"limitations"`
}

// RevisionPlan records the materialized facts planned for one revision. It does
// not perform materialization or path access.
type RevisionPlan struct {
	Kind                          RevisionKind       `json:"kind"`
	Commit                        CommitRef          `json:"commit"`
	ObjectFormat                  GitObjectFormat    `json:"objectFormat,omitempty"`
	TreeID                        string             `json:"treeId,omitempty"`
	MaterializedTreeDigest        Digest             `json:"materializedTreeDigest,omitempty"`
	MaterializationManifestDigest Digest             `json:"materializationManifestDigest,omitempty"`
	SourceSummary                 *SourceSummary     `json:"sourceSummary,omitempty"`
	SourceLimitations             []SourceLimitation `json:"sourceLimitations,omitempty"`
	ScenarioIDs                   []string           `json:"scenarioIds"`
}

// RunPlanConfig records the trusted repository configuration source that
// produced a plan. It records raw file identity only and does not retain YAML.
type RunPlanConfig struct {
	Source    RevisionKind `json:"source"`
	Path      string       `json:"path"`
	Digest    Digest       `json:"digest"`
	SizeBytes int64        `json:"sizeBytes"`
	ObjectID  string       `json:"objectId,omitempty"`
}

// RunPlanEnvironment records the trusted-base workload environment selection as
// inert data. The image is not resolved or pulled by the model package.
type RunPlanEnvironment struct {
	Image       string `json:"image"`
	ImageDigest string `json:"imageDigest"`
	Workdir     string `json:"workdir"`
}

// CollectionPlan records source collection requests as inert plan data.
type CollectionPlan struct {
	FilesystemRoots      []string               `json:"filesystemRoots"`
	FilesystemContents   string                 `json:"filesystemContents"`
	Artifacts            []ExpectedArtifactSpec `json:"artifacts"`
	LogMaxBytesPerStream int64                  `json:"logMaxBytesPerStream"`
}

// ComparisonPlan records deterministic comparison controls for later stages.
type ComparisonPlan struct {
	IgnoreFields []string `json:"ignoreFields"`
	Repetitions  int64    `json:"repetitions"`
}

// PolicyPlan records the selected policy profile as data. Policy evaluation is
// deferred to later packages.
type PolicyPlan struct {
	Profile string `json:"profile"`
}

// RunPlanPlatform records trusted control-plane admission ceilings used when
// the plan was built. These fields are descriptive and do not authorize work.
type RunPlanPlatform struct {
	MaxCPU                   int64       `json:"maxCpu"`
	MaxMemoryBytes           int64       `json:"maxMemoryBytes"`
	MaxDiskBytes             int64       `json:"maxDiskBytes"`
	MaxProcessCount          int64       `json:"maxProcessCount"`
	MaxGlobalTimeoutMillis   int64       `json:"maxGlobalTimeoutMillis"`
	MaxScenarioTimeoutMillis int64       `json:"maxScenarioTimeoutMillis"`
	MaxScenarioCount         int64       `json:"maxScenarioCount"`
	MaxRepetitions           int64       `json:"maxRepetitions"`
	MaxFilesystemRootCount   int64       `json:"maxFilesystemRootCount"`
	MaxArtifactCount         int64       `json:"maxArtifactCount"`
	MaxArtifactBytes         int64       `json:"maxArtifactBytes"`
	MaxLogBytesPerStream     int64       `json:"maxLogBytesPerStream"`
	MaxPlanJSONBytes         int64       `json:"maxPlanJsonBytes"`
	RequiredNetworkMode      NetworkMode `json:"requiredNetworkMode"`
}

// SourceSummary records bounded materialization facts for one revision. The
// planner records these facts without retaining workspace handles or paths.
type SourceSummary struct {
	DirectoryCount             int64 `json:"directoryCount"`
	RegularFileCount           int64 `json:"regularFileCount"`
	ExecutableFileCount        int64 `json:"executableFileCount"`
	SymlinkCount               int64 `json:"symlinkCount"`
	GitlinkCount               int64 `json:"gitlinkCount"`
	LFSPointerCount            int64 `json:"lfsPointerCount"`
	TotalMaterializedFileBytes int64 `json:"totalMaterializedFileBytes"`
	SkippedEntryCount          int64 `json:"skippedEntryCount"`
}

// SourceLimitation records bounded source materialization limitations. Paths
// remain untrusted strings and are not accessed by this package.
type SourceLimitation struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Summary string `json:"summary"`
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
	CPU           int64 `json:"cpu,omitempty"`
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
