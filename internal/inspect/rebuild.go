package inspect

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/mattneel/glassroot/internal/gitstore"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/pipeline"
)

type resolvedPair struct {
	base gitstore.ResolvedRevision
	head gitstore.ResolvedRevision
}

func bindResolvedRevisions(plan model.RunPlan, repoFormat gitstore.ObjectFormat, base, head gitstore.ResolvedRevision) error {
	if err := requireObjectFormatWidth(repoFormat, base.CommitID, "base"); err != nil {
		return err
	}
	if err := requireObjectFormatWidth(repoFormat, head.CommitID, "head"); err != nil {
		return err
	}
	if plan.Base.CommitID != base.CommitID || plan.Base.Kind != model.RevisionKindBase {
		return errCode(CodeRevisionMismatch, "revision", "base commit does not match verified plan", nil)
	}
	if plan.Head.CommitID != head.CommitID || plan.Head.Kind != model.RevisionKindHead {
		return errCode(CodeRevisionMismatch, "revision", "head commit does not match verified plan", nil)
	}
	if plan.Base.TreeID != "" && plan.Base.TreeID != base.TreeID {
		return errCode(CodeTreeMismatch, "revision", "base tree does not match verified plan", nil)
	}
	if plan.Head.TreeID != "" && plan.Head.TreeID != head.TreeID {
		return errCode(CodeTreeMismatch, "revision", "head tree does not match verified plan", nil)
	}
	if len(plan.Revisions) != 2 || plan.Revisions[0].Kind != model.RevisionKindBase || plan.Revisions[1].Kind != model.RevisionKindHead {
		return errCode(CodePlanMismatch, "rebuild", "verified plan revision inventory is invalid", nil)
	}
	if err := bindRevisionPlan(plan.Revisions[0], base, repoFormat, model.RevisionKindBase); err != nil {
		return err
	}
	if err := bindRevisionPlan(plan.Revisions[1], head, repoFormat, model.RevisionKindHead); err != nil {
		return err
	}
	return nil
}

func bindRevisionPlan(rev model.RevisionPlan, resolved gitstore.ResolvedRevision, repoFormat gitstore.ObjectFormat, kind model.RevisionKind) error {
	if rev.Kind != kind || rev.Commit.Kind != kind || rev.Commit.CommitID != resolved.CommitID {
		return errCode(CodeRevisionMismatch, "revision", string(kind)+" revision does not match explicit commit", nil)
	}
	if rev.TreeID != resolved.TreeID || rev.Commit.TreeID != resolved.TreeID {
		return errCode(CodeTreeMismatch, "revision", string(kind)+" tree does not match explicit commit", nil)
	}
	wantFormat := modelFormat(repoFormat)
	if rev.ObjectFormat != wantFormat || rev.Commit.ObjectFormat != wantFormat {
		return errCode(CodeObjectFormatMismatch, "revision", string(kind)+" object format mismatch", nil)
	}
	return nil
}

func requireObjectFormatWidth(format gitstore.ObjectFormat, id string, label string) error {
	width := format.ObjectIDLength()
	if width == 0 || len(id) != width {
		return errCode(CodeObjectFormatMismatch, "revision", label+" object id width does not match repository object format", nil)
	}
	return nil
}

func modelFormat(format gitstore.ObjectFormat) model.GitObjectFormat {
	switch format {
	case gitstore.ObjectFormatSHA1:
		return model.GitObjectFormatSHA1
	case gitstore.ObjectFormatSHA256:
		return model.GitObjectFormatSHA256
	default:
		return ""
	}
}

func pipelineFormat(format model.GitObjectFormat) (pipeline.ObjectFormat, error) {
	switch format {
	case model.GitObjectFormatSHA1:
		return pipeline.ObjectFormatSHA1, nil
	case model.GitObjectFormatSHA256:
		return pipeline.ObjectFormatSHA256, nil
	default:
		return "", errCode(CodeObjectFormatMismatch, "rebuild", "unsupported plan object format", nil)
	}
}

func trustedCommitRef(planRef model.CommitRef, resolved gitstore.ResolvedRevision, kind model.RevisionKind) model.CommitRef {
	out := planRef
	out.Kind = kind
	out.CommitID = resolved.CommitID
	out.TreeID = resolved.TreeID
	out.TreeDigest = model.Digest(resolved.TreeID)
	out.ObjectFormat = modelFormat(resolved.ObjectFormat)
	return out
}

func sourceSnapshotFromRevision(rev model.RevisionPlan) (pipeline.SourceSnapshot, error) {
	format, err := pipelineFormat(rev.ObjectFormat)
	if err != nil {
		return pipeline.SourceSnapshot{}, err
	}
	if rev.SourceSummary == nil {
		return pipeline.SourceSnapshot{}, errCode(CodePlanMismatch, "rebuild", "source summary is missing", nil)
	}
	return pipeline.SourceSnapshot{
		RevisionKind:                  rev.Kind,
		CommitID:                      rev.Commit.CommitID,
		TreeID:                        rev.TreeID,
		ObjectFormat:                  format,
		MaterializedTreeDigest:        rev.MaterializedTreeDigest,
		MaterializationManifestDigest: rev.MaterializationManifestDigest,
		Summary: pipeline.SourceSummary{
			DirectoryCount:             rev.SourceSummary.DirectoryCount,
			RegularFileCount:           rev.SourceSummary.RegularFileCount,
			ExecutableFileCount:        rev.SourceSummary.ExecutableFileCount,
			SymlinkCount:               rev.SourceSummary.SymlinkCount,
			GitlinkCount:               rev.SourceSummary.GitlinkCount,
			LFSPointerCount:            rev.SourceSummary.LFSPointerCount,
			TotalMaterializedFileBytes: rev.SourceSummary.TotalMaterializedFileBytes,
			SkippedEntryCount:          rev.SourceSummary.SkippedEntryCount,
		},
		Limitations: sourceLimitations(rev.SourceLimitations),
	}, nil
}

func sourceLimitations(in []model.SourceLimitation) []pipeline.SourceLimitation {
	out := make([]pipeline.SourceLimitation, len(in))
	for i, lim := range in {
		out[i] = pipeline.SourceLimitation{Code: lim.Code, Path: lim.Path, Summary: lim.Summary}
	}
	return out
}

func platformFromPlan(plan *model.RunPlanPlatform) (pipeline.PlatformConstraints, error) {
	if plan == nil {
		return pipeline.PlatformConstraints{}, errCode(CodePlanMismatch, "rebuild", "platform constraints are missing", nil)
	}
	return pipeline.PlatformConstraints{
		MaxCPU:                   plan.MaxCPU,
		MaxMemoryBytes:           plan.MaxMemoryBytes,
		MaxDiskBytes:             plan.MaxDiskBytes,
		MaxProcessCount:          plan.MaxProcessCount,
		MaxGlobalTimeoutMillis:   plan.MaxGlobalTimeoutMillis,
		MaxScenarioTimeoutMillis: plan.MaxScenarioTimeoutMillis,
		MaxScenarioCount:         plan.MaxScenarioCount,
		MaxRepetitions:           plan.MaxRepetitions,
		MaxFilesystemRootCount:   plan.MaxFilesystemRootCount,
		MaxArtifactCount:         plan.MaxArtifactCount,
		MaxArtifactBytes:         plan.MaxArtifactBytes,
		MaxLogBytesPerStream:     plan.MaxLogBytesPerStream,
		MaxPlanJSONBytes:         plan.MaxPlanJSONBytes,
		RequiredNetworkMode:      plan.RequiredNetworkMode,
	}, nil
}

func requirePlanEquality(verified model.RunPlan, rebuilt *pipeline.FrozenPlan, manifestDigest model.Digest) error {
	if rebuilt == nil {
		return errCode(CodePlanRebuildFailed, "rebuild", "rebuilt plan is nil", nil)
	}
	if rebuilt.Digest() != manifestDigest {
		return errCode(CodePlanMismatch, "rebuild", "rebuilt plan digest does not match verified bundle plan digest", nil)
	}
	rebuiltDoc := rebuilt.Document()
	if !reflect.DeepEqual(rebuiltDoc, verified) {
		v, _ := json.Marshal(verified)
		r := rebuilt.JSON()
		if string(v) != string(r) {
			return errCode(CodePlanMismatch, "rebuild", "rebuilt plan document does not match verified bundle plan", nil)
		}
	}
	return nil
}

func sourceSnapshotFromRevisionForTest(commit, tree, format string) (pipeline.SourceSnapshot, error) {
	mf := model.GitObjectFormatSHA1
	if strings.EqualFold(format, "sha256") {
		mf = model.GitObjectFormatSHA256
	}
	return sourceSnapshotFromRevision(model.RevisionPlan{Kind: model.RevisionKindBase, Commit: model.CommitRef{Kind: model.RevisionKindBase, CommitID: commit, ObjectFormat: mf, TreeID: tree}, ObjectFormat: mf, TreeID: tree, MaterializedTreeDigest: model.Digest("sha256:" + strings.Repeat("1", 64)), MaterializationManifestDigest: model.Digest("sha256:" + strings.Repeat("2", 64)), SourceSummary: &model.SourceSummary{}, SourceLimitations: []model.SourceLimitation{}, ScenarioIDs: []string{"test"}})
}
