package runner_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
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

func TestExecutePlanWithFakeRunnerEmitsDeterministicSyntheticEvents(t *testing.T) {
	plan := mustPlan(t, validPipelineYAML, validPipelineYAML)
	program := fake.Program{
		PlanDigest: plan.Digest(),
		Attempts: []fake.AttemptScript{
			{
				Revision:   model.RevisionKindHead,
				ScenarioID: "test",
				Repetition: 1,
				Events:     []fake.SyntheticEvent{{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindNetworkConnection, Network: &model.NetworkObservation{Operation: "connect", Protocol: "tcp", DestinationHost: "canary.example.invalid", DestinationPort: 443, ResolvedAddresses: []string{}, Result: "denied"}}}},
				Outcome:    runner.AttemptOutcome{Status: runner.AttemptStatusFailed, ExitCode: intPtr(2), DurationMillis: 25},
			},
			{
				Revision:   model.RevisionKindBase,
				ScenarioID: "test",
				Repetition: 1,
				Events:     []fake.SyntheticEvent{{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindProcessStart, Process: &model.ProcessObservation{Operation: "start", ProcessID: 1, ExecutablePath: "/synthetic/go", Arguments: []string{"go", "test"}, Environment: []model.EnvEntry{}, DurationMillis: 0}}}},
				Outcome:    runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 20},
			},
			{Revision: model.RevisionKindBase, ScenarioID: "build", Repetition: 1, Events: []fake.SyntheticEvent{{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindFilesystemWrite, Filesystem: &model.FilesystemObservation{Operation: "write", Path: "/workspace/bin/glassroot", Digest: "sha256:9999999999999999999999999999999999999999999999999999999999999999", SizeBytes: 42}}}}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 30}},
			{Revision: model.RevisionKindHead, ScenarioID: "build", Repetition: 1, Events: []fake.SyntheticEvent{{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindArtifactActivity, Artifact: &model.ArtifactObservation{Operation: "create", ArtifactID: "artifact-head-bin", Path: "/workspace/bin/glassroot", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SizeBytes: 64, Executable: true, SourceEventIDs: []string{}}}}}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 35}},
		},
	}
	backend, err := fake.New(program)
	if err != nil {
		t.Fatalf("fake.New() error = %v", err)
	}
	sink := &collectingSink{limit: 100}
	originalJSON := append([]byte(nil), plan.JSON()...)
	originalDigest := plan.Digest()
	result, err := runner.ExecutePlan(context.Background(), plan, backend, runner.SyntheticTestRequirements(), runner.DefaultLimits(), sink)
	if err != nil {
		t.Fatalf("ExecutePlan() error = %v", err)
	}
	if !result.Complete {
		t.Fatalf("result not complete: %+v", result)
	}
	if result.PlanDigest != plan.Digest() || result.TotalEmittedEvents != uint64(len(sink.events)) {
		t.Fatalf("result identity/count mismatch: %+v events=%d", result, len(sink.events))
	}
	if result.Runner.Name != "fake" || !result.Runner.SyntheticEvidence || result.Runner.ExecutesTargetCode || result.Runner.ProcessEventCollection {
		t.Fatalf("fake capabilities not truthful: %+v", result.Runner)
	}
	if len(result.Limitations) < 2 || !strings.Contains(result.Limitations[0].Summary, "No target code") {
		t.Fatalf("synthetic limitations missing: %+v", result.Limitations)
	}
	if len(result.Attempts) != 4 {
		t.Fatalf("attempt count = %d", len(result.Attempts))
	}
	wantOrder := []struct {
		rev      model.RevisionKind
		scenario string
	}{{model.RevisionKindBase, "test"}, {model.RevisionKindBase, "build"}, {model.RevisionKindHead, "test"}, {model.RevisionKindHead, "build"}}
	for i, want := range wantOrder {
		if result.Attempts[i].Revision != want.rev || result.Attempts[i].ScenarioID != want.scenario || result.Attempts[i].Repetition != 1 {
			t.Fatalf("attempt[%d] order mismatch: %+v", i, result.Attempts[i])
		}
	}
	if result.Attempts[2].Outcome.Status != runner.AttemptStatusFailed || result.Attempts[2].Outcome.ExitCode == nil || *result.Attempts[2].Outcome.ExitCode != 2 {
		t.Fatalf("target failure should be data and preserve exit code: %+v", result.Attempts[2].Outcome)
	}
	for i, ev := range sink.events {
		if ev.SchemaVersion != model.SchemaVersionObservationEventV1Alpha1 || ev.Source != model.ObservationSourceSyntheticTestGenerated {
			t.Fatalf("event[%d] envelope/source mismatch: %+v", i, ev)
		}
		if ev.RunID != "run-0001" || ev.Repetition != 1 || ev.SequenceNumber != int64(i+1) || !strings.HasPrefix(ev.ID, "evt-") {
			t.Fatalf("event[%d] identity mismatch: %+v", i, ev)
		}
	}
	if len(sink.events) != 12 { // start + program + complete for four attempts
		t.Fatalf("event count = %d", len(sink.events))
	}
	golden := jsonLines(t, sink.events)
	if !bytes.Equal(golden, readRunnerTestdata(t, "events.jsonl")) {
		t.Fatalf("golden event stream mismatch\nwant:\n%s\ngot:\n%s", readRunnerTestdata(t, "events.jsonl"), golden)
	}
	if !bytes.Equal(plan.JSON(), originalJSON) || plan.Digest() != originalDigest {
		t.Fatal("FrozenPlan mutated by execution")
	}

	sink2 := &collectingSink{limit: 100}
	result2, err := runner.ExecutePlan(context.Background(), plan, backend, runner.SyntheticTestRequirements(), runner.DefaultLimits(), sink2)
	if err != nil {
		t.Fatalf("second ExecutePlan() error = %v", err)
	}
	if !reflect.DeepEqual(result, result2) || !bytes.Equal(jsonLines(t, sink.events), jsonLines(t, sink2.events)) {
		t.Fatal("fake execution is not deterministic")
	}
}

func TestFakeRunnerRejectedForWorkloadAndMismatchBeforeEvents(t *testing.T) {
	plan := mustPlan(t, validPipelineYAML, validPipelineYAML)
	backend := mustFake(t, fakeProgramForPlan(plan))
	sink := &collectingSink{limit: 10}
	_, err := runner.ExecutePlan(context.Background(), plan, backend, runner.WorkloadRequirements([]model.IsolationTier{model.IsolationTierHardenedContainer}), runner.DefaultLimits(), sink)
	assertRunnerError(t, err, runner.CodeSyntheticRunnerNotAllowed)
	if len(sink.events) != 0 {
		t.Fatalf("capability mismatch emitted events: %+v", sink.events)
	}
}

func TestSinkFailureStopsImmediatelyAndPreservesOriginalError(t *testing.T) {
	plan := mustPlan(t, validPipelineYAML, validPipelineYAML)
	backend := mustFake(t, fakeProgramForPlan(plan))
	boom := errors.New("sink boom")
	sink := &collectingSink{limit: 1, failOn: 2, failErr: boom}
	result, err := runner.ExecutePlan(context.Background(), plan, backend, runner.SyntheticTestRequirements(), runner.DefaultLimits(), sink)
	assertRunnerError(t, err, runner.CodeSinkFailed)
	if !errors.Is(err, boom) {
		t.Fatalf("sink error not discoverable: %v", err)
	}
	if result.Complete || len(sink.events) != 1 {
		t.Fatalf("sink failure should stop after first accepted event: result=%+v events=%d", result, len(sink.events))
	}
}

func TestCancellationAndProgramCoverageFailures(t *testing.T) {
	plan := mustPlan(t, validPipelineYAML, validPipelineYAML)
	t.Run("already cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := runner.ExecutePlan(ctx, plan, mustFake(t, fakeProgramForPlan(plan)), runner.SyntheticTestRequirements(), runner.DefaultLimits(), &collectingSink{limit: 10})
		assertRunnerError(t, err, runner.CodeContextCancelled)
	})
	t.Run("missing script", func(t *testing.T) {
		program := fakeProgramForPlan(plan)
		program.Attempts = program.Attempts[:len(program.Attempts)-1]
		_, err := runner.ExecutePlan(context.Background(), plan, mustFake(t, program), runner.SyntheticTestRequirements(), runner.DefaultLimits(), &collectingSink{limit: 10})
		assertRunnerError(t, err, runner.CodeMissingAttemptScript)
	})
	t.Run("duplicate script", func(t *testing.T) {
		program := fakeProgramForPlan(plan)
		program.Attempts = append(program.Attempts, program.Attempts[0])
		_, err := fake.New(program)
		assertRunnerError(t, err, runner.CodeDuplicateAttemptScript)
	})
}

func TestEventDraftValidation(t *testing.T) {
	valid := runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindProcessStart, Process: &model.ProcessObservation{Operation: "start", ProcessID: 1, ExecutablePath: "/synthetic", Arguments: []string{}, Environment: []model.EnvEntry{}}}
	if err := runner.ValidateEventDraft(valid, runner.DefaultLimits()); err != nil {
		t.Fatalf("valid draft rejected: %v", err)
	}
	invalid := valid
	invalid.Filesystem = &model.FilesystemObservation{Operation: "write", Path: "/x"}
	assertRunnerError(t, runner.ValidateEventDraft(invalid, runner.DefaultLimits()), runner.CodeInvalidEventDraft)
	invalid = runner.EventDraft{Source: model.ObservationSourceHostObserved, Kind: model.ObservationKindProcessStart, Process: valid.Process}
	if err := runner.ValidateEventDraft(invalid, runner.DefaultLimits()); err != nil {
		t.Fatalf("non-fake source may be valid for future real runners: %v", err)
	}
	invalid.Source = model.ObservationSource("\x1b")
	assertRunnerError(t, runner.ValidateEventDraft(invalid, runner.DefaultLimits()), runner.CodeInvalidEventDraft)
}

func TestCapabilityMatcherReportsDeterministicMismatches(t *testing.T) {
	caps := fake.Capabilities()
	caps.Name = "fake"
	req := runner.Requirements{Intent: runner.ExecutionIntentWorkload, AllowedIsolationTiers: []model.IsolationTier{model.IsolationTierHardenedContainer}, TargetExecutionRequired: true, NetworkDenyEnforcementRequired: true, ProcessEventsRequired: true}
	mismatches, err := runner.MatchCapabilities(req, caps)
	if err != nil {
		t.Fatalf("MatchCapabilities() validation error = %v", err)
	}
	if len(mismatches) == 0 {
		t.Fatal("expected mismatches")
	}
	var got []runner.CapabilityMismatchCode
	for _, m := range mismatches {
		got = append(got, m.Code)
	}
	want := []runner.CapabilityMismatchCode{runner.MismatchExecutionIntent, runner.MismatchIsolationTier, runner.MismatchTargetExecution, runner.MismatchNetworkDenyEnforcement, runner.MismatchProcessEvents}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mismatch order = %+v, want %+v; mismatches=%+v", got, want, mismatches)
	}
}

func TestValidatePlanDocumentRejectsLegacyRunnerField(t *testing.T) {
	plan := mustPlan(t, validPipelineYAML, validPipelineYAML)
	doc := plan.Document()
	doc.Runner = model.RunnerCapabilities{Name: "legacy", Version: "v1", IsolationTier: model.IsolationTierFake}
	assertRunnerError(t, runner.ValidatePlanDocumentForTest(doc), runner.CodeInvalidPlanRunnerField)
}

func FuzzValidateRunnerCapabilities(f *testing.F) {
	f.Add("fake", "v1", string(model.IsolationTierFake), false, true, false)
	f.Add("bad\x1b", strings.Repeat("v", 200), "unknown", true, false, true)
	f.Fuzz(func(t *testing.T, name, version, tier string, executes, synthetic, deny bool) {
		_ = runner.ValidateRunnerCapabilities(model.RunnerCapabilities{Name: name, Version: version, IsolationTier: model.IsolationTier(tier), ExecutesTargetCode: executes, SyntheticEvidence: synthetic, EnforcesNetworkDeny: deny})
	})
}

func FuzzValidateEventDraft(f *testing.F) {
	f.Add(string(model.ObservationSourceSyntheticTestGenerated), string(model.ObservationKindObserverWarning), "warn")
	f.Add("\x1b", "unknown", strings.Repeat("x", 2048))
	f.Fuzz(func(t *testing.T, source, kind, message string) {
		_ = runner.ValidateEventDraft(runner.EventDraft{Source: model.ObservationSource(source), Kind: model.ObservationKind(kind), ObserverWarning: &model.ObserverWarningObservation{Code: "fuzz", Message: message, Limitations: []model.Limitation{}}}, runner.DefaultLimits())
	})
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
		{Revision: model.RevisionKindBase, ScenarioID: "test", Repetition: 1, Events: []fake.SyntheticEvent{}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 1}},
		{Revision: model.RevisionKindBase, ScenarioID: "build", Repetition: 1, Events: []fake.SyntheticEvent{}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 1}},
		{Revision: model.RevisionKindHead, ScenarioID: "test", Repetition: 1, Events: []fake.SyntheticEvent{}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 1}},
		{Revision: model.RevisionKindHead, ScenarioID: "build", Repetition: 1, Events: []fake.SyntheticEvent{}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 1}},
	}}
}

type collectingSink struct {
	limit   int
	failOn  int
	failErr error
	events  []model.ObservationEvent
}

func (s *collectingSink) Emit(ctx context.Context, event model.ObservationEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.failOn > 0 && len(s.events)+1 == s.failOn {
		return s.failErr
	}
	if len(s.events) >= s.limit {
		return errors.New("test sink limit")
	}
	event.ID = string(append([]byte(nil), []byte(event.ID)...))
	s.events = append(s.events, event)
	return nil
}

func jsonLines(t *testing.T, events []model.ObservationEvent) []byte {
	t.Helper()
	var b bytes.Buffer
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal event: %v", err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.Bytes()
}

func assertRunnerError(t *testing.T, err error, code runner.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected runner error %s, got nil", code)
	}
	var rerr *runner.Error
	if !errors.As(err, &rerr) {
		t.Fatalf("error %T is not *runner.Error: %v", err, err)
	}
	if rerr.Code != code {
		t.Fatalf("error code = %s, want %s; err=%v", rerr.Code, code, err)
	}
	if strings.ContainsAny(err.Error(), "\x1b\r\n") {
		t.Fatalf("error contains raw control characters: %q", err.Error())
	}
}

func intPtr(v int) *int { return &v }

func mustPlan(t *testing.T, baseYAML, headYAML string) *pipeline.FrozenPlan {
	t.Helper()
	plan, err := pipeline.Build(context.Background(), validBuildRequest(t, baseYAML, headYAML))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return plan
}

func validBuildRequest(t *testing.T, baseYAML, headYAML string) pipeline.BuildRequest {
	t.Helper()
	trusted := loadTrustedForTest(t, baseYAML, headYAML)
	return pipeline.BuildRequest{
		RunID:     "run-0001",
		CreatedAt: testCreatedAt,
		Trusted:   trusted,
		BaseSource: validSourceSnapshot(model.RevisionKindBase, testBaseCommit, testBaseTree,
			"sha256:3333333333333333333333333333333333333333333333333333333333333333",
			"sha256:4444444444444444444444444444444444444444444444444444444444444444"),
		HeadSource: validSourceSnapshot(model.RevisionKindHead, testHeadCommit, testHeadTree,
			"sha256:5555555555555555555555555555555555555555555555555555555555555555",
			"sha256:6666666666666666666666666666666666666666666666666666666666666666"),
		Platform: defaultPlatformConstraintsForTest(),
	}
}

func loadTrustedForTest(t *testing.T, baseYAML, headYAML string) config.TrustedLoadResult {
	t.Helper()
	base := model.CommitRef{Kind: model.RevisionKindBase, Repository: "https://example.invalid/org/repo.git", Ref: "refs/heads/master", CommitID: testBaseCommit, TreeDigest: model.Digest(testBaseTree)}
	head := model.CommitRef{Kind: model.RevisionKindHead, Repository: "https://example.invalid/org/repo.git", Ref: "refs/pull/7/head", CommitID: testHeadCommit, TreeDigest: model.Digest(testHeadTree)}
	source := &memoryRevisionSource{files: map[string]config.RevisionFile{}}
	source.files[key(base, config.PipelinePath)] = config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte(baseYAML), ObjectID: strings.Repeat("a", 40)}
	if headYAML != "" {
		source.files[key(head, config.PipelinePath)] = config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte(headYAML), ObjectID: strings.Repeat("b", 40)}
	}
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
	return pipeline.SourceSnapshot{
		RevisionKind:                  kind,
		CommitID:                      commit,
		TreeID:                        tree,
		ObjectFormat:                  pipeline.ObjectFormatSHA1,
		MaterializedTreeDigest:        model.Digest(treeDigest),
		MaterializationManifestDigest: model.Digest(manifestDigest),
		Summary:                       pipeline.SourceSummary{DirectoryCount: 3, RegularFileCount: 4, ExecutableFileCount: 1, SymlinkCount: 1, GitlinkCount: 1, LFSPointerCount: 1, TotalMaterializedFileBytes: 1234, SkippedEntryCount: 1},
		Limitations:                   []pipeline.SourceLimitation{{Code: "skipped-gitlink", Path: "vendor/submodule", Summary: "gitlink was reported but not traversed or materialized"}},
	}
}

func defaultPlatformConstraintsForTest() pipeline.PlatformConstraints {
	return pipeline.PlatformConstraints{
		MaxCPU:                   config.MaxCPU,
		MaxMemoryBytes:           config.MaxMemoryBytes,
		MaxDiskBytes:             config.MaxDiskBytes,
		MaxProcessCount:          config.MaxProcessCount,
		MaxGlobalTimeoutMillis:   config.MaxTimeoutMillis,
		MaxScenarioTimeoutMillis: config.MaxTimeoutMillis,
		MaxScenarioCount:         config.MaxScenarioCount,
		MaxRepetitions:           config.MaxRepetitions,
		MaxFilesystemRootCount:   config.MaxFilesystemRootCount,
		MaxArtifactCount:         config.MaxArtifactCount,
		MaxArtifactBytes:         config.MaxArtifactBytes,
		MaxLogBytesPerStream:     config.MaxLogBytesPerStream,
		MaxPlanJSONBytes:         pipeline.MaxPlanJSONBytes,
		RequiredNetworkMode:      model.NetworkModeDeny,
	}
}

func readRunnerTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("fake/testdata/v1alpha1/" + name)
	if err != nil {
		t.Fatalf("read runner testdata %s: %v", name, err)
	}
	return data
}

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
