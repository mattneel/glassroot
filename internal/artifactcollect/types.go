package artifactcollect

import (
	"context"
	"io"
	"os"
	"sync"

	"github.com/mattneel/glassroot/internal/model"
)

type Collector struct {
	limits Limits
	hooks  hooks
}

type hooks struct {
	AfterPreflight func() error
	BeforeOpenFile func(logical string) error
	AfterStoreFile func(logical string) error
}

type BoundWorkspace struct {
	mu        sync.Mutex
	root      *os.Root
	path      string
	identity  fileIdentity
	limits    Limits
	hooks     hooks
	closed    bool
	collected bool
}

type AttemptIdentity struct {
	AttemptID  string             `json:"attemptId"`
	Revision   model.RevisionKind `json:"revision"`
	ScenarioID string             `json:"scenarioId"`
	Repetition uint32             `json:"repetition"`
}

type CollectionPlan struct {
	PlanDigest model.Digest    `json:"planDigest"`
	Attempt    AttemptIdentity `json:"attempt"`
	Workdir    string          `json:"workdir"`
	Rules      []ArtifactRule  `json:"rules"`
}

type ArtifactRule struct {
	ID       string `json:"id"`
	Pattern  string `json:"pattern"`
	MaxBytes int64  `json:"maxBytes"`
}

type ArtifactSink interface {
	StoreArtifact(context.Context, ArtifactInput) (StoredArtifact, error)
}

type ArtifactInput struct {
	Attempt      AttemptIdentity
	LogicalPath  string
	DeclaredSize int64
	MaxBytes     int64
	Executable   bool
	SourceMode   SourceModeFacts
	Reader       io.Reader
}

type StoredArtifact struct {
	ContentDigest model.Digest
	SizeBytes     int64
}

type Result struct {
	PlanDigest         model.Digest       `json:"planDigest"`
	Attempt            AttemptIdentity    `json:"attempt"`
	CollectionComplete bool               `json:"collectionComplete"`
	Inventory          InventorySummary   `json:"inventory"`
	Patterns           []PatternResult    `json:"patterns"`
	Artifacts          []ArtifactResult   `json:"artifacts"`
	Limitations        []model.Limitation `json:"limitations"`
}

type InventorySummary struct {
	EntryCount        int `json:"entryCount"`
	DirectoryCount    int `json:"directoryCount"`
	RegularFileCount  int `json:"regularFileCount"`
	SymlinkCount      int `json:"symlinkCount"`
	SpecialEntryCount int `json:"specialEntryCount"`
}

type PatternDisposition string

const (
	PatternDispositionMatched        PatternDisposition = "matched"
	PatternDispositionNoMatch        PatternDisposition = "no-match"
	PatternDispositionBlockedSymlink PatternDisposition = "blocked-symlink"
	PatternDispositionBlockedSpecial PatternDisposition = "blocked-special"
	PatternDispositionIncomplete     PatternDisposition = "incomplete"
)

type PatternResult struct {
	RuleID       string             `json:"ruleId"`
	Pattern      string             `json:"pattern"`
	Disposition  PatternDisposition `json:"disposition"`
	MatchedPaths []string           `json:"matchedPaths"`
	Limitations  []model.Limitation `json:"limitations"`
}

type ArtifactDisposition string

const (
	ArtifactDispositionStored         ArtifactDisposition = "stored"
	ArtifactDispositionOmittedLimit   ArtifactDisposition = "omitted-limit"
	ArtifactDispositionOmittedSymlink ArtifactDisposition = "omitted-symlink"
	ArtifactDispositionOmittedSpecial ArtifactDisposition = "omitted-special"
)

type SourceModeFacts struct {
	Mode       uint32 `json:"mode"`
	Executable bool   `json:"executable"`
	SetUID     bool   `json:"setuid"`
	SetGID     bool   `json:"setgid"`
	Sticky     bool   `json:"sticky"`
}

type ArtifactResult struct {
	LogicalPath     string              `json:"logicalPath"`
	Disposition     ArtifactDisposition `json:"disposition"`
	ContentDigest   model.Digest        `json:"contentDigest,omitempty"`
	SizeBytes       int64               `json:"sizeBytes"`
	KnownSizeBytes  int64               `json:"knownSizeBytes,omitempty"`
	ApplicableLimit int64               `json:"applicableLimit,omitempty"`
	Executable      bool                `json:"executable"`
	SourceMode      SourceModeFacts     `json:"sourceMode"`
	MatchingRuleIDs []string            `json:"matchingRuleIds"`
	Limitations     []model.Limitation  `json:"limitations"`
}

func New(limits Limits) (*Collector, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}
	return &Collector{limits: limits}, nil
}
