package localrun

import (
	"time"

	"github.com/mattneel/glassroot/internal/artifactcollect"
	"github.com/mattneel/glassroot/internal/evidence"
	"github.com/mattneel/glassroot/internal/inspect"
	"github.com/mattneel/glassroot/internal/materialize"
	"github.com/mattneel/glassroot/internal/report"
	"github.com/mattneel/glassroot/internal/runner"
	"github.com/mattneel/glassroot/internal/runner/dockerdev"
)

const (
	SchemaVersionLocalRunV1Alpha1    = "glassroot.dev/local-run/v1alpha1"
	LocalRunProfileDockerDevV1Alpha1 = "glassroot.dev/local-run-profile/docker-dev/v1alpha1"
	PlatformProfileDockerDevV1Alpha1 = "glassroot.dev/platform-profile/docker-dev/v1alpha1"
	MaxOutputPathBytes               = 4096
	MaxRunIDBytes                    = 128
	MaxAttempts                      = 128
	MaxRunDuration                   = 2 * time.Hour
	MaxMetadataBytes                 = 1 << 20
)

type Limits struct {
	MaxOutputPathBytes   int
	MaxRunIDBytes        int
	MaxAttempts          int
	MaxRunDuration       time.Duration
	MaxMetadataBytes     int64
	Materialize          materialize.Limits
	Runner               runner.Limits
	DockerRequestTimeout time.Duration
	DockerDev            dockerdev.Limits
	ArtifactCollect      artifactcollect.Limits
	EvidenceWriter       evidence.Limits
	EvidenceReader       evidence.ReaderLimits
	Inspect              inspect.Limits
	ReportRender         report.RenderLimits
}

func DefaultLimits() Limits {
	return Limits{
		MaxOutputPathBytes:   MaxOutputPathBytes,
		MaxRunIDBytes:        MaxRunIDBytes,
		MaxAttempts:          MaxAttempts,
		MaxRunDuration:       MaxRunDuration,
		MaxMetadataBytes:     MaxMetadataBytes,
		Materialize:          materialize.DefaultLimits(),
		Runner:               runner.DefaultLimits(),
		DockerRequestTimeout: 10 * time.Second,
		DockerDev:            dockerdev.DefaultLimits(),
		ArtifactCollect:      artifactcollect.DefaultLimits(),
		EvidenceWriter:       evidence.DefaultLimits(),
		EvidenceReader:       evidence.DefaultReaderLimits(),
		Inspect:              inspect.DefaultLimits(),
		ReportRender:         report.DefaultRenderLimits(),
	}
}

func (l Limits) validate() error {
	d := DefaultLimits()
	if l.MaxOutputPathBytes <= 0 || l.MaxOutputPathBytes > d.MaxOutputPathBytes ||
		l.MaxRunIDBytes <= 0 || l.MaxRunIDBytes > d.MaxRunIDBytes ||
		l.MaxAttempts <= 0 || l.MaxAttempts > d.MaxAttempts ||
		l.MaxRunDuration <= 0 || l.MaxRunDuration > d.MaxRunDuration ||
		l.MaxMetadataBytes <= 0 || l.MaxMetadataBytes > d.MaxMetadataBytes {
		return errCode(CodeInvalidLimits, "limits", "local run limits are outside supported bounds", nil)
	}
	return nil
}
