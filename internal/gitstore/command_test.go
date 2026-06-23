package gitstore

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCommandAdapterSanitizesInvocations(t *testing.T) {
	runner := &recordingRunner{stdout: []byte("git version 2.43.0\n")}
	adapter := commandAdapter{
		gitPath: "/usr/bin/git",
		gitDir:  "/control/repo.git",
		workDir: t.TempDir(),
		homeDir: t.TempDir(),
		runner:  runner,
	}
	_, err := adapter.runGit(context.Background(), commandSpec{op: "version", stdoutLimit: MaxGitStdoutBytes, stderrLimit: MaxGitStderrBytes, timeout: DefaultGitCommandTimeout, args: []string{"version"}})
	if err != nil {
		t.Fatal(err)
	}
	inv := runner.invocations[0]
	if inv.Executable != "/usr/bin/git" || !filepath.IsAbs(inv.Executable) {
		t.Fatalf("git executable = %q", inv.Executable)
	}
	if inv.Dir == "/control/repo.git" || strings.Contains(inv.Dir, "repo.git") {
		t.Fatalf("cwd selected from repository: %q", inv.Dir)
	}
	if contains(inv.Args, "sh") || contains(inv.Args, "bash") || contains(inv.Args, "cmd.exe") || contains(inv.Args, "powershell") {
		t.Fatalf("shell appeared in argv: %#v", inv.Args)
	}
	for _, env := range inv.Env {
		if strings.HasPrefix(env, "GIT_DIR=") || strings.HasPrefix(env, "GIT_WORK_TREE=") || strings.HasPrefix(env, "GIT_SSH=") || strings.HasPrefix(env, "GIT_CONFIG_COUNT=") {
			t.Fatalf("forbidden inherited env present: %s", env)
		}
	}
	for _, want := range []string{"LC_ALL=C", "LANG=C", "TZ=UTC", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "GIT_NO_REPLACE_OBJECTS=1", "GIT_NO_LAZY_FETCH=1", "GIT_PROTOCOL_FROM_USER=0", "GIT_LITERAL_PATHSPECS=1"} {
		if !contains(inv.Env, want) {
			t.Fatalf("missing env %s in %#v", want, inv.Env)
		}
	}
	if inv.StdoutLimit != MaxGitStdoutBytes || inv.StderrLimit != MaxGitStderrBytes || inv.Timeout != DefaultGitCommandTimeout {
		t.Fatalf("limits = %#v", inv)
	}
}

func TestRepositoryCommandUsesFixedSafeguardsAndPathAfterDoubleDash(t *testing.T) {
	runner := &recordingRunner{stdout: []byte("")}
	adapter := commandAdapter{gitPath: "/usr/bin/git", gitDir: "/control/repo.git", workDir: t.TempDir(), homeDir: t.TempDir(), runner: runner}
	_, _ = adapter.runRepoGit(context.Background(), commandSpec{op: "path", stdoutLimit: 1024, stderrLimit: 1024, timeout: time.Second, args: []string{"ls-tree", "-z", "--", "--attacker-path"}})
	inv := runner.invocations[0]
	for _, want := range []string{"--no-pager", "--no-replace-objects", "--no-optional-locks", "--git-dir=/control/repo.git", "-c", "core.hooksPath=/dev/null", "-c", "protocol.allow=never", "-c", "gc.auto=0", "-c", "maintenance.auto=false", "-c", "submodule.recurse=false"} {
		if !contains(inv.Args, want) {
			t.Fatalf("missing safeguard %q in %#v", want, inv.Args)
		}
	}
	if idx := indexOf(inv.Args, "--"); idx < 0 || idx+1 >= len(inv.Args) || inv.Args[idx+1] != "--attacker-path" {
		t.Fatalf("path not after --: %#v", inv.Args)
	}
}

func TestCommandAdapterRejectsUnlistedSubcommandsAndBoundsErrors(t *testing.T) {
	runner := &recordingRunner{err: errors.New("boom"), stderr: []byte("bad\x1b[31m" + strings.Repeat("x", 10000))}
	adapter := commandAdapter{gitPath: "/usr/bin/git", gitDir: "/control/repo.git", workDir: t.TempDir(), homeDir: t.TempDir(), runner: runner}
	_, err := adapter.runRepoGit(context.Background(), commandSpec{op: "clone", stdoutLimit: 1024, stderrLimit: 64, timeout: time.Second, args: []string{"clone", "https://example.invalid"}})
	if !errors.Is(err, ErrGitCommandFailed) {
		t.Fatalf("clone err = %v", err)
	}
	if len(runner.invocations) != 0 {
		t.Fatalf("disallowed command was invoked")
	}
	_, err = adapter.runRepoGit(context.Background(), commandSpec{op: "cat-file", stdoutLimit: 1024, stderrLimit: 64, timeout: time.Second, args: []string{"cat-file", "-p", "bad"}})
	if err == nil || strings.Contains(err.Error(), "\x1b") || len(err.Error()) > 512 {
		t.Fatalf("unbounded/unsanitized error: %q", err)
	}
}

type recordingRunner struct {
	invocations []gitInvocation
	stdout      []byte
	stderr      []byte
	err         error
}

func (r *recordingRunner) Run(ctx context.Context, inv gitInvocation) (gitCommandOutput, error) {
	r.invocations = append(r.invocations, inv)
	return gitCommandOutput{Stdout: append([]byte(nil), r.stdout...), Stderr: append([]byte(nil), r.stderr...)}, r.err
}

func contains[T comparable](items []T, want T) bool { return indexOf(items, want) >= 0 }
func indexOf[T comparable](items []T, want T) int {
	for i, item := range items {
		if item == want {
			return i
		}
	}
	return -1
}
