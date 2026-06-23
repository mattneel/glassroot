package gitstore

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/model"
)

func TestConfigLoadTrustedUsesExactResolvedCommitObjects(t *testing.T) {
	ctx := context.Background()
	fixture := newGitFixture(t, "sha1")
	baseConfig := pipelineYAML(2)
	headConfig := pipelineYAML(64)
	baseOID := fixture.commitFiles(t, map[string]fileSpec{".glassroot/pipeline.yaml": {data: []byte(baseConfig)}})
	fixture.forceRef(t, "refs/heads/base", baseOID)
	headOID := fixture.commitFiles(t, map[string]fileSpec{".glassroot/pipeline.yaml": {data: []byte(headConfig)}})

	repo := openFixtureBare(t, ctx, fixture)
	defer repo.Close()
	base, err := repo.ResolveCommit(ctx, RefSelector("refs/heads/base"))
	if err != nil {
		t.Fatalf("resolve base: %v", err)
	}
	head, err := repo.ResolveCommit(ctx, ObjectIDSelector(headOID))
	if err != nil {
		t.Fatalf("resolve head: %v", err)
	}
	if base.CommitID != baseOID || head.CommitID != headOID {
		t.Fatalf("resolved commits = %s %s, want %s %s", base.CommitID, head.CommitID, baseOID, headOID)
	}

	fixture.forceRef(t, "refs/heads/base", headOID)
	source := NewRevisionFileSource(repo)
	result, err := config.LoadTrusted(ctx, source, config.TrustedLoadRequest{
		Base: commitRef(model.RevisionKindBase, base),
		Head: commitRef(model.RevisionKindHead, head),
	})
	if err != nil {
		t.Fatalf("LoadTrusted through gitstore: %v", err)
	}
	if result.EffectivePipeline.Resources.CPU != 2 {
		t.Fatalf("head affected effective CPU = %d", result.EffectivePipeline.Resources.CPU)
	}
	if result.HeadAssessment.State != config.HeadStateModifiedValid {
		t.Fatalf("head assessment state = %s", result.HeadAssessment.State)
	}
	assertConfigChange(t, result.HeadAssessment.Changes, "spec.resources.cpu", config.SecurityEffectPrivilegeIncrease)
}

func TestReadPathPreservesRawBytesAndModes(t *testing.T) {
	ctx := context.Background()
	fixture := newGitFixture(t, "sha1")
	rawCRLF := []byte("line1\r\nline2\r\n")
	lfsPointer := []byte("version https://git-lfs.github.com/spec/v1\noid sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\nsize 12\n")
	commit := fixture.commitFiles(t, map[string]fileSpec{
		"regular.txt":     {data: []byte("regular")},
		"exec.sh":         {data: []byte("#!/bin/sh\necho should-not-run\n"), executable: true},
		"empty.txt":       {data: nil},
		"nested/utf-π":    {data: []byte("unicode")},
		"crlf.txt":        {data: rawCRLF},
		"pointer.bin":     {data: lfsPointer},
		"link":            {symlinkTarget: "regular.txt"},
		"submodule-entry": {gitlink: strings.Repeat("1", 40)},
	})
	repo := openFixtureBare(t, ctx, fixture)
	defer repo.Close()
	rev, err := repo.ResolveCommit(ctx, ObjectIDSelector(commit))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	regular, err := repo.ReadPath(ctx, rev, "regular.txt", 1024)
	if err != nil || regular.Kind != EntryRegularFile || string(regular.Data) != "regular" || regular.Executable {
		t.Fatalf("regular file = %#v err=%v", regular, err)
	}
	regular.Data[0] = 'X'
	again, err := repo.ReadPath(ctx, rev, "regular.txt", 1024)
	if err != nil || string(again.Data) != "regular" {
		t.Fatalf("returned bytes aliased command buffer/repo state: %#v err=%v", again, err)
	}
	execFile, err := repo.ReadPath(ctx, rev, "exec.sh", 1024)
	if err != nil || execFile.Kind != EntryExecutableFile || !execFile.Executable {
		t.Fatalf("executable = %#v err=%v", execFile, err)
	}
	empty, err := repo.ReadPath(ctx, rev, "empty.txt", 1024)
	if err != nil || len(empty.Data) != 0 {
		t.Fatalf("empty = %#v err=%v", empty, err)
	}
	crlf, err := repo.ReadPath(ctx, rev, "crlf.txt", 1024)
	if err != nil || !bytes.Equal(crlf.Data, rawCRLF) {
		t.Fatalf("CRLF not preserved: %q err=%v", crlf.Data, err)
	}
	pointer, err := repo.ReadPath(ctx, rev, "pointer.bin", 1024)
	if err != nil || !bytes.Equal(pointer.Data, lfsPointer) {
		t.Fatalf("LFS pointer bytes not returned literally: %q err=%v", pointer.Data, err)
	}
	symlink, err := repo.ReadPath(ctx, rev, "link", 1024)
	if err != nil || symlink.Kind != EntrySymlink || string(symlink.Data) != "regular.txt" {
		t.Fatalf("symlink blob = %#v err=%v", symlink, err)
	}
	gitlink, err := repo.ReadPath(ctx, rev, "submodule-entry", 1024)
	if err != nil || gitlink.Kind != EntryGitlink || len(gitlink.Data) != 0 {
		t.Fatalf("gitlink = %#v err=%v", gitlink, err)
	}
	_, err = repo.ReadPath(ctx, rev, "missing.txt", 1024)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing err = %v, want fs.ErrNotExist", err)
	}
	_, err = repo.ReadPath(ctx, rev, "regular.txt", 3)
	if !errors.Is(err, ErrBlobTooLarge) {
		t.Fatalf("caller byte limit err = %v, want blob-too-large", err)
	}
}

func TestResolveCommitSelectorRulesAndRefMutationNoninterference(t *testing.T) {
	ctx := context.Background()
	fixture := newGitFixture(t, "sha1")
	first := fixture.commitFiles(t, map[string]fileSpec{"a.txt": {data: []byte("a")}})
	second := fixture.commitFiles(t, map[string]fileSpec{"a.txt": {data: []byte("b")}})
	annotatedTag := fixture.annotatedTag(t, "refs/tags/v1", first)
	fixture.lightweightTag(t, "refs/tags/light", first)
	treeOID := fixture.revParse(t, first+"^{tree}")
	blobOID := fixture.revParse(t, first+":a.txt")

	repo := openFixtureBare(t, ctx, fixture)
	defer repo.Close()
	cases := []RevisionSelector{RefSelector("refs/heads/main"), RefSelector("refs/tags/v1"), RefSelector("refs/tags/light"), ObjectIDSelector(first)}
	for _, sel := range cases {
		rev, err := repo.ResolveCommit(ctx, sel)
		if err != nil {
			t.Fatalf("ResolveCommit(%#v): %v; annotated tag %s", sel, err, annotatedTag)
		}
		if rev.CommitID != first && sel.Value != "refs/heads/main" {
			t.Fatalf("ResolveCommit(%#v) = %s, want %s", sel, rev.CommitID, first)
		}
	}
	bad := []RevisionSelector{
		ObjectIDSelector(first[:12]),
		RawSelector("main"),
		RawSelector("refs/heads/main~1"),
		RawSelector("refs/heads/main^{tree}"),
		RawSelector("refs/heads/main:a.txt"),
		RawSelector("--help"),
		ObjectIDSelector(treeOID),
		ObjectIDSelector(blobOID),
	}
	for _, sel := range bad {
		if _, err := repo.ResolveCommit(ctx, sel); err == nil {
			t.Fatalf("ResolveCommit(%#v) succeeded unexpectedly", sel)
		}
	}
	rev, err := repo.ResolveCommit(ctx, ObjectIDSelector(first))
	if err != nil {
		t.Fatal(err)
	}
	fixture.forceRef(t, "refs/heads/main", second)
	file, err := repo.ReadPath(ctx, rev, "a.txt", 1024)
	if err != nil || string(file.Data) != "a" {
		t.Fatalf("resolved revision changed after ref mutation: %q err=%v", file.Data, err)
	}
}

func TestSHA256RepositoryWhenSupported(t *testing.T) {
	ctx := context.Background()
	fixture := newGitFixture(t, "sha256")
	if fixture == nil {
		t.Skip("git lacks sha256 repository support")
	}
	commit := fixture.commitFiles(t, map[string]fileSpec{"file.txt": {data: []byte("sha256")}})
	if len(commit) != 64 {
		t.Fatalf("sha256 commit len = %d", len(commit))
	}
	repo := openFixtureBare(t, ctx, fixture)
	defer repo.Close()
	if repo.ObjectFormat() != ObjectFormatSHA256 {
		t.Fatalf("object format = %s", repo.ObjectFormat())
	}
	rev, err := repo.ResolveCommit(ctx, ObjectIDSelector(commit))
	if err != nil {
		t.Fatalf("resolve sha256: %v", err)
	}
	file, err := repo.ReadPath(ctx, rev, "file.txt", 1024)
	if err != nil || string(file.Data) != "sha256" {
		t.Fatalf("sha256 read = %q err=%v", file.Data, err)
	}
}

func TestWalkTreeDeterministicAndClassifiesEntries(t *testing.T) {
	ctx := context.Background()
	fixture := newGitFixture(t, "sha1")
	commit := fixture.commitFiles(t, map[string]fileSpec{
		"b.txt":     {data: []byte("b")},
		"a/one.txt": {data: []byte("1")},
		"run.sh":    {data: []byte("echo no"), executable: true},
		"link":      {symlinkTarget: "b.txt"},
	})
	repo := openFixtureBare(t, ctx, fixture)
	defer repo.Close()
	rev, err := repo.ResolveCommit(ctx, ObjectIDSelector(commit))
	if err != nil {
		t.Fatal(err)
	}
	first, err := repo.ListTree(ctx, rev)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.ListTree(ctx, rev)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("tree listing not deterministic")
	}
	wantKinds := map[string]EntryKind{"a": EntryDirectory, "a/one.txt": EntryRegularFile, "b.txt": EntryRegularFile, "link": EntrySymlink, "run.sh": EntryExecutableFile}
	for _, entry := range first {
		if want, ok := wantKinds[entry.Path]; ok && entry.Kind != want {
			t.Fatalf("entry %s kind = %s, want %s", entry.Path, entry.Kind, want)
		}
	}
}

func TestPreflightRejectsUnsafeRepositoryMetadata(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		mutate func(t *testing.T, gitDir string)
		want   error
	}{
		{"final path symlink", func(t *testing.T, gitDir string) {
			real := gitDir + "-real"
			if err := os.Rename(gitDir, real); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(real, gitDir); err != nil {
				t.Fatal(err)
			}
		}, ErrInvalidRepositoryPath},
		{"commondir", func(t *testing.T, gitDir string) {
			writeFile(t, filepath.Join(gitDir, "commondir"), []byte("../common"))
		}, ErrUnsupportedRepositoryLayout},
		{"worktrees", func(t *testing.T, gitDir string) { mkdir(t, filepath.Join(gitDir, "worktrees", "x")) }, ErrUnsupportedRepositoryLayout},
		{"grafts", func(t *testing.T, gitDir string) {
			writeFile(t, filepath.Join(gitDir, "info", "grafts"), []byte("bad"))
		}, ErrUnsupportedRepositoryLayout},
		{"alternates", func(t *testing.T, gitDir string) {
			writeFile(t, filepath.Join(gitDir, "objects", "info", "alternates"), []byte("/tmp/objects"))
		}, ErrAlternateObjectStore},
		{"http alternates", func(t *testing.T, gitDir string) {
			writeFile(t, filepath.Join(gitDir, "objects", "info", "http-alternates"), []byte("https://example.invalid"))
		}, ErrAlternateObjectStore},
		{"promisor pack", func(t *testing.T, gitDir string) {
			writeFile(t, filepath.Join(gitDir, "objects", "pack", "pack-test.promisor"), []byte(""))
		}, ErrPartialCloneUnsupported},
		{"include", func(t *testing.T, gitDir string) {
			appendFile(t, filepath.Join(gitDir, "config"), []byte("\n[include]\n\tpath = /tmp/nope\n"))
		}, ErrRepositoryConfigInclude},
		{"includeIf", func(t *testing.T, gitDir string) {
			appendFile(t, filepath.Join(gitDir, "config"), []byte("\n[includeIf \"gitdir:/tmp/*\"]\n\tpath = /tmp/nope\n"))
		}, ErrRepositoryConfigInclude},
		{"partial clone extension", func(t *testing.T, gitDir string) {
			appendFile(t, filepath.Join(gitDir, "config"), []byte("\n[extensions]\n\tpartialClone = origin\n"))
		}, ErrPartialCloneUnsupported},
		{"promisor remote", func(t *testing.T, gitDir string) {
			appendFile(t, filepath.Join(gitDir, "config"), []byte("\n[remote \"origin\"]\n\tpromisor = true\n"))
		}, ErrPartialCloneUnsupported},
		{"alternate refs command", func(t *testing.T, gitDir string) {
			appendFile(t, filepath.Join(gitDir, "config"), []byte("\n[core]\n\talternateRefsCommand = echo bad\n"))
		}, ErrRepositoryConfigInvalid},
		{"unsupported extension", func(t *testing.T, gitDir string) {
			appendFile(t, filepath.Join(gitDir, "config"), []byte("\n[extensions]\n\tunknown = true\n"))
		}, ErrUnsupportedRepositoryLayout},
		{"unsupported object format", func(t *testing.T, gitDir string) {
			appendFile(t, filepath.Join(gitDir, "config"), []byte("\n[extensions]\n\tobjectFormat = md5\n"))
		}, ErrUnsupportedObjectFormat},
		{"malformed config", func(t *testing.T, gitDir string) {
			appendFile(t, filepath.Join(gitDir, "config"), []byte("\n[broken\n\tvalue = nope\n"))
		}, ErrRepositoryConfigInvalid},
		{"oversized config", func(t *testing.T, gitDir string) {
			appendFile(t, filepath.Join(gitDir, "config"), bytes.Repeat([]byte("#"), MaxGitConfigBytes+1))
		}, ErrRepositoryConfigInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newGitFixture(t, "sha1")
			fixture.commitFiles(t, map[string]fileSpec{"a.txt": {data: []byte("a")}})
			tc.mutate(t, fixture.bareDir)
			repo, err := Open(ctx, fixture.bareDir)
			if repo != nil {
				_ = repo.Close()
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("Open() err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestNonBareRepositoryRejected(t *testing.T) {
	ctx := context.Background()
	fixture := newGitFixture(t, "sha1")
	fixture.commitFiles(t, map[string]fileSpec{"a.txt": {data: []byte("a")}})
	repo, err := Open(ctx, filepath.Join(fixture.workDir, ".git"))
	if repo != nil {
		_ = repo.Close()
	}
	if !errors.Is(err, ErrRepositoryNotBare) {
		t.Fatalf("Open(non-bare .git) err = %v, want repository-not-bare", err)
	}
}

func commitRef(kind model.RevisionKind, rev ResolvedRevision) model.CommitRef {
	return model.CommitRef{Kind: kind, Repository: "file:///control-plane.git", Ref: rev.OriginalSelector, CommitID: rev.CommitID, TreeDigest: model.Digest(rev.TreeID)}
}

func assertConfigChange(t *testing.T, changes []config.ConfigChange, path string, effect config.SecurityEffect) {
	t.Helper()
	for _, change := range changes {
		if change.Path == path && change.Effect == effect {
			return
		}
	}
	t.Fatalf("missing change %s/%s in %#v", path, effect, changes)
}

func pipelineYAML(cpu int) string {
	return strings.Replace(`apiVersion: glassroot.dev/v1alpha1
kind: Pipeline
metadata:
  name: default
spec:
  environment:
    image: docker.io/library/golang:1.26@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
    workdir: /workspace
  resources:
    cpu: 2
    memory: 2GiB
    disk: 4GiB
    processes: 256
    timeout: 15m
  network:
    mode: deny
    allow: []
  scenarios:
    - id: test
      name: Unit tests
      shell: /bin/sh
      run: go test ./...
      timeout: 10m
  collect:
    filesystem:
      roots:
        - /workspace
        - /tmp
      contents: metadata-and-digests
    artifacts:
      - path: /workspace/bin/**
        maxBytes: 50MiB
    logs:
      maxBytesPerStream: 10MiB
  compare:
    ignore:
      - field: event.timestamp
      - field: process.pid
    repetitions: 1
  policy:
    profile: strict
`, "cpu: 2", "cpu: "+itoa(cpu), 1)
}

func itoa(v int) string { return strconv.Itoa(v) }

type gitFixture struct {
	root    string
	workDir string
	bareDir string
	gitPath string
}

type fileSpec struct {
	data          []byte
	executable    bool
	symlinkTarget string
	gitlink       string
}

func newGitFixture(t *testing.T, format string) *gitFixture {
	t.Helper()
	root := t.TempDir()
	work := filepath.Join(root, "work")
	bare := filepath.Join(root, "repo.git")
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"init"}
	if format == "sha256" {
		args = append(args, "--object-format=sha256")
	}
	args = append(args, work)
	cmd := exec.Command(gitPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if format == "sha256" {
			return nil
		}
		t.Fatalf("git init: %v\n%s", err, out)
	}
	runTrustedGit(t, gitPath, work, "config", "user.email", "test@example.invalid")
	runTrustedGit(t, gitPath, work, "config", "user.name", "Test User")
	runTrustedGit(t, gitPath, work, "checkout", "-b", "main")
	runTrustedGit(t, gitPath, "", "init", "--bare", bare)
	if format == "sha256" {
		// Recreate the bare store with the matching object format when supported.
		_ = os.RemoveAll(bare)
		runTrustedGit(t, gitPath, "", "init", "--bare", "--object-format=sha256", bare)
	}
	return &gitFixture{root: root, workDir: work, bareDir: bare, gitPath: gitPath}
}

func (f *gitFixture) commitFiles(t *testing.T, files map[string]fileSpec) string {
	t.Helper()
	entries, err := os.ReadDir(f.workDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(f.workDir, entry.Name())); err != nil {
			t.Fatal(err)
		}
	}
	runTrustedGit(t, f.gitPath, f.workDir, "add", "-A")
	for path, spec := range files {
		if spec.gitlink != "" {
			runTrustedGit(t, f.gitPath, f.workDir, "update-index", "--add", "--cacheinfo", "160000,"+spec.gitlink+","+path)
			continue
		}
		full := filepath.Join(f.workDir, filepath.FromSlash(path))
		mkdir(t, filepath.Dir(full))
		if spec.symlinkTarget != "" {
			if runtime.GOOS == "windows" {
				t.Skip("symlink test requires Unix-like platform")
			}
			if err := os.Symlink(spec.symlinkTarget, full); err != nil {
				t.Fatal(err)
			}
		} else {
			writeFile(t, full, spec.data)
			if spec.executable {
				if err := os.Chmod(full, 0o755); err != nil {
					t.Fatal(err)
				}
			}
		}
		runTrustedGit(t, f.gitPath, f.workDir, "add", "--", path)
	}
	runTrustedGit(t, f.gitPath, f.workDir, "commit", "-m", "fixture")
	commit := f.revParse(t, "HEAD")
	runTrustedGit(t, f.gitPath, f.workDir, "push", "--force", f.bareDir, "HEAD:refs/heads/main")
	return commit
}

func (f *gitFixture) annotatedTag(t *testing.T, ref, commit string) string {
	t.Helper()
	name := strings.TrimPrefix(ref, "refs/tags/")
	runTrustedGit(t, f.gitPath, f.workDir, "tag", "-a", name, commit, "-m", "tag")
	runTrustedGit(t, f.gitPath, f.workDir, "push", "--force", f.bareDir, ref+":"+ref)
	return f.revParse(t, ref)
}

func (f *gitFixture) lightweightTag(t *testing.T, ref, commit string) {
	t.Helper()
	name := strings.TrimPrefix(ref, "refs/tags/")
	runTrustedGit(t, f.gitPath, f.workDir, "tag", "-f", name, commit)
	runTrustedGit(t, f.gitPath, f.workDir, "push", "--force", f.bareDir, ref+":"+ref)
}

func (f *gitFixture) forceRef(t *testing.T, ref, oid string) {
	t.Helper()
	runTrustedGit(t, f.gitPath, "", "--git-dir", f.bareDir, "update-ref", ref, oid)
}

func (f *gitFixture) revParse(t *testing.T, rev string) string {
	t.Helper()
	cmd := exec.Command(f.gitPath, "-C", f.workDir, "rev-parse", rev)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse %s: %v\n%s", rev, err, out)
	}
	return strings.TrimSpace(string(out))
}

func openFixtureBare(t *testing.T, ctx context.Context, fixture *gitFixture) *Repository {
	t.Helper()
	repo, err := Open(ctx, fixture.bareDir)
	if err != nil {
		t.Fatalf("Open(%s): %v", fixture.bareDir, err)
	}
	return repo
}

func runTrustedGit(t *testing.T, gitPath, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(gitPath, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
func appendFile(t *testing.T, path string, data []byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
}
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
