package gitstore

import (
	"errors"
	"strings"
	"testing"
)

func TestParseGitVersion(t *testing.T) {
	v, err := ParseGitVersion("git version 2.43.0\n")
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "2.43.0" || !v.AtLeast(MinimumGitVersion) {
		t.Fatalf("version = %#v", v)
	}
	if _, err := ParseGitVersion("git version 2.42.9"); !errors.Is(err, ErrUnsupportedGitVersion) {
		t.Fatalf("old version err = %v", err)
	}
	if _, err := ParseGitVersion("not git"); !errors.Is(err, ErrMalformedGitOutput) {
		t.Fatalf("malformed version err = %v", err)
	}
}

func TestParseLsTreeRecordsAndValidatePaths(t *testing.T) {
	format := ObjectFormatSHA1
	oid := strings.Repeat("a", 40)
	input := "040000 tree " + oid + "\tcmd\x00" +
		"100644 blob " + oid + " 5\tcmd/file.txt\x00" +
		"100755 blob " + oid + " 7\trun.sh\x00" +
		"120000 blob " + oid + " 4\tlink\x00" +
		"160000 commit " + oid + "\tsubmodule\x00"
	entries, err := parseLsTree([]byte(input), format)
	if err != nil {
		t.Fatal(err)
	}
	wantKinds := map[string]EntryKind{
		"cmd":          EntryDirectory,
		"cmd/file.txt": EntryRegularFile,
		"run.sh":       EntryExecutableFile,
		"link":         EntrySymlink,
		"submodule":    EntryGitlink,
	}
	for _, entry := range entries {
		if want := wantKinds[entry.Path]; entry.Kind != want {
			t.Fatalf("entry %s kind = %s, want %s", entry.Path, entry.Kind, want)
		}
	}
	bad := []string{
		"100644 blob " + oid + " 1\t/a\x00",
		"100644 blob " + oid + " 1\ta//b\x00",
		"100644 blob " + oid + " 1\ta/../b\x00",
		"100644 blob " + oid + " 1\ta/.git/config\x00",
		"100644 tree " + oid + " 1\ta\x00",
		"100644 blob " + strings.Repeat("b", 39) + " 1\ta\x00",
		"999999 blob " + oid + " 1\ta\x00",
	}
	for _, raw := range bad {
		if _, err := parseLsTree([]byte(raw), format); err == nil {
			t.Fatalf("parseLsTree(%q) succeeded", raw)
		}
	}
}

func TestTreePathValidationRejectsHostilePaths(t *testing.T) {
	valid := []string{"a", "dir/file.txt", "unicodé/π"}
	for _, path := range valid {
		if err := ValidateGitTreePath(path); err != nil {
			t.Fatalf("valid path %q: %v", path, err)
		}
	}
	bad := []string{"", "/abs", "a//b", "a/./b", "a/../b", "a\\b", "a\x1fb", strings.Repeat("a", MaxTreePathBytes+1), strings.Repeat("a/", MaxTreeDepth) + "z", "a/.Git/config"}
	for _, path := range bad {
		if err := ValidateGitTreePath(path); err == nil {
			t.Fatalf("bad path %q accepted", path)
		}
	}
}

func TestParseCatFileHeader(t *testing.T) {
	oid := strings.Repeat("a", 40)
	header, err := parseCatFileHeader([]byte(oid+" blob 3\nabc"), ObjectFormatSHA1)
	if err != nil {
		t.Fatal(err)
	}
	if header.ObjectID != oid || header.Type != "blob" || header.Size != 3 || header.HeaderBytes != len(oid)+8 {
		t.Fatalf("header = %#v", header)
	}
	bad := [][]byte{
		[]byte(""),
		[]byte(strings.Repeat("a", 39) + " blob 1\n"),
		[]byte(oid + " tree 1\n"),
		[]byte(oid + " blob -1\n"),
		[]byte(oid + " blob 999999999999999999999999\n"),
	}
	for _, raw := range bad {
		if _, err := parseCatFileHeader(raw, ObjectFormatSHA1); err == nil {
			t.Fatalf("bad header accepted: %q", raw)
		}
	}
}

func FuzzParseLsTree(f *testing.F) {
	oid := strings.Repeat("a", 40)
	for _, seed := range [][]byte{[]byte(""), []byte("100644 blob " + oid + " 1\ta\x00"), []byte("120000 blob " + oid + " 4\tlink\x00"), []byte("160000 commit " + oid + "\tsub\x00"), []byte("bad"), []byte("100644 blob bad 1\ta\x00")} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, err := parseLsTree(data, ObjectFormatSHA1)
		assertSanitizedBounded(t, err)
	})
}

func FuzzParseCatFileHeader(f *testing.F) {
	oid := strings.Repeat("a", 40)
	for _, seed := range [][]byte{[]byte(""), []byte(oid + " blob 0\n"), []byte(oid + " tree 1\n"), []byte(oid + " blob 999999999999999999999999\n")} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, err := parseCatFileHeader(data, ObjectFormatSHA1)
		assertSanitizedBounded(t, err)
	})
}

func FuzzValidateGitTreePath(f *testing.F) {
	for _, seed := range []string{"", "a", "a/b", "/a", "a/../b", "a\\b", "a/.git/config", strings.Repeat("a/", 200)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		err := ValidateGitTreePath(s)
		assertSanitizedBounded(t, err)
	})
}

func assertSanitizedBounded(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	msg := err.Error()
	if len(msg) > 2048 {
		t.Fatalf("error too long: %d", len(msg))
	}
	for _, r := range msg {
		if r < 0x20 && r != '\n' && r != '\t' {
			t.Fatalf("raw control char in error %q", msg)
		}
	}
}
