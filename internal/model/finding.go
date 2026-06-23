package model

import "time"

// BehavioralDelta is an independently serialized comparison document.
type BehavioralDelta struct {
	SchemaVersion SchemaVersion `json:"schemaVersion"`
	ID            string        `json:"id"`
	RunID         string        `json:"runId"`
	Base          CommitRef     `json:"base"`
	Head          CommitRef     `json:"head"`
	ScenarioIDs   []string      `json:"scenarioIds"`
	Records       []DeltaRecord `json:"records"`
	Limitations   []Limitation  `json:"limitations"`
}

// DeltaKind names a behavioral difference category.
type DeltaKind string

const (
	DeltaKindAddedProcess            DeltaKind = "added-process"
	DeltaKindAddedFilesystemActivity DeltaKind = "added-filesystem-activity"
	DeltaKindAddedNetworkConnection  DeltaKind = "added-network-connection"
	DeltaKindArtifactChanged         DeltaKind = "artifact-changed"
	DeltaKindObservationIncomplete   DeltaKind = "observation-incomplete"
)

// DeltaRecord records one comparison result with evidence references.
type DeltaRecord struct {
	ID           string        `json:"id"`
	Kind         DeltaKind     `json:"kind"`
	Summary      string        `json:"summary"`
	BaseObserved bool          `json:"baseObserved"`
	HeadObserved bool          `json:"headObserved"`
	Evidence     []EvidenceRef `json:"evidence"`
	ScenarioIDs  []string      `json:"scenarioIds"`
	Limitations  []Limitation  `json:"limitations"`
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
