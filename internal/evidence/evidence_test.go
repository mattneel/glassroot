package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/pipeline"
	"github.com/mattneel/glassroot/internal/runner"
	"github.com/mattneel/glassroot/internal/runner/fake"
)

const (
	testBaseCommit = "1111111111111111111111111111111111111111"
	testHeadCommit = "2222222222222222222222222222222222222222"
	testBaseTree   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testHeadTree   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

var testCreatedAt = time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)

func TestFakeRunnerBundleIntegrationPublishesDeterministicDirectory(t *testing.T) {
	plan := mustPlan(t)
	parent := t.TempDir()
	writer := mustWriter(t, parent, DefaultLimits())
	session, err := writer.Begin(context.Background(), plan)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if session.stagingPathForTest() == "" {
		t.Fatal("test hook should expose staging path before commit")
	}
	if session.finalPathForTest() != "" {
		t.Fatal("final path must not be exposed before commit")
	}

	backend := mustFake(t, fakeProgramForPlan(plan))
	result, err := runner.ExecutePlan(context.Background(), plan, backend, runner.SyntheticTestRequirements(), runner.DefaultLimits(), session)
	if err != nil {
		t.Fatalf("ExecutePlan() error = %v", err)
	}
	bundle, err := session.Commit(context.Background(), Complete(result))
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if bundle.Path == "" || !strings.HasPrefix(bundle.Path, parent+string(os.PathSeparator)) {
		t.Fatalf("published path invalid: %+v", bundle)
	}
	if _, err := os.Stat(session.stagingPathForTest()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("staging path should be renamed away, stat err=%v", err)
	}
	assertMode(t, bundle.Path, 0o700)
	for _, rel := range []string{
		"plan.json",
		"execution.json",
		"manifest.json",
		"attempts/base/test/repetition-0001/events.jsonl",
		"attempts/base/test/repetition-0001/result.json",
	} {
		assertMode(t, filepath.Join(bundle.Path, filepath.FromSlash(rel)), 0o600)
	}

	planBytes := readFile(t, bundle.Path, "plan.json")
	if !bytes.Equal(planBytes, plan.JSON()) {
		t.Fatalf("plan.json was remarshal drift\nwant=%s\ngot=%s", plan.JSON(), planBytes)
	}
	manifestBytes := readFile(t, bundle.Path, "manifest.json")
	if !json.Valid(manifestBytes) {
		t.Fatalf("manifest invalid JSON: %s", manifestBytes)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.SchemaVersion != model.SchemaVersionEvidenceManifestV1Alpha1 || manifest.BundleFormatVersion != BundleFormatV1Alpha1 {
		t.Fatalf("manifest versions mismatch: %+v", manifest)
	}
	if !manifest.ExecutionComplete || !manifest.EvidenceComplete || !manifest.BundleTransactionValid {
		t.Fatalf("complete bundle flags mismatch: %+v", manifest)
	}
	if manifest.PlanDigest != plan.Digest() || manifest.RunID != "run-0001" {
		t.Fatalf("plan/run binding mismatch: %+v", manifest)
	}
	if manifest.ManifestDigest != "" {
		t.Fatal("manifest must not self-list or contain its own digest")
	}
	if bundle.ManifestDigest != manifestDigest(manifestBytes) {
		t.Fatalf("manifest digest mismatch: result=%s recomputed=%s", bundle.ManifestDigest, manifestDigest(manifestBytes))
	}
	if string(bundle.ManifestDigest) != strings.TrimSpace(string(readTestdata(t, "bundle.digest"))) {
		t.Fatalf("golden manifest digest drift: %s", bundle.ManifestDigest)
	}
	if !bytes.Equal(manifestBytes, readTestdata(t, "manifest.json")) {
		t.Fatalf("manifest golden mismatch\nwant=%s\ngot=%s", readTestdata(t, "manifest.json"), manifestBytes)
	}
	if len(manifest.Attempts) != 4 {
		t.Fatalf("manifest attempts = %d", len(manifest.Attempts))
	}
	if got := logicalTree(t, bundle.Path); !reflect.DeepEqual(got, expectedCompleteBundleTree()) {
		t.Fatalf("logical tree mismatch\nwant=%v\ngot=%v", expectedCompleteBundleTree(), got)
	}
	for _, entry := range manifest.Entries {
		if entry.Path == "manifest.json" {
			t.Fatal("manifest self-listed")
		}
		data := readFile(t, bundle.Path, entry.Path)
		if got := testDigestBytes(data); got != entry.Digest {
			t.Fatalf("entry digest mismatch for %s: %s != %s", entry.Path, got, entry.Digest)
		}
		if int64(len(data)) != entry.SizeBytes {
			t.Fatalf("entry size mismatch for %s", entry.Path)
		}
	}
	execution := readFile(t, bundle.Path, "execution.json")
	if bytes.Contains(execution, []byte("go test ./...")) || bytes.Contains(execution, []byte("/tmp/glassroot")) {
		t.Fatalf("execution result leaked command/host path: %s", execution)
	}
	if !bytes.Contains(execution, []byte(`"syntheticEvidence":true`)) || bytes.Contains(readFile(t, bundle.Path, "plan.json"), []byte(`"syntheticEvidence":true`)) {
		t.Fatalf("actual capabilities should be outside plan: execution=%s", execution)
	}
}

func TestCommitPreservesEmptyLimitationsAsArrays(t *testing.T) {
	plan := mustPlan(t)
	writer := mustWriter(t, t.TempDir(), DefaultLimits())
	session, err := writer.Begin(context.Background(), plan)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	backend := mustFake(t, fakeProgramForPlan(plan))
	result, err := runner.ExecutePlan(context.Background(), plan, backend, runner.SyntheticTestRequirements(), runner.DefaultLimits(), session)
	if err != nil {
		t.Fatalf("ExecutePlan() error = %v", err)
	}
	result.Limitations = []model.Limitation{}
	for i := range result.Attempts {
		result.Attempts[i].Limitations = []model.Limitation{}
		result.Attempts[i].Outcome.Limitations = []model.Limitation{}
	}
	bundle, err := session.Commit(context.Background(), Complete(result))
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	execution := readFile(t, bundle.Path, "execution.json")
	if bytes.Contains(execution, []byte(`"limitations":null`)) {
		t.Fatalf("execution limitations serialized as null: %s", execution)
	}
	if !bytes.Contains(execution, []byte(`"limitations":[]`)) {
		t.Fatalf("execution limitations did not serialize as arrays: %s", execution)
	}
	for _, rel := range []string{
		"attempts/base/build/repetition-0001/result.json",
		"attempts/base/test/repetition-0001/result.json",
		"attempts/head/build/repetition-0001/result.json",
		"attempts/head/test/repetition-0001/result.json",
	} {
		attempt := readFile(t, bundle.Path, rel)
		if bytes.Contains(attempt, []byte(`"limitations":null`)) || !bytes.Contains(attempt, []byte(`"limitations":[]`)) {
			t.Fatalf("%s limitations did not serialize as an empty array: %s", rel, attempt)
		}
	}
}

func TestLogsArtifactsAndIncompleteEvidenceAreExplicit(t *testing.T) {
	plan := mustPlan(t)
	parent := t.TempDir()
	limits := DefaultLimits()
	limits.MaxLogBytesPerStream = 5
	limits.MaxSingleArtifactBytes = 4
	writer := mustWriter(t, parent, limits)
	session, err := writer.Begin(context.Background(), plan)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	key := AttemptKey{Revision: model.RevisionKindBase, ScenarioID: "test", Repetition: 1}
	logCap, err := session.OpenLog(context.Background(), key, LogStreamStdout)
	if err != nil {
		t.Fatalf("OpenLog() error = %v", err)
	}
	if n, err := logCap.Write([]byte("abc\x00\xff\x1b[31m")); err != nil || n != len([]byte("abc\x00\xff\x1b[31m")) {
		t.Fatalf("log Write() = %d,%v", n, err)
	}
	if err := logCap.Close(); err != nil {
		t.Fatalf("log Close() error = %v", err)
	}
	if _, err := session.OpenLog(context.Background(), key, LogStreamStdout); err == nil {
		t.Fatal("duplicate log stream should fail")
	}
	artifact, err := session.AddArtifact(context.Background(), ArtifactInput{Attempt: key, LogicalPath: "/workspace/bin/too-large", MaxBytes: 4, Reader: strings.NewReader("12345")})
	if err != nil {
		t.Fatalf("AddArtifact over limit should record omission, got error %v", err)
	}
	if artifact.Disposition != ArtifactDispositionOmittedLimit || artifact.Digest != "" {
		t.Fatalf("over-limit artifact should be omitted explicitly: %+v", artifact)
	}
	backend := mustFake(t, fakeProgramForPlan(plan))
	result, err := runner.ExecutePlan(context.Background(), plan, backend, runner.SyntheticTestRequirements(), runner.DefaultLimits(), session)
	if err != nil {
		t.Fatalf("ExecutePlan() error = %v", err)
	}
	_, err = session.Commit(context.Background(), Complete(result))
	assertEvidenceError(t, err, CodeCompletionInvalid)
	bundle, err := session.Commit(context.Background(), Incomplete(result, FailureRecord{Code: "capture-limit", Stage: "capture", Message: "log truncated and artifact omitted", Category: FailureCategoryCaptureLimit}))
	if err != nil {
		t.Fatalf("incomplete Commit() error = %v", err)
	}
	manifestBytes := readFile(t, bundle.Path, "manifest.json")
	if !bytes.Contains(manifestBytes, []byte(`"evidenceComplete":false`)) || !bytes.Contains(manifestBytes, []byte(`"omitted-limit"`)) || !bytes.Contains(manifestBytes, []byte(`"truncated"`)) {
		t.Fatalf("incomplete evidence not explicit: %s", manifestBytes)
	}
	stored := readFile(t, bundle.Path, "attempts/base/test/repetition-0001/stdout.log")
	if !bytes.Equal(stored, []byte("abc\x00\xff")) {
		t.Fatalf("stored raw bounded log prefix mismatch: %q", stored)
	}
}

func TestEventSinkRejectsOutOfOrderOrForgedEventsWithoutPartialLine(t *testing.T) {
	plan := mustPlan(t)
	writer := mustWriter(t, t.TempDir(), DefaultLimits())
	session, err := writer.Begin(context.Background(), plan)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	events := goldenEvents(t)
	forged := events[0]
	forged.RunID = "other"
	assertEvidenceError(t, session.Emit(context.Background(), forged), CodeInvalidEvent)
	if session.State() != StateFailed {
		t.Fatalf("invalid event should fail session, got %s", session.State())
	}
	if err := session.Abort(); err != nil {
		t.Fatalf("Abort() error = %v", err)
	}

	session, err = writer.Begin(context.Background(), plan)
	if err != nil {
		t.Fatalf("Begin2() error = %v", err)
	}
	if err := session.Emit(context.Background(), events[0]); err != nil {
		t.Fatalf("first event error = %v", err)
	}
	gap := events[2]
	assertEvidenceError(t, session.Emit(context.Background(), gap), CodeEventOrder)
	p := filepath.Join(session.stagingPathForTest(), "attempts/base/test/repetition-0001/events.jsonl")
	data := readHostFile(t, p)
	if bytes.Count(data, []byte("\n")) != 1 || !json.Valid(bytes.TrimSuffix(data, []byte("\n"))) {
		t.Fatalf("event gap left partial line: %q", data)
	}
	_ = session.Abort()
}

func TestArtifactPhysicalPathIsDigestDerivedAndLogicalPathIsData(t *testing.T) {
	plan := mustPlan(t)
	writer := mustWriter(t, t.TempDir(), DefaultLimits())
	session, err := writer.Begin(context.Background(), plan)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	key := AttemptKey{Revision: model.RevisionKindHead, ScenarioID: "build", Repetition: 1}
	content := []byte("artifact bytes")
	res1, err := session.AddArtifact(context.Background(), ArtifactInput{Attempt: key, LogicalPath: "/workspace/bin/glassroot", Reader: bytes.NewReader(content)})
	if err != nil {
		t.Fatalf("AddArtifact() error = %v", err)
	}
	res2, err := session.AddArtifact(context.Background(), ArtifactInput{Attempt: key, LogicalPath: "/workspace/other/glassroot", Reader: bytes.NewReader(content)})
	if err != nil {
		t.Fatalf("AddArtifact duplicate bytes error = %v", err)
	}
	if res1.ObjectPath != res2.ObjectPath || !strings.HasPrefix(res1.ObjectPath, "objects/sha256/") || strings.Contains(res1.ObjectPath, "workspace") {
		t.Fatalf("object path should be digest-derived and deduped: %+v %+v", res1, res2)
	}
	if _, err := session.AddArtifact(context.Background(), ArtifactInput{Attempt: key, LogicalPath: "/workspace/bin/glassroot", Reader: bytes.NewReader(content)}); err == nil {
		t.Fatal("duplicate logical path should fail")
	}
	if _, err := os.Lstat(filepath.Join(session.stagingPathForTest(), res1.ObjectPath)); err != nil {
		t.Fatalf("object not stored at digest path: %v", err)
	}
	assertMode(t, filepath.Join(session.stagingPathForTest(), filepath.FromSlash(res1.ObjectPath)), 0o600)
	_ = session.Abort()
}

func TestAbortAndInjectedPublicationFailureCleanOnlyStaging(t *testing.T) {
	parent := t.TempDir()
	sibling := filepath.Join(parent, "keep")
	if err := os.Mkdir(sibling, 0o700); err != nil {
		t.Fatal(err)
	}
	plan := mustPlan(t)
	writer := mustWriter(t, parent, DefaultLimits())
	session, err := writer.Begin(context.Background(), plan)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	staging := session.stagingPathForTest()
	if err := session.Abort(); err != nil {
		t.Fatalf("Abort() error = %v", err)
	}
	if _, err := os.Stat(staging); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("staging remains after Abort: %v", err)
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Fatalf("unrelated sibling removed: %v", err)
	}

	writer = mustWriterWithHooks(t, parent, DefaultLimits(), testHooks{failFileSync: true})
	_, err = writer.Begin(context.Background(), plan)
	assertEvidenceError(t, err, CodeSyncFailed)
	assertOnlySibling(t, parent, "keep")

	for _, tc := range []struct {
		name  string
		hooks testHooks
		code  ErrorCode
	}{
		{name: "staging-dir-sync", hooks: testHooks{failStagingSync: true}, code: CodeSyncFailed},
		{name: "rename", hooks: testHooks{failRename: true}, code: CodePublishFailed},
		{name: "parent-dir-sync", hooks: testHooks{failParentSync: true}, code: CodeSyncFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer = mustWriterWithHooks(t, parent, DefaultLimits(), tc.hooks)
			session, err = writer.Begin(context.Background(), plan)
			if err != nil {
				t.Fatalf("Begin() error = %v", err)
			}
			result, err := runner.ExecutePlan(context.Background(), plan, mustFake(t, fakeProgramForPlan(plan)), runner.SyntheticTestRequirements(), runner.DefaultLimits(), session)
			if err != nil {
				t.Fatalf("ExecutePlan() error = %v", err)
			}
			_, err = session.Commit(context.Background(), Complete(result))
			assertEvidenceError(t, err, tc.code)
			assertOnlySibling(t, parent, "keep")
		})
	}
}

func TestPathAndManifestFuzzHelpers(t *testing.T) {
	for _, path := range []string{"plan.json", "attempts/base/test/repetition-0001/events.jsonl", "objects/sha256/ab/" + strings.Repeat("a", 64)} {
		if err := ValidateEvidenceEntryPath(path); err != nil {
			t.Fatalf("valid entry path rejected %q: %v", path, err)
		}
	}
	for _, path := range []string{"", "/abs", "../x", "a//b", "a/./b", "a\\b", "a/\x1bb"} {
		if err := ValidateEvidenceEntryPath(path); err == nil {
			t.Fatalf("invalid entry path accepted %q", path)
		}
	}
	if err := ValidateLogicalArtifactPath("/workspace/bin/glassroot"); err != nil {
		t.Fatalf("logical artifact path rejected: %v", err)
	}
	if err := ValidateLogicalArtifactPath("/workspace/../secret"); err == nil {
		t.Fatal("traversal logical artifact path accepted")
	}
}

func FuzzValidateEvidenceEntryPath(f *testing.F) {
	f.Add("plan.json")
	f.Add("../x")
	f.Add("a/\x1bb")
	f.Fuzz(func(t *testing.T, path string) { _ = ValidateEvidenceEntryPath(path) })
}

func FuzzEncodeEventLine(f *testing.F) {
	f.Add(uint64(1), string(model.ObservationKindObserverWarning), "warn")
	f.Add(uint64(0), "unknown", "\x1b")
	f.Fuzz(func(t *testing.T, seq uint64, kind, msg string) {
		event := model.ObservationEvent{SchemaVersion: model.SchemaVersionObservationEventV1Alpha1, ID: "evt-" + strings.Repeat("1", 64), RunID: "run-0001", Revision: model.RevisionKindBase, ScenarioID: "test", Repetition: 1, SequenceNumber: int64(seq), ObservedAt: testCreatedAt, Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKind(kind), ObserverWarning: &model.ObserverWarningObservation{Code: "fuzz", Message: msg, Limitations: []model.Limitation{}}}
		_, _ = encodeEventLine(event, DefaultLimits())
	})
}

func FuzzValidateLogicalArtifactPath(f *testing.F) {
	f.Add("/workspace/bin/glassroot")
	f.Add("../x")
	f.Add("/workspace/\x1b")
	f.Fuzz(func(t *testing.T, path string) { _ = ValidateLogicalArtifactPath(path) })
}

func FuzzNormalizeManifest(f *testing.F) {
	f.Add("plan.json", "plan", "sha256:"+strings.Repeat("1", 64), int64(1))
	f.Add("../x", "bad", "no", int64(-1))
	f.Fuzz(func(t *testing.T, path, role, digest string, size int64) {
		m := Manifest{SchemaVersion: model.SchemaVersionEvidenceManifestV1Alpha1, ID: "bundle-run-0001", RunID: "run-0001", PlanDigest: model.Digest("sha256:" + strings.Repeat("2", 64)), Entries: []ManifestEntry{{Path: path, Role: EntryRole(role), Digest: model.Digest(digest), SizeBytes: size}}}
		_, _ = normalizeManifest(m, DefaultLimits())
	})
}

func mustWriter(t *testing.T, parent string, limits Limits) *Writer {
	t.Helper()
	w, err := NewWriter(parent, limits)
	if err != nil {
		t.Fatalf("NewWriter() error = %v", err)
	}
	return w
}

func mustWriterWithHooks(t *testing.T, parent string, limits Limits, hooks testHooks) *Writer {
	t.Helper()
	w, err := newWriterForTest(parent, limits, hooks)
	if err != nil {
		t.Fatalf("newWriterForTest() error = %v", err)
	}
	return w
}

func mustFake(t *testing.T, program fake.Program) *fake.Runner {
	t.Helper()
	backend, err := fake.New(program)
	if err != nil {
		t.Fatalf("fake.New() error = %v", err)
	}
	return backend
}

func fakeProgramForPlan(plan *pipeline.FrozenPlan) fake.Program {
	return fake.Program{PlanDigest: plan.Digest(), Attempts: []fake.AttemptScript{
		{Revision: model.RevisionKindHead, ScenarioID: "test", Repetition: 1, Events: []fake.SyntheticEvent{{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindNetworkConnection, Network: &model.NetworkObservation{Operation: "connect", Protocol: "tcp", DestinationHost: "canary.example.invalid", DestinationPort: 443, ResolvedAddresses: []string{}, Result: "denied"}}}}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusFailed, ExitCode: intPtr(2), DurationMillis: 25}},
		{Revision: model.RevisionKindBase, ScenarioID: "test", Repetition: 1, Events: []fake.SyntheticEvent{{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindProcessStart, Process: &model.ProcessObservation{Operation: "start", ProcessID: 1, ExecutablePath: "/synthetic/go", Arguments: []string{"go", "test"}, Environment: []model.EnvEntry{}, DurationMillis: 0}}}}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 20}},
		{Revision: model.RevisionKindBase, ScenarioID: "build", Repetition: 1, Events: []fake.SyntheticEvent{{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindFilesystemWrite, Filesystem: &model.FilesystemObservation{Operation: "write", Path: "/workspace/bin/glassroot", Digest: "sha256:9999999999999999999999999999999999999999999999999999999999999999", SizeBytes: 42}}}}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 30}},
		{Revision: model.RevisionKindHead, ScenarioID: "build", Repetition: 1, Events: []fake.SyntheticEvent{{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindArtifactActivity, Artifact: &model.ArtifactObservation{Operation: "create", ArtifactID: "artifact-head-bin", Path: "/workspace/bin/glassroot", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SizeBytes: 64, Executable: true, SourceEventIDs: []string{}}}}}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 35}},
	}}
}

func goldenEvents(t *testing.T) []model.ObservationEvent {
	t.Helper()
	data := readHostFile(t, filepath.Join("..", "runner", "fake", "testdata", "v1alpha1", "events.jsonl"))
	var events []model.ObservationEvent
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var event model.ObservationEvent
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatalf("decode golden event: %v", err)
		}
		events = append(events, event)
	}
	return events
}

func mustPlan(t *testing.T) *pipeline.FrozenPlan {
	t.Helper()
	plan, err := pipeline.Build(context.Background(), validBuildRequest(t, validPipelineYAML, validPipelineYAML))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return plan
}

func validBuildRequest(t *testing.T, baseYAML, headYAML string) pipeline.BuildRequest {
	t.Helper()
	trusted := loadTrustedForTest(t, baseYAML, headYAML)
	return pipeline.BuildRequest{RunID: "run-0001", CreatedAt: testCreatedAt, Trusted: trusted,
		BaseSource: validSourceSnapshot(model.RevisionKindBase, testBaseCommit, testBaseTree, "sha256:3333333333333333333333333333333333333333333333333333333333333333", "sha256:4444444444444444444444444444444444444444444444444444444444444444"),
		HeadSource: validSourceSnapshot(model.RevisionKindHead, testHeadCommit, testHeadTree, "sha256:5555555555555555555555555555555555555555555555555555555555555555", "sha256:6666666666666666666666666666666666666666666666666666666666666666"),
		Platform:   defaultPlatformConstraintsForTest()}
}

func loadTrustedForTest(t *testing.T, baseYAML, headYAML string) config.TrustedLoadResult {
	t.Helper()
	base := model.CommitRef{Kind: model.RevisionKindBase, Repository: "https://example.invalid/org/repo.git", Ref: "refs/heads/master", CommitID: testBaseCommit, TreeDigest: model.Digest(testBaseTree)}
	head := model.CommitRef{Kind: model.RevisionKindHead, Repository: "https://example.invalid/org/repo.git", Ref: "refs/pull/7/head", CommitID: testHeadCommit, TreeDigest: model.Digest(testHeadTree)}
	source := &memoryRevisionSource{files: map[string]config.RevisionFile{}}
	source.files[key(base, config.PipelinePath)] = config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte(baseYAML), ObjectID: strings.Repeat("a", 40)}
	source.files[key(head, config.PipelinePath)] = config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte(headYAML), ObjectID: strings.Repeat("b", 40)}
	trusted, err := config.LoadTrusted(context.Background(), source, config.TrustedLoadRequest{Base: base, Head: head})
	if err != nil {
		t.Fatalf("LoadTrusted() error = %v", err)
	}
	return trusted
}

type memoryRevisionSource struct {
	files map[string]config.RevisionFile
}

func (s *memoryRevisionSource) ReadFile(ctx context.Context, revision model.CommitRef, path string, maxBytes int64) (config.RevisionFile, error) {
	if err := ctx.Err(); err != nil {
		return config.RevisionFile{}, err
	}
	file, ok := s.files[key(revision, path)]
	if !ok {
		return config.RevisionFile{}, fs.ErrNotExist
	}
	if int64(len(file.Data)) > maxBytes {
		return config.RevisionFile{}, config.ErrRevisionFileTooLarge
	}
	file.Data = append([]byte(nil), file.Data...)
	return file, nil
}

func key(ref model.CommitRef, path string) string {
	return string(ref.Kind) + ":" + ref.CommitID + ":" + path
}

func validSourceSnapshot(kind model.RevisionKind, commit, tree, treeDigest, manifestDigest string) pipeline.SourceSnapshot {
	return pipeline.SourceSnapshot{RevisionKind: kind, CommitID: commit, TreeID: tree, ObjectFormat: pipeline.ObjectFormatSHA1, MaterializedTreeDigest: model.Digest(treeDigest), MaterializationManifestDigest: model.Digest(manifestDigest), Summary: pipeline.SourceSummary{DirectoryCount: 3, RegularFileCount: 4, ExecutableFileCount: 1, SymlinkCount: 1, GitlinkCount: 1, LFSPointerCount: 1, TotalMaterializedFileBytes: 1234, SkippedEntryCount: 1}, Limitations: []pipeline.SourceLimitation{{Code: "skipped-gitlink", Path: "vendor/submodule", Summary: "gitlink was reported but not traversed or materialized"}}}
}

func defaultPlatformConstraintsForTest() pipeline.PlatformConstraints {
	return pipeline.PlatformConstraints{MaxCPU: config.MaxCPU, MaxMemoryBytes: config.MaxMemoryBytes, MaxDiskBytes: config.MaxDiskBytes, MaxProcessCount: config.MaxProcessCount, MaxGlobalTimeoutMillis: config.MaxTimeoutMillis, MaxScenarioTimeoutMillis: config.MaxTimeoutMillis, MaxScenarioCount: config.MaxScenarioCount, MaxRepetitions: config.MaxRepetitions, MaxFilesystemRootCount: config.MaxFilesystemRootCount, MaxArtifactCount: config.MaxArtifactCount, MaxArtifactBytes: config.MaxArtifactBytes, MaxLogBytesPerStream: config.MaxLogBytesPerStream, MaxPlanJSONBytes: pipeline.MaxPlanJSONBytes, RequiredNetworkMode: model.NetworkModeDeny}
}

func intPtr(v int) *int { return &v }

func expectedCompleteBundleTree() []string {
	return []string{
		"attempts/", "attempts/base/", "attempts/base/build/", "attempts/base/build/repetition-0001/", "attempts/base/build/repetition-0001/events.jsonl", "attempts/base/build/repetition-0001/result.json", "attempts/base/test/", "attempts/base/test/repetition-0001/", "attempts/base/test/repetition-0001/events.jsonl", "attempts/base/test/repetition-0001/result.json", "attempts/head/", "attempts/head/build/", "attempts/head/build/repetition-0001/", "attempts/head/build/repetition-0001/events.jsonl", "attempts/head/build/repetition-0001/result.json", "attempts/head/test/", "attempts/head/test/repetition-0001/", "attempts/head/test/repetition-0001/events.jsonl", "attempts/head/test/repetition-0001/result.json", "execution.json", "manifest.json", "objects/", "objects/sha256/", "plan.json",
	}
}

func logicalTree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	if err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			rel += "/"
		}
		out = append(out, rel)
		return nil
	}); err != nil {
		t.Fatalf("walk tree: %v", err)
	}
	return out
}

func readFile(t *testing.T, root, rel string) []byte {
	t.Helper()
	return readHostFile(t, filepath.Join(root, filepath.FromSlash(rel)))
}
func readHostFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}
func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	return readHostFile(t, filepath.Join("testdata", "v1alpha1", name))
}

func assertMode(t *testing.T, path string, mode fs.FileMode) {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if st.Mode().Perm() != mode {
		t.Fatalf("mode %s = %o, want %o", path, st.Mode().Perm(), mode)
	}
}
func assertOnlySibling(t *testing.T, parent, name string) {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("read parent: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != name {
		var names []string
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("unexpected parent entries after cleanup: %v", names)
	}
}
func testDigestBytes(data []byte) model.Digest {
	sum := sha256.Sum256(data)
	return model.Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func assertEvidenceError(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected evidence error %s, got nil", code)
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error %T is not *evidence.Error: %v", err, err)
	}
	if e.Code != code {
		t.Fatalf("error code = %s, want %s; err=%v", e.Code, code, err)
	}
	if strings.ContainsAny(err.Error(), "\x1b\r\n") {
		t.Fatalf("error contains raw control characters: %q", err.Error())
	}
}

var _ io.Writer = (*LogCapture)(nil)

const validPipelineYAML = `apiVersion: glassroot.dev/v1alpha1
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
