package observe

import (
	"encoding/json"
	"time"

	"github.com/mattneel/glassroot/internal/evidence"
	"github.com/mattneel/glassroot/internal/model"
)

const (
	TraceSetSchemaV1Alpha1     = "glassroot.dev/normalized-trace/v1alpha1"
	ProfileVersionV1Alpha1     = "glassroot.dev/normalization-profile/v1alpha1"
	ProcessIdentityAlgorithmV1 = "glassroot.dev/normalized-process-id/v1"
	TimestampAlgorithmV1       = "glassroot.dev/source-relative-timestamp/v1"
	PathRootAlgorithmV1        = "glassroot.dev/rooted-posix-path/v1"
	IgnoreFieldEventTimestamp  = "event.timestamp"
	IgnoreFieldProcessPID      = "process.pid"
)

type FactKind string

const (
	FactKindProcessStart           FactKind = FactKind(model.ObservationKindProcessStart)
	FactKindProcessExit            FactKind = FactKind(model.ObservationKindProcessExit)
	FactKindFilesystemCreate       FactKind = FactKind(model.ObservationKindFilesystemCreate)
	FactKindFilesystemRead         FactKind = FactKind(model.ObservationKindFilesystemRead)
	FactKindFilesystemWrite        FactKind = FactKind(model.ObservationKindFilesystemWrite)
	FactKindFilesystemDelete       FactKind = FactKind(model.ObservationKindFilesystemDelete)
	FactKindFilesystemRename       FactKind = FactKind(model.ObservationKindFilesystemRename)
	FactKindFilesystemChmod        FactKind = FactKind(model.ObservationKindFilesystemChmod)
	FactKindDNSQuery               FactKind = FactKind(model.ObservationKindDNSQuery)
	FactKindNetworkConnection      FactKind = FactKind(model.ObservationKindNetworkConnection)
	FactKindArtifactActivity       FactKind = FactKind(model.ObservationKindArtifactActivity)
	FactKindScenarioStarted        FactKind = FactKind(model.ObservationKindScenarioStarted)
	FactKindScenarioCompleted      FactKind = FactKind(model.ObservationKindScenarioCompleted)
	FactKindObserverWarning        FactKind = FactKind(model.ObservationKindObserverWarning)
	FactKindUnsupportedObservation FactKind = FactKind(model.ObservationKindUnsupportedObservation)
	FactKindResourceLimit          FactKind = FactKind(model.ObservationKindResourceLimit)
)

type FactID string
type ProcessID string
type CoverageState string

const (
	CoverageComplete   CoverageState = "complete"
	CoverageIncomplete CoverageState = "incomplete"
	CoverageNotStarted CoverageState = "not-started"
)

type PathNamespace string

const (
	PathNamespaceWorkdirRoot      PathNamespace = "workdir-root"
	PathNamespaceCollectionRoot   PathNamespace = "collection-root"
	PathNamespaceAbsoluteUnmapped PathNamespace = "absolute-unmapped"
	PathNamespaceRelative         PathNamespace = "relative"
	PathNamespaceOpaqueInvalid    PathNamespace = "opaque-invalid"
)

type PathRootAlias struct {
	Namespace PathNamespace `json:"namespace"`
	RootIndex uint32        `json:"rootIndex"`
	Root      string        `json:"root"`
	Alias     string        `json:"alias"`
}
type NormalizationProfile struct {
	Version                  string          `json:"version"`
	IgnoreFields             []string        `json:"ignoreFields"`
	ProcessIdentityAlgorithm string          `json:"processIdentityAlgorithm"`
	TimestampAlgorithm       string          `json:"timestampAlgorithm"`
	PathRootAlgorithm        string          `json:"pathRootAlgorithm"`
	RootAliases              []PathRootAlias `json:"rootAliases"`
}

type ManifestVerification struct {
	Mode                           evidence.VerificationMode `json:"mode"`
	ManifestDigest                 model.Digest              `json:"manifestDigest"`
	ExpectedManifestDigestSupplied bool                      `json:"expectedManifestDigestSupplied"`
	ExpectedManifestDigestMatched  bool                      `json:"expectedManifestDigestMatched"`
	InternallyConsistent           bool                      `json:"internallyConsistent"`
	Limitations                    []model.Limitation        `json:"limitations"`
}

type TraceSetDocument struct {
	SchemaVersion        string                `json:"schemaVersion"`
	Profile              NormalizationProfile  `json:"profile"`
	PlanDigest           model.Digest          `json:"planDigest"`
	ManifestDigest       model.Digest          `json:"manifestDigest"`
	RunID                string                `json:"runId"`
	ManifestVerification ManifestVerification  `json:"manifestVerification"`
	ExecutionComplete    bool                  `json:"executionComplete"`
	EvidenceComplete     bool                  `json:"evidenceComplete"`
	EvidenceContext      model.EvidenceContext `json:"evidenceContext"`
	Attempts             []AttemptTrace        `json:"attempts"`
	Limitations          []model.Limitation    `json:"limitations"`
}

type AttemptTrace struct {
	AttemptID          string                `json:"attemptId"`
	Ordinal            uint64                `json:"ordinal"`
	Revision           model.RevisionKind    `json:"revision"`
	ScenarioID         string                `json:"scenarioId"`
	Repetition         uint32                `json:"repetition"`
	Coverage           CoverageState         `json:"coverage"`
	Result             evidence.CaptureState `json:"result"`
	Events             evidence.CaptureState `json:"events"`
	Stdout             evidence.CaptureState `json:"stdout"`
	Stderr             evidence.CaptureState `json:"stderr"`
	Artifacts          evidence.CaptureState `json:"artifacts"`
	FirstEventSequence uint64                `json:"firstEventSequence,omitempty"`
	LastEventSequence  uint64                `json:"lastEventSequence,omitempty"`
	AcceptedEventCount uint64                `json:"acceptedEventCount"`
	Facts              []Fact                `json:"facts"`
	Limitations        []model.Limitation    `json:"limitations"`
}

type NormalizedTiming struct {
	SourceRelativeNanos      int64 `json:"sourceRelativeNanos"`
	IncludedInSemanticDigest bool  `json:"includedInSemanticDigest"`
	ClockRegression          bool  `json:"clockRegression,omitempty"`
}

type NormalizedPath struct {
	Namespace PathNamespace `json:"namespace"`
	RootIndex uint32        `json:"rootIndex,omitempty"`
	Relative  string        `json:"relative,omitempty"`
	Literal   string        `json:"literal"`
	Display   string        `json:"display"`
}

type ProcessFact struct {
	Operation      string           `json:"operation"`
	StableID       ProcessID        `json:"stableId"`
	ParentStableID ProcessID        `json:"parentStableId,omitempty"`
	ParentRelation string           `json:"parentRelation"`
	Executable     NormalizedPath   `json:"executable"`
	Arguments      []string         `json:"arguments"`
	Environment    []model.EnvEntry `json:"environment"`
	ExitCode       *int             `json:"exitCode,omitempty"`
	DurationMillis int64            `json:"durationMillis"`
}
type FilesystemFact struct {
	Operation  string          `json:"operation"`
	Path       NormalizedPath  `json:"path"`
	OldPath    *NormalizedPath `json:"oldPath,omitempty"`
	Mode       string          `json:"mode,omitempty"`
	Digest     model.Digest    `json:"digest,omitempty"`
	SizeBytes  int64           `json:"sizeBytes"`
	Executable bool            `json:"executable"`
	Truncated  bool            `json:"truncated"`
}
type NetworkFact struct {
	Operation         string   `json:"operation"`
	Protocol          string   `json:"protocol"`
	QueryName         string   `json:"queryName,omitempty"`
	DestinationHost   string   `json:"destinationHost,omitempty"`
	DestinationPort   int      `json:"destinationPort,omitempty"`
	ResolvedAddresses []string `json:"resolvedAddresses"`
	Result            string   `json:"result"`
	DurationMillis    int64    `json:"durationMillis,omitempty"`
}
type ArtifactFact struct {
	Operation      string         `json:"operation"`
	ArtifactID     string         `json:"artifactId"`
	Path           NormalizedPath `json:"path"`
	Digest         model.Digest   `json:"digest,omitempty"`
	SizeBytes      int64          `json:"sizeBytes"`
	Executable     bool           `json:"executable"`
	SourceEventIDs []string       `json:"sourceEventIds"`
}
type ScenarioFact struct {
	Status         model.ScenarioStatus `json:"status"`
	Message        string               `json:"message,omitempty"`
	DurationMillis int64                `json:"durationMillis"`
}
type WarningFact struct {
	Code        string             `json:"code"`
	Message     string             `json:"message"`
	Unsupported bool               `json:"unsupported"`
	Limitations []model.Limitation `json:"limitations"`
}
type ResourceFact struct {
	LimitKind     string `json:"limitKind"`
	LimitValue    int64  `json:"limitValue"`
	Unit          string `json:"unit"`
	ObservedValue int64  `json:"observedValue"`
	Exceeded      bool   `json:"exceeded"`
}

type RawEvidenceReference struct {
	EventStreamDigest model.Digest       `json:"eventStreamDigest,omitempty"`
	EventStreamPath   string             `json:"eventStreamPath,omitempty"`
	EventID           string             `json:"eventId"`
	EventSequence     uint64             `json:"eventSequence"`
	Revision          model.RevisionKind `json:"revision,omitempty"`
	ScenarioID        string             `json:"scenarioId,omitempty"`
	Repetition        uint32             `json:"repetition,omitempty"`
}

type Fact struct {
	ID             FactID                  `json:"id"`
	SemanticDigest model.Digest            `json:"semanticDigest"`
	Kind           FactKind                `json:"kind"`
	Source         model.ObservationSource `json:"source"`
	Timing         NormalizedTiming        `json:"timing"`
	Process        *ProcessFact            `json:"process,omitempty"`
	Filesystem     *FilesystemFact         `json:"filesystem,omitempty"`
	Network        *NetworkFact            `json:"network,omitempty"`
	Artifact       *ArtifactFact           `json:"artifact,omitempty"`
	Scenario       *ScenarioFact           `json:"scenario,omitempty"`
	Warning        *WarningFact            `json:"warning,omitempty"`
	Resource       *ResourceFact           `json:"resource,omitempty"`
	Evidence       []RawEvidenceReference  `json:"evidence"`
	Limitations    []model.Limitation      `json:"limitations"`
}

type TraceSet struct{ doc TraceSetDocument }

func (t *TraceSet) Document() TraceSetDocument {
	if t == nil {
		return TraceSetDocument{}
	}
	return cloneTraceDocument(t.doc)
}

func cloneTraceDocument(in TraceSetDocument) TraceSetDocument {
	var out TraceSetDocument
	data, _ := json.Marshal(in)
	_ = json.Unmarshal(data, &out)
	if out.Attempts == nil {
		out.Attempts = []AttemptTrace{}
	}
	if out.Limitations == nil {
		out.Limitations = []model.Limitation{}
	}
	return out
}

func timeUTC(t time.Time) time.Time { return t.UTC().Round(0) }
