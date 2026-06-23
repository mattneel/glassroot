package gitstore

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type OpenOptions struct {
	GitPath string
	runner  commandRunner
}

type Repository struct {
	gitDir       string
	gitPath      string
	version      GitVersion
	objectFormat ObjectFormat
	cmd          commandAdapter
	cleanup      func() error
}

type RepositoryMetadata struct {
	GitDir       string
	GitPath      string
	GitVersion   GitVersion
	ObjectFormat ObjectFormat
}

func Open(ctx context.Context, gitDir string, options ...OpenOptions) (*Repository, error) {
	var opts OpenOptions
	if len(options) > 0 {
		opts = options[0]
	}
	if err := validateRepositoryPath(gitDir); err != nil {
		return nil, err
	}
	adapter, cleanup, err := newCommandAdapter(ctx, opts.GitPath, gitDir)
	if err != nil {
		return nil, err
	}
	if opts.runner != nil {
		adapter.runner = opts.runner
	}
	version, err := readGitVersion(ctx, adapter)
	if err != nil {
		_ = cleanup()
		return nil, err
	}
	format, err := preflightRepository(ctx, adapter, gitDir)
	if err != nil {
		_ = cleanup()
		return nil, err
	}
	return &Repository{gitDir: gitDir, gitPath: adapter.gitPath, version: version, objectFormat: format, cmd: adapter, cleanup: cleanup}, nil
}

func (r *Repository) Close() error {
	if r == nil || r.cleanup == nil {
		return nil
	}
	return r.cleanup()
}

func (r *Repository) Metadata() RepositoryMetadata {
	return RepositoryMetadata{GitDir: r.gitDir, GitPath: r.gitPath, GitVersion: r.version, ObjectFormat: r.objectFormat}
}

func (r *Repository) GitPath() string            { return r.gitPath }
func (r *Repository) GitVersion() GitVersion     { return r.version }
func (r *Repository) ObjectFormat() ObjectFormat { return r.objectFormat }

func validateRepositoryPath(gitDir string) error {
	if gitDir == "" {
		return pathErr(CodeInvalidRepositoryPath, "open", "path", gitDir, "repository path is required", nil)
	}
	if len(gitDir) > MaxRepositoryPathBytes {
		return pathErr(CodeInvalidRepositoryPath, "open", "path", gitDir, "repository path exceeds limit", nil)
	}
	if strings.ContainsRune(gitDir, 0) || containsControl(gitDir) {
		return pathErr(CodeInvalidRepositoryPath, "open", "path", gitDir, "repository path contains control characters", nil)
	}
	if !filepath.IsAbs(gitDir) {
		return pathErr(CodeInvalidRepositoryPath, "open", "path", gitDir, "repository path must be absolute", nil)
	}
	st, err := os.Lstat(gitDir)
	if err != nil {
		return pathErr(CodeInvalidRepositoryPath, "open", "path", gitDir, "stat repository path", err)
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return pathErr(CodeInvalidRepositoryPath, "open", "path", gitDir, "repository final component is a symlink", nil)
	}
	if !st.IsDir() {
		return pathErr(CodeInvalidRepositoryPath, "open", "path", gitDir, "repository path is not a directory", nil)
	}
	return nil
}

func readGitVersion(ctx context.Context, adapter commandAdapter) (GitVersion, error) {
	out, err := adapter.runGit(ctx, commandSpec{op: "version", args: []string{"version"}, stdoutLimit: MaxGitStdoutBytes, stderrLimit: MaxGitStderrBytes, timeout: DefaultGitCommandTimeout})
	if err != nil {
		return GitVersion{}, err
	}
	return ParseGitVersion(string(out.Stdout))
}

func preflightRepository(ctx context.Context, adapter commandAdapter, gitDir string) (ObjectFormat, error) {
	for _, required := range []string{"config", "HEAD", "objects", "refs"} {
		if _, err := os.Stat(filepath.Join(gitDir, required)); err != nil {
			return "", pathErr(CodeUnsupportedRepositoryLayout, "preflight", required, filepath.Join(gitDir, required), "missing essential repository metadata", err)
		}
	}
	checks := []struct {
		rel  string
		code ErrorCode
	}{
		{"commondir", CodeUnsupportedRepositoryLayout},
		{"info/grafts", CodeUnsupportedRepositoryLayout},
		{"objects/info/alternates", CodeAlternateObjectStore},
		{"objects/info/http-alternates", CodeAlternateObjectStore},
	}
	for _, check := range checks {
		p := filepath.Join(gitDir, filepath.FromSlash(check.rel))
		if exists(p) {
			return "", pathErr(check.code, "preflight", check.rel, p, "unsupported repository metadata present", nil)
		}
	}
	if exists(filepath.Join(gitDir, "worktrees")) {
		return "", pathErr(CodeUnsupportedRepositoryLayout, "preflight", "worktrees", filepath.Join(gitDir, "worktrees"), "linked worktree metadata is unsupported", nil)
	}
	promisor, err := filepath.Glob(filepath.Join(gitDir, "objects", "pack", "*.promisor"))
	if err != nil {
		return "", pathErr(CodeUnsupportedRepositoryLayout, "preflight", "promisor", gitDir, "scan promisor markers", err)
	}
	if len(promisor) > 0 {
		return "", pathErr(CodePartialCloneUnsupported, "preflight", "promisor", promisor[0], "promisor pack marker present", nil)
	}
	configPath := filepath.Join(gitDir, "config")
	st, err := os.Stat(configPath)
	if err != nil {
		return "", pathErr(CodeRepositoryConfigInvalid, "preflight", "config", configPath, "stat config", err)
	}
	if st.Size() > MaxGitConfigBytes {
		return "", pathErr(CodeRepositoryConfigInvalid, "preflight", "config", configPath, "config exceeds byte limit", nil)
	}
	out, err := adapter.runGit(ctx, commandSpec{op: "config", args: []string{"config", "--file", configPath, "--no-includes", "--null", "--list"}, stdoutLimit: MaxGitConfigBytes, stderrLimit: MaxGitStderrBytes, timeout: DefaultGitCommandTimeout})
	if err != nil {
		if errors.Is(err, ErrContextCancelled) || errors.Is(err, ErrGitCommandTimeout) {
			return "", err
		}
		return "", gitErr(CodeRepositoryConfigInvalid, "preflight", "config", "parse repository config", err)
	}
	entries, err := parseConfigList(out.Stdout)
	if err != nil {
		return "", err
	}
	return validateRepositoryConfig(entries)
}

func parseConfigList(data []byte) (map[string][]string, error) {
	entries := make(map[string][]string)
	if len(data) == 0 {
		return entries, nil
	}
	for _, rec := range strings.Split(string(data), "\x00") {
		if rec == "" {
			continue
		}
		parts := strings.SplitN(rec, "\n", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, gitErr(CodeRepositoryConfigInvalid, "preflight", "config", "malformed config output", nil)
		}
		key := strings.ToLower(parts[0])
		entries[key] = append(entries[key], parts[1])
	}
	return entries, nil
}

func validateRepositoryConfig(entries map[string][]string) (ObjectFormat, error) {
	for key := range entries {
		if key == "include.path" || (strings.HasPrefix(key, "includeif.") && strings.HasSuffix(key, ".path")) {
			return "", gitErr(CodeRepositoryConfigInclude, "preflight", "config", "config include is unsupported", nil)
		}
	}
	if last(entries, "core.bare") != "true" {
		return "", gitErr(CodeRepositoryNotBare, "preflight", "config", "repository must be explicitly bare", nil)
	}
	if has(entries, "core.worktree") {
		return "", gitErr(CodeRepositoryNotBare, "preflight", "config", "configured work tree is unsupported", nil)
	}
	if has(entries, "core.alternaterefscommand") {
		return "", gitErr(CodeRepositoryConfigInvalid, "preflight", "config", "alternateRefsCommand is unsupported", nil)
	}
	if last(entries, "extensions.worktreeconfig") != "" {
		return "", gitErr(CodeUnsupportedRepositoryLayout, "preflight", "config", "worktreeConfig extension is unsupported", nil)
	}
	if last(entries, "extensions.partialclone") != "" {
		return "", gitErr(CodePartialCloneUnsupported, "preflight", "config", "partial clone extension is unsupported", nil)
	}
	for key := range entries {
		if strings.HasPrefix(key, "remote.") && (strings.HasSuffix(key, ".promisor") || strings.HasSuffix(key, ".partialclonefilter")) {
			return "", gitErr(CodePartialCloneUnsupported, "preflight", "config", "promisor remote is unsupported", nil)
		}
		if strings.HasPrefix(key, "extensions.") && key != "extensions.objectformat" && key != "extensions.worktreeconfig" && key != "extensions.partialclone" {
			return "", gitErr(CodeUnsupportedRepositoryLayout, "preflight", "config", "unsupported repository extension", nil)
		}
	}
	format := ObjectFormatSHA1
	if v := last(entries, "extensions.objectformat"); v != "" {
		switch v {
		case "sha1":
			format = ObjectFormatSHA1
		case "sha256":
			format = ObjectFormatSHA256
		default:
			return "", gitErr(CodeUnsupportedObjectFormat, "preflight", "config", "unsupported object format", nil)
		}
	}
	return format, nil
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil || !errors.Is(err, fs.ErrNotExist)
}

func last(entries map[string][]string, key string) string {
	values := entries[key]
	if len(values) == 0 {
		return ""
	}
	return strings.ToLower(values[len(values)-1])
}

func has(entries map[string][]string, key string) bool { return len(entries[key]) > 0 }

func containsControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
