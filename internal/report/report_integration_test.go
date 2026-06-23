package report

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/compare"
	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/evidence"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/observe"
	"github.com/mattneel/glassroot/internal/pipeline"
	"github.com/mattneel/glassroot/internal/policy"
	"github.com/mattneel/glassroot/internal/runner"
	"github.com/mattneel/glassroot/internal/runner/fake"
	"github.com/mattneel/glassroot/internal/waiver"
)

func TestBuildReportPreservesApplicationAndDeltaAndRendersSafely(t *testing.T) {
	fx := mustReportFixture(t)
	defer fx.cleanup()

	frozen, err := mustBuilder(t).Build(context.Background(), BuildRequest{Bundle: fx.bundle, Delta: fx.delta, Application: fx.application})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	doc := frozen.Document()
	appDoc := fx.application.Document()
	deltaDoc := fx.delta.Document()
	if len(doc.Policy.AppliedFindings) != len(appDoc.AppliedFindings) {
		t.Fatalf("report findings=%d application findings=%d", len(doc.Policy.AppliedFindings), len(appDoc.AppliedFindings))
	}
	if len(doc.Behavior.Records) != len(deltaDoc.Records) {
		t.Fatalf("report deltas=%d delta records=%d", len(doc.Behavior.Records), len(deltaDoc.Records))
	}
	if !hasNotice(doc, NoticeFakeRunner) || !hasNotice(doc, NoticeSyntheticEvidence) || !hasNotice(doc, NoticeNoTargetCodeExecuted) || !hasNotice(doc, NoticeWaiversApplied) || !hasNotice(doc, NoticeGovernanceFindingsPresent) || !hasNotice(doc, NoticePassedNotProofOfSafety) {
		t.Fatalf("missing required notices: %+v", doc.Notices)
	}
	if bytes.Contains(frozen.JSON(), []byte("bundlePath")) || bytes.Contains(frozen.JSON(), []byte("go test ./...")) || bytes.Contains(frozen.JSON(), []byte("apiVersion: glassroot")) {
		t.Fatalf("report JSON leaked bundle paths, run command, or raw YAML: %s", frozen.JSON())
	}
	md, err := RenderMarkdown(context.Background(), frozen, DefaultRenderLimits())
	if err != nil {
		t.Fatalf("RenderMarkdown() error = %v", err)
	}
	txt, err := RenderTerminal(context.Background(), frozen, DefaultRenderLimits())
	if err != nil {
		t.Fatalf("RenderTerminal() error = %v", err)
	}
	for _, f := range doc.Policy.AppliedFindings {
		if !bytes.Contains(md.Bytes, []byte(f.Original.ID)) || !bytes.Contains(txt.Bytes, []byte(f.Original.ID)) {
			t.Fatalf("finding %s missing from a renderer", f.Original.ID)
		}
	}
	for _, r := range doc.Behavior.Records {
		if !bytes.Contains(md.Bytes, []byte(r.ID)) || !bytes.Contains(txt.Bytes, []byte(r.ID)) {
			t.Fatalf("delta %s missing from a renderer", r.ID)
		}
	}
	if !strings.HasPrefix(string(frozen.Digest()), "sha256:") || !strings.HasPrefix(string(md.Digest), "sha256:") || !strings.HasPrefix(string(txt.Digest), "sha256:") {
		t.Fatalf("missing digest report=%s md=%s txt=%s", frozen.Digest(), md.Digest, txt.Digest)
	}
}

func TestBuildRejectsClosedBundle(t *testing.T) {
	fx := mustReportFixture(t)
	defer fx.cleanup()
	if err := fx.bundle.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	_, err := mustBuilder(t).Build(context.Background(), BuildRequest{Bundle: fx.bundle, Delta: fx.delta, Application: fx.application})
	assertReportError(t, err, CodeBundleClosed)
}

func TestReportGoldenOutputs(t *testing.T) {
	fx := mustReportFixture(t)
	defer fx.cleanup()
	frozen, err := mustBuilder(t).Build(context.Background(), BuildRequest{Bundle: fx.bundle, Delta: fx.delta, Application: fx.application})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	md, err := RenderMarkdown(context.Background(), frozen, DefaultRenderLimits())
	if err != nil {
		t.Fatalf("RenderMarkdown() error = %v", err)
	}
	txt, err := RenderTerminal(context.Background(), frozen, DefaultRenderLimits())
	if err != nil {
		t.Fatalf("RenderTerminal() error = %v", err)
	}
	if os.Getenv("UPDATE_REPORT_GOLDEN") == "1" {
		writeReportTestFile(t, frozen.JSON(), "testdata", "v1alpha1", "report.json")
		writeReportTestFile(t, []byte(frozen.Digest()), "testdata", "v1alpha1", "report.digest")
		writeReportTestFile(t, md.Bytes, "testdata", "v1alpha1", "report.md")
		writeReportTestFile(t, txt.Bytes, "testdata", "v1alpha1", "report.txt")
	}
	assertGolden(t, frozen.JSON(), "testdata", "v1alpha1", "report.json")
	assertGolden(t, []byte(frozen.Digest()), "testdata", "v1alpha1", "report.digest")
	assertGolden(t, md.Bytes, "testdata", "v1alpha1", "report.md")
	assertGolden(t, txt.Bytes, "testdata", "v1alpha1", "report.txt")
}

type reportFixture struct {
	bundle      *evidence.Bundle
	delta       *compare.FrozenDelta
	application *policy.FrozenApplication
	cleanup     func()
}

func mustReportFixture(t *testing.T) reportFixture {
	t.Helper()
	base := reportCommit(model.RevisionKindBase, strings.Repeat("1", 40))
	head := reportCommit(model.RevisionKindHead, strings.Repeat("2", 40))
	source := newReportMemorySource(base, head)
	trusted, err := config.LoadTrusted(context.Background(), source, config.TrustedLoadRequest{Base: base, Head: head})
	if err != nil {
		t.Fatalf("LoadTrusted() error = %v", err)
	}
	plan, err := pipeline.Build(context.Background(), pipeline.BuildRequest{RunID: "run-0001", CreatedAt: time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC), Trusted: trusted, BaseSource: reportSourceSnapshot(model.RevisionKindBase, base.CommitID), HeadSource: reportSourceSnapshot(model.RevisionKindHead, head.CommitID), Platform: reportPlatform()})
	if err != nil {
		t.Fatalf("pipeline.Build() error = %v", err)
	}
	parent := t.TempDir()
	writer, err := evidence.NewWriter(parent, evidence.DefaultLimits())
	if err != nil {
		t.Fatalf("NewWriter() error = %v", err)
	}
	session, err := writer.Begin(context.Background(), plan)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	backend, err := fake.New(reportFakeProgram(plan))
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
	normalizer, err := observe.New(observe.DefaultLimits())
	if err != nil {
		t.Fatalf("observe.New() error = %v", err)
	}
	trace, err := normalizer.Normalize(context.Background(), bundle)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	cmp, err := compare.New(compare.DefaultLimits())
	if err != nil {
		t.Fatalf("compare.New() error = %v", err)
	}
	delta, err := cmp.Compare(context.Background(), trace)
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}
	eval, err := policy.New(policy.DefaultLimits())
	if err != nil {
		t.Fatalf("policy.New() error = %v", err)
	}
	evaluation, err := eval.Evaluate(context.Background(), policy.EvaluationRequest{Profile: policy.PolicyProfileStrict(), Delta: delta})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	netFinding := findEvalRule(t, evaluation.Document(), "GR-NET-001")
	source.putWaiver(base, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte("apiVersion: glassroot.dev/v1alpha1\nkind: WaiverSet\nmetadata:\n  name: default\nspec:\n  waivers:\n    - id: known-network\n      target:\n        findingId: " + netFinding.ID + "\n        ruleId: GR-NET-001\n      owner: mattneel\n      reason: Known deterministic fixture behavior pending removal.\n      issuedAt: \"2026-06-23T00:00:00Z\"\n      expiresAt: \"2026-07-23T00:00:00Z\"\n    - id: expired-artifact\n      target:\n        findingId: finding-" + strings.Repeat("3", 64) + "\n        ruleId: GR-ART-001\n      owner: mattneel\n      reason: Expired deterministic fixture waiver.\n      issuedAt: \"2026-01-01T00:00:00Z\"\n      expiresAt: \"2026-02-01T00:00:00Z\"\n")})
	source.putWaiver(head, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte("apiVersion: glassroot.dev/v1alpha1\nkind: WaiverSet\nmetadata:\n  name: default\nspec:\n  waivers:\n    - id: known-network\n      target:\n        findingId: " + netFinding.ID + "\n        ruleId: GR-NET-001\n      owner: mattneel\n      reason: Known deterministic fixture behavior pending removal.\n      issuedAt: \"2026-06-23T00:00:00Z\"\n      expiresAt: \"2026-08-01T00:00:00Z\"\n")})
	trusted.HeadAssessment = config.HeadAssessment{State: config.HeadStateModifiedValid, BaseFile: trusted.BaseFile, HeadFile: trusted.BaseFile, Changes: []config.ConfigChange{{Path: "spec.resources.cpu", Kind: config.ChangeKindModified, Effect: config.SecurityEffectPrivilegeIncrease}}}
	app, err := policy.NewApplier(policy.DefaultApplicationLimits())
	if err != nil {
		t.Fatalf("NewApplier() error = %v", err)
	}
	application, err := app.Apply(context.Background(), policy.ApplicationRequest{Evaluation: evaluation, Plan: plan, TrustedConfig: trusted, WaiverSource: source, EvaluatedAt: time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	cleanup := func() { _ = bundle.Close(); _ = os.RemoveAll(bundleResult.Path) }
	return reportFixture{bundle: bundle, delta: delta, application: application, cleanup: cleanup}
}

func mustBuilder(t *testing.T) *Builder {
	t.Helper()
	b, err := New(DefaultLimits())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return b
}
func hasNotice(doc Document, code NoticeCode) bool {
	for _, n := range doc.Notices {
		if n.Code == code {
			return true
		}
	}
	return false
}
func findEvalRule(t *testing.T, doc policy.EvaluationDocument, rule string) model.Finding {
	t.Helper()
	for _, f := range doc.Findings {
		if f.RuleID == rule {
			return f
		}
	}
	t.Fatalf("missing rule %s in %+v", rule, doc.Findings)
	return model.Finding{}
}

func assertGolden(t *testing.T, got []byte, parts ...string) {
	t.Helper()
	want := readReportTestFile(t, parts...)
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch %s\nwant=%s\n got=%s", filepath.Join(parts...), want, got)
	}
}
func readReportTestFile(t *testing.T, parts ...string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(parts...), err)
	}
	return data
}
func writeReportTestFile(t *testing.T, data []byte, parts ...string) {
	t.Helper()
	path := filepath.Join(parts...)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

type reportMemorySource struct {
	files map[string]map[string]config.RevisionFile
}

func newReportMemorySource(base, head model.CommitRef) *reportMemorySource {
	s := &reportMemorySource{files: map[string]map[string]config.RevisionFile{}}
	s.put(base, config.PipelinePath, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte(reportPipelineYAML), ObjectID: "base-pipeline"})
	s.put(head, config.PipelinePath, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte(reportPipelineYAML), ObjectID: "head-pipeline"})
	return s
}
func (s *reportMemorySource) putWaiver(rev model.CommitRef, file config.RevisionFile) {
	s.put(rev, waiver.WaiverPath, file)
}
func (s *reportMemorySource) put(rev model.CommitRef, path string, file config.RevisionFile) {
	if s.files[rev.CommitID] == nil {
		s.files[rev.CommitID] = map[string]config.RevisionFile{}
	}
	file.Data = append([]byte(nil), file.Data...)
	s.files[rev.CommitID][path] = file
}
func (s *reportMemorySource) ReadFile(ctx context.Context, rev model.CommitRef, path string, maxBytes int64) (config.RevisionFile, error) {
	if err := ctx.Err(); err != nil {
		return config.RevisionFile{}, err
	}
	file, ok := s.files[rev.CommitID][path]
	if !ok {
		return config.RevisionFile{}, fs.ErrNotExist
	}
	if int64(len(file.Data)) > maxBytes {
		return config.RevisionFile{}, config.ErrRevisionFileTooLarge
	}
	file.Data = append([]byte(nil), file.Data...)
	return file, nil
}

func reportCommit(kind model.RevisionKind, id string) model.CommitRef {
	return model.CommitRef{Kind: kind, CommitID: id, TreeID: strings.Repeat(id[:1], 40), ObjectFormat: model.GitObjectFormatSHA1}
}
func reportSourceSnapshot(kind model.RevisionKind, commit string) pipeline.SourceSnapshot {
	return pipeline.SourceSnapshot{RevisionKind: kind, CommitID: commit, TreeID: strings.Repeat(commit[:1], 40), ObjectFormat: pipeline.ObjectFormatSHA1, MaterializedTreeDigest: model.Digest("sha256:" + strings.Repeat(string(commit[0]), 64)), MaterializationManifestDigest: model.Digest("sha256:" + strings.Repeat(string(commit[1]), 64)), Summary: pipeline.SourceSummary{DirectoryCount: 1, RegularFileCount: 1}}
}
func reportPlatform() pipeline.PlatformConstraints {
	return pipeline.PlatformConstraints{MaxCPU: config.MaxCPU, MaxMemoryBytes: config.MaxMemoryBytes, MaxDiskBytes: config.MaxDiskBytes, MaxProcessCount: config.MaxProcessCount, MaxGlobalTimeoutMillis: config.MaxTimeoutMillis, MaxScenarioTimeoutMillis: config.MaxTimeoutMillis, MaxScenarioCount: config.MaxScenarioCount, MaxRepetitions: config.MaxRepetitions, MaxFilesystemRootCount: config.MaxFilesystemRootCount, MaxArtifactCount: config.MaxArtifactCount, MaxArtifactBytes: config.MaxArtifactBytes, MaxLogBytesPerStream: config.MaxLogBytesPerStream, MaxPlanJSONBytes: pipeline.MaxPlanJSONBytes, RequiredNetworkMode: model.NetworkModeDeny}
}
func reportFakeProgram(plan *pipeline.FrozenPlan) fake.Program {
	parent := int64(101)
	return fake.Program{PlanDigest: plan.Digest(), Attempts: []fake.AttemptScript{{Revision: model.RevisionKindBase, ScenarioID: "test", Repetition: 1, Events: []fake.SyntheticEvent{{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindProcessStart, Process: &model.ProcessObservation{Operation: "start", ProcessID: parent, ExecutablePath: "/workspace/bin/tester", Arguments: []string{}, Environment: []model.EnvEntry{}}}}}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 20}}, {Revision: model.RevisionKindHead, ScenarioID: "test", Repetition: 1, Events: []fake.SyntheticEvent{{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindNetworkConnection, Network: &model.NetworkObservation{Operation: "connect", Protocol: "tcp", DestinationHost: "canary.example.invalid", DestinationPort: 443, ResolvedAddresses: []string{}, Result: "denied"}}}}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusFailed, ExitCode: intPtr(2), DurationMillis: 25}}, {Revision: model.RevisionKindBase, ScenarioID: "build", Repetition: 1, Events: []fake.SyntheticEvent{{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindFilesystemWrite, Filesystem: &model.FilesystemObservation{Operation: "write", Path: "/workspace/bin/glassroot", Digest: "sha256:9999999999999999999999999999999999999999999999999999999999999999", SizeBytes: 42}}}}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 30}}, {Revision: model.RevisionKindHead, ScenarioID: "build", Repetition: 1, Events: []fake.SyntheticEvent{{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindArtifactActivity, Artifact: &model.ArtifactObservation{Operation: "create", ArtifactID: "artifact-head-bin", Path: "/workspace/bin/glassroot", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SizeBytes: 64, Executable: true, SourceEventIDs: []string{}}}}}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 35}}}}
}
func intPtr(v int) *int { return &v }

const reportPipelineYAML = `apiVersion: glassroot.dev/v1alpha1
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
      run: go test ./...
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
