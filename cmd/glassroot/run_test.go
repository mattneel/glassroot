package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/runner/dockerdev"
)

func TestRunDockerDevHelpAndUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"run", "docker-dev", "--help"}, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "usage: glassroot run docker-dev") {
		t.Fatalf("help exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"run", "docker-dev", "relative"}, &stdout, &stderr)
	if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "invalid-output-path") || !strings.Contains(stderr.String(), "usage: glassroot run docker-dev") {
		t.Fatalf("relative exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	out := filepath.Join(t.TempDir(), "out")
	code = run([]string{
		"run", "docker-dev",
		"--git-dir", "/tmp/control.git",
		"--base-commit", strings.Repeat("1", 40),
		"--head-commit", strings.Repeat("2", 40),
		"--docker-socket", "/tmp/docker.sock",
		"--run-id", "run-13c",
		"--created-at", "2026-06-23T12:00:00Z",
		"--evaluated-at", "2026-06-23T12:30:00Z",
		"--acknowledge-unsafe-development-runner", "wrong",
		out,
	}, &stdout, &stderr)
	if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "acknowledgement-invalid") {
		t.Fatalf("wrong ack exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunDockerDevMissingImageClassifiedAsInfrastructure(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"run", "docker-dev",
		"--git-dir", "/tmp/missing.git",
		"--base-commit", strings.Repeat("1", 40),
		"--head-commit", strings.Repeat("2", 40),
		"--docker-socket", "/tmp/missing.sock",
		"--run-id", "run-13c",
		"--created-at", "2026-06-23T12:00:00Z",
		"--evaluated-at", "2026-06-23T12:30:00Z",
		"--acknowledge-unsafe-development-runner", dockerdev.UnsafeDevelopmentAcknowledgementText,
		out,
	}, &stdout, &stderr)
	if code != 3 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "git-open-failed") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
