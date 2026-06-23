package model

import (
	"encoding/json"
	"testing"
)

func TestGR7BAdditiveEventAndCapabilityWireValues(t *testing.T) {
	if ObservationSourceSyntheticTestGenerated != "synthetic-test-generated" {
		t.Fatalf("synthetic source = %q", ObservationSourceSyntheticTestGenerated)
	}
	caps := RunnerCapabilities{
		Name:                "fake",
		Version:             "v1",
		IsolationTier:       IsolationTierFake,
		SyntheticEvidence:   true,
		ExecutesTargetCode:  false,
		EnforcesNetworkDeny: false,
	}
	data, err := json.Marshal(caps)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) {
		t.Fatalf("capabilities JSON invalid: %s", data)
	}
	var round RunnerCapabilities
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatal(err)
	}
	if !round.SyntheticEvidence || round.ExecutesTargetCode || round.EnforcesNetworkDeny {
		t.Fatalf("capability facts did not round-trip: %+v", round)
	}

	event := ObservationEvent{
		SchemaVersion:  SchemaVersionObservationEventV1Alpha1,
		ID:             "evt-test",
		RunID:          "run-0001",
		Revision:       RevisionKindHead,
		ScenarioID:     "test",
		Repetition:     2,
		SequenceNumber: 17,
		Source:         ObservationSourceSyntheticTestGenerated,
		Kind:           ObservationKindObserverWarning,
		ObserverWarning: &ObserverWarningObservation{
			Code:        "synthetic",
			Message:     "synthetic warning",
			Unsupported: true,
			Limitations: []Limitation{},
		},
	}
	data, err = json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) || !containsJSONField(data, "repetition") {
		t.Fatalf("event JSON should include repetition when populated: %s", data)
	}
	var eventRound ObservationEvent
	if err := json.Unmarshal(data, &eventRound); err != nil {
		t.Fatal(err)
	}
	if eventRound.Repetition != 2 || eventRound.Source != ObservationSourceSyntheticTestGenerated {
		t.Fatalf("event additive fields lost: %+v", eventRound)
	}
}

func containsJSONField(data []byte, field string) bool {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return false
	}
	_, ok := object[field]
	return ok
}
