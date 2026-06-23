package githubsource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/mattneel/glassroot/internal/githubcontrollerstore"
	"github.com/mattneel/glassroot/internal/githubsourcestore"
	"github.com/mattneel/glassroot/internal/gitstore"
)

type GitImporter struct {
	SourceRoot string
	GitPath    string
	Limits     Limits
}

func NewGitImporter(sourceRoot, gitPath string, limits Limits) GitImporter {
	return GitImporter{SourceRoot: sourceRoot, GitPath: gitPath, Limits: limits}
}

func (g GitImporter) Import(ctx context.Context, req ImportRequest) (ImportResult, error) {
	limits := g.Limits
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := ValidateSourceImportRequest(req.SourceImportRequest, limits); err != nil {
		return ImportResult{}, err
	}
	if g.SourceRoot == "" || g.GitPath == "" || !filepath.IsAbs(g.GitPath) {
		return ImportResult{}, errCode(CodeInvalidConfig, "import", "git importer configuration rejected", nil)
	}
	if err := githubsourcestore.ValidateSourceRoot(g.SourceRoot, githubsourcestore.DefaultLimits()); err != nil {
		return ImportResult{}, wrap(CodeInvalidConfig, "source-root", "source root rejected", err)
	}
	id := identityFromRequest(req.SourceImportRequest)
	storeID, err := githubsourcestore.ComputeSourceStoreID(id)
	if err != nil {
		return ImportResult{}, wrap(CodeInvalidSourceRequest, "identity", "source identity rejected", err)
	}
	finalPath, err := githubsourcestore.LayoutPath(g.SourceRoot, storeID)
	if err != nil {
		return ImportResult{}, wrap(CodeInvalidConfig, "layout", "source layout rejected", err)
	}
	if exists(finalPath) {
		res, err := g.openExisting(ctx, req.SourceImportRequest, finalPath, storeID)
		if err != nil {
			return ImportResult{}, err
		}
		res.Reused = true
		return res, nil
	}
	return g.importFresh(ctx, req, id, finalPath, storeID)
}

func (g GitImporter) ReuseIfAvailable(ctx context.Context, req githubcontrollerstore.SourceImportRequest) (ImportResult, bool, error) {
	limits := g.Limits
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := ValidateSourceImportRequest(req, limits); err != nil {
		return ImportResult{}, false, err
	}
	id := identityFromRequest(req)
	storeID, err := githubsourcestore.ComputeSourceStoreID(id)
	if err != nil {
		return ImportResult{}, false, wrap(CodeInvalidSourceRequest, "identity", "source identity rejected", err)
	}
	finalPath, err := githubsourcestore.LayoutPath(g.SourceRoot, storeID)
	if err != nil {
		return ImportResult{}, false, wrap(CodeInvalidConfig, "layout", "source layout rejected", err)
	}
	if !exists(finalPath) {
		return ImportResult{}, false, nil
	}
	res, err := g.openExisting(ctx, req, finalPath, storeID)
	if err != nil {
		return ImportResult{}, false, err
	}
	res.Reused = true
	return res, true, nil
}

func identityFromRequest(req githubcontrollerstore.SourceImportRequest) githubsourcestore.Identity {
	return githubsourcestore.Identity{ImportProfileVersion: ImportProfileSmartHTTPShallowV1Alpha1, TargetID: req.TargetID, BaseRepositoryID: req.Base.RepositoryID, HeadRepositoryID: req.Head.RepositoryID, PullRequestNumber: req.PullRequestNumber, BaseCommitID: req.Base.CommitID, HeadCommitID: req.Head.CommitID}
}

func (g GitImporter) importFresh(ctx context.Context, req ImportRequest, id githubsourcestore.Identity, finalPath, storeID string) (ImportResult, error) {
	limits := g.Limits
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	stagingRoot := filepath.Join(g.SourceRoot, ".staging")
	if err := os.MkdirAll(stagingRoot, 0o700); err != nil {
		return ImportResult{}, wrap(CodeGitImportFailed, "staging", "staging root failed", err)
	}
	staging, err := os.MkdirTemp(stagingRoot, "source-import-*")
	if err != nil {
		return ImportResult{}, wrap(CodeGitImportFailed, "staging", "staging create failed", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(staging)
		}
	}()
	if err := os.Chmod(staging, 0o700); err != nil {
		return ImportResult{}, wrap(CodeGitImportFailed, "staging", "staging mode failed", err)
	}
	homeDir := filepath.Join(staging, "home")
	if err := os.Mkdir(homeDir, 0o700); err != nil {
		return ImportResult{}, wrap(CodeGitImportFailed, "staging", "home create failed", err)
	}
	xdgDir := filepath.Join(homeDir, "xdg")
	if err := os.Mkdir(xdgDir, 0o700); err != nil {
		return ImportResult{}, wrap(CodeGitImportFailed, "staging", "xdg create failed", err)
	}
	templateDir := filepath.Join(staging, "template-empty")
	if err := os.Mkdir(templateDir, 0o700); err != nil {
		return ImportResult{}, wrap(CodeGitImportFailed, "staging", "template create failed", err)
	}
	repoDir := filepath.Join(staging, "repository.git")
	remote, err := BuildRemoteURL(req.SourceImportRequest.Base.Owner, req.SourceImportRequest.Base.Name)
	if err != nil {
		return ImportResult{}, err
	}
	format := gitstore.ObjectFormatSHA1
	if len(req.SourceImportRequest.Base.CommitID) == 64 {
		format = gitstore.ObjectFormatSHA256
	}
	var authHeader []byte
	err = req.Token.Use(func(token []byte) error {
		authHeader = BasicAuthHeader(token)
		defer zero(authHeader)
		return gitstore.NewGitHubSourceImporter().Import(ctx, gitstore.GitHubSourceImportConfig{GitPath: g.GitPath, RepositoryDir: repoDir, TemplateDir: templateDir, ObjectFormat: format, RemoteURL: remote, BaseCommitID: req.SourceImportRequest.Base.CommitID, ExpectedHeadID: req.SourceImportRequest.Head.CommitID, PullRequestNumber: req.SourceImportRequest.PullRequestNumber, AuthHeaderEnvName: gitstore.DefaultGitAuthHeaderEnvName, AuthHeaderValue: authHeader, HomeDir: homeDir, XDGConfigDir: xdgDir, Limits: gitstore.GitHubSourceImportLimits{MaxStdoutBytes: 1 << 20, MaxStderrBytes: 64 << 10, Timeout: limits.ImportTimeout}})
	})
	if err != nil {
		return ImportResult{}, wrap(CodeGitImportFailed, "git", "git import failed", err)
	}
	meta, err := g.verifyRepository(ctx, req.SourceImportRequest, repoDir, id, string(format))
	if err != nil {
		return ImportResult{}, err
	}
	encoded, digest, err := githubsourcestore.EncodeMetadata(meta)
	if err != nil {
		return ImportResult{}, wrap(CodeSourceStoreVerifyFailed, "metadata", "metadata encode failed", err)
	}
	if err := os.WriteFile(filepath.Join(staging, "source.json"), encoded, 0o400); err != nil {
		return ImportResult{}, wrap(CodeGitImportFailed, "metadata", "metadata write failed", err)
	}
	if err := makeReadOnly(staging); err != nil {
		return ImportResult{}, wrap(CodeGitImportFailed, "permissions", "store permission failed", err)
	}
	parent := filepath.Dir(finalPath)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return ImportResult{}, wrap(CodeGitImportFailed, "publish", "source parent create failed", err)
	}
	if err := os.Rename(staging, finalPath); err != nil {
		if exists(finalPath) {
			return g.openExisting(ctx, req.SourceImportRequest, finalPath, storeID)
		}
		return ImportResult{}, wrap(CodeGitImportFailed, "publish", "source publish failed", err)
	}
	cleanup = false
	return resultFromMetadata(meta, digest, false), nil
}

func (g GitImporter) verifyRepository(ctx context.Context, req githubcontrollerstore.SourceImportRequest, repoDir string, id githubsourcestore.Identity, format string) (githubsourcestore.Metadata, error) {
	repo, err := gitstore.Open(ctx, repoDir, gitstore.OpenOptions{GitPath: g.GitPath})
	if err != nil {
		return githubsourcestore.Metadata{}, wrap(CodeSourceStoreVerifyFailed, "gitstore", "gitstore preflight failed", err)
	}
	defer repo.Close()
	shallow, err := os.ReadFile(filepath.Join(repoDir, "shallow"))
	if err != nil {
		return githubsourcestore.Metadata{}, wrap(CodeSourceStoreVerifyFailed, "shallow", "shallow metadata missing", err)
	}
	if err := githubsourcestore.ValidateShallowMetadata(shallow, format, githubsourcestore.DefaultLimits().MaxShallowEntries); err != nil {
		return githubsourcestore.Metadata{}, wrap(CodeSourceStoreVerifyFailed, "shallow", "shallow metadata rejected", err)
	}
	base, err := repo.ResolveCommit(ctx, gitstore.RefSelector("refs/glassroot/base"))
	if err != nil {
		return githubsourcestore.Metadata{}, wrap(CodeBaseObjectUnavailable, "verify", "base commit unavailable", err)
	}
	head, err := repo.ResolveCommit(ctx, gitstore.RefSelector("refs/glassroot/head"))
	if err != nil {
		return githubsourcestore.Metadata{}, wrap(CodePullRefUnavailable, "verify", "head commit unavailable", err)
	}
	if base.CommitID != req.Base.CommitID {
		return githubsourcestore.Metadata{}, errCode(CodeBaseObjectUnavailable, "verify", "base commit mismatch", nil)
	}
	if head.CommitID != req.Head.CommitID {
		return githubsourcestore.Metadata{}, errCode(CodePullRefMismatch, "verify", "pull ref mismatch", nil)
	}
	if string(repo.ObjectFormat()) != format {
		return githubsourcestore.Metadata{}, errCode(CodeInvalidObjectFormat, "verify", "object format mismatch", nil)
	}
	meta, err := githubsourcestore.NewMetadata(id, string(repo.ObjectFormat()), base.TreeID, head.TreeID)
	if err != nil {
		return githubsourcestore.Metadata{}, wrap(CodeSourceStoreVerifyFailed, "metadata", "metadata rejected", err)
	}
	return meta, nil
}

func (g GitImporter) openExisting(ctx context.Context, req githubcontrollerstore.SourceImportRequest, path, storeID string) (ImportResult, error) {
	data, err := os.ReadFile(filepath.Join(path, "source.json"))
	if err != nil {
		return ImportResult{}, wrap(CodeSourceStoreVerifyFailed, "metadata", "existing metadata read failed", err)
	}
	if len(data) > 1<<20 {
		return ImportResult{}, errCode(CodeSourceStoreLimit, "metadata", "existing metadata too large", nil)
	}
	var meta githubsourcestore.Metadata
	if err := jsonUnmarshal(data, &meta); err != nil {
		return ImportResult{}, wrap(CodeSourceStoreVerifyFailed, "metadata", "existing metadata decode failed", err)
	}
	_, digest, err := githubsourcestore.EncodeMetadata(meta)
	if err != nil {
		return ImportResult{}, wrap(CodeSourceStoreVerifyFailed, "metadata", "existing metadata invalid", err)
	}
	if meta.SourceStoreID != storeID || meta.TargetID != req.TargetID || meta.BaseCommitID != req.Base.CommitID || meta.HeadCommitID != req.Head.CommitID || meta.BaseRepositoryID != req.Base.RepositoryID || meta.HeadRepositoryID != req.Head.RepositoryID || meta.PullRequestNumber != req.PullRequestNumber {
		return ImportResult{}, errCode(CodeSourceStoreVerifyFailed, "metadata", "existing metadata identity mismatch", nil)
	}
	if _, err := g.verifyRepository(ctx, req, filepath.Join(path, "repository.git"), identityFromRequest(req), meta.ObjectFormat); err != nil {
		return ImportResult{}, err
	}
	return resultFromMetadata(meta, digest, true), nil
}

func resultFromMetadata(meta githubsourcestore.Metadata, digest string, reused bool) ImportResult {
	return ImportResult{SourceStoreID: meta.SourceStoreID, MetadataDigest: digest, ImportProfileVersion: meta.ImportProfileVersion, ObjectFormat: meta.ObjectFormat, BaseCommitID: meta.BaseCommitID, HeadCommitID: meta.HeadCommitID, BaseTreeID: meta.BaseTreeID, HeadTreeID: meta.HeadTreeID, Limitations: append([]string(nil), meta.Limitations...), Reused: reused}
}

func BuildRemoteURL(owner, repo string) (string, error) {
	if !validRoute(owner, 256) || !validRoute(repo, 256) {
		return "", errCode(CodeInvalidRoute, "route", "route rejected", nil)
	}
	u := url.URL{Scheme: "https", Host: "github.com", Path: "/" + owner + "/" + repo + ".git"}
	return u.String(), nil
}

func exists(path string) bool { _, err := os.Lstat(path); return err == nil }

func makeReadOnly(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink rejected")
		}
		if d.IsDir() {
			return os.Chmod(path, 0o500)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("special file rejected")
		}
		return os.Chmod(path, 0o400)
	})
}

func jsonUnmarshal(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
