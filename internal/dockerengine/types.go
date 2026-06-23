package dockerengine

import (
	"context"
	"io"
	"time"
)

const MinimumAPIVersion = "1.44"

type Config struct {
	SocketPath        string
	MinimumAPIVersion string
	RequestTimeout    time.Duration
}

type ServerMetadata struct {
	OSType          string
	Architecture    string
	EngineVersion   string
	APIVersion      string
	CgroupVersion   string
	CgroupDriver    string
	Rootless        bool
	SecurityOptions []string
}

type ImageMetadata struct {
	ID              string
	RepoDigests     []string
	OSType          string
	Architecture    string
	DeclaredVolumes []string
}

type Resources struct {
	NanoCPUs        int64
	MemoryBytes     int64
	MemorySwapBytes int64
	PidsLimit       int64
	ShmSizeBytes    int64
}

type BindMount struct {
	HostPath      string
	ContainerPath string
	ReadWrite     bool
}

type TmpfsMount struct {
	Path      string
	SizeBytes int64
	Options   []string
}

type ContainerSpec struct {
	Name                string
	Runtime             string
	Image               string
	Entrypoint          []string
	Command             []string
	Workdir             string
	Env                 []string
	Hostname            string
	User                string
	NetworkDisabled     bool
	NetworkMode         string
	ExposedPorts        []string
	PublishedPorts      []string
	Privileged          bool
	NoNewPrivileges     bool
	SeccompDefault      bool
	CapDrop             []string
	CapAdd              []string
	Devices             []string
	DeviceRequests      []string
	GroupAdd            []string
	HostPID             bool
	HostIPC             bool
	HostUTS             bool
	ReadOnlyRootfs      bool
	Init                bool
	TTY                 bool
	OpenStdin           bool
	AutoRemove          bool
	RestartPolicy       string
	HealthcheckDisabled bool
	LogDriver           string
	Binds               []BindMount
	Tmpfs               []TmpfsMount
	Resources           Resources
	Labels              map[string]string
}

type CreatedContainer struct {
	ID   string
	Name string
}

type WaitResult struct {
	ExitCode  int
	OOMKilled bool
}

type ContainerState struct {
	ID         string
	ExitCode   int
	OOMKilled  bool
	HostConfig ContainerSpec
}

type ReadCloser = io.ReadCloser

type LogStream string

const (
	LogStreamStdout LogStream = "stdout"
	LogStreamStderr LogStream = "stderr"
)

type OutputCounts struct {
	StdoutAccepted        int64
	StderrAccepted        int64
	StdoutObservedAtLeast int64
	StderrObservedAtLeast int64
	StdoutTruncated       bool
	StderrTruncated       bool
}

type Interface interface {
	Metadata() ServerMetadata
	InspectImage(context.Context, string) (ImageMetadata, error)
	CreateContainer(context.Context, ContainerSpec) (CreatedContainer, error)
	AttachContainer(context.Context, string) (io.ReadCloser, error)
	StartContainer(context.Context, string) error
	WaitContainer(context.Context, string) (WaitResult, error)
	InspectContainer(context.Context, string) (ContainerState, error)
	StopContainer(context.Context, string, time.Duration) error
	KillContainer(context.Context, string) error
	RemoveContainer(context.Context, string) error
	Close() error
}
