package demo

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/evidence"
	"github.com/mattneel/glassroot/internal/gitstore"
	"github.com/mattneel/glassroot/internal/inspect"
	"github.com/mattneel/glassroot/internal/materialize"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/pipeline"
	"github.com/mattneel/glassroot/internal/report"
	"github.com/mattneel/glassroot/internal/runner"
	"github.com/mattneel/glassroot/internal/runner/fake"
)

func New(limits Limits) (*Demo, error) {
	l, err := validateLimits(limits)
	if err != nil {
		return nil, err
	}
	return &Demo{limits: l}, nil
}

func (d *Demo) Create(ctx context.Context, req Request) (res *Result, err error) {
	if d == nil {
		return nil, errCode(CodeInvalidLimits, "demo", "demo is nil", nil)
	}
	if runtime.GOOS != "linux" {
		return nil, errCode(CodeUnsupportedPlatform, "platform", "demo creation is initially supported only on Linux", nil)
	}
	if err := validateRequest(req); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, wrap(CodeContextCancelled, "demo", "context cancelled", err)
	}
	ctx, cancel := context.WithTimeout(ctx, d.limits.MaxDemoDuration)
	defer cancel()

	parent := filepath.Dir(req.OutputDir)
	staging, err := createStaging(parent)
	if err != nil {
		return nil, err
	}
	published := false
	defer func() {
		if !published {
			if rmErr := os.RemoveAll(staging); rmErr != nil && err != nil {
				err = errors.Join(err, errCode(CodeCleanupFailed, "cleanup", "remove staging", rmErr))
			}
		}
	}()

	fs, err := createFixtureStore(ctx, filepath.Join(staging, "fixture.git"), req.Fixture, d.limits)
	if err != nil {
		return nil, err
	}
	repo, err := gitstore.Open(ctx, fs.GitDir)
	if err != nil {
		return nil, wrap(CodeFixtureGitOpenFailed, "fixture-git", "open fixture git store", err)
	}
	defer repo.Close()
	baseRev, err := repo.ResolveCommit(ctx, gitstore.ObjectIDSelector(fs.Base.CommitID))
	if err != nil {
		return nil, wrap(CodeFixtureRevisionFailed, "fixture-git", "resolve base", err)
	}
	headRev, err := repo.ResolveCommit(ctx, gitstore.ObjectIDSelector(fs.Head.CommitID))
	if err != nil {
		return nil, wrap(CodeFixtureRevisionFailed, "fixture-git", "resolve head", err)
	}

	matParent := filepath.Join(staging, "materialize-work")
	if err := os.Mkdir(matParent, 0o700); err != nil {
		return nil, wrap(CodeMaterializationFailed, "materialize", "create materialization parent", err)
	}
	baseMat, err := materializeRevision(ctx, matParent, repo, baseRev, model.RevisionKindBase, d.limits.Materialize)
	if err != nil {
		return nil, err
	}
	headMat, err := materializeRevision(ctx, matParent, repo, headRev, model.RevisionKindHead, d.limits.Materialize)
	if err != nil {
		return nil, err
	}
	if err := os.RemoveAll(matParent); err != nil {
		return nil, wrap(CodeMaterializationCleanupFailed, "materialize", "remove materialization parent", err)
	}

	source := gitstore.NewRevisionFileSource(repo)
	trusted, err := config.LoadTrusted(ctx, source, config.TrustedLoadRequest{Base: fs.Base, Head: fs.Head})
	if err != nil {
		return nil, wrap(CodeTrustedConfigFailed, "trusted-config", "load trusted fixture pipeline", err)
	}
	plan, err := pipeline.Build(ctx, pipeline.BuildRequest{RunID: fixtureRunID(req.Fixture), CreatedAt: fixedTime(fixedPlanCreatedAt), Trusted: trusted, BaseSource: baseMat, HeadSource: headMat, Platform: demoPlatform()})
	if err != nil {
		return nil, wrap(CodePlanBuildFailed, "plan", "build frozen plan", err)
	}

	bundlePath, manifestDigest, err := writeEvidence(ctx, staging, plan, req.Fixture, d.limits)
	if err != nil {
		return nil, err
	}
	evidenceDst := filepath.Join(staging, "evidence")
	if err := os.Rename(bundlePath, evidenceDst); err != nil {
		return nil, wrap(CodeEvidenceRelocateFailed, "evidence", "relocate evidence bundle", err)
	}
	_ = os.RemoveAll(filepath.Dir(bundlePath))
	b, err := evidence.OpenAndVerify(ctx, evidenceDst, d.limits.EvidenceReader, evidence.WithExpectedManifestDigest(manifestDigest))
	if err != nil {
		return nil, wrap(CodeEvidenceVerifyFailed, "evidence", "verify relocated evidence", err)
	}
	if err := b.Close(); err != nil {
		return nil, wrap(CodeEvidenceVerifyFailed, "evidence", "close verification bundle", err)
	}

	inspector, err := inspect.New(d.limits.Inspect)
	if err != nil {
		return nil, wrap(CodeInspectFailed, "inspect", "create inspector", err)
	}
	inspected, err := inspector.Inspect(ctx, inspect.Request{BundleDir: evidenceDst, GitDir: fs.GitDir, BaseCommitID: fs.Base.CommitID, HeadCommitID: fs.Head.CommitID, EvaluatedAt: fixedTime(fixedPolicyEvaluatedAt), ManifestIntegrityMode: inspect.ManifestIntegrityExpectedDigest, ExpectedManifestDigest: manifestDigest})
	if err != nil {
		return nil, wrap(CodeInspectFailed, "inspect", "reconstruct report through inspect", err)
	}
	fr := inspected.Report
	mdOut, err := report.RenderMarkdown(ctx, fr, d.limits.Render)
	if err != nil {
		return nil, wrap(CodeReportRenderFailed, "report", "render markdown", err)
	}
	txtOut, err := report.RenderTerminal(ctx, fr, d.limits.Render)
	if err != nil {
		return nil, wrap(CodeReportRenderFailed, "report", "render terminal", err)
	}
	exit := exitCodeForDisposition(inspected.OverallDisposition)
	if exit == 3 {
		return nil, errCode(CodeReportRenderFailed, "report", "unknown disposition", nil)
	}
	metadata := buildMetadata(req.Fixture, fs, plan, manifestDigest, fr, mdOut.Digest, txtOut.Digest, inspected.OverallDisposition, exit)
	metaBytes, err := encodeMetadata(metadata)
	if err != nil {
		return nil, wrap(CodeMetadataInvalid, "metadata", "encode metadata", err)
	}
	if int64(len(metaBytes)) > d.limits.MaxDemoMetadataBytes {
		return nil, errCode(CodeMetadataInvalid, "metadata", "metadata exceeds limit", nil)
	}
	writes := []struct {
		rel  string
		data []byte
	}{{"report.json", fr.JSON()}, {"report.md", mdOut.Bytes}, {"report.txt", txtOut.Bytes}, {"demo.json", metaBytes}}
	for _, w := range writes {
		if err := writeFileExclusive(filepath.Join(staging, w.rel), w.data); err != nil {
			return nil, wrap(CodeMetadataWriteFailed, "metadata", "write demo output file", err)
		}
	}
	if err := syncDir(staging); err != nil {
		return nil, wrap(CodeSyncFailed, "publish", "sync staging", err)
	}
	if _, err := os.Lstat(req.OutputDir); err == nil {
		return nil, usageErr(CodeOutputAlreadyExists, "publish", "output path already exists", nil)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, wrap(CodePublishCollision, "publish", "check output path", err)
	}
	if err := os.Rename(staging, req.OutputDir); err != nil {
		return nil, wrap(CodePublishFailed, "publish", "publish final output", err)
	}
	if err := syncDir(parent); err != nil {
		_ = os.RemoveAll(req.OutputDir)
		return nil, wrap(CodeSyncFailed, "publish", "sync output parent", err)
	}
	published = true
	return &Result{Report: fr, ManifestDigest: manifestDigest, BaseCommitID: fs.Base.CommitID, HeadCommitID: fs.Head.CommitID, EffectiveDisposition: inspected.OverallDisposition, ExpectedExitCode: exit, Metadata: metadata}, nil
}

func createStaging(parent string) (string, error) {
	for i := 0; i < 16; i++ {
		var b [12]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", wrap(CodeStagingCreateFailed, "staging", "random staging name", err)
		}
		p := filepath.Join(parent, ".glassroot-demo-staging-"+hex.EncodeToString(b[:]))
		err := os.Mkdir(p, 0o700)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", wrap(CodeStagingCreateFailed, "staging", "create staging directory", err)
		}
		return p, nil
	}
	return "", errCode(CodeStagingCreateFailed, "staging", "staging collision limit exceeded", nil)
}

func materializeRevision(ctx context.Context, parent string, repo *gitstore.Repository, rev gitstore.ResolvedRevision, kind model.RevisionKind, limits materialize.Limits) (pipeline.SourceSnapshot, error) {
	m, err := materialize.New(parent, materialize.WithLimits(limits))
	if err != nil {
		return pipeline.SourceSnapshot{}, wrap(CodeMaterializationFailed, "materialize", "initialize materializer", err)
	}
	res, err := m.Materialize(ctx, repo, rev)
	if err != nil {
		return pipeline.SourceSnapshot{}, wrap(CodeMaterializationFailed, "materialize", "materialize fixture revision", err)
	}
	snap := sourceSnapshotFromMaterialization(res, kind)
	if res.Workspace != nil {
		if err := res.Workspace.Close(); err != nil {
			return pipeline.SourceSnapshot{}, wrap(CodeMaterializationCleanupFailed, "materialize", "close materialized workspace", err)
		}
	}
	return snap, nil
}

func sourceSnapshotFromMaterialization(res *materialize.Result, kind model.RevisionKind) pipeline.SourceSnapshot {
	return pipeline.SourceSnapshot{RevisionKind: kind, CommitID: res.Revision.CommitID, TreeID: res.Revision.TreeID, ObjectFormat: pipeline.ObjectFormatSHA1, MaterializedTreeDigest: model.Digest(res.MaterializedTreeDigest), MaterializationManifestDigest: model.Digest(res.MaterializationManifestDigest), Summary: pipeline.SourceSummary{DirectoryCount: int64(res.Summary.Directories), RegularFileCount: int64(res.Summary.RegularFiles), ExecutableFileCount: int64(res.Summary.ExecutableFiles), SymlinkCount: int64(res.Summary.Symlinks), GitlinkCount: int64(res.Summary.Gitlinks), LFSPointerCount: int64(res.Summary.LFSPointers), TotalMaterializedFileBytes: res.Summary.TotalMaterializedFileBytes, SkippedEntryCount: int64(res.Summary.SkippedEntries)}, Limitations: sourceLimitations(res.Limitations)}
}
func sourceLimitations(in []materialize.Limitation) []pipeline.SourceLimitation {
	out := make([]pipeline.SourceLimitation, len(in))
	for i, l := range in {
		out[i] = pipeline.SourceLimitation{Code: l.Code, Path: l.Path, Summary: l.Message}
	}
	return out
}

func demoPlatform() pipeline.PlatformConstraints {
	return pipeline.PlatformConstraints{MaxCPU: config.MaxCPU, MaxMemoryBytes: config.MaxMemoryBytes, MaxDiskBytes: config.MaxDiskBytes, MaxProcessCount: config.MaxProcessCount, MaxGlobalTimeoutMillis: config.MaxTimeoutMillis, MaxScenarioTimeoutMillis: config.MaxTimeoutMillis, MaxScenarioCount: config.MaxScenarioCount, MaxRepetitions: config.MaxRepetitions, MaxFilesystemRootCount: config.MaxFilesystemRootCount, MaxArtifactCount: config.MaxArtifactCount, MaxArtifactBytes: config.MaxArtifactBytes, MaxLogBytesPerStream: config.MaxLogBytesPerStream, MaxPlanJSONBytes: pipeline.MaxPlanJSONBytes, RequiredNetworkMode: model.NetworkModeDeny}
}

func writeEvidence(ctx context.Context, staging string, plan *pipeline.FrozenPlan, fixture Fixture, limits Limits) (string, model.Digest, error) {
	parent := filepath.Join(staging, "evidence-parent")
	if err := os.Mkdir(parent, 0o700); err != nil {
		return "", "", wrap(CodeEvidenceWriteFailed, "evidence", "create evidence parent", err)
	}
	writer, err := evidence.NewWriter(parent, limits.Evidence)
	if err != nil {
		return "", "", wrap(CodeEvidenceWriteFailed, "evidence", "create writer", err)
	}
	session, err := writer.Begin(ctx, plan)
	if err != nil {
		return "", "", wrap(CodeEvidenceWriteFailed, "evidence", "begin evidence session", err)
	}
	program, err := buildFakeProgram(plan, fixture)
	if err != nil {
		_ = session.Abort()
		return "", "", wrap(CodeFakeProgramInvalid, "fake", "build fake program", err)
	}
	backend, err := fake.New(program)
	if err != nil {
		_ = session.Abort()
		return "", "", wrap(CodeFakeProgramInvalid, "fake", "initialize fake backend", err)
	}
	result, err := runner.ExecutePlan(ctx, plan, backend, runner.SyntheticTestRequirements(), limits.Runner, session)
	if err != nil {
		_ = session.Abort()
		return "", "", wrap(CodeFakeExecutionFailed, "fake", "execute fake program", err)
	}
	if err := captureLogsAndArtifacts(ctx, session, plan, fixture); err != nil {
		_ = session.Abort()
		return "", "", wrap(CodeEvidenceWriteFailed, "evidence", "capture logs and artifacts", err)
	}
	br, err := session.Commit(ctx, evidence.Complete(result))
	if err != nil {
		return "", "", wrap(CodeEvidenceWriteFailed, "evidence", "commit evidence bundle", err)
	}
	b, err := evidence.OpenAndVerify(ctx, br.Path, limits.EvidenceReader, evidence.WithExpectedManifestDigest(br.ManifestDigest))
	if err != nil {
		return "", "", wrap(CodeEvidenceVerifyFailed, "evidence", "verify evidence bundle", err)
	}
	if err := b.Close(); err != nil {
		return "", "", wrap(CodeEvidenceVerifyFailed, "evidence", "close evidence bundle", err)
	}
	return br.Path, br.ManifestDigest, nil
}

func captureLogsAndArtifacts(ctx context.Context, session *evidence.Session, plan *pipeline.FrozenPlan, fixture Fixture) error {
	attempts, err := runner.ExpandPlanAttempts(plan)
	if err != nil {
		return err
	}
	for _, a := range attempts {
		key := evidence.AttemptKey{Revision: a.Revision, ScenarioID: a.ScenarioID, Repetition: a.Repetition}
		out, err := session.OpenLog(ctx, key, evidence.LogStreamStdout)
		if err != nil {
			return err
		}
		if _, err := out.Write(stdoutBytes); err != nil {
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		er, err := session.OpenLog(ctx, key, evidence.LogStreamStderr)
		if err != nil {
			return err
		}
		if err := er.Close(); err != nil {
			return err
		}
		paths := artifactPathsFor(fixture, a.Revision)
		datas := artifactBytesFor(fixture, a.Revision)
		type pair struct {
			path string
			data []byte
		}
		pairs := make([]pair, 0, len(datas))
		for i, data := range datas {
			pairs = append(pairs, pair{path: paths[i], data: data})
		}
		sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].path < pairs[j].path })
		for _, p := range pairs {
			size := int64(len(p.data))
			_, err := session.AddArtifact(ctx, evidence.ArtifactInput{Attempt: key, LogicalPath: p.path, DeclaredSize: int64Ptr(size), MaxBytes: int64(len(p.data)), Reader: bytes.NewReader(p.data), MediaType: "application/octet-stream"})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func buildMetadata(f Fixture, fs fixtureStore, plan *pipeline.FrozenPlan, manifest model.Digest, fr *report.FrozenReport, mdDigest, txtDigest model.Digest, disposition model.Disposition, exit int) Metadata {
	doc := fr.Document()
	return Metadata{SchemaVersion: SchemaVersionDemoV1Alpha1, FixtureID: fixtureID(f), FixtureVersion: fixtureVersionV1Alpha1, RunID: doc.RunID, PlanCreatedAt: fixedPlanCreatedAt, PolicyEvaluatedAt: fixedPolicyEvaluatedAt, BaseCommitID: fs.Base.CommitID, BaseTreeID: fs.Base.TreeID, HeadCommitID: fs.Head.CommitID, HeadTreeID: fs.Head.TreeID, ObjectFormat: string(model.GitObjectFormatSHA1), PlanDigest: plan.Digest(), ManifestDigest: manifest, BehavioralDeltaDigest: doc.BehavioralDeltaDigest, PolicyEvaluationDigest: doc.BuiltinPolicyEvaluationDigest, PolicyApplicationDigest: doc.PolicyApplicationDigest, ReportDigest: fr.Digest(), MarkdownDigest: mdDigest, TerminalDigest: txtDigest, EffectiveDisposition: disposition, ExpectedCLIExitCode: exit, RelativePaths: MetadataPaths{FixtureGit: "fixture.git", Evidence: "evidence", ReportJSON: "report.json", ReportMarkdown: "report.md", ReportTerminal: "report.txt"}, KeyEvidence: keyEvidenceFromReport(doc)}
}

func exitCodeForDisposition(d model.Disposition) int {
	switch d {
	case model.DispositionPassed:
		return 0
	case model.DispositionRequiresReview:
		return 4
	case model.DispositionFailed:
		return 5
	default:
		return 3
	}
}
func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
