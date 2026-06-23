package materialize

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateSymlinkTarget(t *testing.T) {
	limits := DefaultLimits()
	valid := map[string]string{
		"link":         "file.txt",
		"dir/link":     "child.txt",
		"dir/sub/link": "../file.txt",
		"dir/link2":    "missing.txt",
	}
	for link, target := range valid {
		meta, err := validateSymlinkTarget(link, []byte(target), limits)
		if err != nil {
			t.Fatalf("valid symlink %s -> %s: %v", link, target, err)
		}
		if meta.ByteLength != int64(len(target)) || meta.TargetDigest == "" {
			t.Fatalf("metadata = %#v", meta)
		}
	}
	invalid := map[string]string{
		"absolute":       "/etc/passwd",
		"escape":         "../../outside",
		"nul":            "bad\x00target",
		"control":        "bad\x1ftarget",
		"backslash":      "bad\\target",
		"empty":          "",
		"dotgit":         ".Git/config",
		"clean-changes":  "a/../b",
		"parent-dot-git": "../.git/config",
	}
	for name, target := range invalid {
		if _, err := validateSymlinkTarget("dir/link", []byte(target), limits); !errors.Is(err, ErrInvalidSymlinkTarget) {
			t.Fatalf("%s target err = %v, want invalid-symlink-target", name, err)
		}
	}
}

func TestParseLFSPointer(t *testing.T) {
	data := []byte("version https://git-lfs.github.com/spec/v1\noid sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\nsize 123\n")
	meta, ok := parseLFSPointer(data)
	if !ok {
		t.Fatalf("canonical pointer not detected")
	}
	if meta.OID != "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" || meta.Size != 123 {
		t.Fatalf("meta = %#v", meta)
	}
	bad := [][]byte{
		[]byte("version https://git-lfs.github.com/spec/v1\noid sha256:ABCDEF\nsize 1\n"),
		[]byte("version https://git-lfs.github.com/spec/v1\noid sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\nsize -1\n"),
		[]byte("not a pointer"),
	}
	for _, input := range bad {
		if _, ok := parseLFSPointer(input); ok {
			t.Fatalf("bad pointer detected: %q", input)
		}
	}
}

func TestDigestEncodingRepeatableAndDistinguishesBoundaries(t *testing.T) {
	entries := []EntryResult{
		{Path: "dir", SourceKind: EntryDirectory, Disposition: DispositionMaterializedDirectory, NormalizedMode: 0o755},
		{Path: "dir/file", SourceKind: EntryRegularFile, Disposition: DispositionMaterializedFile, NormalizedMode: 0o644, SizeBytes: 1, ContentDigest: "sha256:ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb", SourceObjectID: strings.Repeat("a", 40)},
		{Path: "link", SourceKind: EntrySymlink, Disposition: DispositionMaterializedSymlink, NormalizedMode: 0o777, SizeBytes: 4, TargetBytes: 4, TargetDigest: "sha256:3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7", SourceObjectID: strings.Repeat("b", 40)},
		{Path: "sub", SourceKind: EntryGitlink, Disposition: DispositionSkippedGitlink, SourceObjectID: strings.Repeat("c", 40)},
	}
	tree1, manifest1, err := computeMaterializationDigests(entries)
	if err != nil {
		t.Fatal(err)
	}
	tree2, manifest2, err := computeMaterializationDigests([]EntryResult{entries[3], entries[1], entries[0], entries[2]})
	if err != nil {
		t.Fatal(err)
	}
	const wantTree = "sha256:eef78515d18dfda26fd4bf5644ad60819564bfa81b84d81976c505ced9d4e816"
	const wantManifest = "sha256:2cf295155a6e70e0e907aee1ae8810c62160c3b8e1870948a13aa8f51cb719ce"
	if tree1 != wantTree || manifest1 != wantManifest {
		t.Fatalf("golden digests = %s/%s, want %s/%s", tree1, manifest1, wantTree, wantManifest)
	}
	if tree1 != tree2 || manifest1 != manifest2 {
		t.Fatalf("digests not order independent: %s/%s vs %s/%s", tree1, manifest1, tree2, manifest2)
	}
	withoutGitlink := entries[:3]
	tree3, manifest3, err := computeMaterializationDigests(withoutGitlink)
	if err != nil {
		t.Fatal(err)
	}
	if tree1 != tree3 {
		t.Fatalf("gitlink affected materialized-tree digest")
	}
	if manifest1 == manifest3 {
		t.Fatalf("gitlink did not affect manifest digest")
	}
	ambiguousA := []EntryResult{{Path: "ab", SourceKind: EntryRegularFile, Disposition: DispositionMaterializedFile, NormalizedMode: 0o644, SizeBytes: 1, ContentDigest: entries[1].ContentDigest, SourceObjectID: strings.Repeat("a", 40)}}
	ambiguousB := []EntryResult{{Path: "a", SourceKind: EntryRegularFile, Disposition: DispositionMaterializedFile, NormalizedMode: 0o644, SizeBytes: 1, ContentDigest: entries[1].ContentDigest, SourceObjectID: strings.Repeat("a", 40)}, {Path: "b", SourceKind: EntryDirectory, Disposition: DispositionMaterializedDirectory, NormalizedMode: 0o755}}
	tA, mA, _ := computeMaterializationDigests(ambiguousA)
	tB, mB, _ := computeMaterializationDigests(ambiguousB)
	if tA == tB || mA == mB {
		t.Fatalf("record boundary ambiguity: %s/%s", tA, mA)
	}
}

func TestLFSPointerDispositionAffectsManifestOnly(t *testing.T) {
	base := EntryResult{Path: "pointer.bin", SourceKind: EntryRegularFile, Disposition: DispositionMaterializedFile, NormalizedMode: 0o644, SizeBytes: 128, ContentDigest: "sha256:ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb", SourceObjectID: strings.Repeat("a", 40)}
	lfs := base
	lfs.Disposition = DispositionMaterializedLFSPointer
	lfs.LFSPointer = &LFSPointerMetadata{OID: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Size: 123}
	treeBase, manifestBase, err := computeMaterializationDigests([]EntryResult{base})
	if err != nil {
		t.Fatal(err)
	}
	treeLFS, manifestLFS, err := computeMaterializationDigests([]EntryResult{lfs})
	if err != nil {
		t.Fatal(err)
	}
	if treeBase != treeLFS {
		t.Fatalf("LFS disposition changed actual-tree digest: %s vs %s", treeBase, treeLFS)
	}
	if manifestBase == manifestLFS {
		t.Fatalf("LFS disposition did not change manifest digest")
	}
}

func FuzzValidateSymlinkTarget(f *testing.F) {
	for _, seed := range []string{"file.txt", "../outside", "/abs", "a\\b", "", ".git/config"} {
		f.Add("dir/link", []byte(seed))
	}
	f.Fuzz(func(t *testing.T, link string, target []byte) {
		_, err := validateSymlinkTarget(link, target, DefaultLimits())
		assertMaterializeErrorBounded(t, err)
	})
}

func FuzzParseLFSPointer(f *testing.F) {
	for _, seed := range [][]byte{[]byte(""), []byte("version https://git-lfs.github.com/spec/v1\noid sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\nsize 1\n"), []byte("not a pointer")} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseLFSPointer(data)
	})
}

func FuzzMaterializationDigestEncoding(f *testing.F) {
	f.Add("a", "sha256:ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb")
	f.Add("a/b", "sha256:3e23e8160039594a33894f6564e1b1348bbd7a0088d42c4acb73eeaed59c009d")
	f.Fuzz(func(t *testing.T, p, digest string) {
		entries := []EntryResult{{Path: p, SourceKind: EntryRegularFile, Disposition: DispositionMaterializedFile, NormalizedMode: 0o644, SizeBytes: 1, ContentDigest: digest, SourceObjectID: strings.Repeat("a", 40)}}
		_, _, err := computeMaterializationDigests(entries)
		assertMaterializeErrorBounded(t, err)
	})
}

func assertMaterializeErrorBounded(t *testing.T, err error) {
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
			t.Fatalf("raw control character in error %q", msg)
		}
	}
}
