package localrun

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/mattneel/glassroot/internal/artifactcollect"
	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/dockerengine"
	"github.com/mattneel/glassroot/internal/evidence"
	"github.com/mattneel/glassroot/internal/gitstore"
	"github.com/mattneel/glassroot/internal/inspect"
	"github.com/mattneel/glassroot/internal/materialize"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/pipeline"
	"github.com/mattneel/glassroot/internal/report"
	"github.com/mattneel/glassroot/internal/runner"
	"github.com/mattneel/glassroot/internal/runner/dockerdev"
)

func New(limits Limits) (*Runner, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}
	return &Runner{limits: limits}, nil
}

func (r *Runner) Run(ctx context.Context, request Request) (res *Result, err error) {
	if runtime.GOOS != "linux" {
		return nil, errCode(CodeUnsupportedPlatform, "platform", "local docker-dev runs are supported only on Linux", nil)
	}
	if r == nil {
		return nil, errCode(CodeInvalidRequest, "request", "runner is nil", nil)
	}
	if err := r.limits.validate(); err != nil {
		return nil, err
	}
	if err := ValidateRequest(request); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, wrap(CodeContextCancelled, "request", "context cancelled", err)
	}
	ctx, cancel := context.WithTimeout(ctx, r.limits.MaxRunDuration)
	defer cancel()

	parent := filepath.Dir(request.OutputDir)
	staging, err := createStaging(parent)
	if err != nil {
		return nil, wrap(CodeStagingCreateFailed, "publish", "create private staging directory", err)
	}
	published := false
	defer func() {
		if !published {
			cleanupErr := os.RemoveAll(staging)
			if cleanupErr != nil && err != nil {
				err = errors.Join(err, errCode(CodeCleanupFailed, "cleanup", "remove private staging directory", cleanupErr))
			}
		}
	}()

	workspaceParent := filepath.Join(staging, "workspaces")
	evidenceParent := filepath.Join(staging, "evidence-parent")
	for _, dir := range []string{workspaceParent, evidenceParent} {
		if mkErr := ensureDir(dir); mkErr != nil {
			return nil, wrap(CodeStagingCreateFailed, "publish", "create private work directory", mkErr)
		}
	}

	plan, resolved, attempts, attemptWorkspaces, dockerBindings, err := r.preparePlanAndWorkspaces(ctx, request, workspaceParent)
	if err != nil {
		_ = closeAttemptResources(attemptWorkspaces)
		return nil, err
	}
	defer func() {
		if !published {
			if closeErr := closeAttemptResources(attemptWorkspaces); closeErr != nil && err != nil {
				err = errors.Join(err, errCode(CodeWorkspaceCleanupFailed, "workspace", "close attempt workspaces", closeErr))
			}
		}
	}()

	engine, err := dockerengine.Open(ctx, dockerengine.Config{SocketPath: request.DockerSocket, MinimumAPIVersion: dockerengine.MinimumAPIVersion, RequestTimeout: r.limits.DockerRequestTimeout})
	if err != nil {
		return nil, wrap(CodeDockerOpenFailed, "docker", "open explicit Docker Engine socket", err)
	}
	engineOpen := true
	defer func() {
		if engineOpen {
			if closeErr := engine.Close(); closeErr != nil && err != nil {
				err = errors.Join(err, errCode(CodeCleanupFailed, "docker", "close Docker Engine client", closeErr))
			}
		}
	}()
	daemon := engine.Metadata()
	if err := verifyImagePresent(ctx, engine, plan.Document().ExecutionEnvironment.Image); err != nil {
		return nil, err
	}
	dockerLimits := r.runnerDockerLimits(plan)
	dockerRunner, err := dockerdev.New(dockerdev.Config{Engine: engine, Acknowledgement: request.Acknowledgement, Limits: dockerLimits, Workspaces: dockerBindings})
	if err != nil {
		return nil, wrap(CodeRunnerCreateFailed, "docker", "construct docker-dev runner", err)
	}

	execResult, manifestDigest, err := r.executeAndWriteEvidence(ctx, evidenceParent, plan, dockerRunner, attemptWorkspaces)
	if err != nil {
		return nil, err
	}
	if closeErr := closeAttemptResources(attemptWorkspaces); closeErr != nil {
		return nil, wrap(CodeWorkspaceCleanupFailed, "workspace", "close attempt workspaces", closeErr)
	}
	if closeErr := engine.Close(); closeErr != nil {
		engineOpen = false
		return nil, wrap(CodeCleanupFailed, "docker", "close Docker Engine client", closeErr)
	}
	engineOpen = false

	finalEvidence := filepath.Join(staging, "evidence")
	if err := r.verifyRelocateAndVerifyEvidence(ctx, evidenceParent, finalEvidence, manifestDigest); err != nil {
		return nil, err
	}

	inspected, err := r.inspectEvidence(ctx, request, finalEvidence, resolved, manifestDigest)
	if err != nil {
		return nil, err
	}
	md, txt, err := r.renderReports(ctx, inspected.Report)
	if err != nil {
		return nil, err
	}
	exit := exitCodeForDisposition(inspected.OverallDisposition)
	if exit == 3 {
		return nil, errCode(CodeInspectFailed, "inspect", "unknown policy disposition", nil)
	}
	metadata := buildMetadata(request, plan, manifestDigest, inspected.Report, md.Digest, txt.Digest, inspected.OverallDisposition, exit, len(attempts), daemon)
	metadata.ExecutionComplete = execResult.Complete
	metaBytes, err := encodeMetadata(metadata)
	if err != nil {
		return nil, wrap(CodeMetadataWriteFailed, "metadata", "encode run metadata", err)
	}
	if int64(len(metaBytes)) > r.limits.MaxMetadataBytes {
		return nil, errCode(CodeMetadataWriteFailed, "metadata", "run metadata exceeds size limit", nil)
	}
	if err := writeRunOutputs(staging, inspected.Report.JSON(), md.Bytes, txt.Bytes, metaBytes); err != nil {
		return nil, err
	}
	if err := removeAll(workspaceParent); err != nil {
		return nil, wrap(CodeCleanupFailed, "cleanup", "remove private workspaces", err)
	}
	if err := removeAll(evidenceParent); err != nil {
		return nil, wrap(CodeCleanupFailed, "cleanup", "remove private evidence parent", err)
	}
	if err := verifyFinalStagingTree(staging); err != nil {
		return nil, wrap(CodePublishFailed, "publish", "verify final output tree", err)
	}
	if err := syncDir(staging); err != nil {
		return nil, wrap(CodeSyncFailed, "publish", "sync staging directory", err)
	}
	if _, statErr := os.Lstat(request.OutputDir); statErr == nil {
		return nil, errCode(CodePublishCollision, "publish", "output path appeared before final rename", nil)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return nil, wrap(CodePublishFailed, "publish", "inspect output path before rename", statErr)
	}
	if err := os.Rename(staging, request.OutputDir); err != nil {
		return nil, wrap(CodePublishFailed, "publish", "atomically publish output directory", err)
	}
	published = true
	if err := syncDir(parent); err != nil {
		_ = os.RemoveAll(request.OutputDir)
		return nil, wrap(CodeSyncFailed, "publish", "sync output parent directory", err)
	}
	return &Result{Report: inspected.Report, ManifestDigest: manifestDigest, BaseCommitID: resolved.base.CommitID, HeadCommitID: resolved.head.CommitID, OverallDisposition: inspected.OverallDisposition, ExpectedExitCode: exit, Metadata: metadata}, nil
}

func (r *Runner) preparePlanAndWorkspaces(ctx context.Context, request Request, workspaceParent string) (*pipeline.FrozenPlan, resolvedInputs, []runner.AttemptRequest, map[string]*attemptWorkspace, []dockerdev.WorkspaceBinding, error) {
	repo, err := gitstore.Open(ctx, request.GitDir)
	if err != nil {
		return nil, resolvedInputs{}, nil, nil, nil, wrap(CodeGitOpenFailed, "git", "open trusted bare Git store", err)
	}
	defer repo.Close()
	if err := checkCommitWidth(request, repo.ObjectFormat()); err != nil {
		return nil, resolvedInputs{}, nil, nil, nil, err
	}
	base, err := repo.ResolveCommit(ctx, gitstore.ObjectIDSelector(request.BaseCommitID))
	if err != nil {
		return nil, resolvedInputs{}, nil, nil, nil, wrap(CodeRevisionResolveFailed, "git", "resolve base commit", err)
	}
	head, err := repo.ResolveCommit(ctx, gitstore.ObjectIDSelector(request.HeadCommitID))
	if err != nil {
		return nil, resolvedInputs{}, nil, nil, nil, wrap(CodeRevisionResolveFailed, "git", "resolve head commit", err)
	}
	baseRef, err := commitRef(model.RevisionKindBase, base)
	if err != nil {
		return nil, resolvedInputs{}, nil, nil, nil, err
	}
	headRef, err := commitRef(model.RevisionKindHead, head)
	if err != nil {
		return nil, resolvedInputs{}, nil, nil, nil, err
	}
	trusted, err := config.LoadTrusted(ctx, gitstore.NewRevisionFileSource(repo), config.TrustedLoadRequest{Base: baseRef, Head: headRef})
	if err != nil {
		return nil, resolvedInputs{}, nil, nil, nil, wrap(CodeTrustedConfigFailed, "config", "load trusted-base pipeline configuration", err)
	}
	baseSnap, err := materializeSourceSnapshot(ctx, workspaceParent, repo, base, model.RevisionKindBase, r.limits.Materialize)
	if err != nil {
		return nil, resolvedInputs{}, nil, nil, nil, err
	}
	headSnap, err := materializeSourceSnapshot(ctx, workspaceParent, repo, head, model.RevisionKindHead, r.limits.Materialize)
	if err != nil {
		return nil, resolvedInputs{}, nil, nil, nil, err
	}
	plan, err := pipeline.Build(ctx, pipeline.BuildRequest{RunID: request.RunID, CreatedAt: request.CreatedAt, Trusted: trusted, BaseSource: baseSnap, HeadSource: headSnap, Platform: dockerDevPlatform()})
	if err != nil {
		return nil, resolvedInputs{}, nil, nil, nil, wrap(CodePlanBuildFailed, "plan", "build frozen run plan", err)
	}
	attempts, err := runner.ExpandPlanAttempts(plan)
	if err != nil {
		return nil, resolvedInputs{}, nil, nil, nil, wrap(CodePlanBuildFailed, "plan", "expand plan attempts", err)
	}
	if len(attempts) == 0 || len(attempts) > r.limits.MaxAttempts {
		return nil, resolvedInputs{}, nil, nil, nil, errCode(CodeAttemptLimit, "plan", "attempt count exceeds local-run limit", nil)
	}
	collector, err := artifactcollect.New(r.limits.ArtifactCollect)
	if err != nil {
		return nil, resolvedInputs{}, nil, nil, nil, wrap(CodeCollectorBindFailed, "artifacts", "create artifact collector", err)
	}
	materializer, err := materialize.New(workspaceParent, materialize.WithLimits(r.limits.Materialize))
	if err != nil {
		return nil, resolvedInputs{}, nil, nil, nil, wrap(CodeMaterializationFailed, "materialize", "initialize materializer", err)
	}
	attemptMap := make(map[string]*attemptWorkspace, len(attempts))
	bindings := make([]dockerdev.WorkspaceBinding, 0, len(attempts))
	for _, attempt := range attempts {
		rev := base
		if attempt.Revision == model.RevisionKindHead {
			rev = head
		}
		mat, err := materializeAttemptWorkspace(ctx, materializer, repo, rev, attempt)
		if err != nil {
			_ = closeAttemptResources(attemptMap)
			return nil, resolvedInputs{}, nil, nil, nil, err
		}
		bound, err := collector.BindWorkspace(ctx, mat.Workspace.Path())
		if err != nil {
			_ = mat.Workspace.Close()
			_ = closeAttemptResources(attemptMap)
			return nil, resolvedInputs{}, nil, nil, nil, wrap(CodeCollectorBindFailed, "artifacts", "bind artifact collector before execution", err)
		}
		aw := &attemptWorkspace{attempt: attempt, workspace: mat.Workspace, collector: bound}
		attemptMap[attempt.AttemptID] = aw
		bindings = append(bindings, dockerBindingForAttempt(attempt, mat.Workspace.Path()))
	}
	return plan, resolvedInputs{base: base, head: head}, attempts, attemptMap, bindings, nil
}

func (r *Runner) executeAndWriteEvidence(ctx context.Context, parent string, plan *pipeline.FrozenPlan, backend runner.OutputRunner, attempts map[string]*attemptWorkspace) (runner.ExecutionResult, model.Digest, error) {
	writer, err := evidence.NewWriter(parent, r.limits.EvidenceWriter)
	if err != nil {
		return runner.ExecutionResult{}, "", wrap(CodeEvidenceWriteFailed, "evidence", "initialize evidence writer", err)
	}
	session, err := writer.Begin(ctx, plan)
	if err != nil {
		return runner.ExecutionResult{}, "", wrap(CodeEvidenceWriteFailed, "evidence", "begin evidence session", err)
	}
	hooks := newCaptureHooks(session, plan.Digest(), attempts)
	execResult, err := runner.ExecutePlanWithHooks(ctx, plan, backend, runner.WorkloadRequirements([]model.IsolationTier{model.IsolationTierDevelopmentOnly}), r.limits.Runner, session, hooks)
	if err != nil {
		_ = session.Abort()
		return runner.ExecutionResult{}, "", wrap(CodeExecutionFailed, "runner", "execute planned docker-dev attempts", err)
	}
	execResult.Limitations = dedupeModelLimitations(append(execResult.Limitations, dockerDevRunLimitations()...))
	execResult.Limitations = dedupeModelLimitations(append(execResult.Limitations, hooks.limitations...))
	completion := evidence.Complete(execResult)
	if hooks.incomplete {
		completion = evidence.Incomplete(execResult, evidence.FailureRecord{Code: "capture-incomplete", Stage: "capture", Message: "one or more log or artifact captures were truncated, omitted, or blocked", Category: evidence.FailureCategoryCaptureLimit})
	}
	bundle, err := session.Commit(ctx, completion)
	if err != nil {
		_ = session.Abort()
		return runner.ExecutionResult{}, "", wrap(CodeEvidenceWriteFailed, "evidence", "commit evidence bundle", err)
	}
	return execResult, bundle.ManifestDigest, nil
}

func (r *Runner) verifyRelocateAndVerifyEvidence(ctx context.Context, parent, finalEvidence string, digest model.Digest) error {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return wrap(CodeEvidenceVerifyFailed, "evidence", "read evidence parent", err)
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		return errCode(CodeEvidenceVerifyFailed, "evidence", "evidence parent must contain exactly one bundle", nil)
	}
	bundlePath := filepath.Join(parent, entries[0].Name())
	if err := verifyBundle(ctx, bundlePath, digest, r.limits.EvidenceReader); err != nil {
		return err
	}
	if err := os.Rename(bundlePath, finalEvidence); err != nil {
		return wrap(CodeEvidenceRelocateFailed, "evidence", "relocate evidence bundle", err)
	}
	if err := syncDir(filepath.Dir(finalEvidence)); err != nil {
		return wrap(CodeSyncFailed, "evidence", "sync staging after evidence relocation", err)
	}
	if err := verifyBundle(ctx, finalEvidence, digest, r.limits.EvidenceReader); err != nil {
		return err
	}
	return nil
}

func (r *Runner) inspectEvidence(ctx context.Context, request Request, bundlePath string, resolved resolvedInputs, digest model.Digest) (*inspect.Result, error) {
	inspector, err := inspect.New(r.limits.Inspect)
	if err != nil {
		return nil, wrap(CodeInspectFailed, "inspect", "initialize inspector", err)
	}
	res, err := inspector.Inspect(ctx, inspect.Request{BundleDir: bundlePath, GitDir: request.GitDir, BaseCommitID: resolved.base.CommitID, HeadCommitID: resolved.head.CommitID, EvaluatedAt: request.EvaluatedAt, ManifestIntegrityMode: inspect.ManifestIntegrityExpectedDigest, ExpectedManifestDigest: digest})
	if err != nil {
		return nil, wrap(CodeInspectFailed, "inspect", "reconstruct report from verified evidence", err)
	}
	return res, nil
}

func (r *Runner) renderReports(ctx context.Context, fr *report.FrozenReport) (report.RenderedOutput, report.RenderedOutput, error) {
	md, err := report.RenderMarkdown(ctx, fr, r.limits.ReportRender)
	if err != nil {
		return report.RenderedOutput{}, report.RenderedOutput{}, wrap(CodeReportRenderFailed, "report", "render Markdown report", err)
	}
	txt, err := report.RenderTerminal(ctx, fr, r.limits.ReportRender)
	if err != nil {
		return report.RenderedOutput{}, report.RenderedOutput{}, wrap(CodeReportRenderFailed, "report", "render terminal report", err)
	}
	return md, txt, nil
}

func (r *Runner) runnerDockerLimits(plan *pipeline.FrozenPlan) dockerdev.Limits {
	limits := r.limits.DockerDev
	planDoc := plan.Document()
	perStream := minPositive64(limits.MaxStdoutBytes, limits.MaxStderrBytes)
	if planDoc.Collection != nil && planDoc.Collection.LogMaxBytesPerStream > 0 {
		perStream = minPositive64(perStream, planDoc.Collection.LogMaxBytesPerStream)
	}
	perStream = minPositive64(perStream, r.limits.EvidenceWriter.MaxLogBytesPerStream)
	limits.MaxStdoutBytes = perStream
	limits.MaxStderrBytes = perStream
	if limits.MaxTotalOutputBytes <= 0 || limits.MaxTotalOutputBytes > perStream*2 {
		limits.MaxTotalOutputBytes = perStream * 2
	}
	return limits
}

func verifyImagePresent(ctx context.Context, engine dockerengine.Interface, image string) error {
	if _, err := engine.InspectImage(ctx, image); err != nil {
		var de *dockerengine.Error
		if errors.As(err, &de) && de.Code == dockerengine.CodeImageNotPresent {
			return wrap(CodeImageNotPresent, "docker", "required immutable image is not present locally", err)
		}
		return wrap(CodeDockerOpenFailed, "docker", "inspect required immutable image", err)
	}
	return nil
}

func verifyBundle(ctx context.Context, path string, digest model.Digest, limits evidence.ReaderLimits) error {
	bundle, err := evidence.OpenAndVerify(ctx, path, limits, evidence.WithExpectedManifestDigest(digest))
	if err != nil {
		return wrap(CodeEvidenceVerifyFailed, "evidence", "strictly verify evidence bundle", err)
	}
	if err := bundle.Close(); err != nil {
		return wrap(CodeEvidenceVerifyFailed, "evidence", "close verified evidence bundle", err)
	}
	return nil
}

func closeAttemptResources(attempts map[string]*attemptWorkspace) error {
	var out error
	for _, aw := range attempts {
		if aw == nil {
			continue
		}
		if aw.collector != nil {
			out = errors.Join(out, aw.collector.Close())
			aw.collector = nil
		}
		if aw.workspace != nil {
			out = errors.Join(out, aw.workspace.Close())
			aw.workspace = nil
		}
	}
	return out
}

func writeRunOutputs(staging string, reportJSON, markdown, terminal, metadata []byte) error {
	if err := writeFileExclusive(filepath.Join(staging, "report.json"), reportJSON); err != nil {
		return wrap(CodeReportRenderFailed, "report", "write report JSON", err)
	}
	if err := writeFileExclusive(filepath.Join(staging, "report.md"), markdown); err != nil {
		return wrap(CodeReportRenderFailed, "report", "write report Markdown", err)
	}
	if err := writeFileExclusive(filepath.Join(staging, "report.txt"), terminal); err != nil {
		return wrap(CodeReportRenderFailed, "report", "write report terminal text", err)
	}
	if err := writeFileExclusive(filepath.Join(staging, "run.json"), metadata); err != nil {
		return wrap(CodeMetadataWriteFailed, "metadata", "write run metadata", err)
	}
	return nil
}

func dockerDevRunLimitations() []model.Limitation {
	return []model.Limitation{
		{ID: "docker-dev-development-only", Summary: "docker-dev is a development-only backend and is not a hardened security boundary."},
		{ID: "docker-dev-observation-gap", Summary: "docker-dev does not provide comprehensive child-process, filesystem, syscall, or network-broker observation."},
		{ID: "docker-dev-disk-limit-not-enforced", Summary: "docker-dev does not provide a portable exact workspace disk limit."},
		{ID: "docker-dev-root-in-container", Summary: "docker-dev runs the planned command as UID 0 inside an ordinary Docker container with reduced privileges."},
	}
}

func minPositive64(vals ...int64) int64 {
	var out int64
	for _, v := range vals {
		if v <= 0 {
			continue
		}
		if out == 0 || v < out {
			out = v
		}
	}
	return out
}
