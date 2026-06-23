package observe

import (
	"bytes"
	"context"
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
	"github.com/mattneel/glassroot/internal/pipeline"
	"github.com/mattneel/glassroot/internal/runner"
	"github.com/mattneel/glassroot/internal/runner/fake"
)

const (
	observeTestBaseCommit = "1111111111111111111111111111111111111111"
	observeTestHeadCommit = "2222222222222222222222222222222222222222"
	observeTestBaseTree   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	observeTestHeadTree   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

var observeTestCreatedAt = time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)

func TestNormalizeVerifiedBundleProducesDeterministicTypedTrace(t *testing.T) {
	bundle, cleanup := mustVerifiedBundle(t, false)
	defer cleanup()

	normalizer := mustNormalizer(t, DefaultLimits())
	trace, err := normalizer.Normalize(context.Background(), bundle)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	doc := trace.Document()
	if doc.Profile.Version != ProfileVersionV1Alpha1 {
		t.Fatalf("profile version = %q", doc.Profile.Version)
	}
	if doc.ManifestVerification.Mode != evidence.VerificationModeExpectedManifestDigest || !doc.ManifestVerification.ExpectedManifestDigestMatched {
		t.Fatalf("verification mode not retained: %+v", doc.ManifestVerification)
	}
	if !doc.ExecutionComplete || !doc.EvidenceComplete {
		t.Fatalf("complete bundle state not retained: execution=%v evidence=%v", doc.ExecutionComplete, doc.EvidenceComplete)
	}
	if len(doc.Attempts) != 4 {
		t.Fatalf("attempts=%d, want 4", len(doc.Attempts))
	}
	gotOrder := make([]string, 0, len(doc.Attempts))
	for _, attempt := range doc.Attempts {
		gotOrder = append(gotOrder, attempt.AttemptID)
	}
	wantOrder := []string{"att-base-test-r1", "att-base-build-r1", "att-head-test-r1", "att-head-build-r1"}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("attempt order\nwant=%v\n got=%v", wantOrder, gotOrder)
	}

	allFacts := flattenFacts(doc)
	if len(allFacts) == 0 {
		t.Fatal("normalization produced no facts")
	}
	kinds := map[FactKind]bool{}
	for _, fact := range allFacts {
		kinds[fact.Kind] = true
		if fact.ID == "" || !strings.HasPrefix(string(fact.SemanticDigest), "sha256:") {
			t.Fatalf("fact missing identities: %+v", fact)
		}
		if fact.Source != model.ObservationSourceSyntheticTestGenerated {
			t.Fatalf("synthetic provenance not preserved: %+v", fact)
		}
		if len(fact.Evidence) == 0 {
			t.Fatalf("fact missing raw evidence reference: %+v", fact)
		}
		for _, ref := range fact.Evidence {
			if ref.EventID == "" || ref.EventSequence == 0 || ref.EventStreamDigest == "" || ref.EventStreamPath == "" {
				t.Fatalf("incomplete evidence reference: %+v", ref)
			}
			if strings.Contains(ref.EventStreamPath, string(os.PathSeparator)) && os.PathSeparator != '/' {
				t.Fatalf("event stream path should be logical slash path: %q", ref.EventStreamPath)
			}
		}
	}
	for _, kind := range []FactKind{FactKindScenarioStarted, FactKindScenarioCompleted, FactKindProcessStart, FactKindProcessExit, FactKindFilesystemWrite, FactKindNetworkConnection, FactKindArtifactActivity, FactKindObserverWarning, FactKindResourceLimit} {
		if !kinds[kind] {
			t.Fatalf("missing normalized fact kind %s in %v", kind, kinds)
		}
	}
	if got := normalizedTraceJSON(t, doc); bytes.Contains(got, []byte("\"processId\"")) || bytes.Contains(got, []byte("parentProcessId")) || bytes.Contains(got, []byte("/home/")) {
		t.Fatalf("normalized trace leaked raw PID field or host path: %s", got)
	}

	repeat, err := normalizer.Normalize(context.Background(), bundle)
	if err != nil {
		t.Fatalf("repeat Normalize() error = %v", err)
	}
	if !bytes.Equal(normalizedTraceJSON(t, doc), normalizedTraceJSON(t, repeat.Document())) {
		t.Fatalf("normalization is not deterministic\nfirst=%s\nsecond=%s", normalizedTraceJSON(t, doc), normalizedTraceJSON(t, repeat.Document()))
	}
}

func TestNormalizationSemanticDigestsExcludePIDRunIDEventIDAndIgnoredTimestamp(t *testing.T) {
	baseA := model.ObservationEvent{RunID: "run-a", ID: "evt-" + strings.Repeat("1", 64), SequenceNumber: 1, ObservedAt: observeTestCreatedAt, Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindProcessStart, Process: &model.ProcessObservation{Operation: "start", ProcessID: 10, ExecutablePath: "/workspace/bin/tool", Arguments: []string{"--flag"}, Environment: []model.EnvEntry{}}}
	baseB := baseA
	baseB.RunID = "run-b"
	baseB.ID = "evt-" + strings.Repeat("2", 64)
	baseB.SequenceNumber = 99
	baseB.ObservedAt = observeTestCreatedAt.Add(12 * time.Hour)
	baseB.Process = &model.ProcessObservation{Operation: "start", ProcessID: 9999, ExecutablePath: "/workspace/bin/tool", Arguments: []string{"--flag"}, Environment: []model.EnvEntry{}}
	profile := NormalizationProfile{Version: ProfileVersionV1Alpha1, IgnoreFields: []string{IgnoreFieldEventTimestamp, IgnoreFieldProcessPID}, ProcessIdentityAlgorithm: ProcessIdentityAlgorithmV1, TimestampAlgorithm: TimestampAlgorithmV1, PathRootAlgorithm: PathRootAlgorithmV1, RootAliases: []PathRootAlias{{Namespace: PathNamespaceWorkdirRoot, RootIndex: 0, Root: "/workspace", Alias: "@workdir"}}}
	stateA := newAttemptStateForTest(profile)
	stateB := newAttemptStateForTest(profile)
	factA, err := normalizeEventForTest(stateA, baseA, RawEvidenceReference{EventID: baseA.ID, EventSequence: uint64(baseA.SequenceNumber)})
	if err != nil {
		t.Fatalf("normalize A: %v", err)
	}
	factB, err := normalizeEventForTest(stateB, baseB, RawEvidenceReference{EventID: baseB.ID, EventSequence: uint64(baseB.SequenceNumber)})
	if err != nil {
		t.Fatalf("normalize B: %v", err)
	}
	if factA.SemanticDigest != factB.SemanticDigest {
		t.Fatalf("semantic digest should ignore run/event/raw PID/absolute timestamp\nA=%s\nB=%s", factA.SemanticDigest, factB.SemanticDigest)
	}
	factA.ID = factID(model.Digest("sha256:"+strings.Repeat("a", 64)), "att-base-test-r1", factA.SemanticDigest, []string{baseA.ID})
	factB.ID = factID(model.Digest("sha256:"+strings.Repeat("a", 64)), "att-base-test-r1", factB.SemanticDigest, []string{baseB.ID})
	if factA.ID == factB.ID {
		t.Fatal("fact ID must retain raw event provenance even when semantic digest matches")
	}
}

func TestSemanticDigestNormalizesMappedRootPrefix(t *testing.T) {
	profileA := NormalizationProfile{Version: ProfileVersionV1Alpha1, IgnoreFields: []string{IgnoreFieldEventTimestamp, IgnoreFieldProcessPID}, ProcessIdentityAlgorithm: ProcessIdentityAlgorithmV1, TimestampAlgorithm: TimestampAlgorithmV1, PathRootAlgorithm: PathRootAlgorithmV1, RootAliases: []PathRootAlias{{Namespace: PathNamespaceWorkdirRoot, RootIndex: 0, Root: "/workspace", Alias: "@workdir"}}}
	profileB := profileA
	profileB.RootAliases = []PathRootAlias{{Namespace: PathNamespaceWorkdirRoot, RootIndex: 0, Root: "/mnt/workspace", Alias: "@workdir"}}
	eventA := model.ObservationEvent{RunID: "run-a", ID: "evt-" + strings.Repeat("a", 64), SequenceNumber: 1, ObservedAt: observeTestCreatedAt, Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindFilesystemWrite, Filesystem: &model.FilesystemObservation{Operation: "write", Path: "/workspace/bin/out", Digest: model.Digest("sha256:" + strings.Repeat("1", 64)), SizeBytes: 1}}
	eventB := eventA
	eventB.RunID = "run-b"
	eventB.ID = "evt-" + strings.Repeat("b", 64)
	eventB.Filesystem = &model.FilesystemObservation{Operation: "write", Path: "/mnt/workspace/bin/out", Digest: model.Digest("sha256:" + strings.Repeat("1", 64)), SizeBytes: 1}
	factA, err := normalizeEventForTest(newAttemptStateForTest(profileA), eventA, RawEvidenceReference{EventID: eventA.ID, EventSequence: 1})
	if err != nil {
		t.Fatalf("normalize A: %v", err)
	}
	factB, err := normalizeEventForTest(newAttemptStateForTest(profileB), eventB, RawEvidenceReference{EventID: eventB.ID, EventSequence: 1})
	if err != nil {
		t.Fatalf("normalize B: %v", err)
	}
	if factA.SemanticDigest != factB.SemanticDigest {
		t.Fatalf("mapped root prefix should not affect semantics: %s != %s", factA.SemanticDigest, factB.SemanticDigest)
	}
}

func TestProcessIdentityIsSourceScopedAndLineageAware(t *testing.T) {
	profile := NormalizationProfile{Version: ProfileVersionV1Alpha1, IgnoreFields: []string{IgnoreFieldEventTimestamp, IgnoreFieldProcessPID}, ProcessIdentityAlgorithm: ProcessIdentityAlgorithmV1, TimestampAlgorithm: TimestampAlgorithmV1, PathRootAlgorithm: PathRootAlgorithmV1, RootAliases: []PathRootAlias{{Namespace: PathNamespaceWorkdirRoot, RootIndex: 0, Root: "/workspace", Alias: "@workdir"}}}
	state := newAttemptStateForTest(profile)
	parent := int64(1)
	parentEvent := model.ObservationEvent{ID: "evt-" + strings.Repeat("3", 64), SequenceNumber: 1, ObservedAt: observeTestCreatedAt, Source: model.ObservationSourceSandboxRuntimeObserved, Kind: model.ObservationKindProcessStart, Process: &model.ProcessObservation{Operation: "start", ProcessID: parent, ExecutablePath: "/workspace/bin/parent", Arguments: []string{}, Environment: []model.EnvEntry{}}}
	childEvent := model.ObservationEvent{ID: "evt-" + strings.Repeat("4", 64), SequenceNumber: 2, ObservedAt: observeTestCreatedAt.Add(time.Millisecond), Source: model.ObservationSourceSandboxRuntimeObserved, Kind: model.ObservationKindProcessStart, Process: &model.ProcessObservation{Operation: "start", ProcessID: 2, ParentProcessID: &parent, ExecutablePath: "/workspace/bin/child", Arguments: []string{}, Environment: []model.EnvEntry{}}}
	otherSourceChild := childEvent
	otherSourceChild.ID = "evt-" + strings.Repeat("5", 64)
	otherSourceChild.Source = model.ObservationSourceGuestAgentReported
	parentFact, err := normalizeEventForTest(state, parentEvent, RawEvidenceReference{EventID: parentEvent.ID, EventSequence: 1})
	if err != nil {
		t.Fatalf("parent normalize: %v", err)
	}
	childFact, err := normalizeEventForTest(state, childEvent, RawEvidenceReference{EventID: childEvent.ID, EventSequence: 2})
	if err != nil {
		t.Fatalf("child normalize: %v", err)
	}
	otherFact, err := normalizeEventForTest(state, otherSourceChild, RawEvidenceReference{EventID: otherSourceChild.ID, EventSequence: 3})
	if err != nil {
		t.Fatalf("other source normalize: %v", err)
	}
	if childFact.Process.ParentStableID != parentFact.Process.StableID {
		t.Fatalf("child parent link not preserved: child=%+v parent=%+v", childFact.Process, parentFact.Process)
	}
	if otherFact.Process.ParentStableID == childFact.Process.ParentStableID || otherFact.Process.StableID == childFact.Process.StableID {
		t.Fatalf("process identity should be source-scoped: child=%+v other=%+v", childFact.Process, otherFact.Process)
	}
	if bytes.Contains(normalizedTraceJSON(t, TraceSetDocument{Attempts: []AttemptTrace{{Facts: []Fact{parentFact, childFact, otherFact}}}}), []byte("\"processId\"")) {
		t.Fatal("raw PID field leaked into process facts")
	}
}

func TestObservedPathNormalizationIsStructuredAndDoesNotRewriteArbitraryText(t *testing.T) {
	roots := []PathRootAlias{
		{Namespace: PathNamespaceWorkdirRoot, RootIndex: 0, Root: "/workspace", Alias: "@workdir"},
		{Namespace: PathNamespaceCollectionRoot, RootIndex: 1, Root: "/workspace/sub", Alias: "@root1"},
		{Namespace: PathNamespaceCollectionRoot, RootIndex: 2, Root: "/tmp", Alias: "@root2"},
	}
	cases := []struct {
		path      string
		namespace PathNamespace
		rel       string
	}{
		{path: "/workspace", namespace: PathNamespaceWorkdirRoot, rel: ""},
		{path: "/workspace/file", namespace: PathNamespaceWorkdirRoot, rel: "file"},
		{path: "/workspace/sub/file", namespace: PathNamespaceCollectionRoot, rel: "file"},
		{path: "/workspace2/file", namespace: PathNamespaceAbsoluteUnmapped, rel: ""},
		{path: "relative/file", namespace: PathNamespaceRelative, rel: "relative/file"},
		{path: `C:\workspace\file`, namespace: PathNamespaceOpaqueInvalid, rel: ""},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := normalizeObservedPath(tc.path, roots)
			if got.Namespace != tc.namespace || got.Relative != tc.rel {
				t.Fatalf("normalizeObservedPath(%q) = %+v", tc.path, got)
			}
		})
	}
	warning := model.ObservationEvent{ID: "evt-" + strings.Repeat("6", 64), SequenceNumber: 1, ObservedAt: observeTestCreatedAt, Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindObserverWarning, ObserverWarning: &model.ObserverWarningObservation{Code: "contains-path", Message: "literal /workspace text remains hostile prose", Limitations: []model.Limitation{}}}
	profile := NormalizationProfile{Version: ProfileVersionV1Alpha1, IgnoreFields: []string{IgnoreFieldEventTimestamp, IgnoreFieldProcessPID}, RootAliases: roots, ProcessIdentityAlgorithm: ProcessIdentityAlgorithmV1, TimestampAlgorithm: TimestampAlgorithmV1, PathRootAlgorithm: PathRootAlgorithmV1}
	fact, err := normalizeEventForTest(newAttemptStateForTest(profile), warning, RawEvidenceReference{EventID: warning.ID, EventSequence: 1})
	if err != nil {
		t.Fatalf("warning normalize: %v", err)
	}
	if fact.Warning == nil || !strings.Contains(fact.Warning.Message, "/workspace") {
		t.Fatalf("warning prose was rewritten: %+v", fact.Warning)
	}
}

func TestNormalizationRejectsNilClosedAndUnsupportedInputs(t *testing.T) {
	normalizer := mustNormalizer(t, DefaultLimits())
	_, err := normalizer.Normalize(context.Background(), nil)
	assertObserveError(t, err, CodeNilBundle)

	bundle, cleanup := mustVerifiedBundle(t, false)
	defer cleanup()
	if err := bundle.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = normalizer.Normalize(context.Background(), bundle)
	assertObserveError(t, err, CodeBundleClosed)

	profile := NormalizationProfile{Version: ProfileVersionV1Alpha1, IgnoreFields: []string{"all.paths"}, ProcessIdentityAlgorithm: ProcessIdentityAlgorithmV1, TimestampAlgorithm: TimestampAlgorithmV1, PathRootAlgorithm: PathRootAlgorithmV1}
	if err := validateProfile(profile); err == nil {
		t.Fatal("unsupported ignore field accepted")
	}
	unknown := model.ObservationEvent{ID: "evt-" + strings.Repeat("7", 64), SequenceNumber: 1, ObservedAt: observeTestCreatedAt, Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKind("future-kind")}
	_, err = normalizeEventForTest(newAttemptStateForTest(defaultTestProfile()), unknown, RawEvidenceReference{EventID: unknown.ID, EventSequence: 1})
	assertObserveError(t, err, CodeUnsupportedObservationKind)
}

func TestObservationKindInventoryRequiresExplicitNormalizationReview(t *testing.T) {
	want := []model.ObservationKind{
		model.ObservationKindProcessStart,
		model.ObservationKindProcessExit,
		model.ObservationKindFilesystemCreate,
		model.ObservationKindFilesystemRead,
		model.ObservationKindFilesystemWrite,
		model.ObservationKindFilesystemDelete,
		model.ObservationKindFilesystemRename,
		model.ObservationKindFilesystemChmod,
		model.ObservationKindDNSQuery,
		model.ObservationKindNetworkConnection,
		model.ObservationKindArtifactActivity,
		model.ObservationKindScenarioStarted,
		model.ObservationKindScenarioCompleted,
		model.ObservationKindObserverWarning,
		model.ObservationKindUnsupportedObservation,
		model.ObservationKindResourceLimit,
	}
	got := SupportedObservationKinds()
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("supported kind inventory drift\nwant=%v\n got=%v", want, got)
	}
}

func TestGoldenNormalizedTraceFixture(t *testing.T) {
	bundle, cleanup := mustVerifiedBundle(t, true)
	defer cleanup()
	trace, err := mustNormalizer(t, DefaultLimits()).Normalize(context.Background(), bundle)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	got := normalizedTraceJSON(t, trace.Document())
	if os.Getenv("UPDATE_OBSERVE_GOLDEN") == "1" {
		if err := os.WriteFile(filepath.Join("testdata", "v1alpha1", "normalized-trace.json"), got, 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	want := readObserveTestdata(t, "normalized-trace.json")
	if !bytes.Equal(got, want) {
		t.Fatalf("normalized trace golden mismatch\nwant=%s\n got=%s", want, got)
	}
}

func FuzzNormalizeProcessTrace(f *testing.F) {
	f.Add(int64(1), int64(0), "/workspace/bin/tool", "arg")
	f.Add(int64(42), int64(99), "/tmp/tool", "\x1b")
	f.Fuzz(func(t *testing.T, pid, ppid int64, exe, arg string) {
		profile := defaultTestProfile()
		state := newAttemptStateForTest(profile)
		var parent *int64
		if ppid != 0 {
			parent = &ppid
		}
		event := model.ObservationEvent{ID: "evt-" + strings.Repeat("8", 64), SequenceNumber: 1, ObservedAt: observeTestCreatedAt, Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindProcessStart, Process: &model.ProcessObservation{Operation: "start", ProcessID: pid, ParentProcessID: parent, ExecutablePath: exe, Arguments: []string{arg}, Environment: []model.EnvEntry{}}}
		_, _ = normalizeEventForTest(state, event, RawEvidenceReference{EventID: event.ID, EventSequence: 1})
	})
}

func FuzzNormalizeObservedPath(f *testing.F) {
	f.Add("/workspace/bin/tool")
	f.Add("/workspace2/bin/tool")
	f.Add("../workspace")
	f.Add("a\\b")
	f.Fuzz(func(t *testing.T, p string) { _ = normalizeObservedPath(p, defaultTestProfile().RootAliases) })
}

func FuzzEncodeNormalizedFact(f *testing.F) {
	f.Add(string(FactKindObserverWarning), "synthetic-test-generated", "code", "message")
	f.Add("future", "\x1b", "\x00", strings.Repeat("x", 128))
	f.Fuzz(func(t *testing.T, kind, source, code, message string) {
		fact := Fact{Kind: FactKind(kind), Source: model.ObservationSource(source), Warning: &WarningFact{Code: code, Message: message, Unsupported: false}, Evidence: []RawEvidenceReference{{EventID: "evt-" + strings.Repeat("9", 64), EventSequence: 1}}}
		_, _ = semanticDigest(defaultTestProfile(), fact)
	})
}

func mustVerifiedBundle(t *testing.T, includeArtifactAndLog bool) (*evidence.Bundle, func()) {
	t.Helper()
	plan := mustObservePlan(t)
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
	backend, err := fake.New(observeFakeProgramForPlan(plan))
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
	cleanup := func() {
		_ = bundle.Close()
		_ = os.RemoveAll(bundleResult.Path)
	}
	return bundle, cleanup
}

func observeFakeProgramForPlan(plan *pipeline.FrozenPlan) fake.Program {
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

func mustNormalizer(t *testing.T, limits Limits) *Normalizer {
	t.Helper()
	n, err := New(limits)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return n
}

func flattenFacts(doc TraceSetDocument) []Fact {
	var out []Fact
	for _, attempt := range doc.Attempts {
		out = append(out, attempt.Facts...)
	}
	return out
}

func normalizedTraceJSON(t *testing.T, doc TraceSetDocument) []byte {
	t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}
	return data
}

func readObserveTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "v1alpha1", name))
	if err != nil {
		t.Fatalf("read testdata %s: %v", name, err)
	}
	return b
}

func assertObserveError(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected observe error %s, got nil", code)
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error %T is not *observe.Error: %v", err, err)
	}
	if e.Code != code {
		t.Fatalf("error code = %s, want %s; err=%v", e.Code, code, err)
	}
	if strings.ContainsAny(err.Error(), "\x1b\r\n") {
		t.Fatalf("error contains raw control characters: %q", err.Error())
	}
}

func defaultTestProfile() NormalizationProfile {
	return NormalizationProfile{Version: ProfileVersionV1Alpha1, IgnoreFields: []string{IgnoreFieldEventTimestamp, IgnoreFieldProcessPID}, ProcessIdentityAlgorithm: ProcessIdentityAlgorithmV1, TimestampAlgorithm: TimestampAlgorithmV1, PathRootAlgorithm: PathRootAlgorithmV1, RootAliases: []PathRootAlias{{Namespace: PathNamespaceWorkdirRoot, RootIndex: 0, Root: "/workspace", Alias: "@workdir"}, {Namespace: PathNamespaceCollectionRoot, RootIndex: 1, Root: "/tmp", Alias: "@root1"}}}
}

func mustObservePlan(t *testing.T) *pipeline.FrozenPlan {
	t.Helper()
	plan, err := pipeline.Build(context.Background(), observeBuildRequest(t, observePipelineYAML, observePipelineYAML))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return plan
}

func observeBuildRequest(t *testing.T, baseYAML, headYAML string) pipeline.BuildRequest {
	t.Helper()
	trusted := observeTrustedLoad(t, baseYAML, headYAML)
	return pipeline.BuildRequest{RunID: "run-0001", CreatedAt: observeTestCreatedAt, Trusted: trusted,
		BaseSource: observeSourceSnapshot(model.RevisionKindBase, observeTestBaseCommit, observeTestBaseTree, "sha256:3333333333333333333333333333333333333333333333333333333333333333", "sha256:4444444444444444444444444444444444444444444444444444444444444444"),
		HeadSource: observeSourceSnapshot(model.RevisionKindHead, observeTestHeadCommit, observeTestHeadTree, "sha256:5555555555555555555555555555555555555555555555555555555555555555", "sha256:6666666666666666666666666666666666666666666666666666666666666666"),
		Platform:   observePlatformConstraints()}
}

func observeTrustedLoad(t *testing.T, baseYAML, headYAML string) config.TrustedLoadResult {
	t.Helper()
	base := model.CommitRef{Kind: model.RevisionKindBase, Repository: "https://example.invalid/org/repo.git", Ref: "refs/heads/master", CommitID: observeTestBaseCommit, TreeDigest: model.Digest(observeTestBaseTree)}
	head := model.CommitRef{Kind: model.RevisionKindHead, Repository: "https://example.invalid/org/repo.git", Ref: "refs/pull/7/head", CommitID: observeTestHeadCommit, TreeDigest: model.Digest(observeTestHeadTree)}
	source := &observeMemoryRevisionSource{files: map[string]config.RevisionFile{}}
	source.files[observeRevisionKey(base, config.PipelinePath)] = config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte(baseYAML), ObjectID: strings.Repeat("a", 40)}
	source.files[observeRevisionKey(head, config.PipelinePath)] = config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte(headYAML), ObjectID: strings.Repeat("b", 40)}
	trusted, err := config.LoadTrusted(context.Background(), source, config.TrustedLoadRequest{Base: base, Head: head})
	if err != nil {
		t.Fatalf("LoadTrusted() error = %v", err)
	}
	return trusted
}

type observeMemoryRevisionSource struct {
	files map[string]config.RevisionFile
}

func (s *observeMemoryRevisionSource) ReadFile(ctx context.Context, revision model.CommitRef, p string, maxBytes int64) (config.RevisionFile, error) {
	if err := ctx.Err(); err != nil {
		return config.RevisionFile{}, err
	}
	file, ok := s.files[observeRevisionKey(revision, p)]
	if !ok {
		return config.RevisionFile{}, fs.ErrNotExist
	}
	if int64(len(file.Data)) > maxBytes {
		return config.RevisionFile{}, config.ErrRevisionFileTooLarge
	}
	file.Data = append([]byte(nil), file.Data...)
	return file, nil
}

func observeRevisionKey(ref model.CommitRef, p string) string {
	return string(ref.Kind) + ":" + ref.CommitID + ":" + p
}

func observeSourceSnapshot(kind model.RevisionKind, commit, tree, treeDigest, manifestDigest string) pipeline.SourceSnapshot {
	return pipeline.SourceSnapshot{RevisionKind: kind, CommitID: commit, TreeID: tree, ObjectFormat: pipeline.ObjectFormatSHA1, MaterializedTreeDigest: model.Digest(treeDigest), MaterializationManifestDigest: model.Digest(manifestDigest), Summary: pipeline.SourceSummary{DirectoryCount: 3, RegularFileCount: 4, ExecutableFileCount: 1, SymlinkCount: 1, GitlinkCount: 1, LFSPointerCount: 1, TotalMaterializedFileBytes: 1234, SkippedEntryCount: 1}, Limitations: []pipeline.SourceLimitation{{Code: "skipped-gitlink", Path: "vendor/submodule", Summary: "gitlink was reported but not traversed or materialized"}}}
}

func observePlatformConstraints() pipeline.PlatformConstraints {
	return pipeline.PlatformConstraints{MaxCPU: config.MaxCPU, MaxMemoryBytes: config.MaxMemoryBytes, MaxDiskBytes: config.MaxDiskBytes, MaxProcessCount: config.MaxProcessCount, MaxGlobalTimeoutMillis: config.MaxTimeoutMillis, MaxScenarioTimeoutMillis: config.MaxTimeoutMillis, MaxScenarioCount: config.MaxScenarioCount, MaxRepetitions: config.MaxRepetitions, MaxFilesystemRootCount: config.MaxFilesystemRootCount, MaxArtifactCount: config.MaxArtifactCount, MaxArtifactBytes: config.MaxArtifactBytes, MaxLogBytesPerStream: config.MaxLogBytesPerStream, MaxPlanJSONBytes: pipeline.MaxPlanJSONBytes, RequiredNetworkMode: model.NetworkModeDeny}
}

func intPtr(v int) *int { return &v }

const observePipelineYAML = `apiVersion: glassroot.dev/v1alpha1
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
      run: |
        echo "literal shell text stays data"
        go test ./...
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
