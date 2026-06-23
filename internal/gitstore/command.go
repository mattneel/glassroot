package gitstore

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type commandSpec struct {
	op          string
	args        []string
	stdin       []byte
	stdoutLimit int64
	stderrLimit int64
	timeout     time.Duration
	allowNoRepo bool
}

type gitInvocation struct {
	Executable  string
	Args        []string
	Env         []string
	Dir         string
	Stdin       []byte
	StdoutLimit int64
	StderrLimit int64
	Timeout     time.Duration
}

type gitCommandOutput struct {
	Stdout []byte
	Stderr []byte
}

type commandRunner interface {
	Run(context.Context, gitInvocation) (gitCommandOutput, error)
}

type commandAdapter struct {
	gitPath string
	gitDir  string
	workDir string
	homeDir string
	runner  commandRunner
}

func newCommandAdapter(ctx context.Context, gitPath, gitDir string) (commandAdapter, func() error, error) {
	resolved, err := resolveGitPath(gitPath)
	if err != nil {
		return commandAdapter{}, nil, err
	}
	work, err := os.MkdirTemp("", "glassroot-git-work-*")
	if err != nil {
		return commandAdapter{}, nil, gitErr(CodeGitCommandFailed, "command", "mkdir", "create private work dir", err)
	}
	home, err := os.MkdirTemp("", "glassroot-git-home-*")
	if err != nil {
		_ = os.RemoveAll(work)
		return commandAdapter{}, nil, gitErr(CodeGitCommandFailed, "command", "mkdir", "create private home dir", err)
	}
	adapter := commandAdapter{gitPath: resolved, gitDir: gitDir, workDir: work, homeDir: home, runner: execCommandRunner{}}
	cleanup := func() error {
		err1 := os.RemoveAll(work)
		err2 := os.RemoveAll(home)
		if err1 != nil {
			return err1
		}
		return err2
	}
	if err := ctx.Err(); err != nil {
		_ = cleanup()
		return commandAdapter{}, nil, codeForContext(err)
	}
	return adapter, cleanup, nil
}

func resolveGitPath(gitPath string) (string, error) {
	if gitPath == "" {
		p, err := exec.LookPath("git")
		if err != nil {
			return "", gitErr(CodeGitNotFound, "command", "lookpath", "git executable not found", err)
		}
		gitPath = p
	} else if !filepath.IsAbs(gitPath) {
		return "", gitErr(CodeGitNotFound, "command", "git path", "configured git executable must be absolute", nil)
	}
	abs, err := filepath.Abs(gitPath)
	if err != nil {
		return "", gitErr(CodeGitNotFound, "command", "git path", "resolve git executable", err)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", gitErr(CodeGitNotFound, "command", "git path", "stat git executable", err)
	}
	if st.IsDir() {
		return "", gitErr(CodeGitNotFound, "command", "git path", "git executable is a directory", nil)
	}
	return abs, nil
}

func (a commandAdapter) runGit(ctx context.Context, spec commandSpec) (gitCommandOutput, error) {
	if err := ctx.Err(); err != nil {
		return gitCommandOutput{}, codeForContext(err)
	}
	if err := validateAllowedCommand(spec.args); err != nil {
		return gitCommandOutput{}, err
	}
	if spec.stdoutLimit <= 0 {
		spec.stdoutLimit = MaxGitStdoutBytes
	}
	if spec.stderrLimit <= 0 {
		spec.stderrLimit = MaxGitStderrBytes
	}
	if spec.timeout <= 0 {
		spec.timeout = DefaultGitCommandTimeout
	}
	inv := gitInvocation{
		Executable:  a.gitPath,
		Args:        append([]string(nil), spec.args...),
		Env:         a.env(),
		Dir:         a.workDir,
		Stdin:       append([]byte(nil), spec.stdin...),
		StdoutLimit: spec.stdoutLimit,
		StderrLimit: spec.stderrLimit,
		Timeout:     spec.timeout,
	}
	runner := a.runner
	if runner == nil {
		runner = execCommandRunner{}
	}
	out, err := runner.Run(ctx, inv)
	if err != nil {
		var ge *Error
		if errors.As(err, &ge) {
			return out, err
		}
		return out, gitErr(CodeGitCommandFailed, "command", spec.op, string(out.Stderr), err)
	}
	return out, nil
}

func (a commandAdapter) runRepoGit(ctx context.Context, spec commandSpec) (gitCommandOutput, error) {
	base := []string{
		"--no-pager",
		"--no-replace-objects",
		"--no-optional-locks",
		"--git-dir=" + a.gitDir,
		"-c", "core.hooksPath=/dev/null",
		"-c", "protocol.allow=never",
		"-c", "gc.auto=0",
		"-c", "maintenance.auto=false",
		"-c", "submodule.recurse=false",
	}
	spec.args = append(base, spec.args...)
	return a.runGit(ctx, spec)
}

func (a commandAdapter) env() []string {
	path := "/usr/bin:/bin:/usr/local/bin"
	return []string{
		"LC_ALL=C",
		"LANG=C",
		"TZ=UTC",
		"HOME=" + a.homeDir,
		"XDG_CONFIG_HOME=" + filepath.Join(a.homeDir, "xdg"),
		"PATH=" + path,
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
		"GIT_PAGER=cat",
		"PAGER=cat",
	}
}

var allowedSubcommands = map[string]struct{}{
	"version":          {},
	"config":           {},
	"check-ref-format": {},
	"rev-parse":        {},
	"ls-tree":          {},
	"cat-file":         {},
}

func validateAllowedCommand(args []string) error {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") || strings.Contains(arg, "=") || arg == "" {
			continue
		}
		if _, ok := allowedSubcommands[arg]; ok {
			return nil
		}
		return gitErr(CodeGitCommandFailed, "command", "policy", "unsupported git subcommand", nil)
	}
	return gitErr(CodeGitCommandFailed, "command", "policy", "missing git subcommand", nil)
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, inv gitInvocation) (gitCommandOutput, error) {
	cmdCtx := ctx
	cancel := func() {}
	if inv.Timeout > 0 {
		cmdCtx, cancel = context.WithTimeout(ctx, inv.Timeout)
	}
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, inv.Executable, inv.Args...)
	cmd.Dir = inv.Dir
	cmd.Env = inv.Env
	if inv.Stdin != nil {
		cmd.Stdin = bytes.NewReader(inv.Stdin)
	}
	stdout := &limitedBuffer{limit: inv.StdoutLimit}
	stderr := &limitedBuffer{limit: inv.StderrLimit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	out := gitCommandOutput{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if stdout.tooLarge || stderr.tooLarge {
		return out, gitErr(CodeGitOutputTooLarge, "command", firstSubcommand(inv.Args), "git output exceeded limit", err)
	}
	if cmdCtx.Err() != nil {
		if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
			return out, gitErr(CodeGitCommandTimeout, "command", firstSubcommand(inv.Args), "git command timed out", cmdCtx.Err())
		}
		return out, codeForContext(cmdCtx.Err())
	}
	if err != nil {
		return out, gitErr(CodeGitCommandFailed, "command", firstSubcommand(inv.Args), string(out.Stderr), err)
	}
	return out, nil
}

type limitedBuffer struct {
	buf      bytes.Buffer
	limit    int64
	tooLarge bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit < 0 {
		return 0, fs.ErrInvalid
	}
	remaining := b.limit - int64(b.buf.Len())
	if remaining <= 0 {
		b.tooLarge = true
		return len(p), nil
	}
	if int64(len(p)) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		b.tooLarge = true
		return len(p), nil
	}
	_, _ = b.buf.Write(p)
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte { return append([]byte(nil), b.buf.Bytes()...) }

func firstSubcommand(args []string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") || strings.Contains(arg, "=") || arg == "" {
			continue
		}
		if _, ok := allowedSubcommands[arg]; ok {
			return arg
		}
	}
	return "git"
}
