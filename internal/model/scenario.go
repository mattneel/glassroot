package model

import "time"

// Command describes the command argv intended to run inside a future sandbox.
// It is data only and is never executed by this package.
type Command struct {
	Argv             []string   `json:"argv"`
	WorkingDirectory string     `json:"workingDirectory"`
	Environment      []EnvEntry `json:"environment"`
	TimeoutMillis    int64      `json:"timeoutMillis"`
}

// ScenarioPlan records one planned scenario.
type ScenarioPlan struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	Command           Command                `json:"command"`
	ResourceLimits    ResourceLimits         `json:"resourceLimits"`
	NetworkPolicy     NetworkPolicy          `json:"networkPolicy"`
	ExpectedArtifacts []ExpectedArtifactSpec `json:"expectedArtifacts"`
}

// ExpectedArtifactSpec describes an artifact path pattern as data. Paths are
// untrusted strings and are not accessed by this package.
type ExpectedArtifactSpec struct {
	LogicalPath  string `json:"logicalPath"`
	Required     bool   `json:"required"`
	MaxSizeBytes int64  `json:"maxSizeBytes"`
}

// ScenarioStatus records scenario lifecycle state.
type ScenarioStatus string

const (
	ScenarioStatusPlanned    ScenarioStatus = "planned"
	ScenarioStatusRunning    ScenarioStatus = "running"
	ScenarioStatusPassed     ScenarioStatus = "passed"
	ScenarioStatusFailed     ScenarioStatus = "failed"
	ScenarioStatusError      ScenarioStatus = "error"
	ScenarioStatusTimedOut   ScenarioStatus = "timed-out"
	ScenarioStatusCancelled  ScenarioStatus = "cancelled"
	ScenarioStatusSkipped    ScenarioStatus = "skipped"
	ScenarioStatusIncomplete ScenarioStatus = "incomplete"
)

// ScenarioResult is an independently serialized scenario result document.
type ScenarioResult struct {
	SchemaVersion  SchemaVersion    `json:"schemaVersion"`
	ID             string           `json:"id"`
	RunID          string           `json:"runId"`
	Revision       RevisionKind     `json:"revision"`
	ScenarioID     string           `json:"scenarioId"`
	Status         ScenarioStatus   `json:"status"`
	StartedAt      *time.Time       `json:"startedAt,omitempty"`
	CompletedAt    *time.Time       `json:"completedAt,omitempty"`
	DurationMillis int64            `json:"durationMillis"`
	ExitCode       *int             `json:"exitCode,omitempty"`
	Artifacts      []ArtifactRecord `json:"artifacts"`
	Limitations    []Limitation     `json:"limitations"`
}

// RevisionResult groups scenario results by revision for higher-level reports.
type RevisionResult struct {
	Revision        RevisionKind     `json:"revision"`
	Commit          CommitRef        `json:"commit"`
	ScenarioResults []ScenarioResult `json:"scenarioResults"`
	Limitations     []Limitation     `json:"limitations"`
}
