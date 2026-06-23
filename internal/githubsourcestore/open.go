package githubsourcestore

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/mattneel/glassroot/internal/gitstore"
)

type Handle struct {
	metadata Metadata
	repo     *gitstore.Repository
}

func OpenByID(ctx context.Context, sourceRoot string, sourceStoreID string, gitPath string) (*Handle, error) {
	if err := ValidateSourceRoot(sourceRoot, DefaultLimits()); err != nil {
		return nil, err
	}
	storePath, err := LayoutPath(sourceRoot, sourceStoreID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(MetadataPath(storePath))
	if err != nil {
		return nil, wrap(CodeOpenFailed, "open", "metadata read failed", err)
	}
	if len(data) == 0 || len(data) > DefaultLimits().MaxMetadataBytes {
		return nil, errCode(CodeMetadataInvalid, "open", "metadata size rejected", nil)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var meta Metadata
	if err := dec.Decode(&meta); err != nil {
		return nil, wrap(CodeMetadataInvalid, "open", "metadata decode failed", err)
	}
	if err := ValidateMetadata(meta); err != nil {
		return nil, err
	}
	if meta.SourceStoreID != sourceStoreID {
		return nil, errCode(CodeMetadataInvalid, "open", "metadata identity rejected", nil)
	}
	repo, err := gitstore.Open(ctx, filepath.Join(storePath, "repository.git"), gitstore.OpenOptions{GitPath: gitPath})
	if err != nil {
		return nil, wrap(CodeOpenFailed, "open", "gitstore open failed", err)
	}
	base, err := repo.ResolveCommit(ctx, gitstore.RefSelector("refs/glassroot/base"))
	if err != nil {
		_ = repo.Close()
		return nil, wrap(CodeStoreCorrupt, "open", "base ref rejected", err)
	}
	head, err := repo.ResolveCommit(ctx, gitstore.RefSelector("refs/glassroot/head"))
	if err != nil {
		_ = repo.Close()
		return nil, wrap(CodeStoreCorrupt, "open", "head ref rejected", err)
	}
	if base.CommitID != meta.BaseCommitID || base.TreeID != meta.BaseTreeID || head.CommitID != meta.HeadCommitID || head.TreeID != meta.HeadTreeID || string(repo.ObjectFormat()) != meta.ObjectFormat {
		_ = repo.Close()
		return nil, errCode(CodeStoreCorrupt, "open", "repository identities rejected", nil)
	}
	return &Handle{metadata: meta, repo: repo}, nil
}

func (h *Handle) Metadata() Metadata {
	if h == nil {
		return Metadata{}
	}
	cp := h.metadata
	cp.FixedRefs = append([]FixedRef(nil), h.metadata.FixedRefs...)
	cp.Limitations = append([]string(nil), h.metadata.Limitations...)
	return cp
}

func (h *Handle) Repository() *gitstore.Repository {
	if h == nil {
		return nil
	}
	return h.repo
}

func (h *Handle) Close() error {
	if h == nil || h.repo == nil {
		return nil
	}
	return h.repo.Close()
}
