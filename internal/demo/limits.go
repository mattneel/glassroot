package demo

import (
	"time"

	"github.com/mattneel/glassroot/internal/evidence"
	"github.com/mattneel/glassroot/internal/inspect"
	"github.com/mattneel/glassroot/internal/materialize"
	"github.com/mattneel/glassroot/internal/report"
	"github.com/mattneel/glassroot/internal/runner"
)

const (
	MaxOutputPathBytes        = 4096
	MaxFixtureGitFiles        = 32
	MaxFixtureGitBlobBytes    = 1 << 20
	MaxFixtureGitTotalBytes   = 8 << 20
	MaxFixtureGitDepth        = 8
	MaxDemoMetadataBytes      = 1 << 20
	MaxDemoKeyEvidenceRecords = 128
	MaxDemoDuration           = 20 * time.Minute
)

type Limits struct {
	MaxOutputPathBytes        int
	MaxFixtureGitFiles        int
	MaxFixtureGitBlobBytes    int64
	MaxFixtureGitTotalBytes   int64
	MaxFixtureGitDepth        int
	MaxDemoMetadataBytes      int64
	MaxDemoKeyEvidenceRecords int
	MaxDemoDuration           time.Duration
	Materialize               materialize.Limits
	Runner                    runner.Limits
	Evidence                  evidence.Limits
	EvidenceReader            evidence.ReaderLimits
	Inspect                   inspect.Limits
	Render                    report.RenderLimits
}

func DefaultLimits() Limits {
	return Limits{
		MaxOutputPathBytes:        MaxOutputPathBytes,
		MaxFixtureGitFiles:        MaxFixtureGitFiles,
		MaxFixtureGitBlobBytes:    MaxFixtureGitBlobBytes,
		MaxFixtureGitTotalBytes:   MaxFixtureGitTotalBytes,
		MaxFixtureGitDepth:        MaxFixtureGitDepth,
		MaxDemoMetadataBytes:      MaxDemoMetadataBytes,
		MaxDemoKeyEvidenceRecords: MaxDemoKeyEvidenceRecords,
		MaxDemoDuration:           MaxDemoDuration,
		Materialize:               materialize.DefaultLimits(),
		Runner:                    runner.DefaultLimits(),
		Evidence:                  evidence.DefaultLimits(),
		EvidenceReader:            evidence.DefaultReaderLimits(),
		Inspect:                   inspect.DefaultLimits(),
		Render:                    report.DefaultRenderLimits(),
	}
}

func validateLimits(l Limits) (Limits, error) {
	if l.MaxOutputPathBytes <= 0 || l.MaxOutputPathBytes > MaxOutputPathBytes ||
		l.MaxFixtureGitFiles <= 0 || l.MaxFixtureGitFiles > MaxFixtureGitFiles ||
		l.MaxFixtureGitBlobBytes <= 0 || l.MaxFixtureGitBlobBytes > MaxFixtureGitBlobBytes ||
		l.MaxFixtureGitTotalBytes <= 0 || l.MaxFixtureGitTotalBytes > MaxFixtureGitTotalBytes ||
		l.MaxFixtureGitDepth <= 0 || l.MaxFixtureGitDepth > MaxFixtureGitDepth ||
		l.MaxDemoMetadataBytes <= 0 || l.MaxDemoMetadataBytes > MaxDemoMetadataBytes ||
		l.MaxDemoKeyEvidenceRecords <= 0 || l.MaxDemoKeyEvidenceRecords > MaxDemoKeyEvidenceRecords ||
		l.MaxDemoDuration <= 0 || l.MaxDemoDuration > MaxDemoDuration {
		return Limits{}, errCode(CodeInvalidLimits, "limits", "invalid demo limits", nil)
	}
	return l, nil
}
