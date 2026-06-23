package model

import "time"

// BehavioralDelta is an independently serialized comparison document.
type BehavioralDelta struct {
	SchemaVersion               SchemaVersion        `json:"schemaVersion"`
	ID                          string               `json:"id"`
	RunID                       string               `json:"runId"`
	Base                        CommitRef            `json:"base"`
	Head                        CommitRef            `json:"head"`
	PlanDigest                  Digest               `json:"planDigest,omitempty"`
	ManifestDigest              Digest               `json:"manifestDigest,omitempty"`
	ManifestVerificationMode    string               `json:"manifestVerificationMode,omitempty"`
	ExecutionComplete           bool                 `json:"executionComplete,omitempty"`
	EvidenceComplete            bool                 `json:"evidenceComplete,omitempty"`
	ComparisonProfile           ComparisonProfile    `json:"comparisonProfile,omitempty"`
	NormalizationProfileVersion string               `json:"normalizationProfileVersion,omitempty"`
	ScenarioIDs                 []string             `json:"scenarioIds"`
	ScenarioComparisons         []ScenarioComparison `json:"scenarioComparisons,omitempty"`
	Summary                     DeltaSummary         `json:"summary,omitempty"`
	Records                     []DeltaRecord        `json:"records"`
	Limitations                 []Limitation         `json:"limitations"`
}

// DeltaKind names a behavioral difference category.
type DeltaKind string

const (
	DeltaKindAddedProcess            DeltaKind = "added-process"
	DeltaKindAddedFilesystemActivity DeltaKind = "added-filesystem-activity"
	DeltaKindAddedNetworkConnection  DeltaKind = "added-network-connection"
	DeltaKindArtifactChanged         DeltaKind = "artifact-changed"
	DeltaKindObservationIncomplete   DeltaKind = "observation-incomplete"

	DeltaKindAdded            DeltaKind = "added"
	DeltaKindRemoved          DeltaKind = "removed"
	DeltaKindModified         DeltaKind = "modified"
	DeltaKindCountChanged     DeltaKind = "count-changed"
	DeltaKindOrderChanged     DeltaKind = "order-changed"
	DeltaKindStabilityChanged DeltaKind = "stability-changed"
	DeltaKindCoverageChanged  DeltaKind = "coverage-changed"
)

// DeltaRecord records one comparison result with evidence references.
type DeltaRecord struct {
	ID                  string              `json:"id"`
	Kind                DeltaKind           `json:"kind"`
	Summary             string              `json:"summary"`
	BaseObserved        bool                `json:"baseObserved"`
	HeadObserved        bool                `json:"headObserved"`
	Evidence            []EvidenceRef       `json:"evidence"`
	ScenarioIDs         []string            `json:"scenarioIds"`
	Limitations         []Limitation        `json:"limitations"`
	FactKind            string              `json:"factKind,omitempty"`
	Source              ObservationSource   `json:"source,omitempty"`
	AnchorDigest        Digest              `json:"anchorDigest,omitempty"`
	Basis               ComparisonBasis     `json:"basis,omitempty"`
	ChangedFields       []string            `json:"changedFields,omitempty"`
	BaseOccurrence      OccurrenceProfile   `json:"baseOccurrence,omitempty"`
	HeadOccurrence      OccurrenceProfile   `json:"headOccurrence,omitempty"`
	BaseFacts           []DeltaFactSnapshot `json:"baseFacts,omitempty"`
	HeadFacts           []DeltaFactSnapshot `json:"headFacts,omitempty"`
	BaseEvidence        []EvidenceRef       `json:"baseEvidence,omitempty"`
	HeadEvidence        []EvidenceRef       `json:"headEvidence,omitempty"`
	BaseSemanticDigests []Digest            `json:"baseSemanticDigests,omitempty"`
	HeadSemanticDigests []Digest            `json:"headSemanticDigests,omitempty"`
}

// Finding is an independently serialized policy-facing finding document.
type Finding struct {
	SchemaVersion SchemaVersion `json:"schemaVersion"`
	ID            string        `json:"id"`
	RuleID        string        `json:"ruleId"`
	Title         string        `json:"title"`
	Severity      Severity      `json:"severity"`
	Confidence    Confidence    `json:"confidence"`
	Disposition   Disposition   `json:"disposition"`
	Summary       string        `json:"summary"`
	Evidence      []EvidenceRef `json:"evidence"`
	ScenarioIDs   []string      `json:"scenarioIds"`
	BaseObserved  bool          `json:"baseObserved"`
	HeadObserved  bool          `json:"headObserved"`
	Waived        bool          `json:"waived"`
	Waivers       []Waiver      `json:"waivers,omitempty"`
	Limitations   []Limitation  `json:"limitations"`
}

// Waiver records trusted waiver metadata as data. Loading and validation are
// deferred to later policy milestones.
type Waiver struct {
	ID        string        `json:"id"`
	RuleID    string        `json:"ruleId"`
	Scope     string        `json:"scope"`
	Reason    string        `json:"reason"`
	Owner     string        `json:"owner"`
	ExpiresAt *time.Time    `json:"expiresAt,omitempty"`
	Evidence  []EvidenceRef `json:"evidence"`
}

// Severity records finding impact separately from confidence and disposition.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Confidence records evidence confidence separately from severity and disposition.
type Confidence string

const (
	ConfidenceLow     Confidence = "low"
	ConfidenceMedium  Confidence = "medium"
	ConfidenceHigh    Confidence = "high"
	ConfidenceUnknown Confidence = "unknown"
)

// Disposition records policy-facing outcome separately from severity and confidence.
type Disposition string

const (
	DispositionPassed         Disposition = "passed"
	DispositionRequiresReview Disposition = "requires-review"
	DispositionFailed         Disposition = "failed"
	DispositionWaived         Disposition = "waived"
)

// ComparisonProfile records the fixed comparator algorithms that produced a
// behavioral delta. Values are descriptive data, not policy or confidence.
type ComparisonProfile struct {
	Version                       string   `json:"version"`
	RequiredNormalizationProfile  string   `json:"requiredNormalizationProfile"`
	ExactSemanticMatchAlgorithm   string   `json:"exactSemanticMatchAlgorithm"`
	TypedAnchorAlgorithm          string   `json:"typedAnchorAlgorithm"`
	RepetitionAssessmentAlgorithm string   `json:"repetitionAssessmentAlgorithm"`
	OrderAssessmentAlgorithm      string   `json:"orderAssessmentAlgorithm"`
	AbsencePolicy                 string   `json:"absencePolicy"`
	IncludedFactKinds             []string `json:"includedFactKinds"`
}

// ComparisonBasis records the deterministic evidence-state basis for a delta.
type ComparisonBasis string

const (
	ComparisonBasisCompleteObservation  ComparisonBasis = "complete-observation"
	ComparisonBasisSingleSample         ComparisonBasis = "single-sample"
	ComparisonBasisRepetitionVariable   ComparisonBasis = "repetition-variable"
	ComparisonBasisCoverageLimited      ComparisonBasis = "coverage-limited"
	ComparisonBasisAmbiguousCorrelation ComparisonBasis = "ambiguous-correlation"
)

// CoverageAssessment records whether event evidence can establish presence and absence.
type CoverageAssessment string

const (
	CoverageAssessmentComplete CoverageAssessment = "complete"
	CoverageAssessmentPartial  CoverageAssessment = "partial"
	CoverageAssessmentNone     CoverageAssessment = "none"
)

// RepeatabilityAssessment records deterministic repetition behavior without probabilities.
type RepeatabilityAssessment string

const (
	RepeatabilityStable        RepeatabilityAssessment = "stable"
	RepeatabilityVariable      RepeatabilityAssessment = "variable"
	RepeatabilitySingleSample  RepeatabilityAssessment = "single-sample"
	RepeatabilityNotAssessable RepeatabilityAssessment = "not-assessable"
)

type RepetitionOccurrence struct {
	Repetition uint32             `json:"repetition"`
	Coverage   CoverageAssessment `json:"coverage"`
	CountKnown bool               `json:"countKnown"`
	Count      int64              `json:"count"`
}

type OccurrenceProfile struct {
	PlannedRepetitionCount    int64                   `json:"plannedRepetitionCount"`
	Repetitions               []RepetitionOccurrence  `json:"repetitions"`
	CompleteRepetitionCount   int64                   `json:"completeRepetitionCount"`
	IncompleteRepetitionCount int64                   `json:"incompleteRepetitionCount"`
	MinimumKnownCount         int64                   `json:"minimumKnownCount"`
	MaximumKnownCount         int64                   `json:"maximumKnownCount"`
	TotalKnownCount           int64                   `json:"totalKnownCount"`
	Coverage                  CoverageAssessment      `json:"coverage"`
	Repeatability             RepeatabilityAssessment `json:"repeatability"`
}

type ScenarioComparison struct {
	ScenarioID         string             `json:"scenarioId"`
	BaseRepetitions    []AttemptCoverage  `json:"baseRepetitions"`
	HeadRepetitions    []AttemptCoverage  `json:"headRepetitions"`
	Coverage           CoverageAssessment `json:"coverage"`
	RepeatabilityNotes []Limitation       `json:"repeatabilityNotes"`
	Limitations        []Limitation       `json:"limitations"`
}

type AttemptCoverage struct {
	AttemptID  string             `json:"attemptId"`
	Revision   RevisionKind       `json:"revision"`
	Repetition uint32             `json:"repetition"`
	Coverage   CoverageAssessment `json:"coverage"`
}

type DeltaSummary struct {
	TotalRecords     int64 `json:"totalRecords"`
	Added            int64 `json:"added"`
	Removed          int64 `json:"removed"`
	Modified         int64 `json:"modified"`
	CountChanged     int64 `json:"countChanged"`
	OrderChanged     int64 `json:"orderChanged"`
	StabilityChanged int64 `json:"stabilityChanged"`
	CoverageChanged  int64 `json:"coverageChanged"`
}

type DeltaNormalizedPath struct {
	Namespace string `json:"namespace"`
	RootIndex uint32 `json:"rootIndex,omitempty"`
	Relative  string `json:"relative,omitempty"`
	Literal   string `json:"literal"`
	Display   string `json:"display"`
}

type DeltaProcessFact struct {
	Operation      string              `json:"operation"`
	StableID       string              `json:"stableId"`
	ParentStableID string              `json:"parentStableId,omitempty"`
	ParentRelation string              `json:"parentRelation"`
	Executable     DeltaNormalizedPath `json:"executable"`
	Arguments      []string            `json:"arguments"`
	Environment    []EnvEntry          `json:"environment"`
	ExitCode       *int                `json:"exitCode,omitempty"`
	DurationMillis int64               `json:"durationMillis"`
}

type DeltaFilesystemFact struct {
	Operation  string               `json:"operation"`
	Path       DeltaNormalizedPath  `json:"path"`
	OldPath    *DeltaNormalizedPath `json:"oldPath,omitempty"`
	Mode       string               `json:"mode,omitempty"`
	Digest     Digest               `json:"digest,omitempty"`
	SizeBytes  int64                `json:"sizeBytes"`
	Executable bool                 `json:"executable"`
	Truncated  bool                 `json:"truncated"`
}

type DeltaNetworkFact struct {
	Operation         string   `json:"operation"`
	Protocol          string   `json:"protocol"`
	QueryName         string   `json:"queryName,omitempty"`
	DestinationHost   string   `json:"destinationHost,omitempty"`
	DestinationPort   int      `json:"destinationPort,omitempty"`
	ResolvedAddresses []string `json:"resolvedAddresses"`
	Result            string   `json:"result"`
	DurationMillis    int64    `json:"durationMillis,omitempty"`
}

type DeltaArtifactFact struct {
	Operation      string              `json:"operation"`
	ArtifactID     string              `json:"artifactId"`
	Path           DeltaNormalizedPath `json:"path"`
	Digest         Digest              `json:"digest,omitempty"`
	SizeBytes      int64               `json:"sizeBytes"`
	Executable     bool                `json:"executable"`
	SourceEventIDs []string            `json:"sourceEventIds"`
}

type DeltaScenarioFact struct {
	Status         ScenarioStatus `json:"status"`
	Message        string         `json:"message,omitempty"`
	DurationMillis int64          `json:"durationMillis"`
}

type DeltaWarningFact struct {
	Code        string       `json:"code"`
	Message     string       `json:"message"`
	Unsupported bool         `json:"unsupported"`
	Limitations []Limitation `json:"limitations"`
}

type DeltaResourceFact struct {
	LimitKind     string `json:"limitKind"`
	LimitValue    int64  `json:"limitValue"`
	Unit          string `json:"unit"`
	ObservedValue int64  `json:"observedValue"`
	Exceeded      bool   `json:"exceeded"`
}

type DeltaFactSnapshot struct {
	ID             string               `json:"id"`
	SemanticDigest Digest               `json:"semanticDigest"`
	Kind           string               `json:"kind"`
	Source         ObservationSource    `json:"source"`
	Process        *DeltaProcessFact    `json:"process,omitempty"`
	Filesystem     *DeltaFilesystemFact `json:"filesystem,omitempty"`
	Network        *DeltaNetworkFact    `json:"network,omitempty"`
	Artifact       *DeltaArtifactFact   `json:"artifact,omitempty"`
	Scenario       *DeltaScenarioFact   `json:"scenario,omitempty"`
	Warning        *DeltaWarningFact    `json:"warning,omitempty"`
	Resource       *DeltaResourceFact   `json:"resource,omitempty"`
	Evidence       []EvidenceRef        `json:"evidence"`
	Limitations    []Limitation         `json:"limitations"`
}
