package compare

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/evidence"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/observe"
	"github.com/mattneel/glassroot/internal/pipeline"
	"github.com/mattneel/glassroot/internal/runner"
	"github.com/mattneel/glassroot/internal/runner/fake"
)

const (
	compareTestBaseCommit = "1111111111111111111111111111111111111111"
	compareTestHeadCommit = "2222222222222222222222222222222222222222"
	compareTestBaseTree   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	compareTestHeadTree   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

var compareTestCreatedAt = time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)

func TestCompareVerifiedTraceProducesDeterministicBehavioralDelta(t *testing.T) {
	trace, cleanup := mustCompareTrace(t)
	defer cleanup()

	cmp := mustComparator(t, DefaultLimits())
	delta, err := cmp.Compare(context.Background(), trace)
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}
	doc := delta.Document()
	if doc.SchemaVersion != model.SchemaVersionBehavioralDeltaV1Alpha1 {
		t.Fatalf("schemaVersion = %q", doc.SchemaVersion)
	}
	if doc.ComparisonProfile.Version != ComparisonProfileVersionV1Alpha1 {
		t.Fatalf("comparison profile not retained: %+v", doc.ComparisonProfile)
	}
	if doc.NormalizationProfileVersion != observe.ProfileVersionV1Alpha1 {
		t.Fatalf("normalization profile = %q", doc.NormalizationProfileVersion)
	}
	if doc.PlanDigest == "" || doc.ManifestDigest == "" || doc.ManifestVerificationMode != string(evidence.VerificationModeExpectedManifestDigest) {
		t.Fatalf("source binding missing: %+v", doc)
	}
	if !doc.EvidenceContext.SyntheticEvidence || doc.EvidenceContext.ExecutesTargetCode {
		t.Fatalf("evidence context not retained from normalized input: %+v", doc.EvidenceContext)
	}
	if len(doc.ScenarioComparisons) != 2 {
		t.Fatalf("scenario comparisons=%d, want 2", len(doc.ScenarioComparisons))
	}
	if len(doc.Records) == 0 {
		t.Fatal("expected behavioral records")
	}
	kinds := map[model.DeltaKind]bool{}
	for _, rec := range doc.Records {
		kinds[rec.Kind] = true
		if rec.ID == "" || !strings.HasPrefix(rec.ID, "delta-") {
			t.Fatalf("record missing deterministic id: %+v", rec)
		}
		if rec.Basis == "" {
			t.Fatalf("record missing comparison basis: %+v", rec)
		}
		if rec.Kind == model.DeltaKindAdded && len(rec.HeadEvidence) == 0 {
			t.Fatalf("added record missing head evidence: %+v", rec)
		}
		for _, snap := range append(rec.BaseFacts, rec.HeadFacts...) {
			if snap.Process != nil && strings.Contains(normalizedSnapshotJSON(t, snap), "processId") {
				t.Fatalf("snapshot leaked raw PID: %+v", snap)
			}
		}
	}
	for _, want := range []model.DeltaKind{model.DeltaKindAdded, model.DeltaKindRemoved, model.DeltaKindModified} {
		if !kinds[want] {
			t.Fatalf("missing delta kind %s in %v", want, kinds)
		}
	}
	if doc.Summary.TotalRecords != int64(len(doc.Records)) {
		t.Fatalf("summary mismatch: %+v records=%d", doc.Summary, len(doc.Records))
	}
	if len(delta.JSON()) == 0 || !strings.HasPrefix(string(delta.Digest()), "sha256:") {
		t.Fatalf("frozen delta missing JSON/digest")
	}
	again, err := cmp.Compare(context.Background(), trace)
	if err != nil {
		t.Fatalf("repeat Compare() error = %v", err)
	}
	if !bytes.Equal(delta.JSON(), again.JSON()) || delta.Digest() != again.Digest() {
		t.Fatalf("comparison not deterministic\nfirst=%s\nsecond=%s", delta.JSON(), again.JSON())
	}
}

func TestComparisonInputValidationAndOwnership(t *testing.T) {
	cmp := mustComparator(t, DefaultLimits())
	_, err := cmp.Compare(context.Background(), nil)
	assertCompareError(t, err, CodeNilTraceSet)

	duplicateCoord := traceDocumentForUnitTests([]observe.AttemptTrace{
		attemptTrace("att-base-dup-a", model.RevisionKindBase, "dup", 1, observe.CoverageComplete),
		attemptTrace("att-base-dup-b", model.RevisionKindBase, "dup", 1, observe.CoverageComplete),
		attemptTrace("att-head-dup-a", model.RevisionKindHead, "dup", 1, observe.CoverageComplete),
	})
	_, err = cmp.compareDocument(context.Background(), duplicateCoord)
	assertCompareError(t, err, CodeDuplicateAttempt)

	trace, cleanup := mustCompareTrace(t)
	defer cleanup()
	delta, err := cmp.Compare(context.Background(), trace)
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}
	doc := delta.Document()
	jsonBytes := delta.JSON()
	doc.Records = nil
	jsonBytes[0] = '{'
	if len(delta.Document().Records) == 0 || !json.Valid(delta.JSON()) {
		t.Fatal("FrozenDelta returned mutable internal data")
	}
}

func TestOccurrenceProfilesAbsenceRulesAndModificationCorrelation(t *testing.T) {
	base := []observe.AttemptTrace{
		attemptTrace("att-base-unit-r1", model.RevisionKindBase, "unit", 1, observe.CoverageComplete, factFS("base-a", model.RevisionKindBase, "unit", 1, "write", "@workdir/out.txt", "sha256:"+strings.Repeat("a", 64), 1)),
		attemptTrace("att-base-unit-r2", model.RevisionKindBase, "unit", 2, observe.CoverageComplete, factFS("base-b", model.RevisionKindBase, "unit", 2, "write", "@workdir/out.txt", "sha256:"+strings.Repeat("a", 64), 1)),
	}
	head := []observe.AttemptTrace{
		attemptTrace("att-head-unit-r1", model.RevisionKindHead, "unit", 1, observe.CoverageComplete, factFS("head-a", model.RevisionKindHead, "unit", 1, "write", "@workdir/out.txt", "sha256:"+strings.Repeat("b", 64), 1), factNetwork("head-net", 1)),
		attemptTrace("att-head-unit-r2", model.RevisionKindHead, "unit", 2, observe.CoverageIncomplete, factNetwork("head-net-2", 2)),
	}
	doc := traceDocumentForUnitTests(append(base, head...))
	cmp := mustComparator(t, DefaultLimits())
	delta, err := cmp.compareDocument(context.Background(), doc)
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}
	var sawModified, sawCoverageLimited bool
	for _, rec := range delta.Document().Records {
		if rec.Kind == model.DeltaKindModified && rec.FactKind == string(observe.FactKindFilesystemWrite) {
			sawModified = true
			if !reflect.DeepEqual(rec.ChangedFields, []string{"filesystem.digest"}) {
				t.Fatalf("changed fields = %v", rec.ChangedFields)
			}
			if rec.BaseOccurrence.Repeatability != model.RepeatabilityStable || rec.HeadOccurrence.Repeatability != model.RepeatabilityNotAssessable {
				t.Fatalf("occurrence profiles not retained: base=%+v head=%+v", rec.BaseOccurrence, rec.HeadOccurrence)
			}
		}
		if rec.Basis == model.ComparisonBasisCoverageLimited && rec.HeadObserved && rec.FactKind == string(observe.FactKindNetworkConnection) {
			sawCoverageLimited = true
			if len(rec.HeadEvidence) == 0 {
				t.Fatalf("coverage-limited positive observation lost evidence: %+v", rec)
			}
		}
	}
	if !sawModified || !sawCoverageLimited {
		t.Fatalf("expected modified and coverage-limited records; got %+v", delta.Document().Records)
	}
}

func TestComparisonCarriesEvidenceContextWithoutDeltaRecords(t *testing.T) {
	doc := traceDocumentForUnitTests([]observe.AttemptTrace{
		attemptTrace("att-base-unit-r1", model.RevisionKindBase, "unit", 1, observe.CoverageComplete),
		attemptTrace("att-head-unit-r1", model.RevisionKindHead, "unit", 1, observe.CoverageComplete),
	})
	doc.EvidenceComplete = true
	doc.EvidenceContext = model.EvidenceContext{SyntheticEvidence: true, ExecutesTargetCode: false}
	cmp := mustComparator(t, DefaultLimits())
	delta, err := cmp.compareDocument(context.Background(), doc)
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}
	got := delta.Document()
	if len(got.Records) != 0 {
		t.Fatalf("expected zero ordinary delta records, got %+v", got.Records)
	}
	if !got.EvidenceContext.SyntheticEvidence || got.EvidenceContext.ExecutesTargetCode {
		t.Fatalf("zero-record delta lost evidence context: %+v", got.EvidenceContext)
	}
	if !bytes.Contains(delta.JSON(), []byte(`"evidenceContext":{"syntheticEvidence":true,"executesTargetCode":false}`)) {
		t.Fatalf("evidence context missing from frozen delta JSON: %s", delta.JSON())
	}
}

func TestCountStabilityAndStrictOrderRecords(t *testing.T) {
	cmp := mustComparator(t, DefaultLimits())
	one := "sha256:" + strings.Repeat("1", 64)
	two := "sha256:" + strings.Repeat("2", 64)

	countDoc := traceDocumentForUnitTests([]observe.AttemptTrace{
		attemptTrace("att-base-count-r1", model.RevisionKindBase, "count", 1, observe.CoverageComplete, factFS("base-count-a", model.RevisionKindBase, "count", 1, "write", "@workdir/a", one, 1)),
		attemptTrace("att-base-count-r2", model.RevisionKindBase, "count", 2, observe.CoverageComplete, factFS("base-count-b", model.RevisionKindBase, "count", 2, "write", "@workdir/a", one, 2)),
		attemptTrace("att-head-count-r1", model.RevisionKindHead, "count", 1, observe.CoverageComplete, factFS("head-count-a", model.RevisionKindHead, "count", 1, "write", "@workdir/a", one, 3), factFS("head-count-b", model.RevisionKindHead, "count", 1, "write", "@workdir/a", one, 4)),
		attemptTrace("att-head-count-r2", model.RevisionKindHead, "count", 2, observe.CoverageComplete, factFS("head-count-c", model.RevisionKindHead, "count", 2, "write", "@workdir/a", one, 5), factFS("head-count-d", model.RevisionKindHead, "count", 2, "write", "@workdir/a", one, 6)),
	})
	countDelta, err := cmp.compareDocument(context.Background(), countDoc)
	if err != nil {
		t.Fatalf("count Compare() error = %v", err)
	}
	if !hasDeltaKind(countDelta.Document(), model.DeltaKindCountChanged) {
		t.Fatalf("expected count-changed record, got %+v", countDelta.Document().Records)
	}

	stabilityDoc := traceDocumentForUnitTests([]observe.AttemptTrace{
		attemptTrace("att-base-stability-r1", model.RevisionKindBase, "stability", 1, observe.CoverageComplete, factFS("base-stability-a", model.RevisionKindBase, "stability", 1, "write", "@workdir/a", one, 1)),
		attemptTrace("att-base-stability-r2", model.RevisionKindBase, "stability", 2, observe.CoverageComplete, factFS("base-stability-b", model.RevisionKindBase, "stability", 2, "write", "@workdir/a", one, 2)),
		attemptTrace("att-head-stability-r1", model.RevisionKindHead, "stability", 1, observe.CoverageComplete, factFS("head-stability-a", model.RevisionKindHead, "stability", 1, "write", "@workdir/a", one, 3)),
		attemptTrace("att-head-stability-r2", model.RevisionKindHead, "stability", 2, observe.CoverageComplete, factFS("head-stability-b", model.RevisionKindHead, "stability", 2, "write", "@workdir/a", one, 4), factFS("head-stability-c", model.RevisionKindHead, "stability", 2, "write", "@workdir/a", one, 5)),
	})
	stabilityDelta, err := cmp.compareDocument(context.Background(), stabilityDoc)
	if err != nil {
		t.Fatalf("stability Compare() error = %v", err)
	}
	if !hasDeltaKind(stabilityDelta.Document(), model.DeltaKindStabilityChanged) {
		t.Fatalf("expected stability-changed record, got %+v", stabilityDelta.Document().Records)
	}

	singleSampleOrderDoc := traceDocumentForUnitTests([]observe.AttemptTrace{
		attemptTrace("att-base-order-r1", model.RevisionKindBase, "order", 1, observe.CoverageComplete, factFS("base-order-a", model.RevisionKindBase, "order", 1, "write", "@workdir/a", one, 1), factFS("base-order-b", model.RevisionKindBase, "order", 1, "write", "@workdir/b", two, 2)),
		attemptTrace("att-head-order-r1", model.RevisionKindHead, "order", 1, observe.CoverageComplete, factFS("head-order-b", model.RevisionKindHead, "order", 1, "write", "@workdir/b", two, 3), factFS("head-order-a", model.RevisionKindHead, "order", 1, "write", "@workdir/a", one, 4)),
	})
	singleSampleDelta, err := cmp.compareDocument(context.Background(), singleSampleOrderDoc)
	if err != nil {
		t.Fatalf("single-sample order Compare() error = %v", err)
	}
	if hasDeltaKind(singleSampleDelta.Document(), model.DeltaKindOrderChanged) {
		t.Fatalf("single-sample sequence produced order change: %+v", singleSampleDelta.Document().Records)
	}

	stableOrderDoc := traceDocumentForUnitTests([]observe.AttemptTrace{
		attemptTrace("att-base-order-r1", model.RevisionKindBase, "order", 1, observe.CoverageComplete, factFS("base-order-a", model.RevisionKindBase, "order", 1, "write", "@workdir/a", one, 1), factFS("base-order-b", model.RevisionKindBase, "order", 1, "write", "@workdir/b", two, 2)),
		attemptTrace("att-base-order-r2", model.RevisionKindBase, "order", 2, observe.CoverageComplete, factFS("base-order-c", model.RevisionKindBase, "order", 2, "write", "@workdir/a", one, 3), factFS("base-order-d", model.RevisionKindBase, "order", 2, "write", "@workdir/b", two, 4)),
		attemptTrace("att-head-order-r1", model.RevisionKindHead, "order", 1, observe.CoverageComplete, factFS("head-order-b", model.RevisionKindHead, "order", 1, "write", "@workdir/b", two, 5), factFS("head-order-a", model.RevisionKindHead, "order", 1, "write", "@workdir/a", one, 6)),
		attemptTrace("att-head-order-r2", model.RevisionKindHead, "order", 2, observe.CoverageComplete, factFS("head-order-d", model.RevisionKindHead, "order", 2, "write", "@workdir/b", two, 7), factFS("head-order-c", model.RevisionKindHead, "order", 2, "write", "@workdir/a", one, 8)),
	})
	stableOrderDelta, err := cmp.compareDocument(context.Background(), stableOrderDoc)
	if err != nil {
		t.Fatalf("stable order Compare() error = %v", err)
	}
	if !hasDeltaKind(stableOrderDelta.Document(), model.DeltaKindOrderChanged) {
		t.Fatalf("expected order-changed record, got %+v", stableOrderDelta.Document().Records)
	}
}

func TestGoldenBehavioralDeltaFixture(t *testing.T) {
	trace, cleanup := mustCompareTrace(t)
	defer cleanup()
	delta, err := mustComparator(t, DefaultLimits()).Compare(context.Background(), trace)
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}
	got := delta.JSON()
	if os.Getenv("UPDATE_COMPARE_GOLDEN") == "1" {
		if err := os.WriteFile(filepath.Join("testdata", "v1alpha1", "behavioral-delta.json"), got, 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		if err := os.WriteFile(filepath.Join("testdata", "v1alpha1", "behavioral-delta.digest"), []byte(delta.Digest()), 0o600); err != nil {
			t.Fatalf("write digest golden: %v", err)
		}
	}
	want := readCompareTestdata(t, "behavioral-delta.json")
	if !bytes.Equal(got, want) {
		t.Fatalf("behavioral delta golden mismatch\nwant=%s\n got=%s", want, got)
	}
	wantDigest := strings.TrimSpace(string(readCompareTestdata(t, "behavioral-delta.digest")))
	if string(delta.Digest()) != wantDigest {
		t.Fatalf("delta digest = %s, want %s", delta.Digest(), wantDigest)
	}
}

func TestSupportedFactKindInventoryRequiresComparisonReview(t *testing.T) {
	want := make([]observe.FactKind, 0)
	for _, kind := range observe.SupportedObservationKinds() {
		want = append(want, observe.FactKind(kind))
	}
	got := SupportedFactKinds()
	sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fact kind inventory drift\nwant=%v\n got=%v", want, got)
	}
}

func FuzzBuildOccurrenceProfiles(f *testing.F) {
	f.Add(uint32(1), uint32(1), true)
	f.Add(uint32(2), uint32(3), false)
	f.Fuzz(func(t *testing.T, a, b uint32, complete bool) {
		reps := []repetitionFacts{{repetition: 1, coverage: boolCoverage(complete), facts: int(a)}, {repetition: 2, coverage: boolCoverage(complete), facts: int(b)}}
		_, _ = buildOccurrenceProfile(2, reps)
	})
}

func FuzzBuildTypedComparisonAnchor(f *testing.F) {
	f.Add(string(observe.FactKindFilesystemWrite), "synthetic-test-generated", "write", "@workdir/out")
	f.Add(string(observe.FactKindNetworkConnection), "\x1b", "connect", "example.invalid")
	f.Fuzz(func(t *testing.T, kind, source, op, value string) {
		fact := observe.Fact{Kind: observe.FactKind(kind), Source: model.ObservationSource(source), Filesystem: &observe.FilesystemFact{Operation: op, Path: observe.NormalizedPath{Namespace: observe.PathNamespaceRelative, Relative: value, Literal: value, Display: value}}}
		_, _ = typedAnchorDigest(fact)
	})
}

func FuzzEncodeDeltaRecord(f *testing.F) {
	f.Add(string(model.DeltaKindAdded), string(observe.FactKindObserverWarning), "synthetic-test-generated", "sha256:"+strings.Repeat("1", 64))
	f.Add("future", "\x00", "\x1b", "bad")
	f.Fuzz(func(t *testing.T, kind, factKind, source, digest string) {
		rec := model.DeltaRecord{Kind: model.DeltaKind(kind), FactKind: factKind, Source: model.ObservationSource(source), HeadSemanticDigests: []model.Digest{model.Digest(digest)}, Basis: model.ComparisonBasisSingleSample}
		_, _ = deltaRecordID(ComparisonProfileVersionV1Alpha1, observe.ProfileVersionV1Alpha1, "scenario", rec)
	})
}

func boolCoverage(v bool) observe.CoverageState {
	if v {
		return observe.CoverageComplete
	}
	return observe.CoverageIncomplete
}

// helpers below create verified traces using the same public GR-7A/GR-8B/GR-9A
// boundaries as production comparison. They execute only the deterministic fake runner.

func mustCompareTrace(t *testing.T) (*observe.TraceSet, func()) {
	t.Helper()
	bundle, cleanup := mustCompareBundle(t, true)
	trace, err := mustCompareNormalizer(t).Normalize(context.Background(), bundle)
	if err != nil {
		cleanup()
		t.Fatalf("Normalize() error = %v", err)
	}
	return trace, cleanup
}

func mustCompareBundle(t *testing.T, includeArtifactAndLog bool) (*evidence.Bundle, func()) {
	t.Helper()
	plan := mustComparePlan(t)
	parent := t.TempDir()
	writer, err := evidence.NewWriter(parent, evidence.DefaultLimits())
	if err != nil {
		t.Fatalf("NewWriter() error = %v", err)
	}
	session, err := writer.Begin(context.Background(), plan)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if includeArtifactAndLog {
		key := evidence.AttemptKey{Revision: model.RevisionKindBase, ScenarioID: "test", Repetition: 1}
		logCap, err := session.OpenLog(context.Background(), key, evidence.LogStreamStdout)
		if err != nil {
			t.Fatalf("OpenLog() error = %v", err)
		}
		if _, err := logCap.Write([]byte("synthetic log bytes")); err != nil {
			t.Fatalf("log Write() error = %v", err)
		}
		if err := logCap.Close(); err != nil {
			t.Fatalf("log Close() error = %v", err)
		}
		if _, err := session.AddArtifact(context.Background(), evidence.ArtifactInput{Attempt: key, LogicalPath: "/workspace/out/report.txt", Reader: strings.NewReader("artifact bytes")}); err != nil {
			t.Fatalf("AddArtifact() error = %v", err)
		}
	}
	backend, err := fake.New(compareFakeProgramForPlan(plan))
	if err != nil {
		t.Fatalf("fake.New() error = %v", err)
	}
	result, err := runner.ExecutePlan(context.Background(), plan, backend, runner.SyntheticTestRequirements(), runner.DefaultLimits(), session)
	if err != nil {
		t.Fatalf("ExecutePlan() error = %v", err)
	}
	bundleResult, err := session.Commit(context.Background(), evidence.Complete(result))
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	bundle, err := evidence.OpenAndVerify(context.Background(), bundleResult.Path, evidence.DefaultReaderLimits(), evidence.WithExpectedManifestDigest(bundleResult.ManifestDigest))
	if err != nil {
		t.Fatalf("OpenAndVerify() error = %v", err)
	}
	cleanup := func() { _ = bundle.Close(); _ = os.RemoveAll(bundleResult.Path) }
	return bundle, cleanup
}

func compareFakeProgramForPlan(plan *pipeline.FrozenPlan) fake.Program {
	parent := int64(101)
	child := int64(202)
	return fake.Program{PlanDigest: plan.Digest(), Attempts: []fake.AttemptScript{
		{Revision: model.RevisionKindHead, ScenarioID: "test", Repetition: 1, Events: []fake.SyntheticEvent{
			{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindNetworkConnection, Network: &model.NetworkObservation{Operation: "connect", Protocol: "tcp", DestinationHost: "canary.example.invalid", DestinationPort: 443, ResolvedAddresses: []string{}, Result: "denied"}}},
			{OffsetMillis: 12, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindObserverWarning, ObserverWarning: &model.ObserverWarningObservation{Code: "synthetic-warning", Message: "warning with /workspace prose", Unsupported: false, Limitations: []model.Limitation{}}}},
		}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusFailed, ExitCode: intPtr(2), DurationMillis: 25}},
		{Revision: model.RevisionKindBase, ScenarioID: "test", Repetition: 1, Events: []fake.SyntheticEvent{
			{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindProcessStart, Process: &model.ProcessObservation{Operation: "start", ProcessID: parent, ExecutablePath: "/workspace/bin/tester", Arguments: []string{"--root=/workspace"}, Environment: []model.EnvEntry{}}}},
			{OffsetMillis: 11, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindProcessStart, Process: &model.ProcessObservation{Operation: "start", ProcessID: child, ParentProcessID: &parent, ExecutablePath: "/workspace/bin/helper", Arguments: []string{}, Environment: []model.EnvEntry{}}}},
			{OffsetMillis: 12, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindProcessExit, Process: &model.ProcessObservation{Operation: "exit", ProcessID: child, ExecutablePath: "", Arguments: []string{}, Environment: []model.EnvEntry{}, ExitCode: intPtr(0), DurationMillis: 1}}},
			{OffsetMillis: 13, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindProcessExit, Process: &model.ProcessObservation{Operation: "exit", ProcessID: parent, ExecutablePath: "", Arguments: []string{}, Environment: []model.EnvEntry{}, ExitCode: intPtr(0), DurationMillis: 3}}},
		}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 20}},
		{Revision: model.RevisionKindBase, ScenarioID: "build", Repetition: 1, Events: []fake.SyntheticEvent{
			{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindFilesystemWrite, Filesystem: &model.FilesystemObservation{Operation: "write", Path: "/workspace/bin/glassroot", Digest: "sha256:9999999999999999999999999999999999999999999999999999999999999999", SizeBytes: 42}}},
			{OffsetMillis: 11, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindResourceLimit, ResourceLimit: &model.ResourceLimitObservation{LimitKind: "cpu", LimitValue: 100, Unit: "millis", ObservedValue: 99, Exceeded: false}}},
		}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 30}},
		{Revision: model.RevisionKindHead, ScenarioID: "build", Repetition: 1, Events: []fake.SyntheticEvent{
			{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindArtifactActivity, Artifact: &model.ArtifactObservation{Operation: "create", ArtifactID: "artifact-head-bin", Path: "/workspace/bin/glassroot", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SizeBytes: 64, Executable: true, SourceEventIDs: []string{}}}},
		}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 35}},
	}}
}

func mustComparePlan(t *testing.T) *pipeline.FrozenPlan {
	t.Helper()
	plan, err := pipeline.Build(context.Background(), compareBuildRequest(t, comparePipelineYAML, comparePipelineYAML))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return plan
}

func compareBuildRequest(t *testing.T, baseYAML, headYAML string) pipeline.BuildRequest {
	t.Helper()
	trusted := compareTrustedLoad(t, baseYAML, headYAML)
	return pipeline.BuildRequest{RunID: "run-0001", CreatedAt: compareTestCreatedAt, Trusted: trusted,
		BaseSource: compareSourceSnapshot(model.RevisionKindBase, compareTestBaseCommit, compareTestBaseTree, "sha256:3333333333333333333333333333333333333333333333333333333333333333", "sha256:4444444444444444444444444444444444444444444444444444444444444444"),
		HeadSource: compareSourceSnapshot(model.RevisionKindHead, compareTestHeadCommit, compareTestHeadTree, "sha256:5555555555555555555555555555555555555555555555555555555555555555", "sha256:6666666666666666666666666666666666666666666666666666666666666666"),
		Platform:   comparePlatformConstraints()}
}

func compareTrustedLoad(t *testing.T, baseYAML, headYAML string) config.TrustedLoadResult {
	t.Helper()
	base := model.CommitRef{Kind: model.RevisionKindBase, Repository: "https://example.invalid/org/repo.git", Ref: "refs/heads/master", CommitID: compareTestBaseCommit, TreeDigest: model.Digest(compareTestBaseTree)}
	head := model.CommitRef{Kind: model.RevisionKindHead, Repository: "https://example.invalid/org/repo.git", Ref: "refs/pull/7/head", CommitID: compareTestHeadCommit, TreeDigest: model.Digest(compareTestHeadTree)}
	source := &compareMemoryRevisionSource{files: map[string]config.RevisionFile{}}
	source.files[compareRevisionKey(base, config.PipelinePath)] = config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte(baseYAML), ObjectID: strings.Repeat("a", 40)}
	source.files[compareRevisionKey(head, config.PipelinePath)] = config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte(headYAML), ObjectID: strings.Repeat("b", 40)}
	trusted, err := config.LoadTrusted(context.Background(), source, config.TrustedLoadRequest{Base: base, Head: head})
	if err != nil {
		t.Fatalf("LoadTrusted() error = %v", err)
	}
	return trusted
}

type compareMemoryRevisionSource struct {
	files map[string]config.RevisionFile
}

func (s *compareMemoryRevisionSource) ReadFile(ctx context.Context, revision model.CommitRef, p string, maxBytes int64) (config.RevisionFile, error) {
	if err := ctx.Err(); err != nil {
		return config.RevisionFile{}, err
	}
	file, ok := s.files[compareRevisionKey(revision, p)]
	if !ok {
		return config.RevisionFile{}, fs.ErrNotExist
	}
	if int64(len(file.Data)) > maxBytes {
		return config.RevisionFile{}, config.ErrRevisionFileTooLarge
	}
	file.Data = append([]byte(nil), file.Data...)
	return file, nil
}
func compareRevisionKey(ref model.CommitRef, p string) string {
	return string(ref.Kind) + ":" + ref.CommitID + ":" + p
}
func compareSourceSnapshot(kind model.RevisionKind, commit, tree, treeDigest, manifestDigest string) pipeline.SourceSnapshot {
	return pipeline.SourceSnapshot{RevisionKind: kind, CommitID: commit, TreeID: tree, ObjectFormat: pipeline.ObjectFormatSHA1, MaterializedTreeDigest: model.Digest(treeDigest), MaterializationManifestDigest: model.Digest(manifestDigest), Summary: pipeline.SourceSummary{DirectoryCount: 3, RegularFileCount: 4, ExecutableFileCount: 1, SymlinkCount: 1, GitlinkCount: 1, LFSPointerCount: 1, TotalMaterializedFileBytes: 1234, SkippedEntryCount: 1}, Limitations: []pipeline.SourceLimitation{{Code: "skipped-gitlink", Path: "vendor/submodule", Summary: "gitlink was reported but not traversed or materialized"}}}
}
func comparePlatformConstraints() pipeline.PlatformConstraints {
	return pipeline.PlatformConstraints{MaxCPU: config.MaxCPU, MaxMemoryBytes: config.MaxMemoryBytes, MaxDiskBytes: config.MaxDiskBytes, MaxProcessCount: config.MaxProcessCount, MaxGlobalTimeoutMillis: config.MaxTimeoutMillis, MaxScenarioTimeoutMillis: config.MaxTimeoutMillis, MaxScenarioCount: config.MaxScenarioCount, MaxRepetitions: config.MaxRepetitions, MaxFilesystemRootCount: config.MaxFilesystemRootCount, MaxArtifactCount: config.MaxArtifactCount, MaxArtifactBytes: config.MaxArtifactBytes, MaxLogBytesPerStream: config.MaxLogBytesPerStream, MaxPlanJSONBytes: pipeline.MaxPlanJSONBytes, RequiredNetworkMode: model.NetworkModeDeny}
}

func mustCompareNormalizer(t *testing.T) *observe.Normalizer {
	t.Helper()
	n, err := observe.New(observe.DefaultLimits())
	if err != nil {
		t.Fatalf("observe.New() error = %v", err)
	}
	return n
}
func mustComparator(t *testing.T, l Limits) *Comparator {
	t.Helper()
	c, err := New(l)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return c
}
func readCompareTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "v1alpha1", name))
	if err != nil {
		t.Fatalf("read compare testdata %s: %v", name, err)
	}
	return b
}
func normalizedSnapshotJSON(t *testing.T, snap model.DeltaFactSnapshot) string {
	t.Helper()
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func assertCompareError(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected compare error %s, got nil", code)
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error %T is not *compare.Error: %v", err, err)
	}
	if e.Code != code {
		t.Fatalf("error code=%s want=%s err=%v", e.Code, code, err)
	}
	if strings.ContainsAny(err.Error(), "\x1b\r\n") {
		t.Fatalf("error contains raw controls: %q", err.Error())
	}
}
func intPtr(v int) *int { return &v }

func hasDeltaKind(doc model.BehavioralDelta, kind model.DeltaKind) bool {
	for _, rec := range doc.Records {
		if rec.Kind == kind {
			return true
		}
	}
	return false
}

const comparePipelineYAML = `apiVersion: glassroot.dev/v1alpha1
kind: Pipeline
metadata:
  name: default
spec:
  environment:
    image: docker.io/library/golang:1.26@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
    workdir: /workspace
  resources:
    cpu: 2
    memory: 2GiB
    disk: 4GiB
    processes: 256
    timeout: 15m
  network:
    mode: deny
    allow: []
  scenarios:
    - id: test
      name: Unit tests
      shell: /bin/sh
      run: echo test
      timeout: 10m
    - id: build
      name: Build
      shell: /bin/bash
      run: go build ./cmd/glassroot
      timeout: 5m
  collect:
    filesystem:
      roots:
        - /workspace
        - /tmp
      contents: metadata-and-digests
    artifacts:
      - path: /workspace/bin/**
        maxBytes: 50MiB
    logs:
      maxBytesPerStream: 10MiB
  compare:
    ignore:
      - field: event.timestamp
      - field: process.pid
    repetitions: 1
  policy:
    profile: strict
`

func traceDocumentForUnitTests(attempts []observe.AttemptTrace) observe.TraceSetDocument {
	for i := range attempts {
		attempts[i].Ordinal = uint64(i + 1)
	}
	return observe.TraceSetDocument{
		SchemaVersion: observe.TraceSetSchemaV1Alpha1,
		Profile:       observe.NormalizationProfile{Version: observe.ProfileVersionV1Alpha1, IgnoreFields: []string{observe.IgnoreFieldEventTimestamp, observe.IgnoreFieldProcessPID}, ProcessIdentityAlgorithm: observe.ProcessIdentityAlgorithmV1, TimestampAlgorithm: observe.TimestampAlgorithmV1, PathRootAlgorithm: observe.PathRootAlgorithmV1, RootAliases: []observe.PathRootAlias{{Namespace: observe.PathNamespaceWorkdirRoot, RootIndex: 0, Root: "/workspace", Alias: "@workdir"}}},
		PlanDigest:    model.Digest("sha256:" + strings.Repeat("1", 64)), ManifestDigest: model.Digest("sha256:" + strings.Repeat("2", 64)), RunID: "run-0001",
		ManifestVerification: observe.ManifestVerification{Mode: evidence.VerificationModeExpectedManifestDigest, ManifestDigest: model.Digest("sha256:" + strings.Repeat("2", 64)), ExpectedManifestDigestSupplied: true, ExpectedManifestDigestMatched: true, InternallyConsistent: true, Limitations: []model.Limitation{}},
		ExecutionComplete:    true, EvidenceComplete: false, EvidenceContext: model.EvidenceContext{SyntheticEvidence: true, ExecutesTargetCode: false}, Attempts: attempts, Limitations: []model.Limitation{},
	}
}

func attemptTrace(id string, rev model.RevisionKind, scenario string, rep uint32, cov observe.CoverageState, facts ...observe.Fact) observe.AttemptTrace {
	for i := range facts {
		for j := range facts[i].Evidence {
			facts[i].Evidence[j].Revision = rev
			facts[i].Evidence[j].ScenarioID = scenario
			facts[i].Evidence[j].Repetition = rep
		}
	}
	ordinal := uint64(1)
	if rev == model.RevisionKindBase && rep == 2 {
		ordinal = 2
	}
	if rev == model.RevisionKindHead && rep == 1 {
		ordinal = 3
	}
	if rev == model.RevisionKindHead && rep == 2 {
		ordinal = 4
	}
	return observe.AttemptTrace{AttemptID: id, Ordinal: ordinal, Revision: rev, ScenarioID: scenario, Repetition: rep, Coverage: cov, Result: evidence.CaptureStateCaptured, Events: evidence.CaptureStateCaptured, Stdout: evidence.CaptureStateNotProvided, Stderr: evidence.CaptureStateNotProvided, Artifacts: evidence.CaptureStateNotProvided, AcceptedEventCount: uint64(len(facts)), Facts: facts, Limitations: []model.Limitation{}}
}

func factFS(seed string, rev model.RevisionKind, scenario string, rep uint32, op, path, digest string, seq uint64) observe.Fact {
	f := observe.Fact{ID: observe.FactID("fact-" + seed), SemanticDigest: model.Digest("sha256:" + digestSeed(seed, digest)), Kind: observe.FactKindFilesystemWrite, Source: model.ObservationSourceSyntheticTestGenerated, Timing: observe.NormalizedTiming{IncludedInSemanticDigest: false}, Filesystem: &observe.FilesystemFact{Operation: op, Path: observe.NormalizedPath{Namespace: observe.PathNamespaceWorkdirRoot, Relative: strings.TrimPrefix(path, "@workdir/"), Literal: "/workspace/" + strings.TrimPrefix(path, "@workdir/"), Display: path}, Digest: model.Digest(digest), SizeBytes: 1}, Evidence: []observe.RawEvidenceReference{{EventStreamDigest: model.Digest("sha256:" + strings.Repeat("e", 64)), EventStreamPath: "attempts/" + string(rev) + "/" + scenario + "/repetition-000" + itoa(rep) + "/events.jsonl", EventID: "evt-" + seed, EventSequence: seq, Revision: rev, ScenarioID: scenario, Repetition: rep}}, Limitations: []model.Limitation{}}
	return f
}

func factNetwork(seed string, rep uint32) observe.Fact {
	return observe.Fact{ID: observe.FactID("fact-" + seed), SemanticDigest: model.Digest("sha256:" + digestSeed(seed, "")), Kind: observe.FactKindNetworkConnection, Source: model.ObservationSourceSyntheticTestGenerated, Timing: observe.NormalizedTiming{IncludedInSemanticDigest: false}, Network: &observe.NetworkFact{Operation: "connect", Protocol: "tcp", DestinationHost: "new.example.invalid", DestinationPort: 443, Result: "denied", ResolvedAddresses: []string{}}, Evidence: []observe.RawEvidenceReference{{EventStreamDigest: model.Digest("sha256:" + strings.Repeat("e", 64)), EventStreamPath: "attempts/head/unit/repetition-000" + itoa(rep) + "/events.jsonl", EventID: "evt-" + seed, EventSequence: uint64(10 + rep), Revision: model.RevisionKindHead, ScenarioID: "unit", Repetition: rep}}, Limitations: []model.Limitation{}}
}

func digestSeed(seed, digest string) string {
	if strings.HasPrefix(digest, "sha256:") {
		return strings.TrimPrefix(digest, "sha256:")
	}
	h := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(h[:])
}
