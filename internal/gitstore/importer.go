package gitstore

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const DefaultGitAuthHeaderEnvName = "GLASSROOT_GIT_AUTHORIZATION_HEADER"

type GitHubSourceImportLimits struct {
	MaxStdoutBytes int64
	MaxStderrBytes int64
	Timeout        time.Duration
}

type GitHubSourceImportConfig struct {
	GitPath           string
	RepositoryDir     string
	TemplateDir       string
	ObjectFormat      ObjectFormat
	RemoteURL         string
	BaseCommitID      string
	ExpectedHeadID    string
	PullRequestNumber int64
	AuthHeaderEnvName string
	AuthHeaderValue   []byte
	HomeDir           string
	XDGConfigDir      string
	Limits            GitHubSourceImportLimits
}

type ImportCommand struct {
	Executable  string
	Args        []string
	Env         []string
	Dir         string
	StdoutLimit int64
	StderrLimit int64
	Timeout     time.Duration
}

type GitHubSourceImportPlan struct {
	Commands []ImportCommand
}

type GitHubSourceImporter struct {
	runner commandRunner
}

func NewGitHubSourceImporter() GitHubSourceImporter { return GitHubSourceImporter{} }

func NewGitHubSourceImporterForTest(r commandRunner) GitHubSourceImporter {
	return GitHubSourceImporter{runner: r}
}

func BuildGitHubSourceImportPlan(cfg GitHubSourceImportConfig) (GitHubSourceImportPlan, error) {
	if cfg.Limits.MaxStdoutBytes <= 0 {
		cfg.Limits.MaxStdoutBytes = MaxGitStdoutBytes
	}
	if cfg.Limits.MaxStderrBytes <= 0 {
		cfg.Limits.MaxStderrBytes = MaxGitStderrBytes
	}
	if cfg.Limits.Timeout <= 0 {
		cfg.Limits.Timeout = DefaultGitCommandTimeout
	}
	if cfg.GitPath == "" || !filepath.IsAbs(cfg.GitPath) || cfg.RepositoryDir == "" || !filepath.IsAbs(cfg.RepositoryDir) || cfg.TemplateDir == "" || !filepath.IsAbs(cfg.TemplateDir) {
		return GitHubSourceImportPlan{}, gitErr(CodeImporterConfigInvalid, "import", "config", "path rejected", nil)
	}
	if cfg.ObjectFormat != ObjectFormatSHA1 && cfg.ObjectFormat != ObjectFormatSHA256 {
		return GitHubSourceImportPlan{}, gitErr(CodeUnsupportedObjectFormat, "import", "config", "object format rejected", nil)
	}
	if _, err := validateObjectID(cfg.BaseCommitID, cfg.ObjectFormat, false); err != nil {
		return GitHubSourceImportPlan{}, err
	}
	if _, err := validateObjectID(cfg.ExpectedHeadID, cfg.ObjectFormat, false); err != nil {
		return GitHubSourceImportPlan{}, err
	}
	if cfg.PullRequestNumber <= 0 {
		return GitHubSourceImportPlan{}, gitErr(CodeImporterConfigInvalid, "import", "config", "pull request number rejected", nil)
	}
	if cfg.AuthHeaderEnvName == "" {
		cfg.AuthHeaderEnvName = DefaultGitAuthHeaderEnvName
	}
	if !validGitEnvName(cfg.AuthHeaderEnvName) || len(cfg.AuthHeaderValue) == 0 || bytesContainsControlExceptSpace(cfg.AuthHeaderValue) {
		return GitHubSourceImportPlan{}, gitErr(CodeImporterConfigInvalid, "import", "config", "authorization header rejected", nil)
	}
	if err := validateGitHubRemoteURL(cfg.RemoteURL); err != nil {
		return GitHubSourceImportPlan{}, err
	}
	env := sourceImportEnv(cfg.AuthHeaderEnvName, string(cfg.AuthHeaderValue), cfg.HomeDir, cfg.XDGConfigDir)
	commonGitConfig := []string{
		"--no-pager",
		"--no-replace-objects",
		"--no-optional-locks",
		"-c", "credential.helper=",
		"-c", "credential.interactive=false",
		"-c", "credential.useHttpPath=true",
		"-c", "protocol.allow=never",
		"-c", "protocol.https.allow=always",
		"-c", "protocol.file.allow=never",
		"-c", "protocol.ext.allow=never",
		"-c", "protocol.version=2",
		"-c", "http.followRedirects=false",
		"-c", "http.sslVerify=true",
		"-c", "http.proxy=",
		"-c", "core.hooksPath=/dev/null",
		"-c", "gc.auto=0",
		"-c", "maintenance.auto=false",
		"-c", "fetch.writeCommitGraph=false",
		"-c", "fetch.fsckObjects=true",
		"-c", "transfer.fsckObjects=true",
		"-c", "submodule.recurse=false",
	}
	initArgs := append([]string{}, commonGitConfig...)
	initArgs = append(initArgs, "init", "--bare", "--object-format="+string(cfg.ObjectFormat), "--template", cfg.TemplateDir, cfg.RepositoryDir)
	fetchArgs := append([]string{}, commonGitConfig...)
	fetchArgs = append(fetchArgs,
		"-C", cfg.RepositoryDir,
		"--config-env=http."+cfg.RemoteURL+".extraHeader="+cfg.AuthHeaderEnvName,
		"fetch",
		"--depth=1",
		"--no-tags",
		"--no-write-fetch-head",
		"--no-recurse-submodules",
		"--no-auto-maintenance",
		"--no-write-commit-graph",
		cfg.RemoteURL,
		"+"+cfg.BaseCommitID+":refs/glassroot/base",
		"+refs/pull/"+strconv.FormatInt(cfg.PullRequestNumber, 10)+"/head:refs/glassroot/head",
	)
	cmds := []ImportCommand{
		{Executable: cfg.GitPath, Args: initArgs, Env: env, Dir: filepath.Dir(cfg.RepositoryDir), StdoutLimit: cfg.Limits.MaxStdoutBytes, StderrLimit: cfg.Limits.MaxStderrBytes, Timeout: cfg.Limits.Timeout},
		{Executable: cfg.GitPath, Args: fetchArgs, Env: env, Dir: filepath.Dir(cfg.RepositoryDir), StdoutLimit: cfg.Limits.MaxStdoutBytes, StderrLimit: cfg.Limits.MaxStderrBytes, Timeout: cfg.Limits.Timeout},
	}
	return GitHubSourceImportPlan{Commands: cmds}, nil
}

func (i GitHubSourceImporter) Import(ctx context.Context, cfg GitHubSourceImportConfig) error {
	plan, err := BuildGitHubSourceImportPlan(cfg)
	if err != nil {
		return err
	}
	runner := i.runner
	if runner == nil {
		runner = execCommandRunner{}
	}
	for idx, cmd := range plan.Commands {
		out, err := runner.Run(ctx, gitInvocation{Executable: cmd.Executable, Args: cmd.Args, Env: cmd.Env, Dir: cmd.Dir, StdoutLimit: cmd.StdoutLimit, StderrLimit: cmd.StderrLimit, Timeout: cmd.Timeout})
		if err != nil {
			code := CodeGitFetchFailed
			if idx == 0 {
				code = CodeGitInitFailed
			}
			if errorsIsGitCode(err, CodeGitOutputTooLarge) {
				code = CodeGitOutputTooLarge
			}
			if errorsIsGitCode(err, CodeGitCommandTimeout) {
				code = CodeGitCommandTimeout
			}
			return gitErr(code, "import", firstSubcommand(cmd.Args), "git import command failed", err)
		}
		_ = out
	}
	return nil
}

func sourceImportEnv(authName, authValue, homeDir, xdgDir string) []string {
	home := homeDir
	if home == "" {
		home = "/nonexistent"
	}
	xdg := xdgDir
	if xdg == "" {
		xdg = home + "/xdg"
	}
	return []string{
		"LC_ALL=C",
		"LANG=C",
		"TZ=UTC",
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + xdg,
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_ATTR_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_NO_LAZY_FETCH=1",
		"GIT_PROTOCOL_FROM_USER=0",
		"GIT_LITERAL_PATHSPECS=1",
		"GIT_PROTOCOL=version=2",
		"GIT_LFS_SKIP_SMUDGE=1",
		"GIT_PAGER=cat",
		"PAGER=cat",
		authName + "=" + authValue,
	}
}

func validateGitHubRemoteURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || !strings.HasSuffix(u.Path, ".git") {
		return gitErr(CodeImporterConfigInvalid, "import", "remote", "remote URL rejected", nil)
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == ".git" || strings.Contains(parts[0], "..") || strings.Contains(parts[1], "..") {
		return gitErr(CodeImporterConfigInvalid, "import", "remote", "remote URL rejected", nil)
	}
	return nil
}

func validGitEnvName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 && !(r == '_' || (r >= 'A' && r <= 'Z')) {
			return false
		}
		if !(r == '_' || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func bytesContainsControlExceptSpace(b []byte) bool {
	for _, c := range b {
		if c < 0x20 || c == 0x7f {
			return true
		}
	}
	return false
}

func errorsIsGitCode(err error, code ErrorCode) bool {
	var ge *Error
	return errors.As(err, &ge) && ge.Code == code
}
