package materialize

import (
	"sort"
	"strings"

	"github.com/mattneel/glassroot/internal/gitstore"
)

type inventory struct {
	All         []sourceEntry
	Directories []sourceEntry
	Files       []sourceEntry
	Symlinks    []sourceEntry
	Gitlinks    []sourceEntry
	Summary     Summary
}

type sourceEntry struct {
	Path       string
	ObjectID   string
	Kind       EntryKind
	GitKind    gitstore.EntryKind
	Mode       string
	Size       int64
	SizeKnown  bool
	Executable bool
}

func preflightInventory(entries []gitstore.TreeEntry, format gitstore.ObjectFormat, limits Limits) (inventory, error) {
	if err := validateLimits(limits); err != nil {
		return inventory{}, err
	}
	if len(entries) > limits.MaxEntries {
		return inventory{}, errCode(CodeEntryLimit, "preflight", "entries", "tree entry count exceeds limit", nil)
	}
	seen := make(map[string]sourceEntry, len(entries))
	var inv inventory
	var metadataBytes int64
	var totalBlobBytes int64
	for _, entry := range entries {
		if err := validateMaterializationPath(entry.Path, limits); err != nil {
			return inventory{}, err
		}
		kind, err := mapEntryKind(entry.Kind)
		if err != nil {
			return inventory{}, pathErr(CodeInvalidTreeEntry, "preflight", "kind", entry.Path, "unsupported tree entry kind", err)
		}
		if _, ok := seen[entry.Path]; ok {
			return inventory{}, pathErr(CodeDuplicateTreeEntry, "preflight", "path", entry.Path, "duplicate tree path", nil)
		}
		if err := validateObjectID(entry.ObjectID, format); err != nil {
			return inventory{}, pathErr(CodeInvalidObjectID, "preflight", "object", entry.Path, "invalid source object id", err)
		}
		if err := validateMode(entry); err != nil {
			return inventory{}, err
		}
		if entry.Size < 0 {
			return inventory{}, pathErr(CodeInvalidTreeEntry, "preflight", "size", entry.Path, "negative tree entry size", nil)
		}
		if (kind == EntryRegularFile || kind == EntryExecutableFile || kind == EntrySymlink) && !entry.SizeKnown {
			return inventory{}, pathErr(CodeInvalidTreeEntry, "preflight", "size", entry.Path, "blob-backed entry is missing size", nil)
		}
		if kind == EntryRegularFile || kind == EntryExecutableFile || kind == EntrySymlink {
			if kind == EntrySymlink && entry.Size > int64(limits.MaxSymlinkTargetBytes) {
				return inventory{}, pathErr(CodeInvalidSymlinkTarget, "preflight", "size", entry.Path, "symlink target exceeds byte limit", nil)
			}
			if entry.Size > limits.MaxSingleFileBytes {
				return inventory{}, pathErr(CodeTotalBytesLimit, "preflight", "size", entry.Path, "entry exceeds single-file limit", nil)
			}
			if totalBlobBytes > limits.MaxTotalBlobBytes-entry.Size {
				return inventory{}, errCode(CodeTotalBytesLimit, "preflight", "size", "tree exceeds total blob byte limit", nil)
			}
			totalBlobBytes += entry.Size
		}
		metadataBytes += int64(len(entry.Path) + len(entry.ObjectID) + len(entry.Mode) + len(entry.Type) + 64)
		if metadataBytes > int64(limits.MaxInventoryMetadataBytes) {
			return inventory{}, errCode(CodeManifestLimit, "preflight", "metadata", "inventory metadata exceeds limit", nil)
		}
		se := sourceEntry{Path: entry.Path, ObjectID: strings.ToLower(entry.ObjectID), Kind: kind, GitKind: entry.Kind, Mode: entry.Mode, Size: entry.Size, SizeKnown: entry.SizeKnown, Executable: entry.Executable}
		if err := checkPreflightConflicts(seen, se); err != nil {
			return inventory{}, err
		}
		seen[entry.Path] = se
		inv.All = append(inv.All, se)
		switch kind {
		case EntryDirectory:
			inv.Directories = append(inv.Directories, se)
			inv.Summary.Directories++
		case EntryRegularFile:
			inv.Files = append(inv.Files, se)
			inv.Summary.RegularFiles++
			inv.Summary.TotalMaterializedFileBytes += entry.Size
		case EntryExecutableFile:
			inv.Files = append(inv.Files, se)
			inv.Summary.ExecutableFiles++
			inv.Summary.TotalMaterializedFileBytes += entry.Size
		case EntrySymlink:
			inv.Symlinks = append(inv.Symlinks, se)
			inv.Summary.Symlinks++
		case EntryGitlink:
			inv.Gitlinks = append(inv.Gitlinks, se)
			inv.Summary.Gitlinks++
			inv.Summary.SkippedEntries++
		}
	}
	if err := enforceCountLimits(inv, limits); err != nil {
		return inventory{}, err
	}
	for _, entry := range inv.All {
		if err := requireExplicitParents(seen, entry); err != nil {
			return inventory{}, err
		}
	}
	sortInventory(&inv)
	return inv, nil
}

func validateMode(entry gitstore.TreeEntry) error {
	switch entry.Kind {
	case gitstore.EntryDirectory:
		if entry.Mode == "040000" && entry.Type == "tree" {
			return nil
		}
	case gitstore.EntryRegularFile:
		if entry.Mode == "100644" && entry.Type == "blob" {
			return nil
		}
	case gitstore.EntryExecutableFile:
		if entry.Mode == "100755" && entry.Type == "blob" {
			return nil
		}
	case gitstore.EntrySymlink:
		if entry.Mode == "120000" && entry.Type == "blob" {
			return nil
		}
	case gitstore.EntryGitlink:
		if entry.Mode == "160000" && entry.Type == "commit" {
			return nil
		}
	}
	return pathErr(CodeInvalidTreeEntry, "preflight", "mode", entry.Path, "unsupported or contradictory tree mode/type", nil)
}

func checkPreflightConflicts(seen map[string]sourceEntry, entry sourceEntry) error {
	parts := strings.Split(entry.Path, "/")
	for i := 1; i < len(parts); i++ {
		ancestor := strings.Join(parts[:i], "/")
		if existing, ok := seen[ancestor]; ok && existing.Kind != EntryDirectory {
			return pathErr(CodeTreePathConflict, "preflight", "path", entry.Path, "non-directory entry used as ancestor", nil)
		}
	}
	if entry.Kind != EntryDirectory {
		prefix := entry.Path + "/"
		for existing := range seen {
			if strings.HasPrefix(existing, prefix) {
				return pathErr(CodeTreePathConflict, "preflight", "path", entry.Path, "non-directory entry has descendants", nil)
			}
		}
	}
	return nil
}

func requireExplicitParents(seen map[string]sourceEntry, entry sourceEntry) error {
	parts := strings.Split(entry.Path, "/")
	for i := 1; i < len(parts); i++ {
		parent := strings.Join(parts[:i], "/")
		existing, ok := seen[parent]
		if !ok || existing.Kind != EntryDirectory {
			return pathErr(CodeTreePathConflict, "preflight", "path", entry.Path, "required parent directory is absent", nil)
		}
	}
	return nil
}

func enforceCountLimits(inv inventory, limits Limits) error {
	if len(inv.All) > limits.MaxEntries || len(inv.Directories) > limits.MaxDirectories || len(inv.Files) > limits.MaxRegularFiles || len(inv.Symlinks) > limits.MaxSymlinks || len(inv.Gitlinks) > limits.MaxGitlinks {
		return errCode(CodeEntryLimit, "preflight", "counts", "entry kind count exceeds limit", nil)
	}
	if inv.Summary.TotalMaterializedFileBytes > limits.MaxTotalBlobBytes {
		return errCode(CodeTotalBytesLimit, "preflight", "size", "materialized file bytes exceed limit", nil)
	}
	return nil
}

func sortInventory(inv *inventory) {
	byPath := func(entries []sourceEntry) {
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	}
	byPath(inv.All)
	sort.SliceStable(inv.Directories, func(i, j int) bool {
		di, dj := pathDepth(inv.Directories[i].Path), pathDepth(inv.Directories[j].Path)
		if di != dj {
			return di < dj
		}
		return inv.Directories[i].Path < inv.Directories[j].Path
	})
	byPath(inv.Files)
	byPath(inv.Symlinks)
	byPath(inv.Gitlinks)
}

func validateObjectID(id string, format gitstore.ObjectFormat) error {
	if len(id) != format.ObjectIDLength() {
		return errCode(CodeInvalidObjectID, "preflight", "object", "object id has wrong length", nil)
	}
	for _, r := range id {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return errCode(CodeInvalidObjectID, "preflight", "object", "object id must be hexadecimal", nil)
	}
	return nil
}
