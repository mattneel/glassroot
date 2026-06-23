package pipeline

import (
	"time"

	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/model"
)

const (
	MaxPlanJSONBytes       int64 = 16 << 20
	MaxSourceLimitations         = 1000
	MaxLimitationCodeBytes       = 128
	MaxLimitationPathBytes       = 4096
)

type ObjectFormat string

const (
	ObjectFormatSHA1   ObjectFormat = "sha1"
	ObjectFormatSHA256 ObjectFormat = "sha256"
)

type BuildRequest struct {
	RunID      string
	CreatedAt  time.Time
	Trusted    config.TrustedLoadResult
	BaseSource SourceSnapshot
	HeadSource SourceSnapshot
	Platform   PlatformConstraints
}

type SourceSnapshot struct {
	RevisionKind                  model.RevisionKind
	CommitID                      string
	TreeID                        string
	ObjectFormat                  ObjectFormat
	MaterializedTreeDigest        model.Digest
	MaterializationManifestDigest model.Digest
	Summary                       SourceSummary
	Limitations                   []SourceLimitation
}

type SourceSummary struct {
	DirectoryCount             int64
	RegularFileCount           int64
	ExecutableFileCount        int64
	SymlinkCount               int64
	GitlinkCount               int64
	LFSPointerCount            int64
	TotalMaterializedFileBytes int64
	SkippedEntryCount          int64
}

type SourceLimitation struct {
	Code    string
	Path    string
	Summary string
}

type PlatformConstraints struct {
	MaxCPU                   int64
	MaxMemoryBytes           int64
	MaxDiskBytes             int64
	MaxProcessCount          int64
	MaxGlobalTimeoutMillis   int64
	MaxScenarioTimeoutMillis int64
	MaxScenarioCount         int64
	MaxRepetitions           int64
	MaxFilesystemRootCount   int64
	MaxArtifactCount         int64
	MaxArtifactBytes         int64
	MaxLogBytesPerStream     int64
	MaxPlanJSONBytes         int64
	RequiredNetworkMode      model.NetworkMode
}

type FrozenPlan struct {
	doc    model.RunPlan
	json   []byte
	digest model.Digest
}

func (p *FrozenPlan) Document() model.RunPlan {
	if p == nil {
		return model.RunPlan{}
	}
	return cloneRunPlan(p.doc)
}

func (p *FrozenPlan) JSON() []byte {
	if p == nil {
		return nil
	}
	return append([]byte(nil), p.json...)
}

func (p *FrozenPlan) Digest() model.Digest {
	if p == nil {
		return ""
	}
	return p.digest
}
