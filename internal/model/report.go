package model

import "time"

// Report is an independently serialized report model. Rendering is deferred to
// later packages; this type only records structured data.
type Report struct {
	SchemaVersion       SchemaVersion       `json:"schemaVersion"`
	ID                  string              `json:"id"`
	RunID               string              `json:"runId"`
	GeneratedAt         time.Time           `json:"generatedAt"`
	Base                CommitRef           `json:"base"`
	Head                CommitRef           `json:"head"`
	Runner              RunnerCapabilities  `json:"runner"`
	Summary             ReportSummary       `json:"summary"`
	Findings            []Finding           `json:"findings"`
	Deltas              []DeltaRecord       `json:"deltas"`
	Evidence            []EvidenceRef       `json:"evidence"`
	Limitations         []Limitation        `json:"limitations"`
	AttestationMetadata AttestationMetadata `json:"attestationMetadata"`
}

// ReportSummary records deterministic counts and disposition facts. Ordered
// slices are used instead of maps for reproducible future rendering/hashing.
type ReportSummary struct {
	Disposition        Disposition     `json:"disposition"`
	TotalFindings      int64           `json:"totalFindings"`
	FindingsBySeverity []SeverityCount `json:"findingsBySeverity"`
	BaseScenarioCount  int64           `json:"baseScenarioCount"`
	HeadScenarioCount  int64           `json:"headScenarioCount"`
	Limitations        []Limitation    `json:"limitations"`
}

// SeverityCount is an ordered report count bucket.
type SeverityCount struct {
	Severity Severity `json:"severity"`
	Count    int64    `json:"count"`
}

// AttestationMetadata is descriptive metadata only. Signing formats, keys, and
// content-addressing are deferred to later milestones.
type AttestationMetadata struct {
	ProducerName    string    `json:"producerName"`
	ProducerVersion string    `json:"producerVersion"`
	GeneratedAt     time.Time `json:"generatedAt"`
	BuildCommit     string    `json:"buildCommit,omitempty"`
	Notes           []string  `json:"notes"`
}
