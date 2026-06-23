package localrun

import (
	"time"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/pipeline"
)

const (
	dockerDevMaxCPU                   int64 = 16
	dockerDevMaxMemoryBytes           int64 = 64 << 30
	dockerDevMaxDiskBytes             int64 = 64 << 30
	dockerDevMaxProcessCount          int64 = 4096
	dockerDevMaxGlobalTimeoutMillis   int64 = int64((2 * time.Hour) / time.Millisecond)
	dockerDevMaxScenarioTimeoutMillis int64 = int64((1 * time.Hour) / time.Millisecond)
	dockerDevMaxScenarioCount         int64 = 64
	dockerDevMaxRepetitions           int64 = 10
	dockerDevMaxFilesystemRootCount   int64 = 16
	dockerDevMaxArtifactCount         int64 = 64
	dockerDevMaxArtifactBytes         int64 = 1 << 30
	dockerDevMaxLogBytesPerStream     int64 = 100 << 20
)

func dockerDevPlatform() pipeline.PlatformConstraints {
	return pipeline.PlatformConstraints{
		MaxCPU:                   dockerDevMaxCPU,
		MaxMemoryBytes:           dockerDevMaxMemoryBytes,
		MaxDiskBytes:             dockerDevMaxDiskBytes,
		MaxProcessCount:          dockerDevMaxProcessCount,
		MaxGlobalTimeoutMillis:   dockerDevMaxGlobalTimeoutMillis,
		MaxScenarioTimeoutMillis: dockerDevMaxScenarioTimeoutMillis,
		MaxScenarioCount:         dockerDevMaxScenarioCount,
		MaxRepetitions:           dockerDevMaxRepetitions,
		MaxFilesystemRootCount:   dockerDevMaxFilesystemRootCount,
		MaxArtifactCount:         dockerDevMaxArtifactCount,
		MaxArtifactBytes:         dockerDevMaxArtifactBytes,
		MaxLogBytesPerStream:     dockerDevMaxLogBytesPerStream,
		MaxPlanJSONBytes:         pipeline.MaxPlanJSONBytes,
		RequiredNetworkMode:      model.NetworkModeDeny,
	}
}
