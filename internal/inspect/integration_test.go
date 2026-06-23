package inspect

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/compare"
	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/evidence"
	"github.com/mattneel/glassroot/internal/gitstore"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/observe"
	"github.com/mattneel/glassroot/internal/pipeline"
	"github.com/mattneel/glassroot/internal/policy"
	"github.com/mattneel/glassroot/internal/report"
	"github.com/mattneel/glassroot/internal/runner"
	"github.com/mattneel/glassroot/internal/runner/fake"
	"github.com/mattneel/glassroot/internal/waiver"
)

func TestInspectEndToEndReconstructsSupportedPipeline(t *testing.T) {
	fx := newInspectFixture(t, true)
	defer fx.cleanup()

	inspector := NewMust(t)
	result, err := inspector.Inspect(context.Background(), Request{BundleDir: fx.bundlePath, GitDir: fx.gitDir, BaseCommitID: fx.base.CommitID, HeadCommitID: fx.head.CommitID, EvaluatedAt: fx.evaluatedAt, ManifestIntegrityMode: ManifestIntegrityExpectedDigest, ExpectedManifestDigest: fx.manifestDigest})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if result.OverallDisposition != fx.directApplication.Document().OverallEffectiveDisposition {
		t.Fatalf("disposition = %s, want %s", result.OverallDisposition, fx.directApplication.Document().OverallEffectiveDisposition)
	}
	if !bytes.Equal(result.Report.JSON(), fx.directReport.JSON()) || result.Report.Digest() != fx.directReport.Digest() {
		t.Fatalf("inspect report differs from direct stage report\ninspect=%s\ndirect=%s", result.Report.JSON(), fx.directReport.JSON())
	}
	if bytes.Contains(result.Report.JSON(), []byte(fx.bundlePath)) || bytes.Contains(result.Report.JSON(), []byte(fx.gitDir)) || bytes.Contains(result.Report.JSON(), []byte("apiVersion: glassroot")) {
		t.Fatalf("report leaked host path or raw YAML: %s", result.Report.JSON())
	}
	app := result.Report.Document().Policy
	if app.TrustedConfigAuthority.HeadState != config.HeadStateModifiedValid || app.TrustedWaiverAuthority.HeadState == waiver.HeadStateUnchanged {
		t.Fatalf("head config/waiver proposal was not represented: config=%s waiver=%s", app.TrustedConfigAuthority.HeadState, app.TrustedWaiverAuthority.HeadState)
	}
	if app.Summary.AppliedWaivers == 0 {
		t.Fatalf("trusted base waiver did not apply: %+v", app)
	}
}

func TestInspectIntegrityModesAndRevisionMismatch(t *testing.T) {
	fx := newInspectFixture(t, false)
	defer fx.cleanup()
	inspector := NewMust(t)
	_, err := inspector.Inspect(context.Background(), Request{BundleDir: fx.bundlePath, GitDir: fx.gitDir, BaseCommitID: fx.base.CommitID, HeadCommitID: fx.head.CommitID, EvaluatedAt: fx.evaluatedAt, ManifestIntegrityMode: ManifestIntegrityExpectedDigest, ExpectedManifestDigest: model.Digest("sha256:" + strings.Repeat("0", 64))})
	assertInspectError(t, err, CodeBundleOpenFailed)

	result, err := inspector.Inspect(context.Background(), Request{BundleDir: fx.bundlePath, GitDir: fx.gitDir, BaseCommitID: fx.base.CommitID, HeadCommitID: fx.head.CommitID, EvaluatedAt: fx.evaluatedAt, ManifestIntegrityMode: ManifestIntegrityInternalConsistencyOnly, AllowInternalConsistencyOnly: true})
	if err != nil {
		t.Fatalf("internal-consistency inspect error = %v", err)
	}
	if result.VerificationMode != VerificationModeInternalConsistencyOnly || result.Report.Document().ManifestVerificationMode != string(VerificationModeInternalConsistencyOnly) {
		t.Fatalf("verification mode not retained: result=%s report=%s", result.VerificationMode, result.Report.Document().ManifestVerificationMode)
	}

	_, err = inspector.Inspect(context.Background(), Request{BundleDir: fx.bundlePath, GitDir: fx.gitDir, BaseCommitID: fx.head.CommitID, HeadCommitID: fx.base.CommitID, EvaluatedAt: fx.evaluatedAt, ManifestIntegrityMode: ManifestIntegrityExpectedDigest, ExpectedManifestDigest: fx.manifestDigest})
	assertInspectError(t, err, CodeRevisionMismatch)
}

type inspectFixture struct {
	gitDir            string
	bundlePath        string
	manifestDigest    model.Digest
	base              model.CommitRef
	head              model.CommitRef
	evaluatedAt       time.Time
	directReport      *report.FrozenReport
	directApplication *policy.FrozenApplication
	cleanup           func()
}

func newInspectFixture(t *testing.T, withWaiver bool) inspectFixture {
	t.Helper()
	preRepo := newLooseGitRepo(t)
	preBase, preHead := preRepo.commits(t, "")
	preBundle, preManifest := writeInspectBundle(t, preRepo.dir, preBase, preHead)
	preEval, preCleanup := evaluateBundleForWaiver(t, preBundle, preManifest, preRepo.dir, preBase, preHead)
	netFinding := findingForRuleInInspect(t, preEval.Document(), "GR-NET-001")
	preCleanup()
	_ = os.RemoveAll(preBundle)

	repo := newLooseGitRepo(t)
	waiverText := ""
	if withWaiver {
		waiverText = waiverYAMLForInspect(netFinding.ID, netFinding.RuleID)
	}
	base, head := repo.commits(t, waiverText)
	bundlePath, manifestDigest := writeInspectBundle(t, repo.dir, base, head)

	bundle, err := evidence.OpenAndVerify(context.Background(), bundlePath, evidence.DefaultReaderLimits(), evidence.WithExpectedManifestDigest(manifestDigest))
	if err != nil {
		t.Fatalf("OpenAndVerify final: %v", err)
	}
	directDelta, directEval, directApp := directStagesUntilApplication(t, bundle, repo.dir, base, head, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))
	reportBuilder, err := report.New(report.DefaultLimits())
	if err != nil {
		t.Fatalf("report.New: %v", err)
	}
	directReport, err := reportBuilder.Build(context.Background(), report.BuildRequest{Bundle: bundle, Delta: directDelta, Application: directApp})
	if err != nil {
		t.Fatalf("Build report: %v", err)
	}
	if err := bundle.Close(); err != nil {
		t.Fatalf("close direct bundle: %v", err)
	}
	_ = directEval
	return inspectFixture{gitDir: repo.dir, bundlePath: bundlePath, manifestDigest: manifestDigest, base: base, head: head, evaluatedAt: time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC), directReport: directReport, directApplication: directApp, cleanup: func() { _ = os.RemoveAll(repo.dir); _ = os.RemoveAll(bundlePath); _ = os.RemoveAll(preRepo.dir) }}
}

func evaluateBundleForWaiver(t *testing.T, bundlePath string, digest model.Digest, gitDir string, base, head model.CommitRef) (*policy.FrozenEvaluation, func()) {
	t.Helper()
	bundle, err := evidence.OpenAndVerify(context.Background(), bundlePath, evidence.DefaultReaderLimits(), evidence.WithExpectedManifestDigest(digest))
	if err != nil {
		t.Fatalf("OpenAndVerify pre: %v", err)
	}
	delta, eval, _ := directStagesUntilApplication(t, bundle, gitDir, base, head, time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))
	_ = delta
	return eval, func() { _ = bundle.Close() }
}

func directStagesUntilApplication(t *testing.T, bundle *evidence.Bundle, gitDir string, base, head model.CommitRef, at time.Time) (*compare.FrozenDelta, *policy.FrozenEvaluation, *policy.FrozenApplication) {
	t.Helper()
	n, err := observe.New(observe.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	trace, err := n.Normalize(context.Background(), bundle)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	cmp, err := compare.New(compare.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	delta, err := cmp.Compare(context.Background(), trace)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	evalr, err := policy.New(policy.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	eval, err := evalr.Evaluate(context.Background(), policy.EvaluationRequest{Profile: policy.PolicyProfileStrict(), Delta: delta})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	repo, err := gitstore.Open(context.Background(), gitDir)
	if err != nil {
		t.Fatalf("gitstore.Open: %v", err)
	}
	defer repo.Close()
	source := gitstore.NewRevisionFileSource(repo)
	trusted, err := config.LoadTrusted(context.Background(), source, config.TrustedLoadRequest{Base: base, Head: head})
	if err != nil {
		t.Fatalf("LoadTrusted: %v", err)
	}
	plan := mustRebuildPlanForInspectTest(t, bundle.Plan(), trusted)
	app, err := policy.NewApplier(policy.DefaultApplicationLimits())
	if err != nil {
		t.Fatal(err)
	}
	application, err := app.Apply(context.Background(), policy.ApplicationRequest{Evaluation: eval, Plan: plan, TrustedConfig: trusted, WaiverSource: source, EvaluatedAt: at})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return delta, eval, application
}

func mustRebuildPlanForInspectTest(t *testing.T, doc model.RunPlan, trusted config.TrustedLoadResult) *pipeline.FrozenPlan {
	t.Helper()
	base, err := sourceSnapshotFromRevision(doc.Revisions[0])
	if err != nil {
		t.Fatal(err)
	}
	head, err := sourceSnapshotFromRevision(doc.Revisions[1])
	if err != nil {
		t.Fatal(err)
	}
	platform, err := platformFromPlan(doc.Platform)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := pipeline.Build(context.Background(), pipeline.BuildRequest{RunID: doc.RunID, CreatedAt: doc.CreatedAt, Trusted: trusted, BaseSource: base, HeadSource: head, Platform: platform})
	if err != nil {
		t.Fatalf("pipeline.Build: %v", err)
	}
	return plan
}

func writeInspectBundle(t *testing.T, gitDir string, base, head model.CommitRef) (string, model.Digest) {
	t.Helper()
	repo, err := gitstore.Open(context.Background(), gitDir)
	if err != nil {
		t.Fatalf("Open git: %v", err)
	}
	defer repo.Close()
	source := gitstore.NewRevisionFileSource(repo)
	trusted, err := config.LoadTrusted(context.Background(), source, config.TrustedLoadRequest{Base: base, Head: head})
	if err != nil {
		t.Fatalf("LoadTrusted: %v", err)
	}
	plan, err := pipeline.Build(context.Background(), pipeline.BuildRequest{RunID: "run-0001", CreatedAt: time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC), Trusted: trusted, BaseSource: inspectSourceSnapshot(model.RevisionKindBase, base), HeadSource: inspectSourceSnapshot(model.RevisionKindHead, head), Platform: inspectPlatform()})
	if err != nil {
		t.Fatalf("pipeline.Build: %v", err)
	}
	parent := t.TempDir()
	writer, err := evidence.NewWriter(parent, evidence.DefaultLimits())
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	session, err := writer.Begin(context.Background(), plan)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	backend, err := fake.New(inspectFakeProgram(plan))
	if err != nil {
		t.Fatalf("fake.New: %v", err)
	}
	result, err := runner.ExecutePlan(context.Background(), plan, backend, runner.SyntheticTestRequirements(), runner.DefaultLimits(), session)
	if err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}
	bundleResult, err := session.Commit(context.Background(), evidence.Complete(result))
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return bundleResult.Path, bundleResult.ManifestDigest
}

func inspectSourceSnapshot(kind model.RevisionKind, ref model.CommitRef) pipeline.SourceSnapshot {
	return pipeline.SourceSnapshot{RevisionKind: kind, CommitID: ref.CommitID, TreeID: ref.TreeID, ObjectFormat: pipeline.ObjectFormatSHA1, MaterializedTreeDigest: model.Digest("sha256:" + strings.Repeat(string(ref.CommitID[0]), 64)), MaterializationManifestDigest: model.Digest("sha256:" + strings.Repeat(string(ref.CommitID[1]), 64)), Summary: pipeline.SourceSummary{DirectoryCount: 2, RegularFileCount: 2}}
}

func inspectPlatform() pipeline.PlatformConstraints {
	return pipeline.PlatformConstraints{MaxCPU: config.MaxCPU, MaxMemoryBytes: config.MaxMemoryBytes, MaxDiskBytes: config.MaxDiskBytes, MaxProcessCount: config.MaxProcessCount, MaxGlobalTimeoutMillis: config.MaxTimeoutMillis, MaxScenarioTimeoutMillis: config.MaxTimeoutMillis, MaxScenarioCount: config.MaxScenarioCount, MaxRepetitions: config.MaxRepetitions, MaxFilesystemRootCount: config.MaxFilesystemRootCount, MaxArtifactCount: config.MaxArtifactCount, MaxArtifactBytes: config.MaxArtifactBytes, MaxLogBytesPerStream: config.MaxLogBytesPerStream, MaxPlanJSONBytes: pipeline.MaxPlanJSONBytes, RequiredNetworkMode: model.NetworkModeDeny}
}

func inspectFakeProgram(plan *pipeline.FrozenPlan) fake.Program {
	parent := int64(101)
	return fake.Program{PlanDigest: plan.Digest(), Attempts: []fake.AttemptScript{{Revision: model.RevisionKindBase, ScenarioID: "test", Repetition: 1, Events: []fake.SyntheticEvent{{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindProcessStart, Process: &model.ProcessObservation{Operation: "start", ProcessID: parent, ExecutablePath: "/workspace/bin/tester", Arguments: []string{}, Environment: []model.EnvEntry{}}}}}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 20}}, {Revision: model.RevisionKindHead, ScenarioID: "test", Repetition: 1, Events: []fake.SyntheticEvent{{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindNetworkConnection, Network: &model.NetworkObservation{Operation: "connect", Protocol: "tcp", DestinationHost: "canary.example.invalid", DestinationPort: 443, ResolvedAddresses: []string{}, Result: "denied"}}}}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusFailed, ExitCode: intPtr(2), DurationMillis: 25}}, {Revision: model.RevisionKindBase, ScenarioID: "build", Repetition: 1, Events: []fake.SyntheticEvent{{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindFilesystemWrite, Filesystem: &model.FilesystemObservation{Operation: "write", Path: "/workspace/bin/glassroot", Digest: "sha256:9999999999999999999999999999999999999999999999999999999999999999", SizeBytes: 42}}}}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 30}}, {Revision: model.RevisionKindHead, ScenarioID: "build", Repetition: 1, Events: []fake.SyntheticEvent{{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindArtifactActivity, Artifact: &model.ArtifactObservation{Operation: "create", ArtifactID: "artifact-head-bin", Path: "/workspace/bin/glassroot", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SizeBytes: 64, Executable: true, SourceEventIDs: []string{}}}}}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 35}}}}
}

func findingForRuleInInspect(t *testing.T, doc policy.EvaluationDocument, rule string) model.Finding {
	t.Helper()
	for _, f := range doc.Findings {
		if f.RuleID == rule {
			return f
		}
	}
	t.Fatalf("missing %s in %+v", rule, doc.Findings)
	return model.Finding{}
}

func waiverYAMLForInspect(findingID, ruleID string) string {
	return "apiVersion: glassroot.dev/v1alpha1\nkind: WaiverSet\nmetadata:\n  name: default\nspec:\n  waivers:\n    - id: known-network\n      target:\n        findingId: " + findingID + "\n        ruleId: " + ruleID + "\n      owner: mattneel\n      reason: Known deterministic fixture behavior pending removal.\n      issuedAt: \"2026-06-23T00:00:00Z\"\n      expiresAt: \"2026-07-23T00:00:00Z\"\n"
}

func intPtr(v int) *int { return &v }

type looseGitRepo struct{ dir string }

func newLooseGitRepo(t *testing.T) *looseGitRepo {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo.git")
	for _, rel := range []string{"objects", "refs/heads"} {
		if err := os.MkdirAll(filepath.Join(dir, rel), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte("[core]\n\trepositoryformatversion = 0\n\tfilemode = true\n\tbare = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return &looseGitRepo{dir: dir}
}

func (r *looseGitRepo) commits(t *testing.T, baseWaiver string) (model.CommitRef, model.CommitRef) {
	t.Helper()
	baseTree := r.tree(t, map[string][]byte{config.PipelinePath: []byte(inspectPipelineYAML), waiver.WaiverPath: []byte(baseWaiver)})
	baseCommit := r.commit(t, baseTree, "base")
	headTree := r.tree(t, map[string][]byte{config.PipelinePath: []byte(strings.Replace(inspectPipelineYAML, "cpu: 2", "cpu: 3", 1)), waiver.WaiverPath: []byte(strings.Replace(baseWaiver, "2026-07-23T00:00:00Z", "2026-08-01T00:00:00Z", 1))})
	headCommit := r.commit(t, headTree, "head")
	base := model.CommitRef{Kind: model.RevisionKindBase, Repository: "https://example.invalid/org/repo.git", Ref: "refs/heads/main", CommitID: baseCommit, ObjectFormat: model.GitObjectFormatSHA1, TreeID: baseTree, TreeDigest: model.Digest(baseTree)}
	head := model.CommitRef{Kind: model.RevisionKindHead, Repository: "https://example.invalid/org/repo.git", Ref: "refs/pull/1/head", CommitID: headCommit, ObjectFormat: model.GitObjectFormatSHA1, TreeID: headTree, TreeDigest: model.Digest(headTree)}
	return base, head
}

func (r *looseGitRepo) tree(t *testing.T, files map[string][]byte) string {
	t.Helper()
	glassrootEntries := map[string]string{}
	for path, data := range files {
		if !strings.HasPrefix(path, ".glassroot/") {
			t.Fatalf("unexpected path %s", path)
		}
		name := strings.TrimPrefix(path, ".glassroot/")
		glassrootEntries[name] = r.object(t, "blob", data)
	}
	glassrootTree := r.treeObject(t, glassrootEntries)
	return r.treeObject(t, map[string]string{".glassroot": glassrootTree})
}

func (r *looseGitRepo) treeObject(t *testing.T, entries map[string]string) string {
	t.Helper()
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	var body bytes.Buffer
	for _, name := range keys {
		mode := "100644"
		if name == ".glassroot" {
			mode = "40000"
		}
		body.WriteString(mode)
		body.WriteByte(' ')
		body.WriteString(name)
		body.WriteByte(0)
		raw, err := hex.DecodeString(entries[name])
		if err != nil {
			t.Fatal(err)
		}
		body.Write(raw)
	}
	return r.object(t, "tree", body.Bytes())
}

func (r *looseGitRepo) commit(t *testing.T, tree, msg string) string {
	t.Helper()
	body := []byte("tree " + tree + "\nauthor Glassroot <glassroot@example.invalid> 1782180000 +0000\ncommitter Glassroot <glassroot@example.invalid> 1782180000 +0000\n\n" + msg + "\n")
	return r.object(t, "commit", body)
}

func (r *looseGitRepo) object(t *testing.T, typ string, body []byte) string {
	t.Helper()
	head := []byte(fmt.Sprintf("%s %d\x00", typ, len(body)))
	store := append(append([]byte(nil), head...), body...)
	sum := sha1.Sum(store)
	oid := hex.EncodeToString(sum[:])
	dir := filepath.Join(r.dir, "objects", oid[:2])
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	if _, err := zw.Write(store); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, oid[2:]), z.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return oid
}

const inspectPipelineYAML = `apiVersion: glassroot.dev/v1alpha1
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
