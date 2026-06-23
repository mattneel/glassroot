package dockerdev

import (
	"context"
	"time"

	"github.com/mattneel/glassroot/internal/dockerengine"
	"github.com/mattneel/glassroot/internal/model"
)

const (
	UnsafeDevelopmentAcknowledgementText = "I understand docker-dev is not a security boundary"
	RunnerName                           = "docker-dev"
	RunnerVersion                        = "v1"
)

type UnsafeDevelopmentAcknowledgement struct{ accepted bool }

func AcknowledgeUnsafeDevelopmentRunner(exactText string) (UnsafeDevelopmentAcknowledgement, error) {
	if exactText != UnsafeDevelopmentAcknowledgementText {
		return UnsafeDevelopmentAcknowledgement{}, errCode(CodeAcknowledgementRequired, "acknowledgement", "", "exact unsafe-development acknowledgement is required", nil)
	}
	return UnsafeDevelopmentAcknowledgement{accepted: true}, nil
}

type Limits struct {
	MaxStdoutBytes      int64
	MaxStderrBytes      int64
	MaxTotalOutputBytes int64
	TmpfsSizeBytes      int64
	ShmSizeBytes        int64
	StopGracePeriod     time.Duration
}

func DefaultLimits() Limits {
	return Limits{MaxStdoutBytes: 1 << 20, MaxStderrBytes: 1 << 20, MaxTotalOutputBytes: 2 << 20, TmpfsSizeBytes: 64 << 20, ShmSizeBytes: 16 << 20, StopGracePeriod: 2 * time.Second}
}

type Config struct {
	Engine          dockerengine.Interface
	Acknowledgement UnsafeDevelopmentAcknowledgement
	Limits          Limits
	Workspaces      []WorkspaceBinding
}

type WorkspaceBinding struct {
	AttemptID              string
	Revision               model.RevisionKind
	ScenarioID             string
	Repetition             uint32
	HostPath               string
	MaterializedTreeDigest model.Digest
}

type Runner struct {
	engine     dockerengine.Interface
	limits     Limits
	workspaces map[string]WorkspaceBinding
}

func (r *Runner) Capabilities(context.Context) (model.RunnerCapabilities, error) {
	return Capabilities(), nil
}

func Capabilities() model.RunnerCapabilities {
	return model.RunnerCapabilities{Name: RunnerName, Version: RunnerVersion, IsolationTier: model.IsolationTierDevelopmentOnly, ExecutesTargetCode: true, SyntheticEvidence: false, EnforcesNetworkDeny: true, FreshKernel: false, BrokeredNetwork: false, ProcessEventCollection: false, FilesystemEventCollection: false, SyscallEventCollection: false, ArtifactHashing: false, SnapshotSupport: false}
}
