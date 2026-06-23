package materialize

import "github.com/mattneel/glassroot/internal/gitstore"

type EntryKind string

const (
	EntryDirectory      EntryKind = "directory"
	EntryRegularFile    EntryKind = "regular-file"
	EntryExecutableFile EntryKind = "executable-file"
	EntrySymlink        EntryKind = "symlink"
	EntryGitlink        EntryKind = "gitlink"
)

type Disposition string

const (
	DispositionMaterializedDirectory  Disposition = "materialized-directory"
	DispositionMaterializedFile       Disposition = "materialized-file"
	DispositionMaterializedExecutable Disposition = "materialized-executable"
	DispositionMaterializedSymlink    Disposition = "materialized-symlink"
	DispositionSkippedGitlink         Disposition = "skipped-gitlink"
	DispositionMaterializedLFSPointer Disposition = "materialized-lfs-pointer"
)

type Result struct {
	Workspace                     *Workspace
	Revision                      gitstore.ResolvedRevision
	MaterializedTreeDigest        string
	MaterializationManifestDigest string
	Summary                       Summary
	Entries                       []EntryResult
	Limitations                   []Limitation
}

type Summary struct {
	Directories                int
	RegularFiles               int
	ExecutableFiles            int
	Symlinks                   int
	Gitlinks                   int
	LFSPointers                int
	TotalMaterializedFileBytes int64
	SkippedEntries             int
}

type EntryResult struct {
	Path           string
	SourceObjectID string
	SourceKind     EntryKind
	Disposition    Disposition
	NormalizedMode uint32
	SizeBytes      int64
	ContentDigest  string
	TargetDigest   string
	TargetBytes    int64
	LFSPointer     *LFSPointerMetadata
}

type LFSPointerMetadata struct {
	OID  string
	Size int64
}

type Limitation struct {
	Code    string
	Path    string
	Message string
}

func mapEntryKind(kind gitstore.EntryKind) (EntryKind, error) {
	switch kind {
	case gitstore.EntryDirectory:
		return EntryDirectory, nil
	case gitstore.EntryRegularFile:
		return EntryRegularFile, nil
	case gitstore.EntryExecutableFile:
		return EntryExecutableFile, nil
	case gitstore.EntrySymlink:
		return EntrySymlink, nil
	case gitstore.EntryGitlink:
		return EntryGitlink, nil
	default:
		return "", errCode(CodeInvalidTreeEntry, "preflight", "kind", "unsupported tree entry kind", nil)
	}
}
