// Package model defines Glassroot's versioned, data-only core wire structures.
package model

// SchemaVersion identifies the wire-format version for independently serialized
// Glassroot documents and newline-delimited observation events.
type SchemaVersion string

const (
	SchemaVersionRunV1Alpha1              SchemaVersion = "glassroot.dev/run/v1alpha1"
	SchemaVersionRunPlanV1Alpha1          SchemaVersion = "glassroot.dev/run-plan/v1alpha1"
	SchemaVersionObservationEventV1Alpha1 SchemaVersion = "glassroot.dev/observation-event/v1alpha1"
	SchemaVersionScenarioResultV1Alpha1   SchemaVersion = "glassroot.dev/scenario-result/v1alpha1"
	SchemaVersionBehavioralDeltaV1Alpha1  SchemaVersion = "glassroot.dev/behavioral-delta/v1alpha1"
	SchemaVersionEvidenceManifestV1Alpha1 SchemaVersion = "glassroot.dev/evidence-manifest/v1alpha1"
	SchemaVersionFindingV1Alpha1          SchemaVersion = "glassroot.dev/finding/v1alpha1"
	SchemaVersionReportV1Alpha1           SchemaVersion = "glassroot.dev/report/v1alpha1"
)
