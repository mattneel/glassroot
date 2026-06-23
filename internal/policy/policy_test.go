package policy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/compare"
	"github.com/mattneel/glassroot/internal/model"
)

func TestPolicyEvaluationFromDeterministicDelta(t *testing.T) {
	eval := mustEvaluator(t)
	delta := policyDeltaDoc(
		headRecord("delta-proc", model.DeltaKindAdded, "process-start", model.ObservationSourceSyntheticTestGenerated, model.ComparisonBasisSingleSample, processSnapshot("fact-proc", true)),
		headRecord("delta-net", model.DeltaKindAdded, "network-connection", model.ObservationSourceSyntheticTestGenerated, model.ComparisonBasisSingleSample, networkSnapshot("fact-net", "denied")),
		headRecord("delta-art", model.DeltaKindAdded, "artifact-activity", model.ObservationSourceSyntheticTestGenerated, model.ComparisonBasisSingleSample, artifactSnapshot("fact-art", true)),
		headRecord("delta-warning", model.DeltaKindAdded, "observer-warning", model.ObservationSourceSyntheticTestGenerated, model.ComparisonBasisSingleSample, warningSnapshot("fact-warning", "hostile /workspace/path must not be interpolated")),
	)

	frozen, err := eval.evaluateDeltaDocument(context.Background(), PolicyProfileStrict(), delta, []byte(`{"delta":"owned"}`), model.Digest("sha256:"+strings.Repeat("9", 64)))
	if err != nil {
		t.Fatalf("evaluateDeltaDocument() error = %v", err)
	}
	doc := frozen.Document()
	if doc.SchemaVersion != SchemaVersionPolicyEvaluationV1Alpha1 {
		t.Fatalf("schemaVersion = %q", doc.SchemaVersion)
	}
	if doc.PolicyProfileName != PolicyProfileNameStrict || doc.PolicyProfileVersion != PolicyProfileVersionStrictV1Alpha1 || doc.BuiltinRuleSetVersion != BuiltinRuleSetVersionStrictV1Alpha1 {
		t.Fatalf("policy identity not retained: %+v", doc)
	}
	if doc.OverallDisposition != model.DispositionRequiresReview {
		t.Fatalf("overall disposition = %q", doc.OverallDisposition)
	}
	wantRules := map[string]bool{"GR-OBS-001": false, "GR-PROC-001": false, "GR-NET-001": false, "GR-ART-001": false, "GR-FS-001": false}
	for _, f := range doc.Findings {
		if _, ok := wantRules[f.RuleID]; ok {
			wantRules[f.RuleID] = true
		}
		if f.Waived || f.Disposition == model.DispositionWaived {
			t.Fatalf("GR-10A finding was waived: %+v", f)
		}
		if strings.Contains(f.Title+f.Summary, "hostile") || strings.Contains(f.Title+f.Summary, "/workspace") || strings.Contains(f.Title+f.Summary, "evil.example.invalid") {
			t.Fatalf("finding interpolated hostile data: %+v", f)
		}
		if f.RuleID != "GR-OBS-001" && f.Confidence != model.ConfidenceLow {
			t.Fatalf("synthetic target behavior confidence = %q in %+v", f.Confidence, f)
		}
	}
	for rule, saw := range wantRules {
		if !saw {
			t.Fatalf("missing rule %s in findings: %+v", rule, doc.Findings)
		}
	}
	if doc.Summary.TotalFindings != int64(len(doc.Findings)) || doc.Summary.Waived != 0 || doc.Summary.RequiresReview == 0 {
		t.Fatalf("summary mismatch: %+v findings=%d", doc.Summary, len(doc.Findings))
	}
	if len(frozen.JSON()) == 0 || !strings.HasPrefix(string(frozen.Digest()), "sha256:") {
		t.Fatalf("frozen evaluation missing JSON/digest")
	}
}

func TestIncompleteEvidenceProducesSingleFailedObservationFinding(t *testing.T) {
	eval := mustEvaluator(t)
	delta := policyDeltaDoc()
	delta.ExecutionComplete = false
	delta.EvidenceComplete = false
	frozen, err := eval.evaluateDeltaDocument(context.Background(), PolicyProfileStrict(), delta, []byte(`{}`), model.Digest("sha256:"+strings.Repeat("8", 64)))
	if err != nil {
		t.Fatalf("evaluateDeltaDocument() error = %v", err)
	}
	doc := frozen.Document()
	if doc.OverallDisposition != model.DispositionFailed {
		t.Fatalf("overall = %q", doc.OverallDisposition)
	}
	var obsFailed int
	for _, f := range doc.Findings {
		if f.RuleID == "GR-OBS-001" && f.Disposition == model.DispositionFailed {
			obsFailed++
			if f.Severity != model.SeverityHigh || f.Confidence != model.ConfidenceHigh {
				t.Fatalf("incomplete evidence finding severity/confidence = %s/%s", f.Severity, f.Confidence)
			}
		}
	}
	if obsFailed != 1 {
		t.Fatalf("failed observation findings=%d, want 1: %+v", obsFailed, doc.Findings)
	}
}

func TestRuleDirectionExclusionsAndCoverageLimitedPositiveObservation(t *testing.T) {
	eval := mustEvaluator(t)
	removedProc := baseRecord("delta-proc-removed", model.DeltaKindRemoved, "process-start", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, processSnapshot("fact-proc-base", true))
	countDecrease := twoSideRecord("delta-net-count-decrease", model.DeltaKindCountChanged, "network-connection", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, networkSnapshot("fact-net-base", "allowed"), networkSnapshot("fact-net-head", "allowed"))
	countDecrease.BaseOccurrence = occurrence(2, 2, model.CoverageAssessmentComplete, model.RepeatabilityStable)
	countDecrease.HeadOccurrence = occurrence(2, 1, model.CoverageAssessmentComplete, model.RepeatabilityStable)
	coverageLimitedHead := headRecord("delta-net-limited", model.DeltaKindCoverageChanged, "network-connection", model.ObservationSourceWorkloadReported, model.ComparisonBasisCoverageLimited, networkSnapshot("fact-net-limited", "denied"))
	delta := policyDeltaDoc(removedProc, countDecrease, coverageLimitedHead)
	frozen, err := eval.evaluateDeltaDocument(context.Background(), PolicyProfileStrict(), delta, []byte(`{}`), model.Digest("sha256:"+strings.Repeat("7", 64)))
	if err != nil {
		t.Fatalf("evaluateDeltaDocument() error = %v", err)
	}
	doc := frozen.Document()
	if hasFinding(doc, "GR-PROC-001", "delta-proc-removed") {
		t.Fatalf("process removal triggered GR-PROC-001: %+v", doc.Findings)
	}
	if hasFinding(doc, "GR-NET-001", "delta-net-count-decrease") {
		t.Fatalf("network count decrease triggered GR-NET-001: %+v", doc.Findings)
	}
	limited := findingForRecord(doc, "GR-NET-001", "delta-net-limited")
	if limited == nil {
		t.Fatalf("coverage-limited positive network observation did not trigger: %+v", doc.Findings)
	}
	if limited.Confidence != model.ConfidenceLow || !limited.HeadObserved || limited.BaseObserved {
		t.Fatalf("coverage-limited finding flags/confidence wrong: %+v", *limited)
	}
}

func TestFilesystemDeterminismAndResourceRules(t *testing.T) {
	eval := mustEvaluator(t)
	fsRead := headRecord("delta-fs-read", model.DeltaKindAdded, "filesystem-read", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, filesystemSnapshot("fact-fs-read", "read", "absolute-unmapped", false))
	fsWrite := headRecord("delta-fs-write", model.DeltaKindAdded, "filesystem-write", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, filesystemSnapshot("fact-fs-write", "write", "absolute-unmapped", false))
	fsMapped := headRecord("delta-fs-mapped", model.DeltaKindAdded, "filesystem-write", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, filesystemSnapshot("fact-fs-mapped", "write", "workdir-root", false))
	resource := headRecord("delta-resource", model.DeltaKindAdded, "resource-limit", model.ObservationSourceSandboxRuntimeObserved, model.ComparisonBasisCompleteObservation, resourceSnapshot("fact-resource"))
	stability := twoSideRecord("delta-stability", model.DeltaKindStabilityChanged, "filesystem-write", model.ObservationSourceHostObserved, model.ComparisonBasisRepetitionVariable, filesystemSnapshot("fact-stable-base", "write", "workdir-root", false), filesystemSnapshot("fact-variable-head", "write", "workdir-root", false))
	stability.BaseOccurrence = occurrence(2, 2, model.CoverageAssessmentComplete, model.RepeatabilityStable)
	stability.HeadOccurrence = occurrence(2, 3, model.CoverageAssessmentComplete, model.RepeatabilityVariable)

	frozen, err := eval.evaluateDeltaDocument(context.Background(), PolicyProfileStrict(), policyDeltaDoc(fsRead, fsWrite, fsMapped, resource, stability), []byte(`{}`), model.Digest("sha256:"+strings.Repeat("6", 64)))
	if err != nil {
		t.Fatalf("evaluateDeltaDocument() error = %v", err)
	}
	doc := frozen.Document()
	readFinding := findingForRecord(doc, "GR-FS-002", "delta-fs-read")
	if readFinding == nil || readFinding.Severity != model.SeverityMedium {
		t.Fatalf("absolute-unmapped read should trigger medium GR-FS-002: %+v", doc.Findings)
	}
	writeFinding := findingForRecord(doc, "GR-FS-002", "delta-fs-write")
	if writeFinding == nil || writeFinding.Severity != model.SeverityHigh {
		t.Fatalf("absolute-unmapped write should trigger high GR-FS-002: %+v", doc.Findings)
	}
	if hasFinding(doc, "GR-FS-002", "delta-fs-mapped") {
		t.Fatalf("mapped workdir path triggered outside-root rule: %+v", doc.Findings)
	}
	if findingForRecord(doc, "GR-LIMIT-001", "delta-resource") == nil {
		t.Fatalf("resource-limit behavior did not trigger GR-LIMIT-001: %+v", doc.Findings)
	}
	if findingForRecord(doc, "GR-DET-001", "delta-stability") == nil {
		t.Fatalf("degraded repeatability did not trigger GR-DET-001: %+v", doc.Findings)
	}
}

func TestSeverityConfidenceDispositionAreIndependent(t *testing.T) {
	cases := []struct {
		basis model.ComparisonBasis
		src   model.ObservationSource
		want  model.Confidence
	}{
		{model.ComparisonBasisCompleteObservation, model.ObservationSourceHostObserved, model.ConfidenceHigh},
		{model.ComparisonBasisSingleSample, model.ObservationSourceHostObserved, model.ConfidenceMedium},
		{model.ComparisonBasisRepetitionVariable, model.ObservationSourceGuestAgentReported, model.ConfidenceMedium},
		{model.ComparisonBasisCompleteObservation, model.ObservationSourceWorkloadReported, model.ConfidenceMedium},
		{model.ComparisonBasisCompleteObservation, model.ObservationSourceModelInferred, model.ConfidenceLow},
		{model.ComparisonBasisCompleteObservation, model.ObservationSourceSyntheticTestGenerated, model.ConfidenceLow},
	}
	for _, tt := range cases {
		if got := classifyPolicyConfidence(tt.basis, tt.src, true); got != tt.want {
			t.Fatalf("confidence(%s,%s)= %s want %s", tt.basis, tt.src, got, tt.want)
		}
	}
	if got := classifyEvidenceStateConfidence(); got != model.ConfidenceHigh {
		t.Fatalf("evidence-state confidence = %s", got)
	}
}

func TestFindingIDDeterministicAndExcludesRunAndEvidenceProvenance(t *testing.T) {
	eval := mustEvaluator(t)
	rec := headRecord("delta-net", model.DeltaKindAdded, "network-connection", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, networkSnapshot("fact-net", "denied"))
	d1 := policyDeltaDoc(rec)
	d2 := policyDeltaDoc(rec)
	d2.RunID = "run-9999"
	d2.Records[0].HeadEvidence[0].EventIDs = []string{"different-event"}
	d2.Records[0].Evidence = d2.Records[0].HeadEvidence
	f1, err := eval.evaluateDeltaDocument(context.Background(), PolicyProfileStrict(), d1, []byte(`{}`), model.Digest("sha256:"+strings.Repeat("1", 64)))
	if err != nil {
		t.Fatalf("first evaluation error = %v", err)
	}
	f2, err := eval.evaluateDeltaDocument(context.Background(), PolicyProfileStrict(), d2, []byte(`{}`), model.Digest("sha256:"+strings.Repeat("2", 64)))
	if err != nil {
		t.Fatalf("second evaluation error = %v", err)
	}
	id1 := findingForRule(f1.Document(), "GR-NET-001").ID
	id2 := findingForRule(f2.Document(), "GR-NET-001").ID
	if id1 != id2 || !strings.HasPrefix(id1, "finding-") {
		t.Fatalf("finding IDs should be provenance-independent: %s vs %s", id1, id2)
	}
	changed := policyDeltaDoc(headRecord("delta-art", model.DeltaKindAdded, "artifact-activity", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, artifactSnapshot("fact-art", false)))
	f3, err := eval.evaluateDeltaDocument(context.Background(), PolicyProfileStrict(), changed, []byte(`{}`), model.Digest("sha256:"+strings.Repeat("3", 64)))
	if err != nil {
		t.Fatalf("third evaluation error = %v", err)
	}
	if findingForRule(f3.Document(), "GR-ART-001").ID == id1 {
		t.Fatalf("different semantic policy condition reused finding ID")
	}
}

func TestFrozenEvaluationOwnershipAndValidation(t *testing.T) {
	eval := mustEvaluator(t)
	_, err := eval.Evaluate(context.Background(), EvaluationRequest{Profile: PolicyProfileStrict()})
	assertPolicyError(t, err, CodeNilDelta)

	delta := policyDeltaDoc(headRecord("delta-net", model.DeltaKindAdded, "network-connection", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, networkSnapshot("fact-net", "denied")))
	frozen, err := eval.evaluateDeltaDocument(context.Background(), PolicyProfileStrict(), delta, []byte(`{}`), model.Digest("sha256:"+strings.Repeat("4", 64)))
	if err != nil {
		t.Fatalf("evaluateDeltaDocument() error = %v", err)
	}
	doc := frozen.Document()
	js := frozen.JSON()
	doc.Findings = nil
	js[0] = '{'
	if len(frozen.Document().Findings) == 0 || !bytes.Equal(frozen.JSON(), frozen.JSON()) {
		t.Fatal("FrozenEvaluation exposed mutable internals")
	}
	if frozen.Digest() == "" || len(frozen.JSON()) == 0 {
		t.Fatal("FrozenEvaluation missing immutable digest/json")
	}
}

func FuzzClassifyPolicyConfidence(f *testing.F) {
	f.Add(string(model.ComparisonBasisCompleteObservation), string(model.ObservationSourceHostObserved))
	f.Add(string(model.ComparisonBasisCoverageLimited), string(model.ObservationSourceSyntheticTestGenerated))
	f.Fuzz(func(t *testing.T, basis, source string) {
		_ = classifyPolicyConfidence(model.ComparisonBasis(basis), model.ObservationSource(source), true)
	})
}

func FuzzEncodeFindingID(f *testing.F) {
	f.Add("GR-NET-001", "v1", "delta-1", "scenario")
	f.Add("\x1b", "\x00", "", "")
	f.Fuzz(func(t *testing.T, rule, version, record, scenario string) {
		_, _ = findingID(PolicyProfileVersionStrictV1Alpha1, BuiltinRuleSetVersionStrictV1Alpha1, rule, version, []string{record}, "scope", []string{scenario})
	})
}

func FuzzEvaluateDeltaRecord(f *testing.F) {
	f.Add(string(model.DeltaKindAdded), "network-connection", string(model.ObservationSourceHostObserved), string(model.ComparisonBasisSingleSample))
	f.Add("future", "\x00", "\x1b", "bad")
	f.Fuzz(func(t *testing.T, kind, factKind, source, basis string) {
		eval := mustEvaluator(t)
		delta := policyDeltaDoc(headRecord("delta-fuzz", model.DeltaKind(kind), factKind, model.ObservationSource(source), model.ComparisonBasis(basis), networkSnapshot("fact-fuzz", "denied")))
		_, _ = eval.evaluateDeltaDocument(context.Background(), PolicyProfileStrict(), delta, []byte(`{}`), model.Digest("sha256:"+strings.Repeat("5", 64)))
	})
}

func mustEvaluator(t *testing.T) *Evaluator {
	t.Helper()
	e, err := New(DefaultLimits())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return e
}

func policyDeltaDoc(records ...model.DeltaRecord) model.BehavioralDelta {
	for i := range records {
		if records[i].ScenarioIDs == nil {
			records[i].ScenarioIDs = []string{"test"}
		}
		if records[i].Evidence == nil {
			records[i].Evidence = append(append([]model.EvidenceRef{}, records[i].BaseEvidence...), records[i].HeadEvidence...)
		}
		if records[i].Limitations == nil {
			records[i].Limitations = []model.Limitation{}
		}
	}
	return model.BehavioralDelta{
		SchemaVersion:               model.SchemaVersionBehavioralDeltaV1Alpha1,
		ID:                          "delta-test",
		RunID:                       "run-0001",
		PlanDigest:                  model.Digest("sha256:" + strings.Repeat("a", 64)),
		ManifestDigest:              model.Digest("sha256:" + strings.Repeat("b", 64)),
		ManifestVerificationMode:    "expected-manifest-digest",
		ExecutionComplete:           true,
		EvidenceComplete:            true,
		ComparisonProfile:           model.ComparisonProfile{Version: compare.ComparisonProfileVersionV1Alpha1, RequiredNormalizationProfile: "glassroot.dev/normalization-profile/v1alpha1", IncludedFactKinds: []string{"artifact-activity", "dns-query", "filesystem-chmod", "filesystem-create", "filesystem-delete", "filesystem-read", "filesystem-rename", "filesystem-write", "network-connection", "observer-warning", "process-exit", "process-start", "resource-limit", "scenario-completed", "scenario-started", "unsupported-observation"}},
		NormalizationProfileVersion: "glassroot.dev/normalization-profile/v1alpha1",
		ScenarioIDs:                 []string{"test"},
		ScenarioComparisons:         []model.ScenarioComparison{{ScenarioID: "test", BaseRepetitions: []model.AttemptCoverage{}, HeadRepetitions: []model.AttemptCoverage{}, Coverage: model.CoverageAssessmentComplete, RepeatabilityNotes: []model.Limitation{}, Limitations: []model.Limitation{}}},
		Records:                     records,
		Summary:                     summarizeDeltaRecords(records),
		Limitations:                 []model.Limitation{},
	}
}

func summarizeDeltaRecords(records []model.DeltaRecord) model.DeltaSummary {
	s := model.DeltaSummary{TotalRecords: int64(len(records))}
	for _, r := range records {
		switch r.Kind {
		case model.DeltaKindAdded:
			s.Added++
		case model.DeltaKindRemoved:
			s.Removed++
		case model.DeltaKindModified:
			s.Modified++
		case model.DeltaKindCountChanged:
			s.CountChanged++
		case model.DeltaKindOrderChanged:
			s.OrderChanged++
		case model.DeltaKindStabilityChanged:
			s.StabilityChanged++
		case model.DeltaKindCoverageChanged:
			s.CoverageChanged++
		}
	}
	return s
}

func headRecord(id string, kind model.DeltaKind, factKind string, source model.ObservationSource, basis model.ComparisonBasis, fact model.DeltaFactSnapshot) model.DeltaRecord {
	rec := model.DeltaRecord{ID: id, Kind: kind, FactKind: factKind, Source: source, Basis: basis, ScenarioIDs: []string{"test"}, HeadObserved: true, HeadFacts: []model.DeltaFactSnapshot{fact}, HeadEvidence: evidence("head", 2), HeadSemanticDigests: []model.Digest{fact.SemanticDigest}, BaseOccurrence: emptyOccurrence(), HeadOccurrence: occurrence(1, 1, model.CoverageAssessmentComplete, model.RepeatabilitySingleSample), Evidence: evidence("head", 2), ChangedFields: []string{}, Limitations: []model.Limitation{}}
	return rec
}

func baseRecord(id string, kind model.DeltaKind, factKind string, source model.ObservationSource, basis model.ComparisonBasis, fact model.DeltaFactSnapshot) model.DeltaRecord {
	return model.DeltaRecord{ID: id, Kind: kind, FactKind: factKind, Source: source, Basis: basis, ScenarioIDs: []string{"test"}, BaseObserved: true, BaseFacts: []model.DeltaFactSnapshot{fact}, BaseEvidence: evidence("base", 1), BaseSemanticDigests: []model.Digest{fact.SemanticDigest}, BaseOccurrence: occurrence(1, 1, model.CoverageAssessmentComplete, model.RepeatabilitySingleSample), HeadOccurrence: emptyOccurrence(), Evidence: evidence("base", 1), ChangedFields: []string{}, Limitations: []model.Limitation{}}
}

func twoSideRecord(id string, kind model.DeltaKind, factKind string, source model.ObservationSource, basis model.ComparisonBasis, base, head model.DeltaFactSnapshot) model.DeltaRecord {
	return model.DeltaRecord{ID: id, Kind: kind, FactKind: factKind, Source: source, Basis: basis, ScenarioIDs: []string{"test"}, BaseObserved: true, HeadObserved: true, BaseFacts: []model.DeltaFactSnapshot{base}, HeadFacts: []model.DeltaFactSnapshot{head}, BaseEvidence: evidence("base", 1), HeadEvidence: evidence("head", 2), BaseSemanticDigests: []model.Digest{base.SemanticDigest}, HeadSemanticDigests: []model.Digest{head.SemanticDigest}, BaseOccurrence: occurrence(1, 1, model.CoverageAssessmentComplete, model.RepeatabilitySingleSample), HeadOccurrence: occurrence(1, 1, model.CoverageAssessmentComplete, model.RepeatabilitySingleSample), Evidence: append(evidence("base", 1), evidence("head", 2)...), ChangedFields: []string{"network.result"}, Limitations: []model.Limitation{}}
}

func processSnapshot(id string, start bool) model.DeltaFactSnapshot {
	kind := "process-start"
	op := "start"
	if !start {
		kind = "process-exit"
		op = "exit"
	}
	return snapshotBase(id, kind, &model.DeltaProcessFact{Operation: op, StableID: "proc-" + strings.Repeat("1", 64), ParentRelation: "root", Executable: model.DeltaNormalizedPath{Namespace: "workdir-root", Literal: "/workspace/bin/tool", Relative: "bin/tool", Display: "@workdir/bin/tool"}, Arguments: []string{}, Environment: []model.EnvEntry{}, DurationMillis: 1})
}

func networkSnapshot(id, result string) model.DeltaFactSnapshot {
	return snapshotBase(id, "network-connection", &model.DeltaNetworkFact{Operation: "connect", Protocol: "tcp", DestinationHost: "evil.example.invalid", DestinationPort: 443, Result: result, ResolvedAddresses: []string{}})
}

func artifactSnapshot(id string, executable bool) model.DeltaFactSnapshot {
	return snapshotBase(id, "artifact-activity", &model.DeltaArtifactFact{Operation: "create", ArtifactID: "artifact-1", Path: model.DeltaNormalizedPath{Namespace: "workdir-root", Literal: "/workspace/out/bin", Relative: "out/bin", Display: "@workdir/out/bin"}, Digest: model.Digest("sha256:" + strings.Repeat("c", 64)), SizeBytes: 7, Executable: executable, SourceEventIDs: []string{}})
}

func filesystemSnapshot(id, op, namespace string, executable bool) model.DeltaFactSnapshot {
	literal := "/workspace/out"
	relative := "out"
	display := "@workdir/out"
	if namespace == "absolute-unmapped" {
		literal = "/etc/passwd"
		relative = ""
		display = "/etc/passwd"
	}
	return snapshotBase(id, "filesystem-"+op, &model.DeltaFilesystemFact{Operation: op, Path: model.DeltaNormalizedPath{Namespace: namespace, Literal: literal, Relative: relative, Display: display}, Digest: model.Digest("sha256:" + strings.Repeat("d", 64)), SizeBytes: 1, Executable: executable})
}

func resourceSnapshot(id string) model.DeltaFactSnapshot {
	return snapshotBase(id, "resource-limit", &model.DeltaResourceFact{LimitKind: "cpu", LimitValue: 100, Unit: "millis", ObservedValue: 150, Exceeded: true})
}

func warningSnapshot(id, msg string) model.DeltaFactSnapshot {
	return snapshotBase(id, "observer-warning", &model.DeltaWarningFact{Code: "warning-code", Message: msg, Unsupported: false, Limitations: []model.Limitation{}})
}

func snapshotBase(id, kind string, payload any) model.DeltaFactSnapshot {
	s := model.DeltaFactSnapshot{ID: id, SemanticDigest: model.Digest("sha256:" + digestNibble(id)), Kind: kind, Source: model.ObservationSourceSyntheticTestGenerated, Evidence: evidence("head", 2), Limitations: []model.Limitation{}}
	switch p := payload.(type) {
	case *model.DeltaProcessFact:
		s.Process = p
	case *model.DeltaNetworkFact:
		s.Network = p
	case *model.DeltaArtifactFact:
		s.Artifact = p
	case *model.DeltaFilesystemFact:
		s.Filesystem = p
	case *model.DeltaWarningFact:
		s.Warning = p
	case *model.DeltaResourceFact:
		s.Resource = p
	}
	return s
}

func evidence(rev string, seq uint64) []model.EvidenceRef {
	path := "attempts/" + rev + "/test/repetition-0001/events.jsonl"
	return []model.EvidenceRef{{Digest: model.Digest("sha256:" + strings.Repeat("e", 64)), EventStreamDigest: model.Digest("sha256:" + strings.Repeat("e", 64)), EventStreamPath: path, BundlePath: &path, EventIDs: []string{"evt-" + rev}, EventSequence: seq, Revision: model.RevisionKind(rev), ScenarioID: "test", Repetition: 1}}
}

func occurrence(reps, total int64, cov model.CoverageAssessment, rep model.RepeatabilityAssessment) model.OccurrenceProfile {
	return model.OccurrenceProfile{PlannedRepetitionCount: reps, Repetitions: []model.RepetitionOccurrence{{Repetition: 1, Coverage: cov, CountKnown: cov == model.CoverageAssessmentComplete, Count: total}}, CompleteRepetitionCount: reps, MinimumKnownCount: total, MaximumKnownCount: total, TotalKnownCount: total, Coverage: cov, Repeatability: rep}
}

func emptyOccurrence() model.OccurrenceProfile {
	return model.OccurrenceProfile{Repetitions: []model.RepetitionOccurrence{}, Coverage: model.CoverageAssessmentNone, Repeatability: model.RepeatabilityNotAssessable}
}

func digestNibble(seed string) string {
	n := byte('0' + byte(len(seed)%10))
	return strings.Repeat(string(n), 64)
}

func hasFinding(doc EvaluationDocument, rule, record string) bool {
	return findingForRecord(doc, rule, record) != nil
}

func findingForRecord(doc EvaluationDocument, rule, record string) *model.Finding {
	for i := range doc.Findings {
		if doc.Findings[i].RuleID != rule {
			continue
		}
		for _, id := range doc.Findings[i].DeltaRecordIDs {
			if id == record {
				return &doc.Findings[i]
			}
		}
	}
	return nil
}

func findingForRule(doc EvaluationDocument, rule string) model.Finding {
	for _, f := range doc.Findings {
		if f.RuleID == rule {
			return f
		}
	}
	panic("missing finding " + rule)
}

func assertPolicyError(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected policy error %s, got nil", code)
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error %T is not *policy.Error: %v", err, err)
	}
	if e.Code != code {
		t.Fatalf("error code=%s want=%s err=%v", e.Code, code, err)
	}
	if strings.ContainsAny(err.Error(), "\x1b\r\n") {
		t.Fatalf("error contains raw controls: %q", err.Error())
	}
}

func TestPolicyRuleCatalogInventory(t *testing.T) {
	want := []string{"GR-ART-001", "GR-DET-001", "GR-FS-001", "GR-FS-002", "GR-LIMIT-001", "GR-NET-001", "GR-OBS-001", "GR-PROC-001"}
	got := emittedRuleIDs()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("emitted rule inventory drift\nwant=%v\n got=%v", want, got)
	}
	for _, reserved := range []string{"GR-CONFIG-001", "GR-WAIVER-001"} {
		if ruleByID(reserved).Emit {
			t.Fatalf("reserved rule %s is emitted in GR-10A", reserved)
		}
	}
}

func readPolicyTestFile(t *testing.T, parts ...string) []byte {
	t.Helper()
	path := filepath.Join(parts...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func writePolicyTestFile(t *testing.T, data []byte, parts ...string) {
	t.Helper()
	path := filepath.Join(parts...)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestGoldenPolicyEvaluationFromComparatorGolden(t *testing.T) {
	deltaBytes := readPolicyTestFile(t, "..", "compare", "testdata", "v1alpha1", "behavioral-delta.json")
	var delta model.BehavioralDelta
	if err := json.Unmarshal(deltaBytes, &delta); err != nil {
		t.Fatalf("unmarshal comparator golden delta: %v", err)
	}
	deltaDigest := model.Digest(strings.TrimSpace(string(readPolicyTestFile(t, "..", "compare", "testdata", "v1alpha1", "behavioral-delta.digest"))))
	eval := mustEvaluator(t)
	frozen, err := eval.evaluateDeltaDocument(context.Background(), PolicyProfileStrict(), delta, deltaBytes, deltaDigest)
	if err != nil {
		t.Fatalf("evaluate comparator golden delta: %v", err)
	}
	got := frozen.JSON()
	if os.Getenv("UPDATE_POLICY_GOLDEN") == "1" {
		writePolicyTestFile(t, got, "testdata", "v1alpha1", "policy-evaluation.json")
		writePolicyTestFile(t, []byte(frozen.Digest()), "testdata", "v1alpha1", "policy-evaluation.digest")
	}
	want := readPolicyTestFile(t, "testdata", "v1alpha1", "policy-evaluation.json")
	if !bytes.Equal(got, want) {
		t.Fatalf("policy evaluation golden mismatch\nwant=%s\n got=%s", want, got)
	}
	wantDigest := strings.TrimSpace(string(readPolicyTestFile(t, "testdata", "v1alpha1", "policy-evaluation.digest")))
	if string(frozen.Digest()) != wantDigest {
		t.Fatalf("evaluation digest = %s, want %s", frozen.Digest(), wantDigest)
	}
}
