package localrun

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/runner/dockerdev"
)

func TestParseCLIArgumentsRequiresExplicitTrustedInputs(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "run-out")
	args := []string{
		"--git-dir", "/tmp/control.git",
		"--base-commit", strings.Repeat("1", 40),
		"--head-commit", strings.Repeat("2", 40),
		"--docker-socket", "/tmp/docker.sock",
		"--run-id", "run-13c_1",
		"--created-at", "2026-06-23T12:00:00Z",
		"--evaluated-at", "2026-06-23T12:30:00Z",
		"--acknowledge-unsafe-development-runner", dockerdev.UnsafeDevelopmentAcknowledgementText,
		"--format", "markdown",
		outDir,
	}
	parsed, err := ParseCLIArguments(args)
	if err != nil {
		t.Fatalf("ParseCLIArguments() error = %v", err)
	}
	if parsed.Help || parsed.Format != "markdown" || parsed.Request.OutputDir != outDir {
		t.Fatalf("parsed request mismatch: %+v", parsed)
	}
	if parsed.Request.GitDir != "/tmp/control.git" || parsed.Request.DockerSocket != "/tmp/docker.sock" {
		t.Fatalf("trusted path fields not preserved: %+v", parsed.Request)
	}
}

func TestParseCLIArgumentsRejectsUnsafeOrImplicitInputs(t *testing.T) {
	validOut := filepath.Join(t.TempDir(), "out")
	base := []string{
		"--git-dir", "/tmp/control.git",
		"--base-commit", strings.Repeat("1", 40),
		"--head-commit", strings.Repeat("2", 40),
		"--docker-socket", "/tmp/docker.sock",
		"--run-id", "run-13c",
		"--created-at", "2026-06-23T12:00:00Z",
		"--evaluated-at", "2026-06-23T12:30:00Z",
		"--acknowledge-unsafe-development-runner", dockerdev.UnsafeDevelopmentAcknowledgementText,
		validOut,
	}
	cases := []struct {
		name string
		args []string
		code ErrorCode
	}{
		{"missing acknowledgement", withoutFlag(base, "--acknowledge-unsafe-development-runner"), CodeAcknowledgementInvalid},
		{"wrong acknowledgement", replaceFlag(base, "--acknowledge-unsafe-development-runner", "yes"), CodeAcknowledgementInvalid},
		{"duplicate socket", append([]string{"--docker-socket", "/tmp/other.sock"}, base...), CodeInvalidRequest},
		{"relative output", replaceLast(base, "relative/out"), CodeInvalidOutputPath},
		{"uppercase commit", replaceFlag(base, "--base-commit", strings.Repeat("A", 40)), CodeInvalidBaseCommit},
		{"revision expression", replaceFlag(base, "--head-commit", strings.Repeat("2", 40)+"^"), CodeInvalidHeadCommit},
		{"timezone offset", replaceFlag(base, "--created-at", "2026-06-23T12:00:00-04:00"), CodeInvalidCreatedAt},
		{"bad run id", replaceFlag(base, "--run-id", "Run 13C"), CodeInvalidRunID},
		{"unsupported format", replaceFlag(base, "--format", "html"), CodeInvalidRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseCLIArguments(tc.args)
			assertLocalRunError(t, err, tc.code)
		})
	}
}

func TestValidateRequestRejectsPathOverlapsAndExistingOutput(t *testing.T) {
	parent := t.TempDir()
	gitDir := filepath.Join(parent, "repo.git")
	if err := mkdir0700(gitDir); err != nil {
		t.Fatal(err)
	}
	existingOut := filepath.Join(parent, "existing")
	if err := mkdir0700(existingOut); err != nil {
		t.Fatal(err)
	}
	req := validRequestForTest(filepath.Join(parent, "out"))
	req.GitDir = gitDir
	req.DockerSocket = filepath.Join(parent, "docker.sock")
	if err := ValidateRequest(req); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	req.OutputDir = existingOut
	assertLocalRunError(t, ValidateRequest(req), CodeOutputAlreadyExists)
	req.OutputDir = filepath.Join(gitDir, "child")
	assertLocalRunError(t, ValidateRequest(req), CodeInvalidOutputPath)
	req.OutputDir = filepath.Join(parent, "sock-child")
	req.DockerSocket = filepath.Join(req.OutputDir, "docker.sock")
	assertLocalRunError(t, ValidateRequest(req), CodeInvalidOutputPath)
}

func TestParseCLIHelpHasNoSideEffects(t *testing.T) {
	parsed, err := ParseCLIArguments([]string{"--help"})
	if err != nil || !parsed.Help {
		t.Fatalf("help parse = %+v, %v", parsed, err)
	}
}

func validRequestForTest(out string) Request {
	created, _ := ParseTime("2026-06-23T12:00:00Z", CodeInvalidCreatedAt)
	evaluated, _ := ParseTime("2026-06-23T12:30:00Z", CodeInvalidEvaluatedAt)
	ack, _ := dockerdev.AcknowledgeUnsafeDevelopmentRunner(dockerdev.UnsafeDevelopmentAcknowledgementText)
	return Request{OutputDir: out, GitDir: "/tmp/control.git", DockerSocket: "/tmp/docker.sock", BaseCommitID: strings.Repeat("1", 40), HeadCommitID: strings.Repeat("2", 40), RunID: "run-13c", CreatedAt: created, EvaluatedAt: evaluated, Acknowledgement: ack}
}

func replaceFlag(args []string, flag, value string) []string {
	out := append([]string(nil), args...)
	for i := 0; i < len(out); i++ {
		if out[i] == flag && i+1 < len(out) {
			out[i+1] = value
			return out
		}
	}
	return append([]string{flag, value}, out...)
}

func withoutFlag(args []string, flag string) []string {
	out := []string{}
	for i := 0; i < len(args); i++ {
		if args[i] == flag {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

func replaceLast(args []string, value string) []string {
	out := append([]string(nil), args...)
	out[len(out)-1] = value
	return out
}
