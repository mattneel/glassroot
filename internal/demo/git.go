package demo

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mattneel/glassroot/internal/gitstore"
	"github.com/mattneel/glassroot/internal/model"
)

type fixtureStore struct {
	GitDir string
	Base   model.CommitRef
	Head   model.CommitRef
}

type looseWriter struct {
	root  string
	total int64
	count int
}

func createFixtureStore(ctx context.Context, gitDir string, fixture Fixture, limits Limits) (fixtureStore, error) {
	if err := ctx.Err(); err != nil {
		return fixtureStore{}, wrap(CodeContextCancelled, "fixture-git", "context cancelled", err)
	}
	if err := os.Mkdir(gitDir, 0o700); err != nil {
		return fixtureStore{}, wrap(CodeFixtureStoreFailed, "fixture-git", "create fixture git directory", err)
	}
	for _, rel := range []string{"objects", "objects/info", "objects/pack", "refs", "refs/heads", "refs/tags"} {
		if err := os.MkdirAll(filepath.Join(gitDir, rel), 0o700); err != nil {
			return fixtureStore{}, wrap(CodeFixtureStoreFailed, "fixture-git", "create fixture git layout", err)
		}
	}
	if err := writeFileExclusive(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n")); err != nil {
		return fixtureStore{}, err
	}
	cfg := []byte("[core]\n\trepositoryformatversion = 0\n\tfilemode = true\n\tbare = true\n")
	if err := writeFileExclusive(filepath.Join(gitDir, "config"), cfg); err != nil {
		return fixtureStore{}, err
	}
	w := &looseWriter{root: gitDir}
	baseTree, err := w.treeFromFiles(sourceFiles(fixture, false), limits)
	if err != nil {
		return fixtureStore{}, err
	}
	headTree, err := w.treeFromFiles(sourceFiles(fixture, true), limits)
	if err != nil {
		return fixtureStore{}, err
	}
	baseCommit, err := w.commit(baseTree, "glassroot demo fake "+string(fixture)+" base\n")
	if err != nil {
		return fixtureStore{}, err
	}
	headCommit, err := w.commit(headTree, "glassroot demo fake "+string(fixture)+" head\n")
	if err != nil {
		return fixtureStore{}, err
	}
	if err := writeFileExclusive(filepath.Join(gitDir, "refs", "heads", "main"), []byte(headCommit+"\n")); err != nil {
		return fixtureStore{}, err
	}
	base := demoCommitRef(model.RevisionKindBase, baseCommit, baseTree)
	head := demoCommitRef(model.RevisionKindHead, headCommit, headTree)
	repo, err := gitstore.Open(ctx, gitDir)
	if err != nil {
		return fixtureStore{}, wrap(CodeFixtureGitOpenFailed, "fixture-git", "open generated fixture store", err)
	}
	defer repo.Close()
	if _, err := repo.ResolveCommit(ctx, gitstore.ObjectIDSelector(baseCommit)); err != nil {
		return fixtureStore{}, wrap(CodeFixtureRevisionFailed, "fixture-git", "resolve generated base commit", err)
	}
	if _, err := repo.ResolveCommit(ctx, gitstore.ObjectIDSelector(headCommit)); err != nil {
		return fixtureStore{}, wrap(CodeFixtureRevisionFailed, "fixture-git", "resolve generated head commit", err)
	}
	return fixtureStore{GitDir: gitDir, Base: base, Head: head}, nil
}

func demoCommitRef(kind model.RevisionKind, commit, tree string) model.CommitRef {
	ref := "refs/heads/main"
	if kind == model.RevisionKindHead {
		ref = "refs/heads/main"
	}
	return model.CommitRef{Kind: kind, Repository: "glassroot.dev/fake-demo", Ref: ref, CommitID: commit, ObjectFormat: model.GitObjectFormatSHA1, TreeID: tree, TreeDigest: model.Digest(tree)}
}

func (w *looseWriter) treeFromFiles(files map[string][]byte, limits Limits) (string, error) {
	if len(files) > limits.MaxFixtureGitFiles {
		return "", errCode(CodeFixtureObjectInvalid, "fixture-git", "too many fixture files", nil)
	}
	root := node{dirs: map[string]*node{}, files: map[string][]byte{}}
	for p, data := range files {
		if int64(len(data)) > limits.MaxFixtureGitBlobBytes {
			return "", errCode(CodeFixtureObjectInvalid, "fixture-git", "fixture blob too large", nil)
		}
		w.total += int64(len(data))
		w.count++
		if w.total > limits.MaxFixtureGitTotalBytes {
			return "", errCode(CodeFixtureObjectInvalid, "fixture-git", "fixture total bytes too large", nil)
		}
		parts := strings.Split(p, "/")
		if len(parts) > limits.MaxFixtureGitDepth {
			return "", errCode(CodeFixtureObjectInvalid, "fixture-git", "fixture path too deep", nil)
		}
		cur := &root
		for _, part := range parts[:len(parts)-1] {
			if cur.dirs[part] == nil {
				cur.dirs[part] = &node{dirs: map[string]*node{}, files: map[string][]byte{}}
			}
			cur = cur.dirs[part]
		}
		cur.files[parts[len(parts)-1]] = append([]byte(nil), data...)
	}
	return w.writeTree(&root)
}

type node struct {
	dirs  map[string]*node
	files map[string][]byte
}

func (w *looseWriter) writeTree(n *node) (string, error) {
	entries := []treeEntry{}
	for name, data := range n.files {
		oid, err := w.object("blob", data)
		if err != nil {
			return "", err
		}
		entries = append(entries, treeEntry{name: name, mode: "100644", oid: oid})
	}
	for name, child := range n.dirs {
		oid, err := w.writeTree(child)
		if err != nil {
			return "", err
		}
		entries = append(entries, treeEntry{name: name, mode: "40000", oid: oid})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	var body bytes.Buffer
	for _, e := range entries {
		body.WriteString(e.mode)
		body.WriteByte(' ')
		body.WriteString(e.name)
		body.WriteByte(0)
		raw, err := hex.DecodeString(e.oid)
		if err != nil {
			return "", err
		}
		body.Write(raw)
	}
	return w.object("tree", body.Bytes())
}

type treeEntry struct{ name, mode, oid string }

func (w *looseWriter) commit(tree, msg string) (string, error) {
	body := []byte("tree " + tree + "\nauthor Glassroot Demo <demo@glassroot.dev> 1782172800 +0000\ncommitter Glassroot Demo <demo@glassroot.dev> 1782172800 +0000\n\n" + msg)
	return w.object("commit", body)
}

func (w *looseWriter) object(typ string, body []byte) (string, error) {
	header := []byte(fmt.Sprintf("%s %d\x00", typ, len(body)))
	store := append(append([]byte(nil), header...), body...)
	sum := sha1.Sum(store)
	oid := hex.EncodeToString(sum[:])
	dir := filepath.Join(w.root, "objects", oid[:2])
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", wrap(CodeFixtureStoreFailed, "fixture-git", "create object directory", err)
	}
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	if _, err := zw.Write(store); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	objPath := filepath.Join(dir, oid[2:])
	if _, err := os.Lstat(objPath); err == nil {
		return oid, nil
	}
	if err := writeFileExclusive(objPath, z.Bytes()); err != nil {
		return "", err
	}
	return oid, nil
}

func writeFileExclusive(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return wrap(CodeFixtureStoreFailed, "fixture-git", "create fixture file", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return wrap(CodeFixtureStoreFailed, "fixture-git", "write fixture file", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return wrap(CodeSyncFailed, "fixture-git", "sync fixture file", err)
	}
	if err := f.Close(); err != nil {
		return wrap(CodeFixtureStoreFailed, "fixture-git", "close fixture file", err)
	}
	return nil
}
