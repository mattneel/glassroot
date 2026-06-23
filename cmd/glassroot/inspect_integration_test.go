package main

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

	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/evidence"
	"github.com/mattneel/glassroot/internal/gitstore"
	"github.com/mattneel/glassroot/internal/inspect"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/pipeline"
	"github.com/mattneel/glassroot/internal/report"
	"github.com/mattneel/glassroot/internal/runner"
	"github.com/mattneel/glassroot/internal/runner/fake"
	"github.com/mattneel/glassroot/internal/waiver"
)

func TestInspectCommandFullStackOutputsAndExitCodes(t *testing.T) {
	fx := newCLIInspectFixture(t, nil)
	defer fx.cleanup()
	wantResult := inspectByAPIForCLI(t, fx, false)

	for _, tc := range []struct {
		format string
		want   []byte
	}{
		{"terminal", renderTerminalForCLI(t, wantResult)},
		{"markdown", renderMarkdownForCLI(t, wantResult)},
		{"json", wantResult.Report.JSON()},
	} {
		t.Run(tc.format, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(cliInspectArgs(fx, tc.format, false, fx.manifestDigest), &stdout, &stderr)
			if code != 4 || stderr.Len() != 0 {
				t.Fatalf("exit=%d stderr=%q", code, stderr.String())
			}
			if !bytes.Equal(stdout.Bytes(), tc.want) {
				t.Fatalf("%s output differed", tc.format)
			}
			for _, forbidden := range []string{fx.bundlePath, fx.gitDir, "apiVersion: glassroot", "go test ./..."} {
				if bytes.Contains(stdout.Bytes(), []byte(forbidden)) {
					t.Fatalf("%s output leaked forbidden value %q", tc.format, forbidden)
				}
			}
		})
	}

	t.Run("internal consistency explicit", func(t *testing.T) {
		want := inspectByAPIForCLI(t, fx, true)
		var stdout, stderr bytes.Buffer
		code := run(cliInspectArgs(fx, "terminal", true, ""), &stdout, &stderr)
		if code != 4 || stderr.Len() != 0 || !bytes.Equal(stdout.Bytes(), renderTerminalForCLI(t, want)) {
			t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
		if !strings.Contains(stdout.String(), "internal-consistency-only") {
			t.Fatalf("internal-consistency notice missing from output")
		}
	})

	t.Run("cwd and environment do not affect output", func(t *testing.T) {
		want := renderTerminalForCLI(t, wantResult)
		oldwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(t.TempDir()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chdir(oldwd) })
		t.Setenv("TERM", "xterm-256color")
		t.Setenv("LC_ALL", "tr_TR.UTF-8")
		var stdout, stderr bytes.Buffer
		code := run(cliInspectArgs(fx, "terminal", false, fx.manifestDigest), &stdout, &stderr)
		if code != 4 || stderr.Len() != 0 || !bytes.Equal(stdout.Bytes(), want) {
			t.Fatalf("exit=%d stdout changed=%v stderr=%q", code, !bytes.Equal(stdout.Bytes(), want), stderr.String())
		}
	})

	t.Run("expected digest mismatch exits three", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run(cliInspectArgs(fx, "terminal", false, model.Digest("sha256:"+strings.Repeat("0", 64))), &stdout, &stderr)
		if code != 3 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "bundle-open-failed") {
			t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	})

	t.Run("revision mismatch exits three", func(t *testing.T) {
		args := cliInspectArgs(fx, "terminal", false, fx.manifestDigest)
		for i := range args {
			if args[i] == "--base-commit" {
				args[i+1] = fx.head.CommitID
			}
		}
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr)
		if code != 3 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "revision-mismatch") {
			t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	})

	failedWaiver := "not: a valid waiver set\n"
	failed := newCLIInspectFixture(t, &failedWaiver)
	defer failed.cleanup()
	t.Run("invalid trusted-base waiver produces report-selected failure", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run(cliInspectArgs(failed, "terminal", false, failed.manifestDigest), &stdout, &stderr)
		if code != 5 || stdout.Len() == 0 || stderr.Len() != 0 {
			t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
		if !strings.Contains(stdout.String(), "GR-WAIVER-001") {
			t.Fatalf("waiver governance finding missing from failure output")
		}
	})
}

func TestInspectCommandOutputWriterFailure(t *testing.T) {
	fx := newCLIInspectFixture(t, nil)
	defer fx.cleanup()
	var stderr bytes.Buffer
	code := run(cliInspectArgs(fx, "terminal", false, fx.manifestDigest), &shortWriter{limit: 8}, &stderr)
	if code != 3 || !strings.Contains(stderr.String(), "output-failed") {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
}

func inspectByAPIForCLI(t *testing.T, fx cliInspectFixture, internalOnly bool) *inspect.Result {
	t.Helper()
	i, err := inspect.New(inspect.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	req := inspect.Request{BundleDir: fx.bundlePath, GitDir: fx.gitDir, BaseCommitID: fx.base.CommitID, HeadCommitID: fx.head.CommitID, EvaluatedAt: fx.evaluatedAt}
	if internalOnly {
		req.ManifestIntegrityMode = inspect.ManifestIntegrityInternalConsistencyOnly
		req.AllowInternalConsistencyOnly = true
	} else {
		req.ManifestIntegrityMode = inspect.ManifestIntegrityExpectedDigest
		req.ExpectedManifestDigest = fx.manifestDigest
	}
	res, err := i.Inspect(context.Background(), req)
	if err != nil {
		t.Fatalf("Inspect API: %v", err)
	}
	return res
}

func renderTerminalForCLI(t *testing.T, res *inspect.Result) []byte {
	t.Helper()
	out, err := report.RenderTerminal(context.Background(), res.Report, report.DefaultRenderLimits())
	if err != nil {
		t.Fatal(err)
	}
	return out.Bytes
}

func renderMarkdownForCLI(t *testing.T, res *inspect.Result) []byte {
	t.Helper()
	out, err := report.RenderMarkdown(context.Background(), res.Report, report.DefaultRenderLimits())
	if err != nil {
		t.Fatal(err)
	}
	return out.Bytes
}

func cliInspectArgs(fx cliInspectFixture, format string, internalOnly bool, digest model.Digest) []string {
	args := []string{"inspect", "--git-dir", fx.gitDir, "--base-commit", fx.base.CommitID, "--head-commit", fx.head.CommitID, "--evaluated-at", fx.evaluatedAt.Format(inspect.TimeFormat), "--format", format}
	if internalOnly {
		args = append(args, "--allow-internal-consistency-only")
	} else {
		args = append(args, "--expected-manifest-digest", string(digest))
	}
	return append(args, fx.bundlePath)
}

type cliInspectFixture struct {
	gitDir         string
	bundlePath     string
	manifestDigest model.Digest
	base           model.CommitRef
	head           model.CommitRef
	evaluatedAt    time.Time
	cleanup        func()
}

func newCLIInspectFixture(t *testing.T, baseWaiver *string) cliInspectFixture {
	t.Helper()
	repo := newCLILooseGitRepo(t)
	base, head := repo.commits(t, baseWaiver)
	bundlePath, manifestDigest := cliWriteInspectBundle(t, repo.dir, base, head)
	return cliInspectFixture{gitDir: repo.dir, bundlePath: bundlePath, manifestDigest: manifestDigest, base: base, head: head, evaluatedAt: time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC), cleanup: func() { _ = os.RemoveAll(repo.dir); _ = os.RemoveAll(bundlePath) }}
}

func cliWriteInspectBundle(t *testing.T, gitDir string, base, head model.CommitRef) (string, model.Digest) {
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
	plan, err := pipeline.Build(context.Background(), pipeline.BuildRequest{RunID: "run-cli-0001", CreatedAt: time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC), Trusted: trusted, BaseSource: cliSourceSnapshot(model.RevisionKindBase, base), HeadSource: cliSourceSnapshot(model.RevisionKindHead, head), Platform: cliPlatform()})
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
	backend, err := fake.New(cliFakeProgram(plan))
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

func cliSourceSnapshot(kind model.RevisionKind, ref model.CommitRef) pipeline.SourceSnapshot {
	return pipeline.SourceSnapshot{RevisionKind: kind, CommitID: ref.CommitID, TreeID: ref.TreeID, ObjectFormat: pipeline.ObjectFormatSHA1, MaterializedTreeDigest: model.Digest("sha256:" + strings.Repeat(string(ref.CommitID[0]), 64)), MaterializationManifestDigest: model.Digest("sha256:" + strings.Repeat(string(ref.CommitID[1]), 64)), Summary: pipeline.SourceSummary{DirectoryCount: 2, RegularFileCount: 2}}
}

func cliPlatform() pipeline.PlatformConstraints {
	return pipeline.PlatformConstraints{MaxCPU: config.MaxCPU, MaxMemoryBytes: config.MaxMemoryBytes, MaxDiskBytes: config.MaxDiskBytes, MaxProcessCount: config.MaxProcessCount, MaxGlobalTimeoutMillis: config.MaxTimeoutMillis, MaxScenarioTimeoutMillis: config.MaxTimeoutMillis, MaxScenarioCount: config.MaxScenarioCount, MaxRepetitions: config.MaxRepetitions, MaxFilesystemRootCount: config.MaxFilesystemRootCount, MaxArtifactCount: config.MaxArtifactCount, MaxArtifactBytes: config.MaxArtifactBytes, MaxLogBytesPerStream: config.MaxLogBytesPerStream, MaxPlanJSONBytes: pipeline.MaxPlanJSONBytes, RequiredNetworkMode: model.NetworkModeDeny}
}

func cliFakeProgram(plan *pipeline.FrozenPlan) fake.Program {
	return fake.Program{PlanDigest: plan.Digest(), Attempts: []fake.AttemptScript{{Revision: model.RevisionKindBase, ScenarioID: "test", Repetition: 1, Events: []fake.SyntheticEvent{}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: cliIntPtr(0), DurationMillis: 20}}, {Revision: model.RevisionKindHead, ScenarioID: "test", Repetition: 1, Events: []fake.SyntheticEvent{{OffsetMillis: 10, Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindNetworkConnection, Network: &model.NetworkObservation{Operation: "connect", Protocol: "tcp", DestinationHost: "canary.example.invalid", DestinationPort: 443, ResolvedAddresses: []string{}, Result: "denied"}}}}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: cliIntPtr(0), DurationMillis: 25}}}}
}

func cliIntPtr(v int) *int { return &v }

type cliLooseGitRepo struct{ dir string }

func newCLILooseGitRepo(t *testing.T) *cliLooseGitRepo {
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
	return &cliLooseGitRepo{dir: dir}
}

func (r *cliLooseGitRepo) commits(t *testing.T, baseWaiver *string) (model.CommitRef, model.CommitRef) {
	t.Helper()
	baseFiles := map[string][]byte{config.PipelinePath: []byte(cliInspectPipelineYAML)}
	headFiles := map[string][]byte{config.PipelinePath: []byte(strings.Replace(cliInspectPipelineYAML, "cpu: 2", "cpu: 3", 1))}
	if baseWaiver != nil {
		baseFiles[waiver.WaiverPath] = []byte(*baseWaiver)
		headFiles[waiver.WaiverPath] = []byte(strings.Replace(*baseWaiver, "2026-07-23T00:00:00Z", "2026-08-01T00:00:00Z", 1))
	}
	baseTree := r.tree(t, baseFiles)
	baseCommit := r.commit(t, baseTree, "base")
	headTree := r.tree(t, headFiles)
	headCommit := r.commit(t, headTree, "head")
	base := model.CommitRef{Kind: model.RevisionKindBase, Repository: "https://example.invalid/org/repo.git", Ref: "refs/heads/main", CommitID: baseCommit, ObjectFormat: model.GitObjectFormatSHA1, TreeID: baseTree, TreeDigest: model.Digest(baseTree)}
	head := model.CommitRef{Kind: model.RevisionKindHead, Repository: "https://example.invalid/org/repo.git", Ref: "refs/pull/1/head", CommitID: headCommit, ObjectFormat: model.GitObjectFormatSHA1, TreeID: headTree, TreeDigest: model.Digest(headTree)}
	return base, head
}

func (r *cliLooseGitRepo) tree(t *testing.T, files map[string][]byte) string {
	t.Helper()
	entries := map[string]string{}
	for path, data := range files {
		if !strings.HasPrefix(path, ".glassroot/") {
			t.Fatalf("unexpected path %s", path)
		}
		entries[strings.TrimPrefix(path, ".glassroot/")] = r.object(t, "blob", data)
	}
	glassrootTree := r.treeObject(t, entries)
	return r.treeObject(t, map[string]string{".glassroot": glassrootTree})
}

func (r *cliLooseGitRepo) treeObject(t *testing.T, entries map[string]string) string {
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

func (r *cliLooseGitRepo) commit(t *testing.T, tree, msg string) string {
	t.Helper()
	body := []byte("tree " + tree + "\nauthor Glassroot <glassroot@example.invalid> 1782180000 +0000\ncommitter Glassroot <glassroot@example.invalid> 1782180000 +0000\n\n" + msg + "\n")
	return r.object(t, "commit", body)
}

func (r *cliLooseGitRepo) object(t *testing.T, typ string, body []byte) string {
	t.Helper()
	store := append([]byte(fmt.Sprintf("%s %d\x00", typ, len(body))), body...)
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

const cliInspectPipelineYAML = `apiVersion: glassroot.dev/v1alpha1
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
  collect:
    filesystem:
      roots:
        - /workspace
      contents: metadata-and-digests
    artifacts: []
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
