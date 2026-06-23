package materialize

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/gitstore"
)

func TestPreflightRejectsHostileInventoryBeforeWrites(t *testing.T) {
	limits := DefaultLimits()
	oid := strings.Repeat("a", 40)
	valid := []gitstore.TreeEntry{
		{Path: "dir", Kind: gitstore.EntryDirectory, Mode: "040000", Type: "tree", ObjectID: oid},
		{Path: "dir/file.txt", Kind: gitstore.EntryRegularFile, Mode: "100644", Type: "blob", ObjectID: oid, SizeKnown: true, Size: 5},
		{Path: "run.sh", Kind: gitstore.EntryExecutableFile, Mode: "100755", Type: "blob", ObjectID: oid, SizeKnown: true, Size: 7},
		{Path: "link", Kind: gitstore.EntrySymlink, Mode: "120000", Type: "blob", ObjectID: oid, SizeKnown: true, Size: 4},
		{Path: "submodule", Kind: gitstore.EntryGitlink, Mode: "160000", Type: "commit", ObjectID: oid},
	}
	inv, err := preflightInventory(valid, gitstore.ObjectFormatSHA1, limits)
	if err != nil {
		t.Fatalf("valid inventory: %v", err)
	}
	if len(inv.Directories) != 1 || len(inv.Files) != 2 || len(inv.Symlinks) != 1 || len(inv.Gitlinks) != 1 {
		t.Fatalf("partitioned inventory = %#v", inv)
	}
	cases := []struct {
		name string
		mut  func([]gitstore.TreeEntry) []gitstore.TreeEntry
		want error
	}{
		{"unknown kind", func(in []gitstore.TreeEntry) []gitstore.TreeEntry {
			in[1].Kind = gitstore.EntryKind("device")
			return in
		}, ErrInvalidTreeEntry},
		{"bad object", func(in []gitstore.TreeEntry) []gitstore.TreeEntry {
			in[1].ObjectID = strings.Repeat("b", 39)
			return in
		}, ErrInvalidObjectID},
		{"duplicate", func(in []gitstore.TreeEntry) []gitstore.TreeEntry { return append(in, in[1]) }, ErrDuplicateTreeEntry},
		{"file ancestor", func(in []gitstore.TreeEntry) []gitstore.TreeEntry {
			in[0] = gitstore.TreeEntry{Path: "dir", Kind: gitstore.EntryRegularFile, Mode: "100644", Type: "blob", ObjectID: oid, SizeKnown: true, Size: 1}
			return in
		}, ErrTreePathConflict},
		{"missing parent", func(in []gitstore.TreeEntry) []gitstore.TreeEntry { return in[1:2] }, ErrTreePathConflict},
		{"absolute", func(in []gitstore.TreeEntry) []gitstore.TreeEntry { in[1].Path = "/abs"; return in }, ErrInvalidTreeEntry},
		{"traversal", func(in []gitstore.TreeEntry) []gitstore.TreeEntry { in[1].Path = "dir/../x"; return in }, ErrInvalidTreeEntry},
		{"backslash", func(in []gitstore.TreeEntry) []gitstore.TreeEntry { in[1].Path = "dir\\x"; return in }, ErrInvalidTreeEntry},
		{"control", func(in []gitstore.TreeEntry) []gitstore.TreeEntry { in[1].Path = "dir/\x1fx"; return in }, ErrInvalidTreeEntry},
		{"dot git", func(in []gitstore.TreeEntry) []gitstore.TreeEntry {
			in[0].Path = ".Git"
			in[1].Path = ".Git/config"
			return in
		}, ErrInvalidTreeEntry},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			copyEntries := append([]gitstore.TreeEntry(nil), valid...)
			_, err := preflightInventory(tc.mut(copyEntries), gitstore.ObjectFormatSHA1, limits)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestPreflightLimitsAndOverflow(t *testing.T) {
	oid := strings.Repeat("a", 40)
	limits := DefaultLimits()
	limits.MaxRegularFiles = 1
	entries := []gitstore.TreeEntry{
		{Path: "a.txt", Kind: gitstore.EntryRegularFile, Mode: "100644", Type: "blob", ObjectID: oid, SizeKnown: true, Size: 1},
		{Path: "b.txt", Kind: gitstore.EntryRegularFile, Mode: "100644", Type: "blob", ObjectID: oid, SizeKnown: true, Size: 1},
	}
	if _, err := preflightInventory(entries, gitstore.ObjectFormatSHA1, limits); !errors.Is(err, ErrEntryLimit) {
		t.Fatalf("count limit err = %v, want entry-limit", err)
	}
	limits = DefaultLimits()
	limits.MaxTotalBlobBytes = 3
	entries = []gitstore.TreeEntry{{Path: "a.txt", Kind: gitstore.EntryRegularFile, Mode: "100644", Type: "blob", ObjectID: oid, SizeKnown: true, Size: 4}}
	if _, err := preflightInventory(entries, gitstore.ObjectFormatSHA1, limits); !errors.Is(err, ErrTotalBytesLimit) {
		t.Fatalf("byte limit err = %v, want total-bytes-limit", err)
	}
	entries[0].Size = -1
	if _, err := preflightInventory(entries, gitstore.ObjectFormatSHA1, DefaultLimits()); !errors.Is(err, ErrInvalidTreeEntry) {
		t.Fatalf("negative size err = %v, want invalid-tree-entry", err)
	}
}

func TestValidateMaterializationPathBounds(t *testing.T) {
	limits := DefaultLimits()
	for _, valid := range []string{"a", "a/b", "unicodé/π"} {
		if err := validateMaterializationPath(valid, limits); err != nil {
			t.Fatalf("valid path %q: %v", valid, err)
		}
	}
	bad := []string{"", "/a", "a//b", "a/./b", "a/../b", "a\\b", "a\x1fb", "a/.git/config", strings.Repeat("a/", limits.MaxPathDepth) + "z", strings.Repeat("a", limits.MaxPathBytes+1)}
	for _, p := range bad {
		if err := validateMaterializationPath(p, limits); err == nil {
			t.Fatalf("bad path %q accepted", p)
		}
	}
}

func FuzzValidateMaterializationInventory(f *testing.F) {
	f.Add("a.txt", int64(1), true)
	f.Add("dir/file.txt", int64(5), true)
	f.Add("/abs", int64(1), true)
	f.Add("a/../b", int64(1), true)
	f.Fuzz(func(t *testing.T, p string, size int64, known bool) {
		oid := strings.Repeat("a", 40)
		entries := []gitstore.TreeEntry{{Path: p, Kind: gitstore.EntryRegularFile, Mode: "100644", Type: "blob", ObjectID: oid, SizeKnown: known, Size: size}}
		_, err := preflightInventory(entries, gitstore.ObjectFormatSHA1, DefaultLimits())
		assertMaterializeErrorBounded(t, err)
	})
}
