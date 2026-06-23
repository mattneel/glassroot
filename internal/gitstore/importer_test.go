package gitstore

import (
	"strings"
	"testing"
	"time"
)

func TestBuildGitHubSourceImportPlanUsesExactFetchAndKeepsTokenOutOfArgv(t *testing.T) {
	token := "gr_token_high_entropy_canary"
	cfg := GitHubSourceImportConfig{
		GitPath:           "/usr/bin/git",
		RepositoryDir:     "/control/staging/repository.git",
		TemplateDir:       "/control/staging/template-empty",
		ObjectFormat:      ObjectFormatSHA1,
		RemoteURL:         "https://github.com/owner/repo.git",
		BaseCommitID:      strings.Repeat("1", 40),
		ExpectedHeadID:    strings.Repeat("2", 40),
		PullRequestNumber: 7,
		AuthHeaderEnvName: "GLASSROOT_GIT_AUTHORIZATION_HEADER",
		AuthHeaderValue:   []byte("Authorization: Basic " + token),
		Limits:            GitHubSourceImportLimits{MaxStdoutBytes: 1024, MaxStderrBytes: 1024, Timeout: time.Minute},
	}
	plan, err := BuildGitHubSourceImportPlan(cfg)
	if err != nil {
		t.Fatalf("BuildGitHubSourceImportPlan: %v", err)
	}
	if len(plan.Commands) != 2 {
		t.Fatalf("command count = %d, want init and fetch", len(plan.Commands))
	}
	init := strings.Join(plan.Commands[0].Args, " ")
	if !strings.Contains(init, "init") || !strings.Contains(init, "--bare") || !strings.Contains(init, "--template") || !strings.Contains(init, "--object-format=sha1") {
		t.Fatalf("init argv missing fixed bare/template/object-format: %#v", plan.Commands[0].Args)
	}
	fetchArgs := plan.Commands[1].Args
	fetch := strings.Join(fetchArgs, " ")
	for _, want := range []string{"fetch", "--depth=1", "--no-tags", "--no-write-fetch-head", "--no-recurse-submodules", "--no-auto-maintenance", "--no-write-commit-graph", "+" + strings.Repeat("1", 40) + ":refs/glassroot/base", "+refs/pull/7/head:refs/glassroot/head"} {
		if !strings.Contains(fetch, want) {
			t.Fatalf("fetch argv missing %q in %#v", want, fetchArgs)
		}
	}
	for _, forbidden := range []string{"--filter", "clone", "checkout", "credential-store", token} {
		if strings.Contains(fetch, forbidden) || strings.Contains(init, forbidden) {
			t.Fatalf("forbidden %q in argv: init=%q fetch=%q", forbidden, init, fetch)
		}
	}
	for _, arg := range append(append([]string{}, plan.Commands[0].Args...), fetchArgs...) {
		if arg == "submodule" {
			t.Fatalf("submodule command must not appear in argv: %#v", fetchArgs)
		}
	}
	if !contains(fetchArgs, "--config-env=http.https://github.com/owner/repo.git.extraHeader=GLASSROOT_GIT_AUTHORIZATION_HEADER") {
		t.Fatalf("missing exact URL-scoped --config-env extraHeader: %#v", fetchArgs)
	}
	if !contains(plan.Commands[1].Env, "GIT_CONFIG_GLOBAL=/dev/null") || !contains(plan.Commands[1].Env, "GIT_TERMINAL_PROMPT=0") || !contains(plan.Commands[1].Env, "GIT_PROTOCOL=version=2") {
		t.Fatalf("missing sanitized env in %#v", plan.Commands[1].Env)
	}
	for _, env := range plan.Commands[1].Env {
		if strings.HasPrefix(env, "HTTP_PROXY=") || strings.HasPrefix(env, "HTTPS_PROXY=") || strings.HasPrefix(env, "GIT_SSH=") || strings.HasPrefix(env, "GIT_TRACE=") {
			t.Fatalf("forbidden inherited env present: %s", env)
		}
	}
	if !contains(plan.Commands[1].Env, "GLASSROOT_GIT_AUTHORIZATION_HEADER=Authorization: Basic "+token) {
		t.Fatalf("auth header must be present only in fixed env var: %#v", plan.Commands[1].Env)
	}
}
