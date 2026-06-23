package materialize

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/gitstore"
)

func TestMaterializeMixedTreeAndCleanup(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GR-6B materialization is Linux-only")
	}
	ctx := context.Background()
	fixture := newMaterializeGitFixture(t, "sha1")
	lfsPointer := []byte("version https://git-lfs.github.com/spec/v1\noid sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\nsize 12\n")
	commitA := fixture.commitFiles(t, map[string]materializeFileSpec{
		"dir/file.txt":       {data: []byte("regular\n")},
		"dir/exec.sh":        {data: []byte("#!/bin/sh\necho should-not-run\n"), executable: true},
		"empty.txt":          {data: nil},
		"binary.bin":         {data: []byte{'a', 0, 'b', '\n'}},
		"crlf.txt":           {data: []byte("one\r\ntwo\r\n")},
		"unicodé/π.txt":      {data: []byte("unicode")},
		"link-to-file":       {symlinkTarget: "dir/file.txt"},
		"dangling-contained": {symlinkTarget: "dir/missing.txt"},
		"pointer.bin":        {data: lfsPointer},
		"submodule-entry":    {gitlink: strings.Repeat("1", 40)},
	})
	fixture.forceRef(t, "refs/heads/main", commitA)

	repo := openMaterializeBare(t, ctx, fixture)
	defer repo.Close()
	revA, err := repo.ResolveCommit(ctx, gitstore.RefSelector("refs/heads/main"))
	if err != nil {
		t.Fatal(err)
	}
	commitB := fixture.commitFiles(t, map[string]materializeFileSpec{"dir/file.txt": {data: []byte("changed\n")}})
	fixture.forceRef(t, "refs/heads/main", commitB)
	parent := filepath.Join(t.TempDir(), "workspaces")
	mustMkdir(t, parent)
	m, err := New(parent)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := m.Materialize(ctx, repo, revA)
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if res.Workspace == nil || res.Workspace.Path() == "" {
		t.Fatalf("missing workspace in result")
	}
	defer res.Workspace.Close()
	if got := readTestFile(t, filepath.Join(res.Workspace.Path(), "dir", "file.txt")); got != "regular\n" {
		t.Fatalf("ref mutation affected materialization: %q", got)
	}
	assertFileMode(t, filepath.Join(res.Workspace.Path(), "dir", "file.txt"), 0o644)
	assertFileMode(t, filepath.Join(res.Workspace.Path(), "dir", "exec.sh"), 0o755)
	assertFileMode(t, res.Workspace.Path(), 0o700)
	if target, err := os.Readlink(filepath.Join(res.Workspace.Path(), "link-to-file")); err != nil || target != "dir/file.txt" {
		t.Fatalf("symlink target = %q err=%v", target, err)
	}
	if _, err := os.Lstat(filepath.Join(res.Workspace.Path(), "submodule-entry")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("gitlink materialized unexpectedly: %v", err)
	}
	if !bytes.Equal(readTestBytes(t, filepath.Join(res.Workspace.Path(), "pointer.bin")), lfsPointer) {
		t.Fatalf("LFS pointer bytes changed")
	}
	if res.Summary.Directories == 0 || res.Summary.RegularFiles == 0 || res.Summary.ExecutableFiles != 1 || res.Summary.Symlinks != 2 || res.Summary.Gitlinks != 1 || res.Summary.LFSPointers != 1 {
		t.Fatalf("summary = %#v", res.Summary)
	}
	if res.MaterializedTreeDigest == "" || res.MaterializationManifestDigest == "" || res.MaterializedTreeDigest == res.MaterializationManifestDigest {
		t.Fatalf("unexpected digests: tree=%s manifest=%s", res.MaterializedTreeDigest, res.MaterializationManifestDigest)
	}
	assertNoRawSymlinkTargets(t, res)
	workspacePath := res.Workspace.Path()
	if err := res.Workspace.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := res.Workspace.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := os.Stat(workspacePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace remains after Close: %v", err)
	}
}

func TestMaterializeSHA256RepositoryWhenSupported(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GR-6B materialization is Linux-only")
	}
	ctx := context.Background()
	fixture := newMaterializeGitFixture(t, "sha256")
	if fixture == nil {
		t.Skip("git lacks sha256 repository support")
	}
	commit := fixture.commitFiles(t, map[string]materializeFileSpec{"file.txt": {data: []byte("sha256")}})
	repo := openMaterializeBare(t, ctx, fixture)
	defer repo.Close()
	rev, err := repo.ResolveCommit(ctx, gitstore.ObjectIDSelector(commit))
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	res, err := m.Materialize(ctx, repo, rev)
	if err != nil {
		t.Fatalf("Materialize sha256: %v", err)
	}
	defer res.Workspace.Close()
	if got := readTestFile(t, filepath.Join(res.Workspace.Path(), "file.txt")); got != "sha256" {
		t.Fatalf("sha256 materialized %q", got)
	}
}

func TestParentValidationAndUnsupportedPlatformCode(t *testing.T) {
	if runtime.GOOS != "linux" {
		_, err := New(t.TempDir())
		if !errors.Is(err, ErrUnsupportedPlatform) {
			t.Fatalf("New outside Linux err = %v, want unsupported-platform", err)
		}
		return
	}
	if _, err := New("relative"); !errors.Is(err, ErrInvalidParent) {
		t.Fatalf("relative parent err = %v, want invalid-parent", err)
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(file); !errors.Is(err, ErrParentNotDirectory) {
		t.Fatalf("file parent err = %v, want parent-not-directory", err)
	}
	real := filepath.Join(t.TempDir(), "real")
	mustMkdir(t, real)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := New(link); !errors.Is(err, ErrParentSymlink) {
		t.Fatalf("symlink parent err = %v, want parent-symlink", err)
	}
}

func TestMaterializeRejectsParentInsideRepository(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GR-6B materialization is Linux-only")
	}
	ctx := context.Background()
	fixture := newMaterializeGitFixture(t, "sha1")
	commit := fixture.commitFiles(t, map[string]materializeFileSpec{"file.txt": {data: []byte("x")}})
	repo := openMaterializeBare(t, ctx, fixture)
	defer repo.Close()
	rev, err := repo.ResolveCommit(ctx, gitstore.ObjectIDSelector(commit))
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(fixture.bareDir, "objects")
	m, err := New(parent)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Materialize(ctx, repo, rev)
	if !errors.Is(err, ErrParentOverlapsRepository) {
		t.Fatalf("Materialize parent overlap err = %v, want parent-overlaps-repository", err)
	}
}

func TestFailureCleansPartialWorkspace(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GR-6B materialization is Linux-only")
	}
	ctx := context.Background()
	fixture := newMaterializeGitFixture(t, "sha1")
	commit := fixture.commitFiles(t, map[string]materializeFileSpec{"dir/a.txt": {data: []byte("a")}, "dir/b.txt": {data: []byte("b")}})
	repo := openMaterializeBare(t, ctx, fixture)
	defer repo.Close()
	rev, err := repo.ResolveCommit(ctx, gitstore.ObjectIDSelector(commit))
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	m, err := New(parent)
	if err != nil {
		t.Fatal(err)
	}
	m.hooks.BeforeOpenFile = func(workspacePath, repoPath string) error {
		if repoPath == "dir/b.txt" {
			return errors.New("injected failure")
		}
		return nil
	}
	_, err = m.Materialize(ctx, repo, rev)
	if err == nil {
		t.Fatalf("expected injected failure")
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("partial workspace remains: %#v", entries)
	}
}

func TestHostRaceDirectorySymlinkCanary(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GR-6B materialization is Linux-only")
	}
	ctx := context.Background()
	fixture := newMaterializeGitFixture(t, "sha1")
	commit := fixture.commitFiles(t, map[string]materializeFileSpec{"safe/file.txt": {data: []byte("payload")}})
	repo := openMaterializeBare(t, ctx, fixture)
	defer repo.Close()
	rev, err := repo.ResolveCommit(ctx, gitstore.ObjectIDSelector(commit))
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	mustMkdir(t, outside)
	canary := filepath.Join(outside, "file.txt")
	if err := os.WriteFile(canary, []byte("canary"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := New(parent)
	if err != nil {
		t.Fatal(err)
	}
	m.hooks.BeforeOpenFile = func(workspacePath, repoPath string) error {
		if repoPath != "safe/file.txt" {
			return nil
		}
		victim := filepath.Join(workspacePath, "safe")
		if err := os.Remove(victim); err != nil {
			return err
		}
		return os.Symlink(outside, victim)
	}
	_, err = m.Materialize(ctx, repo, rev)
	if err == nil {
		t.Fatalf("expected race failure")
	}
	if got := readTestFile(t, canary); got != "canary" {
		t.Fatalf("outside canary modified: %q", got)
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Fatalf("partial workspace remains after race: %#v", entries)
	}
}

func TestDescriptorChmodCannotBeRedirected(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GR-6B materialization is Linux-only")
	}
	ctx := context.Background()
	fixture := newMaterializeGitFixture(t, "sha1")
	commit := fixture.commitFiles(t, map[string]materializeFileSpec{"file.sh": {data: []byte("#!/bin/sh\n"), executable: true}})
	repo := openMaterializeBare(t, ctx, fixture)
	defer repo.Close()
	rev, err := repo.ResolveCommit(ctx, gitstore.ObjectIDSelector(commit))
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.sh")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := New(parent)
	if err != nil {
		t.Fatal(err)
	}
	m.hooks.BeforeFileChmod = func(workspacePath, repoPath string) error {
		victim := filepath.Join(workspacePath, filepath.FromSlash(repoPath))
		if err := os.Remove(victim); err != nil {
			return err
		}
		return os.Symlink(outside, victim)
	}
	_, err = m.Materialize(ctx, repo, rev)
	if err == nil {
		t.Fatalf("expected post-open replacement failure")
	}
	assertFileMode(t, outside, 0o644)
}

func assertNoRawSymlinkTargets(t *testing.T, res *Result) {
	t.Helper()
	for _, entry := range res.Entries {
		if entry.SourceKind == EntrySymlink {
			if strings.Contains(entry.Path, "dir/file.txt") || strings.Contains(entry.ContentDigest, "dir/file.txt") {
				t.Fatalf("raw symlink target leaked in entry: %#v", entry)
			}
			if entry.TargetBytes == 0 || entry.TargetDigest == "" {
				t.Fatalf("missing symlink metadata: %#v", entry)
			}
		}
	}
}

// Trusted test helpers use fixed Git commands to create temporary repositories. They never execute repository contents.
type materializeGitFixture struct {
	workDir string
	bareDir string
	gitPath string
}

type materializeFileSpec struct {
	data          []byte
	executable    bool
	symlinkTarget string
	gitlink       string
}

func newMaterializeGitFixture(t *testing.T, format string) *materializeGitFixture {
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
	runTrustedMaterializeGit(t, gitPath, work, "config", "user.email", "test@example.invalid")
	runTrustedMaterializeGit(t, gitPath, work, "config", "user.name", "Test User")
	runTrustedMaterializeGit(t, gitPath, work, "checkout", "-b", "main")
	initBare := []string{"init", "--bare"}
	if format == "sha256" {
		initBare = append(initBare, "--object-format=sha256")
	}
	initBare = append(initBare, bare)
	runTrustedMaterializeGit(t, gitPath, "", initBare...)
	return &materializeGitFixture{workDir: work, bareDir: bare, gitPath: gitPath}
}

func (f *materializeGitFixture) commitFiles(t *testing.T, files map[string]materializeFileSpec) string {
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
	runTrustedMaterializeGit(t, f.gitPath, f.workDir, "add", "-A")
	for p, spec := range files {
		if spec.gitlink != "" {
			runTrustedMaterializeGit(t, f.gitPath, f.workDir, "update-index", "--add", "--cacheinfo", "160000,"+spec.gitlink+","+p)
			continue
		}
		full := filepath.Join(f.workDir, filepath.FromSlash(p))
		mustMkdir(t, filepath.Dir(full))
		if spec.symlinkTarget != "" {
			if err := os.Symlink(spec.symlinkTarget, full); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.WriteFile(full, spec.data, 0o644); err != nil {
				t.Fatal(err)
			}
			if spec.executable {
				if err := os.Chmod(full, 0o755); err != nil {
					t.Fatal(err)
				}
			}
		}
		runTrustedMaterializeGit(t, f.gitPath, f.workDir, "add", "--", p)
	}
	runTrustedMaterializeGit(t, f.gitPath, f.workDir, "commit", "-m", "fixture")
	commit := revParseMaterialize(t, f.gitPath, f.workDir, "HEAD")
	runTrustedMaterializeGit(t, f.gitPath, f.workDir, "push", "--force", f.bareDir, "HEAD:refs/heads/main")
	return commit
}

func (f *materializeGitFixture) forceRef(t *testing.T, ref, oid string) {
	t.Helper()
	runTrustedMaterializeGit(t, f.gitPath, "", "--git-dir", f.bareDir, "update-ref", ref, oid)
}

func openMaterializeBare(t *testing.T, ctx context.Context, fixture *materializeGitFixture) *gitstore.Repository {
	t.Helper()
	repo, err := gitstore.Open(ctx, fixture.bareDir)
	if err != nil {
		t.Fatalf("Open(%s): %v", fixture.bareDir, err)
	}
	return repo
}

func revParseMaterialize(t *testing.T, gitPath, dir, rev string) string {
	t.Helper()
	cmd := exec.Command(gitPath, "-C", dir, "rev-parse", rev)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse %s: %v\n%s", rev, err, out)
	}
	return strings.TrimSpace(string(out))
}

func runTrustedMaterializeGit(t *testing.T, gitPath, dir string, args ...string) {
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

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, p string) string {
	t.Helper()
	return string(readTestBytes(t, p))
}

func readTestBytes(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func assertFileMode(t *testing.T, p string, mode os.FileMode) {
	t.Helper()
	st, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != mode {
		t.Fatalf("mode(%s) = %o, want %o", p, st.Mode().Perm(), mode)
	}
}
