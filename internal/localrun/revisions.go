package localrun

import (
	"context"
	"fmt"

	"github.com/mattneel/glassroot/internal/artifactcollect"
	"github.com/mattneel/glassroot/internal/gitstore"
	"github.com/mattneel/glassroot/internal/materialize"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/pipeline"
	"github.com/mattneel/glassroot/internal/runner"
	"github.com/mattneel/glassroot/internal/runner/dockerdev"
)

type resolvedInputs struct {
	base gitstore.ResolvedRevision
	head gitstore.ResolvedRevision
}

type attemptWorkspace struct {
	attempt   runner.AttemptRequest
	workspace *materialize.Workspace
	collector *artifactcollect.BoundWorkspace
}

func checkCommitWidth(req Request, format gitstore.ObjectFormat) error {
	want := format.ObjectIDLength()
	if want <= 0 {
		return errCode(CodeRevisionResolveFailed, "revision", "unsupported repository object format", nil)
	}
	if len(req.BaseCommitID) != want {
		return usageErr(CodeInvalidBaseCommit, "request", fmt.Sprintf("base commit must be %d lowercase hex characters for repository object format", want), nil)
	}
	if len(req.HeadCommitID) != want {
		return usageErr(CodeInvalidHeadCommit, "request", fmt.Sprintf("head commit must be %d lowercase hex characters for repository object format", want), nil)
	}
	return nil
}

func commitRef(kind model.RevisionKind, resolved gitstore.ResolvedRevision) (model.CommitRef, error) {
	mf, err := modelObjectFormat(resolved.ObjectFormat)
	if err != nil {
		return model.CommitRef{}, err
	}
	return model.CommitRef{Kind: kind, Repository: "glassroot.local/explicit-bare-git", Ref: "explicit-object-id", CommitID: resolved.CommitID, ObjectFormat: mf, TreeID: resolved.TreeID, TreeDigest: model.Digest(resolved.TreeID)}, nil
}

func modelObjectFormat(format gitstore.ObjectFormat) (model.GitObjectFormat, error) {
	switch format {
	case gitstore.ObjectFormatSHA1:
		return model.GitObjectFormatSHA1, nil
	case gitstore.ObjectFormatSHA256:
		return model.GitObjectFormatSHA256, nil
	default:
		return "", errCode(CodeRevisionResolveFailed, "revision", "unsupported object format", nil)
	}
}

func pipelineObjectFormat(format gitstore.ObjectFormat) (pipeline.ObjectFormat, error) {
	switch format {
	case gitstore.ObjectFormatSHA1:
		return pipeline.ObjectFormatSHA1, nil
	case gitstore.ObjectFormatSHA256:
		return pipeline.ObjectFormatSHA256, nil
	default:
		return "", errCode(CodeRevisionResolveFailed, "revision", "unsupported object format", nil)
	}
}

func materializeSourceSnapshot(ctx context.Context, parent string, repo *gitstore.Repository, rev gitstore.ResolvedRevision, kind model.RevisionKind, limits materialize.Limits) (pipeline.SourceSnapshot, error) {
	m, err := materialize.New(parent, materialize.WithLimits(limits))
	if err != nil {
		return pipeline.SourceSnapshot{}, wrap(CodeMaterializationFailed, "materialize", "initialize materializer", err)
	}
	res, err := m.Materialize(ctx, repo, rev)
	if err != nil {
		return pipeline.SourceSnapshot{}, wrap(CodeMaterializationFailed, "materialize", "materialize source revision", err)
	}
	defer func() { _ = res.Workspace.Close() }()
	snap, err := sourceSnapshotFromMaterialization(res, kind)
	if err != nil {
		return pipeline.SourceSnapshot{}, err
	}
	if err := res.Workspace.Close(); err != nil {
		return pipeline.SourceSnapshot{}, wrap(CodeWorkspaceCleanupFailed, "materialize", "close source snapshot workspace", err)
	}
	return snap, nil
}

func materializeAttemptWorkspace(ctx context.Context, m *materialize.Materializer, repo *gitstore.Repository, rev gitstore.ResolvedRevision, attempt runner.AttemptRequest) (*materialize.Result, error) {
	res, err := m.Materialize(ctx, repo, rev)
	if err != nil {
		return nil, wrap(CodeMaterializationFailed, "materialize", "materialize attempt workspace", err)
	}
	if model.Digest(res.MaterializedTreeDigest) != attempt.MaterializedTreeDigest || model.Digest(res.MaterializationManifestDigest) != attempt.MaterializationManifestDigest {
		_ = res.Workspace.Close()
		return nil, errCode(CodeSourceSnapshotMismatch, "materialize", "attempt workspace digest does not match frozen plan", nil)
	}
	return res, nil
}

func sourceSnapshotFromMaterialization(res *materialize.Result, kind model.RevisionKind) (pipeline.SourceSnapshot, error) {
	if res == nil {
		return pipeline.SourceSnapshot{}, errCode(CodeMaterializationFailed, "materialize", "missing materialization result", nil)
	}
	format, err := pipelineObjectFormat(res.Revision.ObjectFormat)
	if err != nil {
		return pipeline.SourceSnapshot{}, err
	}
	return pipeline.SourceSnapshot{
		RevisionKind:                  kind,
		CommitID:                      res.Revision.CommitID,
		TreeID:                        res.Revision.TreeID,
		ObjectFormat:                  format,
		MaterializedTreeDigest:        model.Digest(res.MaterializedTreeDigest),
		MaterializationManifestDigest: model.Digest(res.MaterializationManifestDigest),
		Summary: pipeline.SourceSummary{
			DirectoryCount:             int64(res.Summary.Directories),
			RegularFileCount:           int64(res.Summary.RegularFiles),
			ExecutableFileCount:        int64(res.Summary.ExecutableFiles),
			SymlinkCount:               int64(res.Summary.Symlinks),
			GitlinkCount:               int64(res.Summary.Gitlinks),
			LFSPointerCount:            int64(res.Summary.LFSPointers),
			TotalMaterializedFileBytes: res.Summary.TotalMaterializedFileBytes,
			SkippedEntryCount:          int64(res.Summary.SkippedEntries),
		},
		Limitations: sourceLimitations(res.Limitations),
	}, nil
}

func sourceLimitations(in []materialize.Limitation) []pipeline.SourceLimitation {
	out := make([]pipeline.SourceLimitation, len(in))
	for i, lim := range in {
		out[i] = pipeline.SourceLimitation{Code: lim.Code, Path: lim.Path, Summary: lim.Message}
	}
	return out
}

func dockerBindingForAttempt(a runner.AttemptRequest, path string) dockerdev.WorkspaceBinding {
	return dockerdev.WorkspaceBinding{AttemptID: a.AttemptID, Revision: a.Revision, ScenarioID: a.ScenarioID, Repetition: a.Repetition, HostPath: path, MaterializedTreeDigest: a.MaterializedTreeDigest}
}
