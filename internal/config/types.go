package config

const (
	APIVersionV1Alpha1 = "glassroot.dev/v1alpha1"
	KindPipeline       = "Pipeline"
)

const (
	MaxPipelineBytes      = 1 << 20
	MaxYAMLDepth          = 32
	MaxYAMLNodes          = 20000
	MaxDiagnostics        = 100
	MaxGeneralStringBytes = 4 << 10
	MaxRunBytes           = 64 << 10
	MaxPathBytes          = 4096
	MaxImageBytes         = 512
	MaxIdentifierBytes    = 64
	MaxScenarioNameBytes  = 256
)

const (
	MinCPU                 int64 = 1
	MaxCPU                 int64 = 64
	MinMemoryBytes         int64 = 16 * 1024 * 1024
	MaxMemoryBytes         int64 = 1 * 1024 * 1024 * 1024 * 1024
	MinDiskBytes           int64 = 16 * 1024 * 1024
	MaxDiskBytes           int64 = 16 * 1024 * 1024 * 1024 * 1024
	MinProcessCount        int64 = 1
	MaxProcessCount        int64 = 65535
	MinTimeoutMillis       int64 = 100
	MaxTimeoutMillis       int64 = 24 * 60 * 60 * 1000
	MinArtifactBytes       int64 = 1
	MaxArtifactBytes       int64 = 1 * 1024 * 1024 * 1024
	MinLogBytesPerStream   int64 = 1
	MaxLogBytesPerStream   int64 = 100 * 1024 * 1024
	MinRepetitions         int64 = 1
	MaxRepetitions         int64 = 10
	MinScenarioCount             = 1
	MaxScenarioCount             = 64
	MaxFilesystemRootCount       = 16
	MaxArtifactCount             = 64
	MaxCompareIgnoreCount        = 32
)

const (
	NetworkModeDeny             = "deny"
	FilesystemContentsMetadata  = "metadata-and-digests"
	CompareIgnoreEventTimestamp = "event.timestamp"
	CompareIgnoreProcessPID     = "process.pid"
	PolicyProfileStrict         = "strict"
	ShellBinSH                  = "/bin/sh"
	ShellBinBash                = "/bin/bash"
	ShellUsrBinBash             = "/usr/bin/bash"
)

type StringValue struct {
	Present bool
	Null    bool
	Value   string
	Line    int
	Column  int
}

type IntValue struct {
	Present bool
	Null    bool
	Value   int64
	Line    int
	Column  int
}

type SequencePresence struct {
	Present bool
	Null    bool
	Line    int
	Column  int
}

type Document struct {
	APIVersion StringValue
	Kind       StringValue
	Metadata   Metadata
	Spec       Spec
}

type Metadata struct {
	Present bool
	Null    bool
	Name    StringValue
	Line    int
	Column  int
}

type Spec struct {
	Present          bool
	Null             bool
	Line             int
	Column           int
	Environment      Environment
	Resources        Resources
	Network          Network
	ScenariosPresent bool
	ScenariosNull    bool
	ScenariosLine    int
	ScenariosColumn  int
	Scenarios        []Scenario
	Collect          Collect
	Compare          Compare
	Policy           Policy
}

type Environment struct {
	Present bool
	Null    bool
	Line    int
	Column  int
	Image   StringValue
	Workdir StringValue
}

type Resources struct {
	Present   bool
	Null      bool
	Line      int
	Column    int
	CPU       IntValue
	Memory    StringValue
	Disk      StringValue
	Processes IntValue
	Timeout   StringValue
}

type Network struct {
	Present  bool
	Null     bool
	Line     int
	Column   int
	Mode     StringValue
	Allow    SequencePresence
	AllowLen int
}

type Scenario struct {
	Line    int
	Column  int
	Null    bool
	ID      StringValue
	Name    StringValue
	Shell   StringValue
	Run     StringValue
	Timeout StringValue
}

type Collect struct {
	Present          bool
	Null             bool
	Line             int
	Column           int
	Filesystem       FilesystemCollect
	ArtifactsPresent bool
	ArtifactsNull    bool
	ArtifactsLine    int
	ArtifactsColumn  int
	Artifacts        []ArtifactCollect
	Logs             LogsCollect
}

type FilesystemCollect struct {
	Present      bool
	Null         bool
	Line         int
	Column       int
	RootsPresent bool
	RootsNull    bool
	RootsLine    int
	RootsColumn  int
	Roots        []StringValue
	Contents     StringValue
}

type ArtifactCollect struct {
	Line     int
	Column   int
	Null     bool
	Path     StringValue
	MaxBytes StringValue
}

type LogsCollect struct {
	Present           bool
	Null              bool
	Line              int
	Column            int
	MaxBytesPerStream StringValue
}

type Compare struct {
	Present       bool
	Null          bool
	Line          int
	Column        int
	IgnorePresent bool
	IgnoreNull    bool
	IgnoreLine    int
	IgnoreColumn  int
	Ignore        []IgnoreField
	Repetitions   IntValue
}

type IgnoreField struct {
	Line   int
	Column int
	Null   bool
	Field  StringValue
}

type Policy struct {
	Present bool
	Null    bool
	Line    int
	Column  int
	Profile StringValue
}

type ValidatedPipeline struct {
	Name        string
	Image       string
	ImageDigest string
	Workdir     string
	Resources   ValidatedResources
	Network     ValidatedNetwork
	Scenarios   []ValidatedScenario
	Collect     ValidatedCollect
	Compare     ValidatedCompare
	Policy      ValidatedPolicy
}

type ValidatedResources struct {
	CPU           int64
	MemoryBytes   int64
	DiskBytes     int64
	ProcessCount  int64
	TimeoutMillis int64
}

type ValidatedNetwork struct {
	Mode  string
	Allow []string
}

type ValidatedScenario struct {
	ID            string
	Name          string
	Shell         string
	Run           string
	TimeoutMillis int64
}

type ValidatedCollect struct {
	FilesystemRoots      []string
	FilesystemContents   string
	Artifacts            []ValidatedArtifact
	LogMaxBytesPerStream int64
}

type ValidatedArtifact struct {
	Path     string
	MaxBytes int64
}

type ValidatedCompare struct {
	IgnoreFields []string
	Repetitions  int64
}

type ValidatedPolicy struct {
	Profile string
}
