package model

import "time"

// ArtifactRecord records an artifact as data. Paths remain untrusted strings.
type ArtifactRecord struct {
	ID             string       `json:"id"`
	Revision       RevisionKind `json:"revision"`
	ScenarioID     string       `json:"scenarioId"`
	Path           string       `json:"path"`
	Digest         Digest       `json:"digest"`
	SizeBytes      int64        `json:"sizeBytes"`
	Executable     bool         `json:"executable"`
	SourceEventIDs []string     `json:"sourceEventIds"`
	Limitations    []Limitation `json:"limitations"`
}

// EvidenceManifest is an independently serialized manifest of evidence entries
// and artifacts. It does not read, write, hash, or verify evidence content.
type EvidenceManifest struct {
	SchemaVersion SchemaVersion    `json:"schemaVersion"`
	ID            string           `json:"id"`
	RunID         string           `json:"runId"`
	CreatedAt     time.Time        `json:"createdAt"`
	Entries       []EvidenceEntry  `json:"entries"`
	Artifacts     []ArtifactRecord `json:"artifacts"`
	Limitations   []Limitation     `json:"limitations"`
}

// EvidenceEntry records one logical evidence bundle entry.
type EvidenceEntry struct {
	Kind        string            `json:"kind"`
	Revision    RevisionKind      `json:"revision"`
	ScenarioID  string            `json:"scenarioId"`
	Path        string            `json:"path"`
	Digest      Digest            `json:"digest"`
	SizeBytes   int64             `json:"sizeBytes"`
	MediaType   string            `json:"mediaType"`
	Source      ObservationSource `json:"source"`
	EventIDs    []string          `json:"eventIds"`
	Truncated   bool              `json:"truncated"`
	Limitations []Limitation      `json:"limitations"`
}

// EvidenceRef links a finding or delta to evidence content, event IDs, and an
// optional logical bundle path. The path is data only and is not accessed here.
type EvidenceRef struct {
	Digest            Digest       `json:"digest,omitempty"`
	EventIDs          []string     `json:"eventIds"`
	BundlePath        *string      `json:"bundlePath,omitempty"`
	EventStreamDigest Digest       `json:"eventStreamDigest,omitempty"`
	EventStreamPath   string       `json:"eventStreamPath,omitempty"`
	EventSequence     uint64       `json:"eventSequence,omitempty"`
	Revision          RevisionKind `json:"revision,omitempty"`
	ScenarioID        string       `json:"scenarioId,omitempty"`
	Repetition        uint32       `json:"repetition,omitempty"`
}
