package model

import (
	"encoding/json"
	"testing"
)

func TestGR10AAdditiveFindingWireFields(t *testing.T) {
	finding := Finding{
		SchemaVersion:  SchemaVersionFindingV1Alpha1,
		ID:             "finding-test",
		RuleID:         "GR-NET-001",
		RuleVersion:    "v1alpha1",
		Title:          "New or changed network behavior",
		Severity:       SeverityHigh,
		Confidence:     ConfidenceLow,
		Disposition:    DispositionRequiresReview,
		Summary:        "Fixed summary.",
		DeltaRecordIDs: []string{"delta-test"},
		Evidence:       []EvidenceRef{},
		ScenarioIDs:    []string{"unit"},
		HeadObserved:   true,
		Waived:         false,
		Limitations:    []Limitation{},
	}
	data, err := json.Marshal(finding)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) || !containsJSONField(data, "ruleVersion") || !containsJSONField(data, "deltaRecordIds") {
		t.Fatalf("GR-10A additive finding fields missing from JSON: %s", data)
	}
	var round Finding
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatal(err)
	}
	if round.RuleVersion != finding.RuleVersion || len(round.DeltaRecordIDs) != 1 || round.DeltaRecordIDs[0] != "delta-test" {
		t.Fatalf("additive finding fields did not round-trip: %+v", round)
	}
}
