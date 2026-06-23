package materialize

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"sort"

	"github.com/mattneel/glassroot/internal/gitstore"
)

func (m *Materializer) Materialize(ctx context.Context, repo *gitstore.Repository, revision gitstore.ResolvedRevision) (*Result, error) {
	if m == nil {
		return nil, errCode(CodeInvalidParent, "materialize", "receiver", "materializer is nil", nil)
	}
	if err := validateParentAgainstRepository(m.parentDir, repo); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, contextErr(err)
	}
	workCtx, cancel := context.WithTimeout(ctx, m.limits.MaxDuration)
	defer cancel()

	entries, err := repo.ListTree(workCtx, revision)
	if err != nil {
		return nil, errCode(CodeSourceTreeFailed, "source", "list-tree", "read source tree inventory", err)
	}
	inv, err := preflightInventory(entries, repo.ObjectFormat(), m.limits)
	if err != nil {
		return nil, err
	}
	workspace, err := m.createWorkspace()
	if err != nil {
		return nil, err
	}
	res, err := m.materializeInventory(workCtx, repo, revision, workspace, inv)
	if err != nil {
		return nil, cleanupAfterFailure(workspace, err)
	}
	res.Workspace = workspace
	return res, nil
}

func (m *Materializer) materializeInventory(ctx context.Context, repo *gitstore.Repository, revision gitstore.ResolvedRevision, workspace *Workspace, inv inventory) (*Result, error) {
	result := &Result{Revision: revision, Summary: inv.Summary}
	if err := ctx.Err(); err != nil {
		return nil, contextErr(err)
	}
	for _, dir := range inv.Directories {
		if err := workspace.root.Mkdir(dir.Path, 0o755); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return nil, pathErr(CodeDestinationEntryExists, "directory", "mkdir", dir.Path, "destination entry already exists", err)
			}
			return nil, pathErr(CodeDirectoryCreateFailed, "directory", "mkdir", dir.Path, "create destination directory", err)
		}
		result.Entries = append(result.Entries, EntryResult{Path: dir.Path, SourceObjectID: dir.ObjectID, SourceKind: EntryDirectory, Disposition: DispositionMaterializedDirectory, NormalizedMode: 0o755})
	}
	for _, file := range inv.Files {
		entry, err := m.materializeFile(ctx, repo, workspace, file)
		if err != nil {
			return nil, err
		}
		result.Entries = append(result.Entries, entry)
		if entry.LFSPointer != nil {
			result.Summary.LFSPointers++
		}
	}
	for _, link := range inv.Symlinks {
		entry, err := m.materializeSymlink(ctx, repo, workspace, link)
		if err != nil {
			return nil, err
		}
		result.Entries = append(result.Entries, entry)
	}
	for _, gitlink := range inv.Gitlinks {
		result.Entries = append(result.Entries, EntryResult{Path: gitlink.Path, SourceObjectID: gitlink.ObjectID, SourceKind: EntryGitlink, Disposition: DispositionSkippedGitlink})
		if len(result.Limitations) < m.limits.MaxReportedLimitations {
			result.Limitations = append(result.Limitations, Limitation{Code: "skipped-gitlink", Path: gitlink.Path, Message: "gitlink was reported but not traversed or materialized"})
		}
	}
	sort.SliceStable(result.Entries, func(i, j int) bool { return result.Entries[i].Path < result.Entries[j].Path })
	sort.SliceStable(result.Limitations, func(i, j int) bool { return result.Limitations[i].Path < result.Limitations[j].Path })
	treeDigest, manifestDigest, err := computeMaterializationDigests(result.Entries)
	if err != nil {
		return nil, err
	}
	result.MaterializedTreeDigest = treeDigest
	result.MaterializationManifestDigest = manifestDigest
	return result, nil
}

func (m *Materializer) materializeFile(ctx context.Context, repo *gitstore.Repository, workspace *Workspace, entry sourceEntry) (EntryResult, error) {
	if err := ctx.Err(); err != nil {
		return EntryResult{}, contextErr(err)
	}
	if m.hooks.BeforeOpenFile != nil {
		if err := m.hooks.BeforeOpenFile(workspace.path, entry.Path); err != nil {
			return EntryResult{}, pathErr(CodeFileCreateFailed, "file", "test-hook", entry.Path, "before open hook failed", err)
		}
	}
	f, err := workspace.root.OpenFile(entry.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return EntryResult{}, pathErr(CodeDestinationEntryExists, "file", "open", entry.Path, "destination entry already exists", err)
		}
		return EntryResult{}, pathErr(CodeFileCreateFailed, "file", "open", entry.Path, "create destination file", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
	}()
	counter := &countingWriter{w: f}
	meta, err := repo.CopyBlob(ctx, entry.ObjectID, entry.Size, minInt64(m.limits.MaxSingleFileBytes, entry.Size), counter)
	if err != nil {
		return EntryResult{}, pathErr(CodeBlobReadFailed, "file", "copy-blob", entry.Path, "copy source blob", err)
	}
	if counter.n != entry.Size || meta.SizeBytes != entry.Size {
		return EntryResult{}, pathErr(CodeBlobSizeMismatch, "file", "copy-blob", entry.Path, "blob size mismatch", nil)
	}
	st, err := f.Stat()
	if err != nil {
		return EntryResult{}, pathErr(CodeFileWriteFailed, "file", "stat", entry.Path, "stat written file", err)
	}
	if st.Size() != entry.Size {
		return EntryResult{}, pathErr(CodeBlobSizeMismatch, "file", "stat", entry.Path, "written file size mismatch", nil)
	}
	if m.hooks.BeforeFileChmod != nil {
		if err := m.hooks.BeforeFileChmod(workspace.path, entry.Path); err != nil {
			return EntryResult{}, pathErr(CodeFileModeFailed, "file", "test-hook", entry.Path, "before chmod hook failed", err)
		}
	}
	mode := uint32(0o644)
	disposition := DispositionMaterializedFile
	if entry.Kind == EntryExecutableFile {
		mode = 0o755
		disposition = DispositionMaterializedExecutable
	}
	if err := f.Chmod(os.FileMode(mode)); err != nil {
		return EntryResult{}, pathErr(CodeFileModeFailed, "file", "chmod", entry.Path, "set normalized file mode", err)
	}
	if err := f.Close(); err != nil {
		closed = true
		return EntryResult{}, pathErr(CodeFileWriteFailed, "file", "close", entry.Path, "close written file", err)
	}
	closed = true
	lst, err := workspace.root.Lstat(entry.Path)
	if err != nil {
		return EntryResult{}, pathErr(CodeFileWriteFailed, "file", "lstat", entry.Path, "verify destination file", err)
	}
	if !lst.Mode().IsRegular() || lst.Mode().Perm() != os.FileMode(mode) {
		return EntryResult{}, pathErr(CodeFileModeFailed, "file", "verify", entry.Path, "destination file was replaced or has unexpected mode", nil)
	}
	result := EntryResult{Path: entry.Path, SourceObjectID: entry.ObjectID, SourceKind: entry.Kind, Disposition: disposition, NormalizedMode: mode, SizeBytes: entry.Size, ContentDigest: meta.ContentDigest}
	// LFS pointers are materialized as their raw Git blob bytes. ReadPath is not
	// used; this bounded second copy detects only small canonical pointer text.
	if entry.Size <= int64(MaxSymlinkTargetBytes) {
		var buf bytes.Buffer
		if _, err := repo.CopyBlob(ctx, entry.ObjectID, entry.Size, int64(MaxSymlinkTargetBytes), &buf); err != nil {
			return EntryResult{}, pathErr(CodeBlobReadFailed, "file", "lfs-detect", entry.Path, "read blob for LFS pointer detection", err)
		} else if pointer, ok := parseLFSPointer(buf.Bytes()); ok {
			result.Disposition = DispositionMaterializedLFSPointer
			result.LFSPointer = &pointer
		}
	}
	return result, nil
}

func (m *Materializer) materializeSymlink(ctx context.Context, repo *gitstore.Repository, workspace *Workspace, entry sourceEntry) (EntryResult, error) {
	if err := ctx.Err(); err != nil {
		return EntryResult{}, contextErr(err)
	}
	var target bytes.Buffer
	meta, err := repo.CopyBlob(ctx, entry.ObjectID, entry.Size, int64(m.limits.MaxSymlinkTargetBytes), &target)
	if err != nil {
		return EntryResult{}, pathErr(CodeBlobReadFailed, "symlink", "copy-blob", entry.Path, "read symlink blob", err)
	}
	if meta.SizeBytes != entry.Size {
		return EntryResult{}, pathErr(CodeBlobSizeMismatch, "symlink", "copy-blob", entry.Path, "symlink blob size mismatch", nil)
	}
	targetMeta, err := validateSymlinkTarget(entry.Path, target.Bytes(), m.limits)
	if err != nil {
		return EntryResult{}, err
	}
	if err := workspace.root.Symlink(string(target.Bytes()), entry.Path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return EntryResult{}, pathErr(CodeDestinationEntryExists, "symlink", "symlink", entry.Path, "destination entry already exists", err)
		}
		return EntryResult{}, pathErr(CodeSymlinkCreateFailed, "symlink", "symlink", entry.Path, "create symlink", err)
	}
	return EntryResult{Path: entry.Path, SourceObjectID: entry.ObjectID, SourceKind: EntrySymlink, Disposition: DispositionMaterializedSymlink, NormalizedMode: 0o777, SizeBytes: entry.Size, TargetDigest: targetMeta.TargetDigest, TargetBytes: targetMeta.ByteLength}, nil
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.n += int64(n)
	if n != len(p) && err == nil {
		return n, io.ErrShortWrite
	}
	return n, err
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
