package model

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestV1Alpha1CompatibilityFixturesRoundTrip(t *testing.T) {
	t.Run("run plan", func(t *testing.T) {
		fixture := readFixture(t, "run-plan.json")
		decoded := decodeRunPlan(t, fixture)
		if decoded.SchemaVersion != SchemaVersionRunPlanV1Alpha1 {
			t.Fatalf("schemaVersion = %q, want %q", decoded.SchemaVersion, SchemaVersionRunPlanV1Alpha1)
		}
		if decoded.Runner.IsolationTier != IsolationTierFake {
			t.Fatalf("runner isolation tier = %q, want %q", decoded.Runner.IsolationTier, IsolationTierFake)
		}
		if !decoded.Runner.ProcessEventCollection || !decoded.Runner.ArtifactHashing {
			t.Fatalf("runner capabilities lost security-relevant facts: %+v", decoded.Runner)
		}
		if len(decoded.Revisions) != 2 || decoded.Revisions[0].Kind != RevisionKindBase || decoded.Revisions[1].Kind != RevisionKindHead {
			t.Fatalf("revisions do not exercise base/head identity: %+v", decoded.Revisions)
		}
		if len(decoded.Scenarios) != 1 || decoded.Scenarios[0].NetworkPolicy.Mode != NetworkModeDeny {
			t.Fatalf("scenario does not preserve deny network policy: %+v", decoded.Scenarios)
		}
		roundTripRunPlan(t, decoded, SchemaVersionRunPlanV1Alpha1)
	})

	t.Run("observation event", func(t *testing.T) {
		fixture := readFixture(t, "observation-event.json")
		decoded := decodeObservationEvent(t, fixture)
		if decoded.SchemaVersion != SchemaVersionObservationEventV1Alpha1 {
			t.Fatalf("schemaVersion = %q, want %q", decoded.SchemaVersion, SchemaVersionObservationEventV1Alpha1)
		}
		if decoded.Revision != RevisionKindHead || decoded.Source != ObservationSourceNetworkBrokerObserved || decoded.Kind != ObservationKindNetworkConnection {
			t.Fatalf("event provenance/kind mismatch: %+v", decoded)
		}
		if decoded.SequenceNumber != 9007199254740993 {
			t.Fatalf("sequenceNumber = %d, want exact large integer", decoded.SequenceNumber)
		}
		if decoded.Network == nil || decoded.Network.DestinationHost != "updates.example.invalid" || decoded.Network.DestinationPort != 443 {
			t.Fatalf("network payload mismatch: %+v", decoded.Network)
		}
		roundTripObservationEvent(t, decoded, SchemaVersionObservationEventV1Alpha1)
	})

	t.Run("scenario result", func(t *testing.T) {
		fixture := readFixture(t, "scenario-result.json")
		decoded := decodeScenarioResult(t, fixture)
		if decoded.SchemaVersion != SchemaVersionScenarioResultV1Alpha1 {
			t.Fatalf("schemaVersion = %q, want %q", decoded.SchemaVersion, SchemaVersionScenarioResultV1Alpha1)
		}
		if decoded.ExitCode == nil || *decoded.ExitCode != 0 {
			t.Fatalf("exitCode = %+v, want observed zero", decoded.ExitCode)
		}
		if decoded.Revision != RevisionKindHead || decoded.Status != ScenarioStatusPassed {
			t.Fatalf("scenario result identity/status mismatch: %+v", decoded)
		}
		roundTripScenarioResult(t, decoded, SchemaVersionScenarioResultV1Alpha1)
	})

	t.Run("evidence manifest", func(t *testing.T) {
		fixture := readFixture(t, "evidence-manifest.json")
		decoded := decodeEvidenceManifest(t, fixture)
		if decoded.SchemaVersion != SchemaVersionEvidenceManifestV1Alpha1 {
			t.Fatalf("schemaVersion = %q, want %q", decoded.SchemaVersion, SchemaVersionEvidenceManifestV1Alpha1)
		}
		if len(decoded.Entries) != 2 || decoded.Entries[0].Source != ObservationSourceSandboxRuntimeObserved {
			t.Fatalf("evidence entries do not preserve provenance: %+v", decoded.Entries)
		}
		if decoded.Entries[1].SizeBytes != 9007199254740995 {
			t.Fatalf("sizeBytes = %d, want exact large integer", decoded.Entries[1].SizeBytes)
		}
		roundTripEvidenceManifest(t, decoded, SchemaVersionEvidenceManifestV1Alpha1)
	})

	t.Run("behavioral delta", func(t *testing.T) {
		fixture := readFixture(t, "behavioral-delta.json")
		decoded := decodeBehavioralDelta(t, fixture)
		if decoded.SchemaVersion != SchemaVersionBehavioralDeltaV1Alpha1 {
			t.Fatalf("schemaVersion = %q, want %q", decoded.SchemaVersion, SchemaVersionBehavioralDeltaV1Alpha1)
		}
		if len(decoded.Records) != 1 || decoded.Records[0].Kind != DeltaKindAddedNetworkConnection {
			t.Fatalf("delta record mismatch: %+v", decoded.Records)
		}
		if decoded.Records[0].BaseObserved || !decoded.Records[0].HeadObserved {
			t.Fatalf("delta base/head observation flags mismatch: %+v", decoded.Records[0])
		}
		roundTripBehavioralDelta(t, decoded, SchemaVersionBehavioralDeltaV1Alpha1)
	})

	t.Run("finding", func(t *testing.T) {
		fixture := readFixture(t, "finding.json")
		decoded := decodeFinding(t, fixture)
		if decoded.SchemaVersion != SchemaVersionFindingV1Alpha1 {
			t.Fatalf("schemaVersion = %q, want %q", decoded.SchemaVersion, SchemaVersionFindingV1Alpha1)
		}
		if decoded.Severity != SeverityHigh || decoded.Confidence != ConfidenceHigh || decoded.Disposition != DispositionRequiresReview {
			t.Fatalf("finding severity/confidence/disposition mismatch: %+v", decoded)
		}
		if len(decoded.Evidence) != 1 || len(decoded.Evidence[0].EventIDs) != 1 || decoded.Evidence[0].BundlePath == nil {
			t.Fatalf("finding evidence reference incomplete: %+v", decoded.Evidence)
		}
		roundTripFinding(t, decoded, SchemaVersionFindingV1Alpha1)
	})

	t.Run("report", func(t *testing.T) {
		fixture := readFixture(t, "report.json")
		decoded := decodeReport(t, fixture)
		if decoded.SchemaVersion != SchemaVersionReportV1Alpha1 {
			t.Fatalf("schemaVersion = %q, want %q", decoded.SchemaVersion, SchemaVersionReportV1Alpha1)
		}
		if decoded.Summary.Disposition != DispositionRequiresReview || len(decoded.Findings) != 1 {
			t.Fatalf("report summary/finding mismatch: %+v", decoded.Summary)
		}
		if decoded.AttestationMetadata.ProducerName != "glassroot" || len(decoded.AttestationMetadata.Notes) == 0 {
			t.Fatalf("attestation metadata should remain descriptive: %+v", decoded.AttestationMetadata)
		}
		roundTripReport(t, decoded, SchemaVersionReportV1Alpha1)
	})
}

func TestTopLevelWireDocumentsEmitSchemaVersion(t *testing.T) {
	exitZero := 0
	bundlePath := "head/unit/events.jsonl"
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	runner := RunnerCapabilities{Name: "glassroot-fake-runner", Version: "v0.0.0-test", IsolationTier: IsolationTierFake}
	commit := CommitRef{Kind: RevisionKindBase, Repository: "https://example.invalid/org/repo.git", Ref: "refs/heads/master", CommitID: "1111111111111111111111111111111111111111"}
	evidence := []EvidenceRef{{Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", EventIDs: []string{"evt-1"}, BundlePath: &bundlePath}}

	check := func(name string, data []byte, err error, want SchemaVersion) {
		t.Helper()
		if err != nil {
			t.Fatalf("Marshal(%s) error = %v", name, err)
		}
		assertSchemaVersionInJSON(t, data, want)
	}

	data, err := json.Marshal(Run{SchemaVersion: SchemaVersionRunV1Alpha1, ID: "run-1", CreatedAt: now, Base: commit, Head: commit, Runner: runner, Limitations: []Limitation{}})
	check("Run", data, err, SchemaVersionRunV1Alpha1)

	data, err = json.Marshal(RunPlan{SchemaVersion: SchemaVersionRunPlanV1Alpha1, ID: "plan-1", RunID: "run-1", CreatedAt: now, Base: commit, Head: commit, Revisions: []RevisionPlan{}, Scenarios: []ScenarioPlan{}, Runner: runner, ResourceLimits: ResourceLimits{}, NetworkPolicy: NetworkPolicy{Mode: NetworkModeDeny, Allowed: []NetworkAllowRule{}}, Environment: []EnvEntry{}, Limitations: []Limitation{}})
	check("RunPlan", data, err, SchemaVersionRunPlanV1Alpha1)

	data, err = json.Marshal(ObservationEvent{SchemaVersion: SchemaVersionObservationEventV1Alpha1, ID: "evt-1", RunID: "run-1", Revision: RevisionKindHead, ScenarioID: "unit", SequenceNumber: 1, ObservedAt: now, Source: ObservationSourceHostObserved, Kind: ObservationKindScenarioStarted, Scenario: &ScenarioObservation{Status: ScenarioStatusRunning}})
	check("ObservationEvent", data, err, SchemaVersionObservationEventV1Alpha1)

	data, err = json.Marshal(ScenarioResult{SchemaVersion: SchemaVersionScenarioResultV1Alpha1, ID: "result-1", RunID: "run-1", Revision: RevisionKindHead, ScenarioID: "unit", Status: ScenarioStatusPassed, ExitCode: &exitZero, Artifacts: []ArtifactRecord{}, Limitations: []Limitation{}})
	check("ScenarioResult", data, err, SchemaVersionScenarioResultV1Alpha1)

	data, err = json.Marshal(BehavioralDelta{SchemaVersion: SchemaVersionBehavioralDeltaV1Alpha1, ID: "delta-1", RunID: "run-1", Base: commit, Head: commit, ScenarioIDs: []string{"unit"}, Records: []DeltaRecord{}, Limitations: []Limitation{}})
	check("BehavioralDelta", data, err, SchemaVersionBehavioralDeltaV1Alpha1)

	data, err = json.Marshal(EvidenceManifest{SchemaVersion: SchemaVersionEvidenceManifestV1Alpha1, ID: "manifest-1", RunID: "run-1", CreatedAt: now, Entries: []EvidenceEntry{}, Artifacts: []ArtifactRecord{}, Limitations: []Limitation{}})
	check("EvidenceManifest", data, err, SchemaVersionEvidenceManifestV1Alpha1)

	data, err = json.Marshal(Finding{SchemaVersion: SchemaVersionFindingV1Alpha1, ID: "finding-1", RuleID: "GR-NET-001", Title: "New outbound destination", Severity: SeverityHigh, Confidence: ConfidenceHigh, Disposition: DispositionRequiresReview, Summary: "Head attempted a synthetic connection absent from base.", Evidence: evidence, ScenarioIDs: []string{"unit"}, HeadObserved: true, Limitations: []Limitation{}})
	check("Finding", data, err, SchemaVersionFindingV1Alpha1)

	data, err = json.Marshal(Report{SchemaVersion: SchemaVersionReportV1Alpha1, ID: "report-1", RunID: "run-1", GeneratedAt: now, Base: commit, Head: commit, Runner: runner, Summary: ReportSummary{Disposition: DispositionRequiresReview, FindingsBySeverity: []SeverityCount{}}, Findings: []Finding{}, Evidence: evidence, Deltas: []DeltaRecord{}, Limitations: []Limitation{}, AttestationMetadata: AttestationMetadata{ProducerName: "glassroot", ProducerVersion: "dev", GeneratedAt: now}})
	check("Report", data, err, SchemaVersionReportV1Alpha1)
}

func TestSchemaVersionAndEnumWireValues(t *testing.T) {
	schemaVersions := []struct {
		name string
		got  SchemaVersion
		want string
	}{
		{"run", SchemaVersionRunV1Alpha1, "glassroot.dev/run/v1alpha1"},
		{"run-plan", SchemaVersionRunPlanV1Alpha1, "glassroot.dev/run-plan/v1alpha1"},
		{"observation-event", SchemaVersionObservationEventV1Alpha1, "glassroot.dev/observation-event/v1alpha1"},
		{"scenario-result", SchemaVersionScenarioResultV1Alpha1, "glassroot.dev/scenario-result/v1alpha1"},
		{"behavioral-delta", SchemaVersionBehavioralDeltaV1Alpha1, "glassroot.dev/behavioral-delta/v1alpha1"},
		{"evidence-manifest", SchemaVersionEvidenceManifestV1Alpha1, "glassroot.dev/evidence-manifest/v1alpha1"},
		{"finding", SchemaVersionFindingV1Alpha1, "glassroot.dev/finding/v1alpha1"},
		{"report", SchemaVersionReportV1Alpha1, "glassroot.dev/report/v1alpha1"},
	}
	for _, tt := range schemaVersions {
		if string(tt.got) != tt.want {
			t.Fatalf("%s schema version = %q, want %q", tt.name, tt.got, tt.want)
		}
	}

	revisionKinds := []struct{ got, want RevisionKind }{{RevisionKindBase, "base"}, {RevisionKindHead, "head"}}
	for _, tt := range revisionKinds {
		if tt.got != tt.want {
			t.Fatalf("revision kind = %q, want %q", tt.got, tt.want)
		}
	}

	objectFormats := []struct{ got, want GitObjectFormat }{{GitObjectFormatSHA1, "sha1"}, {GitObjectFormatSHA256, "sha256"}}
	for _, tt := range objectFormats {
		if tt.got != tt.want {
			t.Fatalf("git object format = %q, want %q", tt.got, tt.want)
		}
	}

	isolationTiers := []struct{ got, want IsolationTier }{{IsolationTierFake, "fake"}, {IsolationTierDevelopmentOnly, "development-only"}, {IsolationTierHardenedContainer, "hardened-container"}, {IsolationTierMicroVM, "microvm"}}
	for _, tt := range isolationTiers {
		if tt.got != tt.want {
			t.Fatalf("isolation tier = %q, want %q", tt.got, tt.want)
		}
	}

	observationSources := []struct{ got, want ObservationSource }{
		{ObservationSourceHostObserved, "host-observed"},
		{ObservationSourceNetworkBrokerObserved, "network-broker-observed"},
		{ObservationSourceSandboxRuntimeObserved, "sandbox-runtime-observed"},
		{ObservationSourceGuestAgentReported, "guest-agent-reported"},
		{ObservationSourceWorkloadReported, "workload-reported"},
		{ObservationSourceStaticAnalysisDerived, "static-analysis-derived"},
		{ObservationSourceModelInferred, "model-inferred"},
	}
	for _, tt := range observationSources {
		if tt.got != tt.want {
			t.Fatalf("observation source = %q, want %q", tt.got, tt.want)
		}
	}

	severities := []struct{ got, want Severity }{{SeverityInfo, "info"}, {SeverityLow, "low"}, {SeverityMedium, "medium"}, {SeverityHigh, "high"}, {SeverityCritical, "critical"}}
	for _, tt := range severities {
		if tt.got != tt.want {
			t.Fatalf("severity = %q, want %q", tt.got, tt.want)
		}
	}

	confidences := []struct{ got, want Confidence }{{ConfidenceLow, "low"}, {ConfidenceMedium, "medium"}, {ConfidenceHigh, "high"}, {ConfidenceUnknown, "unknown"}}
	for _, tt := range confidences {
		if tt.got != tt.want {
			t.Fatalf("confidence = %q, want %q", tt.got, tt.want)
		}
	}

	dispositions := []struct{ got, want Disposition }{{DispositionPassed, "passed"}, {DispositionRequiresReview, "requires-review"}, {DispositionFailed, "failed"}, {DispositionWaived, "waived"}}
	for _, tt := range dispositions {
		if tt.got != tt.want {
			t.Fatalf("disposition = %q, want %q", tt.got, tt.want)
		}
	}

	networkModes := []struct{ got, want NetworkMode }{{NetworkModeDeny, "deny"}, {NetworkModeAllowlist, "allowlist"}}
	for _, tt := range networkModes {
		if tt.got != tt.want {
			t.Fatalf("network mode = %q, want %q", tt.got, tt.want)
		}
	}

	observationKinds := []struct{ got, want ObservationKind }{
		{ObservationKindProcessStart, "process-start"},
		{ObservationKindProcessExit, "process-exit"},
		{ObservationKindFilesystemCreate, "filesystem-create"},
		{ObservationKindFilesystemRead, "filesystem-read"},
		{ObservationKindFilesystemWrite, "filesystem-write"},
		{ObservationKindFilesystemDelete, "filesystem-delete"},
		{ObservationKindFilesystemRename, "filesystem-rename"},
		{ObservationKindFilesystemChmod, "filesystem-chmod"},
		{ObservationKindDNSQuery, "dns-query"},
		{ObservationKindNetworkConnection, "network-connection"},
		{ObservationKindArtifactActivity, "artifact-activity"},
		{ObservationKindScenarioStarted, "scenario-started"},
		{ObservationKindScenarioCompleted, "scenario-completed"},
		{ObservationKindObserverWarning, "observer-warning"},
		{ObservationKindUnsupportedObservation, "unsupported-observation"},
		{ObservationKindResourceLimit, "resource-limit"},
	}
	for _, tt := range observationKinds {
		if tt.got != tt.want {
			t.Fatalf("observation kind = %q, want %q", tt.got, tt.want)
		}
	}

	scenarioStatuses := []struct{ got, want ScenarioStatus }{
		{ScenarioStatusPlanned, "planned"},
		{ScenarioStatusRunning, "running"},
		{ScenarioStatusPassed, "passed"},
		{ScenarioStatusFailed, "failed"},
		{ScenarioStatusError, "error"},
		{ScenarioStatusTimedOut, "timed-out"},
		{ScenarioStatusCancelled, "cancelled"},
		{ScenarioStatusSkipped, "skipped"},
		{ScenarioStatusIncomplete, "incomplete"},
	}
	for _, tt := range scenarioStatuses {
		if tt.got != tt.want {
			t.Fatalf("scenario status = %q, want %q", tt.got, tt.want)
		}
	}

	deltaKinds := []struct{ got, want DeltaKind }{
		{DeltaKindAddedProcess, "added-process"},
		{DeltaKindAddedFilesystemActivity, "added-filesystem-activity"},
		{DeltaKindAddedNetworkConnection, "added-network-connection"},
		{DeltaKindArtifactChanged, "artifact-changed"},
		{DeltaKindObservationIncomplete, "observation-incomplete"},
	}
	for _, tt := range deltaKinds {
		if tt.got != tt.want {
			t.Fatalf("delta kind = %q, want %q", tt.got, tt.want)
		}
	}
}

func TestMissingSchemaVersionRemainsObservable(t *testing.T) {
	withoutVersion := []byte(`{"id":"evt-missing-version","runId":"run-0001","revision":"head","scenarioId":"unit","sequenceNumber":1,"observedAt":"2026-01-02T03:04:06Z","source":"host-observed","kind":"scenario-started","scenario":{"status":"running","durationMillis":0}}`)
	event := decodeObservationEvent(t, withoutVersion)
	if event.SchemaVersion != "" {
		t.Fatalf("missing schemaVersion should remain observable as zero value, got %q", event.SchemaVersion)
	}
}

func TestOptionalExitCodeAndTimestampPreserveAbsentVersusZero(t *testing.T) {
	withoutObservedValues := []byte(`{"schemaVersion":"glassroot.dev/scenario-result/v1alpha1","id":"result-absent","runId":"run-0001","revision":"head","scenarioId":"unit","status":"running","durationMillis":0,"artifacts":[],"limitations":[]}`)
	withZeroValues := []byte(`{"schemaVersion":"glassroot.dev/scenario-result/v1alpha1","id":"result-zero","runId":"run-0001","revision":"head","scenarioId":"unit","status":"passed","startedAt":"1970-01-01T00:00:00Z","completedAt":"1970-01-01T00:00:00Z","durationMillis":0,"exitCode":0,"artifacts":[],"limitations":[]}`)

	without := decodeScenarioResult(t, withoutObservedValues)
	if without.ExitCode != nil || without.StartedAt != nil || without.CompletedAt != nil {
		t.Fatalf("absent optional fields should remain nil: %+v", without)
	}

	withZero := decodeScenarioResult(t, withZeroValues)
	if withZero.ExitCode == nil || *withZero.ExitCode != 0 {
		t.Fatalf("zero exit code should remain observed: %+v", withZero.ExitCode)
	}
	if withZero.StartedAt == nil || withZero.CompletedAt == nil {
		t.Fatalf("zero-value Unix timestamps should remain observed pointers: %+v", withZero)
	}
}

func TestLargeIntegersRoundTripWithoutFloatConversion(t *testing.T) {
	event := decodeObservationEvent(t, readFixture(t, "observation-event.json"))
	if event.SequenceNumber != 9007199254740993 {
		t.Fatalf("sequenceNumber = %d, want exact 2^53+1 value", event.SequenceNumber)
	}
	marshaled, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal(event) error = %v", err)
	}
	if !bytes.Contains(marshaled, []byte("9007199254740993")) {
		t.Fatalf("marshaled event lost exact large sequence number: %s", marshaled)
	}

	manifest := decodeEvidenceManifest(t, readFixture(t, "evidence-manifest.json"))
	if manifest.Entries[1].SizeBytes != 9007199254740995 {
		t.Fatalf("sizeBytes = %d, want exact large integer", manifest.Entries[1].SizeBytes)
	}
	marshaled, err = json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal(manifest) error = %v", err)
	}
	if !bytes.Contains(marshaled, []byte("9007199254740995")) {
		t.Fatalf("marshaled manifest lost exact large size: %s", marshaled)
	}
}

func TestFixturesContainNoNullArraysOrPlaceholders(t *testing.T) {
	fixtures := []string{"run-plan.json", "observation-event.json", "scenario-result.json", "evidence-manifest.json", "behavioral-delta.json", "finding.json", "report.json"}
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			fixture := readFixture(t, name)
			if bytes.Contains(fixture, []byte(": null")) || bytes.Contains(fixture, []byte(":null")) {
				t.Fatalf("fixture contains null where arrays and optional fields should be omitted or populated: %s", name)
			}
			for _, marker := range []string{"TODO", "TBD", "REPLACE", "PLACEHOLDER", "<", ">"} {
				if strings.Contains(string(fixture), marker) {
					t.Fatalf("fixture contains unresolved placeholder marker %q", marker)
				}
			}
		})
	}
}

func TestCompatibilityPolicyDocumented(t *testing.T) {
	adr, err := os.ReadFile(filepath.Join("..", "..", "docs", "adr", "0001-core-model-schema-versioning.md"))
	if err != nil {
		t.Fatalf("read ADR: %v", err)
	}
	text := string(adr)
	for _, want := range []string{
		"adding a genuinely optional field may remain within v1alpha1",
		"removing or renaming a field is incompatible",
		"changing a field's JSON type is incompatible",
		"changing the meaning or units of a field is incompatible",
		"changing an enum wire value is incompatible",
		"incompatible changes require a new schema version",
		"compatibility fixtures must not be casually rewritten",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("ADR does not document compatibility policy phrase %q", want)
		}
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", "v1alpha1", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	if !json.Valid(data) {
		t.Fatalf("fixture %s is not valid JSON", name)
	}
	return data
}

func decodeRunPlan(t *testing.T, data []byte) RunPlan {
	t.Helper()
	var target RunPlan
	if err := json.Unmarshal(data, &target); err != nil {
		t.Fatalf("Unmarshal(RunPlan) error = %v", err)
	}
	return target
}

func decodeObservationEvent(t *testing.T, data []byte) ObservationEvent {
	t.Helper()
	var target ObservationEvent
	if err := json.Unmarshal(data, &target); err != nil {
		t.Fatalf("Unmarshal(ObservationEvent) error = %v", err)
	}
	return target
}

func decodeScenarioResult(t *testing.T, data []byte) ScenarioResult {
	t.Helper()
	var target ScenarioResult
	if err := json.Unmarshal(data, &target); err != nil {
		t.Fatalf("Unmarshal(ScenarioResult) error = %v", err)
	}
	return target
}

func decodeEvidenceManifest(t *testing.T, data []byte) EvidenceManifest {
	t.Helper()
	var target EvidenceManifest
	if err := json.Unmarshal(data, &target); err != nil {
		t.Fatalf("Unmarshal(EvidenceManifest) error = %v", err)
	}
	return target
}

func decodeBehavioralDelta(t *testing.T, data []byte) BehavioralDelta {
	t.Helper()
	var target BehavioralDelta
	if err := json.Unmarshal(data, &target); err != nil {
		t.Fatalf("Unmarshal(BehavioralDelta) error = %v", err)
	}
	return target
}

func decodeFinding(t *testing.T, data []byte) Finding {
	t.Helper()
	var target Finding
	if err := json.Unmarshal(data, &target); err != nil {
		t.Fatalf("Unmarshal(Finding) error = %v", err)
	}
	return target
}

func decodeReport(t *testing.T, data []byte) Report {
	t.Helper()
	var target Report
	if err := json.Unmarshal(data, &target); err != nil {
		t.Fatalf("Unmarshal(Report) error = %v", err)
	}
	return target
}

func assertSchemaVersionInJSON(t *testing.T, data []byte, want SchemaVersion) {
	t.Helper()
	var observed struct {
		SchemaVersion SchemaVersion `json:"schemaVersion"`
	}
	if err := json.Unmarshal(data, &observed); err != nil {
		t.Fatalf("Unmarshal emitted JSON: %v", err)
	}
	if observed.SchemaVersion != want {
		t.Fatalf("emitted schemaVersion = %q, want %q; json=%s", observed.SchemaVersion, want, data)
	}
}

func roundTripRunPlan(t *testing.T, decoded RunPlan, want SchemaVersion) {
	t.Helper()
	marshaled, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal(RunPlan) error = %v", err)
	}
	assertSchemaVersionInJSON(t, marshaled, want)
	reparsed := decodeRunPlan(t, marshaled)
	if !reflect.DeepEqual(decoded, reparsed) {
		t.Fatalf("round trip changed RunPlan:\noriginal=%#v\nreparsed=%#v", decoded, reparsed)
	}
}

func roundTripObservationEvent(t *testing.T, decoded ObservationEvent, want SchemaVersion) {
	t.Helper()
	marshaled, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal(ObservationEvent) error = %v", err)
	}
	assertSchemaVersionInJSON(t, marshaled, want)
	reparsed := decodeObservationEvent(t, marshaled)
	if !reflect.DeepEqual(decoded, reparsed) {
		t.Fatalf("round trip changed ObservationEvent:\noriginal=%#v\nreparsed=%#v", decoded, reparsed)
	}
}

func roundTripScenarioResult(t *testing.T, decoded ScenarioResult, want SchemaVersion) {
	t.Helper()
	marshaled, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal(ScenarioResult) error = %v", err)
	}
	assertSchemaVersionInJSON(t, marshaled, want)
	reparsed := decodeScenarioResult(t, marshaled)
	if !reflect.DeepEqual(decoded, reparsed) {
		t.Fatalf("round trip changed ScenarioResult:\noriginal=%#v\nreparsed=%#v", decoded, reparsed)
	}
}

func roundTripEvidenceManifest(t *testing.T, decoded EvidenceManifest, want SchemaVersion) {
	t.Helper()
	marshaled, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal(EvidenceManifest) error = %v", err)
	}
	assertSchemaVersionInJSON(t, marshaled, want)
	reparsed := decodeEvidenceManifest(t, marshaled)
	if !reflect.DeepEqual(decoded, reparsed) {
		t.Fatalf("round trip changed EvidenceManifest:\noriginal=%#v\nreparsed=%#v", decoded, reparsed)
	}
}

func roundTripBehavioralDelta(t *testing.T, decoded BehavioralDelta, want SchemaVersion) {
	t.Helper()
	marshaled, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal(BehavioralDelta) error = %v", err)
	}
	assertSchemaVersionInJSON(t, marshaled, want)
	reparsed := decodeBehavioralDelta(t, marshaled)
	if !reflect.DeepEqual(decoded, reparsed) {
		t.Fatalf("round trip changed BehavioralDelta:\noriginal=%#v\nreparsed=%#v", decoded, reparsed)
	}
}

func roundTripFinding(t *testing.T, decoded Finding, want SchemaVersion) {
	t.Helper()
	marshaled, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal(Finding) error = %v", err)
	}
	assertSchemaVersionInJSON(t, marshaled, want)
	reparsed := decodeFinding(t, marshaled)
	if !reflect.DeepEqual(decoded, reparsed) {
		t.Fatalf("round trip changed Finding:\noriginal=%#v\nreparsed=%#v", decoded, reparsed)
	}
}

func roundTripReport(t *testing.T, decoded Report, want SchemaVersion) {
	t.Helper()
	marshaled, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal(Report) error = %v", err)
	}
	assertSchemaVersionInJSON(t, marshaled, want)
	reparsed := decodeReport(t, marshaled)
	if !reflect.DeepEqual(decoded, reparsed) {
		t.Fatalf("round trip changed Report:\noriginal=%#v\nreparsed=%#v", decoded, reparsed)
	}
}
