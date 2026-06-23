package gitstore

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type EntryKind string

const (
	EntryDirectory      EntryKind = "directory"
	EntryRegularFile    EntryKind = "regular-file"
	EntryExecutableFile EntryKind = "executable-file"
	EntrySymlink        EntryKind = "symlink"
	EntryGitlink        EntryKind = "gitlink"
)

type TreeEntry struct {
	Mode       string
	Type       string
	ObjectID   string
	Size       int64
	SizeKnown  bool
	Path       string
	Kind       EntryKind
	Executable bool
}

func (r *Repository) WalkTree(ctx context.Context, revision ResolvedRevision, visit func(TreeEntry) error) error {
	entries, err := r.ListTree(ctx, revision)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := visit(entry); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ListTree(ctx context.Context, revision ResolvedRevision) ([]TreeEntry, error) {
	if err := r.validateResolvedRevision(revision); err != nil {
		return nil, err
	}
	out, err := r.cmd.runRepoGit(ctx, commandSpec{op: "ls-tree", args: []string{"ls-tree", "-z", "-r", "-t", "-l", revision.TreeID}, stdoutLimit: MaxTreeListingBytes, stderrLimit: MaxGitStderrBytes, timeout: DefaultGitCommandTimeout})
	if err != nil {
		return nil, err
	}
	return parseLsTree(out.Stdout, r.objectFormat)
}

func (r *Repository) treeEntryForPath(ctx context.Context, revision ResolvedRevision, requestedPath string) (TreeEntry, error) {
	if err := r.validateResolvedRevision(revision); err != nil {
		return TreeEntry{}, err
	}
	if err := validateRequestedPath(requestedPath); err != nil {
		return TreeEntry{}, err
	}
	out, err := r.cmd.runRepoGit(ctx, commandSpec{op: "ls-tree", args: []string{"ls-tree", "-z", "-l", revision.TreeID, "--", requestedPath}, stdoutLimit: MaxGitStdoutBytes, stderrLimit: MaxGitStderrBytes, timeout: DefaultGitCommandTimeout})
	if err != nil {
		return TreeEntry{}, err
	}
	entries, err := parseLsTree(out.Stdout, r.objectFormat)
	if err != nil {
		return TreeEntry{}, err
	}
	if len(entries) == 0 {
		return TreeEntry{}, fsNotExist(requestedPath)
	}
	if len(entries) != 1 || entries[0].Path != requestedPath {
		return TreeEntry{}, gitErr(CodeTreeInvalid, "tree", "path", "path lookup returned unexpected entries", nil)
	}
	return entries[0], nil
}

func (r *Repository) validateResolvedRevision(revision ResolvedRevision) error {
	if revision.ObjectFormat != "" && revision.ObjectFormat != r.objectFormat {
		return gitErr(CodeUnsupportedObjectFormat, "revision", "format", "resolved revision object format does not match repository", nil)
	}
	if _, err := validateObjectID(revision.CommitID, r.objectFormat, true); err != nil {
		return err
	}
	if _, err := validateObjectID(revision.TreeID, r.objectFormat, true); err != nil {
		return err
	}
	return nil
}

func parseLsTree(data []byte, format ObjectFormat) ([]TreeEntry, error) {
	if len(data) > MaxTreeListingBytes {
		return nil, gitErr(CodeGitOutputTooLarge, "tree", "ls-tree", "tree listing exceeds byte limit", nil)
	}
	if len(data) == 0 {
		return nil, nil
	}
	records := bytes.Split(data, []byte{0})
	if len(records) > 0 && len(records[len(records)-1]) == 0 {
		records = records[:len(records)-1]
	}
	if len(records) > MaxTreeEntries {
		return nil, gitErr(CodeTreeEntryLimit, "tree", "ls-tree", "tree entry count exceeds limit", nil)
	}
	entries := make([]TreeEntry, 0, len(records))
	seen := make(map[string]EntryKind, len(records))
	for _, rec := range records {
		if len(rec) == 0 {
			return nil, gitErr(CodeTreeInvalid, "tree", "ls-tree", "empty tree record", nil)
		}
		entry, err := parseLsTreeRecord(rec, format)
		if err != nil {
			return nil, err
		}
		if prev, ok := seen[entry.Path]; ok {
			if prev != entry.Kind {
				return nil, pathErr(CodeTreePathConflict, "tree", "ls-tree", entry.Path, "conflicting repeated path", nil)
			}
			return nil, pathErr(CodeDuplicateTreePath, "tree", "ls-tree", entry.Path, "duplicate tree path", nil)
		}
		if err := checkPathConflicts(seen, entry); err != nil {
			return nil, err
		}
		seen[entry.Path] = entry.Kind
		entries = append(entries, entry)
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func parseLsTreeRecord(rec []byte, format ObjectFormat) (TreeEntry, error) {
	tab := bytes.IndexByte(rec, '\t')
	if tab < 0 {
		return TreeEntry{}, gitErr(CodeTreeInvalid, "tree", "ls-tree", "missing path separator", nil)
	}
	header := string(rec[:tab])
	pathBytes := rec[tab+1:]
	if !utf8.Valid(pathBytes) {
		return TreeEntry{}, pathErr(CodeInvalidTreePath, "tree", "ls-tree", string(pathBytes), "path is not valid UTF-8", nil)
	}
	p := string(pathBytes)
	if err := ValidateGitTreePath(p); err != nil {
		return TreeEntry{}, err
	}
	fields := strings.Fields(header)
	if len(fields) != 3 && len(fields) != 4 {
		return TreeEntry{}, gitErr(CodeTreeInvalid, "tree", "ls-tree", "malformed tree header", nil)
	}
	mode, typ, oid := fields[0], fields[1], fields[2]
	oid, err := validateObjectID(oid, format, false)
	if err != nil {
		return TreeEntry{}, err
	}
	entry := TreeEntry{Mode: mode, Type: typ, ObjectID: oid, Path: p}
	if len(fields) == 4 && fields[3] != "-" {
		size, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil || size < 0 {
			return TreeEntry{}, gitErr(CodeTreeInvalid, "tree", "ls-tree", "malformed tree entry size", nil)
		}
		entry.Size = size
		entry.SizeKnown = true
	}
	kind, executable, err := classifyTreeMode(mode, typ)
	if err != nil {
		return TreeEntry{}, err
	}
	entry.Kind = kind
	entry.Executable = executable
	return entry, nil
}

func classifyTreeMode(mode, typ string) (EntryKind, bool, error) {
	switch mode {
	case "040000":
		if typ == "tree" {
			return EntryDirectory, false, nil
		}
	case "100644":
		if typ == "blob" {
			return EntryRegularFile, false, nil
		}
	case "100755":
		if typ == "blob" {
			return EntryExecutableFile, true, nil
		}
	case "120000":
		if typ == "blob" {
			return EntrySymlink, false, nil
		}
	case "160000":
		if typ == "commit" {
			return EntryGitlink, false, nil
		}
	}
	return "", false, gitErr(CodeUnsupportedEntryMode, "tree", "ls-tree", fmt.Sprintf("unsupported mode/type %s/%s", mode, typ), nil)
}

func checkPathConflicts(seen map[string]EntryKind, entry TreeEntry) error {
	parts := strings.Split(entry.Path, "/")
	for i := 1; i < len(parts); i++ {
		ancestor := strings.Join(parts[:i], "/")
		if kind, ok := seen[ancestor]; ok && kind != EntryDirectory {
			return pathErr(CodeTreePathConflict, "tree", "ls-tree", entry.Path, "non-directory path used as ancestor", nil)
		}
	}
	prefix := entry.Path + "/"
	if entry.Kind != EntryDirectory {
		for existing := range seen {
			if strings.HasPrefix(existing, prefix) {
				return pathErr(CodeTreePathConflict, "tree", "ls-tree", entry.Path, "path conflicts with existing descendants", nil)
			}
		}
	}
	return nil
}
