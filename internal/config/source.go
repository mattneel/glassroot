package config

import (
	"context"
	"errors"
	"io/fs"

	"github.com/mattneel/glassroot/internal/model"
)

const PipelinePath = ".glassroot/pipeline.yaml"

type EntryKind string

const (
	EntryKindRegularFile EntryKind = "regular-file"
	EntryKindSymlink     EntryKind = "symlink"
	EntryKindGitlink     EntryKind = "gitlink"
	EntryKindDirectory   EntryKind = "directory"
	EntryKindOther       EntryKind = "other"
)

var ErrRevisionFileTooLarge = errors.New("revision file exceeds byte limit")
var ErrRevisionFileMissing = fs.ErrNotExist

// RevisionFileSource reads one raw repository file from an already-selected
// immutable revision. Implementations must return raw blob bytes without
// clean/smudge filters, text conversion, symlink following, submodule traversal,
// LFS fetching, checkout state, or working-tree fallback. GR-5 only defines and
// consumes this contract; GR-6 will implement Git-backed enforcement.
type RevisionFileSource interface {
	ReadFile(ctx context.Context, revision model.CommitRef, path string, maxBytes int64) (RevisionFile, error)
}

type RevisionFile struct {
	Kind       EntryKind
	Data       []byte
	Executable bool
	ObjectID   string
}
