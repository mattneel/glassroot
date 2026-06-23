package localrun

import (
	"time"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/report"
	"github.com/mattneel/glassroot/internal/runner/dockerdev"
)

type Runner struct {
	limits Limits
}

type Request struct {
	OutputDir       string
	GitDir          string
	DockerSocket    string
	BaseCommitID    string
	HeadCommitID    string
	RunID           string
	CreatedAt       time.Time
	EvaluatedAt     time.Time
	Acknowledgement dockerdev.UnsafeDevelopmentAcknowledgement
}

type Result struct {
	Report             *report.FrozenReport
	ManifestDigest     model.Digest
	BaseCommitID       string
	HeadCommitID       string
	OverallDisposition model.Disposition
	ExpectedExitCode   int
	Metadata           Metadata
}

type CLIRequest struct {
	Request Request
	Format  string
	Help    bool
}

type Metadata struct {
	SchemaVersion           string             `json:"schemaVersion"`
	LocalRunProfileVersion  string             `json:"localRunProfileVersion"`
	PlatformProfileVersion  string             `json:"platformProfileVersion"`
	RunID                   string             `json:"runId"`
	CreatedAt               time.Time          `json:"createdAt"`
	EvaluatedAt             time.Time          `json:"evaluatedAt"`
	BaseCommitID            string             `json:"baseCommitId"`
	BaseTreeID              string             `json:"baseTreeId"`
	HeadCommitID            string             `json:"headCommitId"`
	HeadTreeID              string             `json:"headTreeId"`
	ObjectFormat            string             `json:"objectFormat"`
	ImmutableImage          string             `json:"immutableImage"`
	PlanDigest              model.Digest       `json:"planDigest"`
	ManifestDigest          model.Digest       `json:"manifestDigest"`
	BehavioralDeltaDigest   model.Digest       `json:"behavioralDeltaDigest"`
	PolicyEvaluationDigest  model.Digest       `json:"policyEvaluationDigest"`
	PolicyApplicationDigest model.Digest       `json:"policyApplicationDigest"`
	ReportDigest            model.Digest       `json:"reportDigest"`
	MarkdownDigest          model.Digest       `json:"markdownDigest"`
	TerminalDigest          model.Digest       `json:"terminalDigest"`
	Runner                  MetadataRunner     `json:"runner"`
	ExecutionComplete       bool               `json:"executionComplete"`
	EvidenceComplete        bool               `json:"evidenceComplete"`
	EffectiveDisposition    model.Disposition  `json:"effectiveDisposition"`
	ExpectedCLIExitCode     int                `json:"expectedCliExitCode"`
	RelativePaths           MetadataPaths      `json:"relativePaths"`
	Daemon                  MetadataDaemon     `json:"daemon"`
	AttemptCount            int                `json:"attemptCount"`
	LimitationCount         int                `json:"limitationCount"`
	Limitations             []model.Limitation `json:"limitations"`
}

type MetadataRunner struct {
	Name                string              `json:"name"`
	Version             string              `json:"version"`
	IsolationTier       model.IsolationTier `json:"isolationTier"`
	ExecutesTargetCode  bool                `json:"executesTargetCode"`
	SyntheticEvidence   bool                `json:"syntheticEvidence"`
	EnforcesNetworkDeny bool                `json:"enforcesNetworkDeny"`
	ProcessEvents       bool                `json:"processEventCollection"`
	FilesystemEvents    bool                `json:"filesystemEventCollection"`
	SyscallEvents       bool                `json:"syscallEventCollection"`
	ArtifactHashing     bool                `json:"artifactHashing"`
	SnapshotSupport     bool                `json:"snapshotSupport"`
}

type MetadataPaths struct {
	Evidence       string `json:"evidence"`
	ReportJSON     string `json:"reportJson"`
	ReportMarkdown string `json:"reportMarkdown"`
	ReportTerminal string `json:"reportTerminal"`
}

type MetadataDaemon struct {
	EngineVersion string   `json:"engineVersion,omitempty"`
	APIVersion    string   `json:"apiVersion,omitempty"`
	OSType        string   `json:"osType,omitempty"`
	Architecture  string   `json:"architecture,omitempty"`
	CgroupVersion string   `json:"cgroupVersion,omitempty"`
	CgroupDriver  string   `json:"cgroupDriver,omitempty"`
	Rootless      bool     `json:"rootless"`
	Security      []string `json:"securityOptions"`
}
