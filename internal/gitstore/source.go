package gitstore

import (
	"context"
	"errors"
	"io/fs"

	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/model"
)

type RevisionFileSource struct {
	repo *Repository
}

func NewRevisionFileSource(repo *Repository) *RevisionFileSource {
	return &RevisionFileSource{repo: repo}
}

func (s *RevisionFileSource) ReadFile(ctx context.Context, revision model.CommitRef, requestedPath string, maxBytes int64) (config.RevisionFile, error) {
	if s == nil || s.repo == nil {
		return config.RevisionFile{}, gitErr(CodeGitCommandFailed, "source", "read", "nil repository", nil)
	}
	resolved, err := resolvedFromCommitRef(s.repo.objectFormat, s.repo.version, revision.CommitID, string(revision.TreeDigest))
	if err != nil {
		return config.RevisionFile{}, err
	}
	if resolved.TreeID == "" {
		resolved.TreeID, err = s.repo.treeForCommit(ctx, resolved.CommitID)
		if err != nil {
			return config.RevisionFile{}, err
		}
	}
	file, err := s.repo.ReadPath(ctx, resolved, requestedPath, maxBytes)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return config.RevisionFile{}, fs.ErrNotExist
		}
		return config.RevisionFile{}, err
	}
	return config.RevisionFile{Kind: toConfigEntryKind(file.Kind), Data: append([]byte(nil), file.Data...), Executable: file.Executable, ObjectID: file.ObjectID}, nil
}

func toConfigEntryKind(kind EntryKind) config.EntryKind {
	switch kind {
	case EntryRegularFile:
		return config.EntryKindRegularFile
	case EntryExecutableFile:
		return config.EntryKindRegularFile
	case EntrySymlink:
		return config.EntryKindSymlink
	case EntryGitlink:
		return config.EntryKindGitlink
	case EntryDirectory:
		return config.EntryKindDirectory
	default:
		return config.EntryKindOther
	}
}
