package demo

import (
	"encoding/json"
	"time"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/report"
)

const (
	SchemaVersionDemoV1Alpha1 = "glassroot.dev/fake-demo/v1alpha1"

	FixtureBehaviorChangeID = "glassroot.dev/fake-demo-fixture/behavior-change/v1alpha1"
	FixtureControlID        = "glassroot.dev/fake-demo-fixture/control/v1alpha1"

	fixtureVersionV1Alpha1 = "v1alpha1"
	fixedPlanCreatedAt     = "2026-06-23T00:00:00Z"
	fixedPolicyEvaluatedAt = "2026-06-24T00:00:00Z"
)

type Fixture string

const (
	FixtureBehaviorChange Fixture = "behavior-change"
	FixtureControl        Fixture = "control"
)

type Demo struct{ limits Limits }

type Request struct {
	Fixture   Fixture
	OutputDir string
}

type Result struct {
	Report               *report.FrozenReport
	ManifestDigest       model.Digest
	BaseCommitID         string
	HeadCommitID         string
	EffectiveDisposition model.Disposition
	ExpectedExitCode     int
	Metadata             Metadata
}

type Metadata struct {
	SchemaVersion           string              `json:"schemaVersion"`
	FixtureID               string              `json:"fixtureId"`
	FixtureVersion          string              `json:"fixtureVersion"`
	RunID                   string              `json:"runId"`
	PlanCreatedAt           string              `json:"planCreatedAt"`
	PolicyEvaluatedAt       string              `json:"policyEvaluatedAt"`
	BaseCommitID            string              `json:"baseCommitId"`
	BaseTreeID              string              `json:"baseTreeId"`
	HeadCommitID            string              `json:"headCommitId"`
	HeadTreeID              string              `json:"headTreeId"`
	ObjectFormat            string              `json:"objectFormat"`
	PlanDigest              model.Digest        `json:"planDigest"`
	ManifestDigest          model.Digest        `json:"manifestDigest"`
	BehavioralDeltaDigest   model.Digest        `json:"behavioralDeltaDigest"`
	PolicyEvaluationDigest  model.Digest        `json:"policyEvaluationDigest"`
	PolicyApplicationDigest model.Digest        `json:"policyApplicationDigest"`
	ReportDigest            model.Digest        `json:"reportDigest"`
	MarkdownDigest          model.Digest        `json:"markdownDigest"`
	TerminalDigest          model.Digest        `json:"terminalDigest"`
	EffectiveDisposition    model.Disposition   `json:"effectiveDisposition"`
	ExpectedCLIExitCode     int                 `json:"expectedCliExitCode"`
	RelativePaths           MetadataPaths       `json:"relativePaths"`
	KeyEvidence             []KeyEvidenceRecord `json:"keyEvidence"`
}

type MetadataPaths struct {
	FixtureGit     string `json:"fixtureGit"`
	Evidence       string `json:"evidence"`
	ReportJSON     string `json:"reportJson"`
	ReportMarkdown string `json:"reportMarkdown"`
	ReportTerminal string `json:"reportTerminal"`
}

type KeyEvidenceRecord struct {
	Category          string             `json:"category"`
	RuleID            string             `json:"ruleId,omitempty"`
	FindingID         string             `json:"findingId,omitempty"`
	DeltaRecordIDs    []string           `json:"deltaRecordIds"`
	EventIDs          []string           `json:"eventIds"`
	EventStreamDigest model.Digest       `json:"eventStreamDigest,omitempty"`
	EventStreamPath   string             `json:"eventStreamPath,omitempty"`
	Revision          model.RevisionKind `json:"revision,omitempty"`
	ScenarioID        string             `json:"scenarioId,omitempty"`
	Repetition        uint32             `json:"repetition,omitempty"`
}

func (m Metadata) PolicyEvaluatedAtTime() time.Time {
	t, _ := time.Parse(time.RFC3339, m.PolicyEvaluatedAt)
	return t.UTC().Round(0)
}

func EncodeMetadataForTest(md Metadata) ([]byte, error) { return encodeMetadata(md) }

func encodeMetadata(md Metadata) ([]byte, error) {
	if md.KeyEvidence == nil {
		md.KeyEvidence = []KeyEvidenceRecord{}
	}
	for i := range md.KeyEvidence {
		if md.KeyEvidence[i].DeltaRecordIDs == nil {
			md.KeyEvidence[i].DeltaRecordIDs = []string{}
		}
		if md.KeyEvidence[i].EventIDs == nil {
			md.KeyEvidence[i].EventIDs = []string{}
		}
	}
	return json.Marshal(md)
}

func fixtureID(f Fixture) string {
	switch f {
	case FixtureBehaviorChange:
		return FixtureBehaviorChangeID
	case FixtureControl:
		return FixtureControlID
	default:
		return ""
	}
}

func fixtureRunID(f Fixture) string {
	switch f {
	case FixtureBehaviorChange:
		return "gr12-behavior-change"
	case FixtureControl:
		return "gr12-control"
	default:
		return ""
	}
}
