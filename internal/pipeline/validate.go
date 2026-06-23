package pipeline

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/model"
)

const runIDPattern = `^[a-z0-9][a-z0-9._-]{0,127}$`

func validateRunID(id string) error {
	matched, err := regexp.MatchString(runIDPattern, id)
	if err != nil || id == "" || len(id) > 128 || !utf8.ValidString(id) || !matched || containsControlOrSpace(id) {
		return errCode(CodeInvalidRunID, "request", "runId", "runId must match ^[a-z0-9][a-z0-9._-]{0,127}$", nil)
	}
	return nil
}

func validateSourceSnapshot(snapshot SourceSnapshot, want model.RevisionKind) (SourceSnapshot, error) {
	out := cloneSourceSnapshot(snapshot)
	if out.RevisionKind != want {
		return SourceSnapshot{}, errCode(CodeRevisionMismatch, "source", "revisionKind", fmt.Sprintf("source kind must be %s", want), nil)
	}
	modelFormat, width, err := modelFormatAndWidth(out.ObjectFormat)
	if err != nil {
		return SourceSnapshot{}, err
	}
	if err := validateObjectID(out.CommitID, width); err != nil {
		return SourceSnapshot{}, errCode(CodeInvalidObjectID, "source", "commitId", "commit id must be exact lowercase hex", err)
	}
	if err := validateObjectID(out.TreeID, width); err != nil {
		return SourceSnapshot{}, errCode(CodeInvalidObjectID, "source", "treeId", "tree id must be exact lowercase hex", err)
	}
	if err := validateDigest(out.MaterializedTreeDigest); err != nil {
		return SourceSnapshot{}, errCode(CodeInvalidSourceDigest, "source", "materializedTreeDigest", "materialized tree digest must be sha256 lowercase hex", err)
	}
	if err := validateDigest(out.MaterializationManifestDigest); err != nil {
		return SourceSnapshot{}, errCode(CodeInvalidSourceDigest, "source", "materializationManifestDigest", "manifest digest must be sha256 lowercase hex", err)
	}
	if err := validateSourceSummary(out.Summary); err != nil {
		return SourceSnapshot{}, err
	}
	if len(out.Limitations) > MaxSourceLimitations {
		return SourceSnapshot{}, errCode(CodeInvalidSourceSnapshot, "source", "limitations", "too many source limitations", nil)
	}
	seen := make(map[string]struct{}, len(out.Limitations))
	for i, lim := range out.Limitations {
		p := fmt.Sprintf("limitations[%d]", i)
		if lim.Code == "" || len(lim.Code) > MaxLimitationCodeBytes || !utf8.ValidString(lim.Code) || containsControlOrSpace(lim.Code) {
			return SourceSnapshot{}, errCode(CodeInvalidSourceSnapshot, "source", p+".code", "limitation code is required and bounded", nil)
		}
		if lim.Path != "" {
			if len(lim.Path) > MaxLimitationPathBytes || !validRepositoryPath(lim.Path) {
				return SourceSnapshot{}, errCode(CodeInvalidSourceSnapshot, "source", p+".path", "limitation path must be a bounded repository-relative path", nil)
			}
		}
		if !utf8.ValidString(lim.Summary) || containsControl(lim.Summary) || len(lim.Summary) > config.MaxGeneralStringBytes {
			return SourceSnapshot{}, errCode(CodeInvalidSourceSnapshot, "source", p+".summary", "limitation summary is invalid", nil)
		}
		key := lim.Code + "\x00" + lim.Path + "\x00" + lim.Summary
		if _, ok := seen[key]; ok {
			return SourceSnapshot{}, errCode(CodeInvalidSourceSnapshot, "source", p, "duplicate source limitation", nil)
		}
		seen[key] = struct{}{}
	}
	out.ObjectFormat = ObjectFormat(modelFormat)
	return out, nil
}

func validateSourceSummary(s SourceSummary) error {
	values := []struct {
		path string
		v    int64
	}{
		{"directoryCount", s.DirectoryCount},
		{"regularFileCount", s.RegularFileCount},
		{"executableFileCount", s.ExecutableFileCount},
		{"symlinkCount", s.SymlinkCount},
		{"gitlinkCount", s.GitlinkCount},
		{"lfsPointerCount", s.LFSPointerCount},
		{"totalMaterializedFileBytes", s.TotalMaterializedFileBytes},
		{"skippedEntryCount", s.SkippedEntryCount},
	}
	for _, item := range values {
		if item.v < 0 {
			return errCode(CodeInvalidSourceSummary, "source", item.path, "source summary counts must be nonnegative", nil)
		}
	}
	if s.LFSPointerCount > s.RegularFileCount+s.ExecutableFileCount {
		return errCode(CodeInvalidSourceSummary, "source", "lfsPointerCount", "LFS pointer count cannot exceed file count", nil)
	}
	if s.SkippedEntryCount < s.GitlinkCount {
		return errCode(CodeInvalidSourceSummary, "source", "skippedEntryCount", "skipped entries must include gitlinks", nil)
	}
	return nil
}

func validatePlatformConstraints(p PlatformConstraints) (PlatformConstraints, error) {
	if p.RequiredNetworkMode != model.NetworkModeDeny {
		return PlatformConstraints{}, errCode(CodeUnsupportedNetworkPolicy, "platform", "requiredNetworkMode", "only deny networking is supported", nil)
	}
	checks := []struct {
		path string
		got  int64
		max  int64
	}{
		{"maxCpu", p.MaxCPU, config.MaxCPU},
		{"maxMemoryBytes", p.MaxMemoryBytes, config.MaxMemoryBytes},
		{"maxDiskBytes", p.MaxDiskBytes, config.MaxDiskBytes},
		{"maxProcessCount", p.MaxProcessCount, config.MaxProcessCount},
		{"maxGlobalTimeoutMillis", p.MaxGlobalTimeoutMillis, config.MaxTimeoutMillis},
		{"maxScenarioTimeoutMillis", p.MaxScenarioTimeoutMillis, config.MaxTimeoutMillis},
		{"maxScenarioCount", p.MaxScenarioCount, int64(config.MaxScenarioCount)},
		{"maxRepetitions", p.MaxRepetitions, config.MaxRepetitions},
		{"maxFilesystemRootCount", p.MaxFilesystemRootCount, int64(config.MaxFilesystemRootCount)},
		{"maxArtifactCount", p.MaxArtifactCount, int64(config.MaxArtifactCount)},
		{"maxArtifactBytes", p.MaxArtifactBytes, config.MaxArtifactBytes},
		{"maxLogBytesPerStream", p.MaxLogBytesPerStream, config.MaxLogBytesPerStream},
		{"maxPlanJsonBytes", p.MaxPlanJSONBytes, MaxPlanJSONBytes},
	}
	for _, check := range checks {
		if check.got <= 0 || check.got > check.max {
			return PlatformConstraints{}, errCode(CodeInvalidPlatformConstraints, "platform", check.path, fmt.Sprintf("constraint must be positive and no greater than %d", check.max), nil)
		}
	}
	return p, nil
}

func validateTrustedConsistency(trusted config.TrustedLoadResult, base, head SourceSnapshot) error {
	if trusted.EffectiveSource.Source != config.EffectiveSourceBase || trusted.EffectiveSource.Path != config.PipelinePath || trusted.BaseFile.Path != config.PipelinePath || trusted.EffectiveSource.File.Path != config.PipelinePath {
		return errCode(CodeInvalidTrustedConfig, "trusted", "effectiveSource", "effective configuration must come from base pipeline path", nil)
	}
	if trusted.Base.CommitID == "" || trusted.Head.CommitID == "" {
		return errCode(CodeInvalidTrustedConfig, "trusted", "commitId", "trusted result must include base and head commit ids", nil)
	}
	if trusted.Base.CommitID != base.CommitID || trusted.Head.CommitID != head.CommitID {
		return errCode(CodeTrustedConfigMismatch, "trusted", "commitId", "trusted commits must match source snapshots", nil)
	}
	if trusted.EffectivePipeline.Name == "" || len(trusted.EffectivePipeline.Scenarios) == 0 {
		return errCode(CodeInvalidTrustedConfig, "trusted", "effectivePipeline", "effective pipeline is incomplete", nil)
	}
	if trusted.EffectiveSource.File.Revision.CommitID != trusted.Base.CommitID || trusted.BaseFile.Revision.CommitID != trusted.Base.CommitID {
		return errCode(CodeTrustedConfigMismatch, "trusted", "baseFile.revision", "base configuration metadata must match trusted base commit", nil)
	}
	if trusted.BaseFile.Digest == "" || trusted.EffectiveSource.File.Digest == "" || trusted.BaseFile.Digest != trusted.EffectiveSource.File.Digest {
		return errCode(CodeInvalidTrustedConfig, "trusted", "baseFile.digest", "base configuration digest is required", nil)
	}
	if err := validateDigest(trusted.BaseFile.Digest); err != nil {
		return errCode(CodeInvalidTrustedConfig, "trusted", "baseFile.digest", "base configuration digest must be sha256 lowercase hex", err)
	}
	if trusted.BaseFile.SizeBytes <= 0 || trusted.EffectiveSource.File.SizeBytes != trusted.BaseFile.SizeBytes {
		return errCode(CodeInvalidTrustedConfig, "trusted", "baseFile.sizeBytes", "base configuration size is required", nil)
	}
	return nil
}

func enforcePlatform(pipeline config.ValidatedPipeline, platform PlatformConstraints) error {
	checks := []struct {
		path string
		got  int64
		max  int64
	}{
		{"spec.resources.cpu", pipeline.Resources.CPU, platform.MaxCPU},
		{"spec.resources.memoryBytes", pipeline.Resources.MemoryBytes, platform.MaxMemoryBytes},
		{"spec.resources.diskBytes", pipeline.Resources.DiskBytes, platform.MaxDiskBytes},
		{"spec.resources.processCount", pipeline.Resources.ProcessCount, platform.MaxProcessCount},
		{"spec.resources.timeoutMillis", pipeline.Resources.TimeoutMillis, platform.MaxGlobalTimeoutMillis},
		{"spec.scenarios", int64(len(pipeline.Scenarios)), platform.MaxScenarioCount},
		{"spec.compare.repetitions", pipeline.Compare.Repetitions, platform.MaxRepetitions},
		{"spec.collect.filesystem.roots", int64(len(pipeline.Collect.FilesystemRoots)), platform.MaxFilesystemRootCount},
		{"spec.collect.artifacts", int64(len(pipeline.Collect.Artifacts)), platform.MaxArtifactCount},
		{"spec.collect.logs.maxBytesPerStream", pipeline.Collect.LogMaxBytesPerStream, platform.MaxLogBytesPerStream},
	}
	for _, check := range checks {
		if check.got > check.max {
			return errCode(CodePlatformLimitExceeded, "platform", check.path, fmt.Sprintf("requested %d exceeds platform ceiling %d", check.got, check.max), nil)
		}
	}
	for i, scenario := range pipeline.Scenarios {
		if scenario.TimeoutMillis > platform.MaxScenarioTimeoutMillis {
			return errCode(CodePlatformLimitExceeded, "platform", fmt.Sprintf("spec.scenarios[%d].timeoutMillis", i), fmt.Sprintf("requested %d exceeds platform ceiling %d", scenario.TimeoutMillis, platform.MaxScenarioTimeoutMillis), nil)
		}
	}
	for i, artifact := range pipeline.Collect.Artifacts {
		if artifact.MaxBytes > platform.MaxArtifactBytes {
			return errCode(CodePlatformLimitExceeded, "platform", fmt.Sprintf("spec.collect.artifacts[%d].maxBytes", i), fmt.Sprintf("requested %d exceeds platform ceiling %d", artifact.MaxBytes, platform.MaxArtifactBytes), nil)
		}
	}
	if pipeline.Network.Mode != string(platform.RequiredNetworkMode) {
		return errCode(CodeUnsupportedNetworkPolicy, "platform", "spec.network.mode", "pipeline network mode does not match platform requirement", nil)
	}
	return nil
}

func modelFormatAndWidth(format ObjectFormat) (model.GitObjectFormat, int, error) {
	switch format {
	case ObjectFormatSHA1:
		return model.GitObjectFormatSHA1, 40, nil
	case ObjectFormatSHA256:
		return model.GitObjectFormatSHA256, 64, nil
	default:
		return "", 0, errCode(CodeInvalidObjectFormat, "source", "objectFormat", "object format must be sha1 or sha256", nil)
	}
}

func validateObjectID(id string, width int) error {
	if len(id) != width || !utf8.ValidString(id) {
		return fmt.Errorf("object id length must be %d", width)
	}
	for _, r := range id {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return fmt.Errorf("object id must be lowercase hexadecimal")
	}
	return nil
}

func validateDigest(d model.Digest) error {
	s := string(d)
	if !strings.HasPrefix(s, "sha256:") || len(s) != len("sha256:")+64 || !utf8.ValidString(s) {
		return fmt.Errorf("digest must be sha256:<64 lowercase hex>")
	}
	for _, r := range strings.TrimPrefix(s, "sha256:") {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return fmt.Errorf("digest must be lowercase hexadecimal")
	}
	return nil
}

func validRepositoryPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.ContainsRune(p, 0) || strings.Contains(p, "\\") || !utf8.ValidString(p) || containsControl(p) || len(p) > MaxLimitationPathBytes {
		return false
	}
	if path.Clean(p) != p {
		return false
	}
	parts := strings.Split(p, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.EqualFold(part, ".git") {
			return false
		}
	}
	return true
}

func containsControlOrSpace(s string) bool {
	for _, r := range s {
		if r <= 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func containsControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func contextError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return errCode(CodeContextCancelled, "request", "context", "context cancelled", err)
	}
	return nil
}
