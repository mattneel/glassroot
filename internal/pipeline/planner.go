package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/model"
)

func Build(ctx context.Context, request BuildRequest) (*FrozenPlan, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if err := validateRunID(request.RunID); err != nil {
		return nil, err
	}
	createdAt, err := normalizeCreatedAt(request.CreatedAt)
	if err != nil {
		return nil, err
	}
	platform, err := validatePlatformConstraints(request.Platform)
	if err != nil {
		return nil, err
	}
	baseSource, err := validateSourceSnapshot(request.BaseSource, model.RevisionKindBase)
	if err != nil {
		return nil, err
	}
	headSource, err := validateSourceSnapshot(request.HeadSource, model.RevisionKindHead)
	if err != nil {
		return nil, err
	}
	trusted := request.Trusted
	if err := validateTrustedConsistency(trusted, baseSource, headSource); err != nil {
		return nil, err
	}
	pipeline := cloneValidatedPipeline(trusted.EffectivePipeline)
	if err := enforcePlatform(pipeline, platform); err != nil {
		return nil, err
	}

	doc, err := buildDocument(request.RunID, createdAt, trusted, pipeline, baseSource, headSource, platform)
	if err != nil {
		return nil, err
	}
	if err := normalizeRunPlan(&doc); err != nil {
		return nil, err
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return nil, errCode(CodeSerializationFailed, "freeze", "json", "marshal run plan", err)
	}
	if int64(len(data)) > platform.MaxPlanJSONBytes || int64(len(data)) > MaxPlanJSONBytes {
		return nil, errCode(CodePlanTooLarge, "freeze", "json", fmt.Sprintf("plan JSON size %d exceeds limit", len(data)), nil)
	}
	return &FrozenPlan{doc: cloneRunPlan(doc), json: append([]byte(nil), data...), digest: planJSONDigest(data)}, nil
}

func normalizeCreatedAt(t time.Time) (time.Time, error) {
	if t.IsZero() {
		return time.Time{}, errCode(CodeInvalidCreatedAt, "request", "createdAt", "createdAt is required", nil)
	}
	_, offset := t.Zone()
	if offset != 0 {
		return time.Time{}, errCode(CodeInvalidCreatedAt, "request", "createdAt", "createdAt must represent UTC", nil)
	}
	out := t.UTC().Round(0)
	if out.Before(minCreatedAt()) || !out.Before(maxCreatedAt()) {
		return time.Time{}, errCode(CodeInvalidCreatedAt, "request", "createdAt", "createdAt is outside supported range", nil)
	}
	return out, nil
}

func minCreatedAt() time.Time { return time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC) }
func maxCreatedAt() time.Time { return time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC) }

func buildDocument(runID string, createdAt time.Time, trusted config.TrustedLoadResult, pipeline config.ValidatedPipeline, baseSource, headSource SourceSnapshot, platform PlatformConstraints) (model.RunPlan, error) {
	baseCommit := commitRefForPlan(trusted.Base, baseSource)
	headCommit := commitRefForPlan(trusted.Head, headSource)
	scenarioIDs := scenarioIDs(pipeline.Scenarios)
	resourceLimits := model.ResourceLimits{
		CPU:           pipeline.Resources.CPU,
		TimeoutMillis: pipeline.Resources.TimeoutMillis,
		MemoryBytes:   pipeline.Resources.MemoryBytes,
		DiskBytes:     pipeline.Resources.DiskBytes,
		CPUMillis:     pipeline.Resources.CPU * 1000,
		ProcessCount:  pipeline.Resources.ProcessCount,
	}
	network := model.NetworkPolicy{Mode: model.NetworkMode(pipeline.Network.Mode), Allowed: []model.NetworkAllowRule{}}
	collectionArtifacts := make([]model.ExpectedArtifactSpec, len(pipeline.Collect.Artifacts))
	for i, artifact := range pipeline.Collect.Artifacts {
		collectionArtifacts[i] = model.ExpectedArtifactSpec{LogicalPath: artifact.Path, Required: false, MaxSizeBytes: artifact.MaxBytes}
	}
	doc := model.RunPlan{
		SchemaVersion:        model.SchemaVersionRunPlanV1Alpha1,
		ID:                   runID + "-plan",
		RunID:                runID,
		CreatedAt:            createdAt,
		PipelineName:         pipeline.Name,
		Configuration:        &model.RunPlanConfig{Source: model.RevisionKindBase, Path: config.PipelinePath, Digest: trusted.BaseFile.Digest, SizeBytes: trusted.BaseFile.SizeBytes, ObjectID: trusted.BaseFile.ObjectID},
		ExecutionEnvironment: &model.RunPlanEnvironment{Image: pipeline.Image, ImageDigest: pipeline.ImageDigest, Workdir: pipeline.Workdir},
		Base:                 baseCommit,
		Head:                 headCommit,
		Revisions:            []model.RevisionPlan{revisionPlan(baseSource, baseCommit, scenarioIDs), revisionPlan(headSource, headCommit, scenarioIDs)},
		Scenarios:            scenarioPlans(pipeline, resourceLimits, network, collectionArtifacts),
		Runner:               model.RunnerCapabilities{},
		ResourceLimits:       resourceLimits,
		NetworkPolicy:        network,
		Environment:          []model.EnvEntry{},
		Collection: &model.CollectionPlan{
			FilesystemRoots:      append([]string(nil), pipeline.Collect.FilesystemRoots...),
			FilesystemContents:   pipeline.Collect.FilesystemContents,
			Artifacts:            append([]model.ExpectedArtifactSpec(nil), collectionArtifacts...),
			LogMaxBytesPerStream: pipeline.Collect.LogMaxBytesPerStream,
		},
		Comparison:  &model.ComparisonPlan{IgnoreFields: append([]string(nil), pipeline.Compare.IgnoreFields...), Repetitions: pipeline.Compare.Repetitions},
		Policy:      &model.PolicyPlan{Profile: pipeline.Policy.Profile},
		Platform:    platformForModel(platform),
		Limitations: []model.Limitation{},
	}
	return doc, nil
}

func commitRefForPlan(ref model.CommitRef, source SourceSnapshot) model.CommitRef {
	out := ref
	format, _, _ := modelFormatAndWidth(source.ObjectFormat)
	out.Kind = source.RevisionKind
	out.CommitID = source.CommitID
	out.ObjectFormat = format
	out.TreeID = source.TreeID
	out.TreeDigest = ""
	return out
}

func revisionPlan(source SourceSnapshot, commit model.CommitRef, scenarioIDs []string) model.RevisionPlan {
	format, _, _ := modelFormatAndWidth(source.ObjectFormat)
	return model.RevisionPlan{
		Kind:                          source.RevisionKind,
		Commit:                        commit,
		ObjectFormat:                  format,
		TreeID:                        source.TreeID,
		MaterializedTreeDigest:        source.MaterializedTreeDigest,
		MaterializationManifestDigest: source.MaterializationManifestDigest,
		SourceSummary:                 sourceSummaryForModel(source.Summary),
		SourceLimitations:             sourceLimitationsForModel(source.Limitations),
		ScenarioIDs:                   append([]string(nil), scenarioIDs...),
	}
}

func scenarioPlans(pipeline config.ValidatedPipeline, limits model.ResourceLimits, network model.NetworkPolicy, artifacts []model.ExpectedArtifactSpec) []model.ScenarioPlan {
	out := make([]model.ScenarioPlan, len(pipeline.Scenarios))
	for i, scenario := range pipeline.Scenarios {
		scenarioLimits := limits
		scenarioLimits.TimeoutMillis = scenario.TimeoutMillis
		out[i] = model.ScenarioPlan{
			ID:          scenario.ID,
			Name:        scenario.Name,
			Shell:       scenario.Shell,
			Run:         scenario.Run,
			Repetitions: pipeline.Compare.Repetitions,
			Command: model.Command{
				Argv:             []string{},
				WorkingDirectory: pipeline.Workdir,
				Environment:      []model.EnvEntry{},
				TimeoutMillis:    scenario.TimeoutMillis,
			},
			ResourceLimits:    scenarioLimits,
			NetworkPolicy:     model.NetworkPolicy{Mode: network.Mode, Allowed: []model.NetworkAllowRule{}},
			ExpectedArtifacts: append([]model.ExpectedArtifactSpec(nil), artifacts...),
		}
	}
	return out
}

func sourceSummaryForModel(in SourceSummary) *model.SourceSummary {
	return &model.SourceSummary{
		DirectoryCount:             in.DirectoryCount,
		RegularFileCount:           in.RegularFileCount,
		ExecutableFileCount:        in.ExecutableFileCount,
		SymlinkCount:               in.SymlinkCount,
		GitlinkCount:               in.GitlinkCount,
		LFSPointerCount:            in.LFSPointerCount,
		TotalMaterializedFileBytes: in.TotalMaterializedFileBytes,
		SkippedEntryCount:          in.SkippedEntryCount,
	}
}

func sourceLimitationsForModel(in []SourceLimitation) []model.SourceLimitation {
	out := make([]model.SourceLimitation, len(in))
	for i, lim := range in {
		out[i] = model.SourceLimitation{Code: lim.Code, Path: lim.Path, Summary: lim.Summary}
	}
	return out
}

func platformForModel(in PlatformConstraints) *model.RunPlanPlatform {
	return &model.RunPlanPlatform{
		MaxCPU:                   in.MaxCPU,
		MaxMemoryBytes:           in.MaxMemoryBytes,
		MaxDiskBytes:             in.MaxDiskBytes,
		MaxProcessCount:          in.MaxProcessCount,
		MaxGlobalTimeoutMillis:   in.MaxGlobalTimeoutMillis,
		MaxScenarioTimeoutMillis: in.MaxScenarioTimeoutMillis,
		MaxScenarioCount:         in.MaxScenarioCount,
		MaxRepetitions:           in.MaxRepetitions,
		MaxFilesystemRootCount:   in.MaxFilesystemRootCount,
		MaxArtifactCount:         in.MaxArtifactCount,
		MaxArtifactBytes:         in.MaxArtifactBytes,
		MaxLogBytesPerStream:     in.MaxLogBytesPerStream,
		MaxPlanJSONBytes:         in.MaxPlanJSONBytes,
		RequiredNetworkMode:      in.RequiredNetworkMode,
	}
}

func normalizeRunPlan(doc *model.RunPlan) error {
	if doc.SchemaVersion != model.SchemaVersionRunPlanV1Alpha1 {
		return errCode(CodeModelInvariant, "normalize", "schemaVersion", "unexpected run-plan schema version", nil)
	}
	if doc.Revisions == nil || doc.Scenarios == nil || doc.Environment == nil || doc.Limitations == nil {
		return errCode(CodeModelInvariant, "normalize", "arrays", "required arrays must be non-nil", nil)
	}
	if len(doc.Revisions) != 2 || doc.Revisions[0].Kind != model.RevisionKindBase || doc.Revisions[1].Kind != model.RevisionKindHead {
		return errCode(CodeModelInvariant, "normalize", "revisions", "revisions must be base then head", nil)
	}
	for i := range doc.Revisions {
		if doc.Revisions[i].ScenarioIDs == nil || doc.Revisions[i].SourceLimitations == nil {
			return errCode(CodeModelInvariant, "normalize", fmt.Sprintf("revisions[%d]", i), "revision arrays must be non-nil", nil)
		}
		sort.SliceStable(doc.Revisions[i].SourceLimitations, func(a, b int) bool {
			left, right := doc.Revisions[i].SourceLimitations[a], doc.Revisions[i].SourceLimitations[b]
			if left.Code != right.Code {
				return left.Code < right.Code
			}
			if left.Path != right.Path {
				return left.Path < right.Path
			}
			return left.Summary < right.Summary
		})
	}
	for i := range doc.Scenarios {
		if doc.Scenarios[i].Command.Argv == nil {
			doc.Scenarios[i].Command.Argv = []string{}
		}
		if doc.Scenarios[i].Command.Environment == nil {
			doc.Scenarios[i].Command.Environment = []model.EnvEntry{}
		}
		if doc.Scenarios[i].NetworkPolicy.Allowed == nil {
			doc.Scenarios[i].NetworkPolicy.Allowed = []model.NetworkAllowRule{}
		}
		if doc.Scenarios[i].ExpectedArtifacts == nil {
			doc.Scenarios[i].ExpectedArtifacts = []model.ExpectedArtifactSpec{}
		}
	}
	if doc.NetworkPolicy.Allowed == nil {
		doc.NetworkPolicy.Allowed = []model.NetworkAllowRule{}
	}
	if doc.Collection != nil {
		if doc.Collection.FilesystemRoots == nil {
			doc.Collection.FilesystemRoots = []string{}
		}
		if doc.Collection.Artifacts == nil {
			doc.Collection.Artifacts = []model.ExpectedArtifactSpec{}
		}
	}
	if doc.Comparison != nil && doc.Comparison.IgnoreFields == nil {
		doc.Comparison.IgnoreFields = []string{}
	}
	return nil
}

func cloneValidatedPipeline(in config.ValidatedPipeline) config.ValidatedPipeline {
	out := in
	out.Network.Allow = append([]string(nil), in.Network.Allow...)
	out.Scenarios = append([]config.ValidatedScenario(nil), in.Scenarios...)
	out.Collect.FilesystemRoots = append([]string(nil), in.Collect.FilesystemRoots...)
	out.Collect.Artifacts = append([]config.ValidatedArtifact(nil), in.Collect.Artifacts...)
	out.Compare.IgnoreFields = append([]string(nil), in.Compare.IgnoreFields...)
	return out
}

func scenarioIDs(scenarios []config.ValidatedScenario) []string {
	out := make([]string, len(scenarios))
	for i, scenario := range scenarios {
		out[i] = scenario.ID
	}
	return out
}
