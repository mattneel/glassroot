package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDemoFakeHelpAndUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"demo", "fake", "--help"}, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "usage: glassroot demo fake") {
		t.Fatalf("help exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"demo", "fake", "relative"}, &stdout, &stderr)
	if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "invalid-output-path") {
		t.Fatalf("relative exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"demo", "fake", "--fixture", "unknown", "/tmp/out"}, &stdout, &stderr)
	if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "invalid-fixture") {
		t.Fatalf("unknown fixture exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestDemoFakeCommandPublishesAndPrintsSelectedReport(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("demo creation is linux-only")
	}
	out := filepath.Join(t.TempDir(), "demo-out")
	var stdout, stderr bytes.Buffer
	code := run([]string{"demo", "fake", "--fixture", "behavior-change", "--format", "terminal", out}, &stdout, &stderr)
	if code != 4 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stored, err := os.ReadFile(filepath.Join(out, "report.txt"))
	if err != nil {
		t.Fatalf("read report.txt: %v", err)
	}
	if !bytes.Equal(stdout.Bytes(), stored) {
		t.Fatalf("stdout differs from report.txt")
	}
	if bytes.Contains(stdout.Bytes(), []byte(out)) || !bytes.Contains(stdout.Bytes(), []byte("fake")) {
		t.Fatalf("stdout leaked path or omitted fake notice")
	}
}

func TestDemoFakeJSONOutputHasNoAddedNewline(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("demo creation is linux-only")
	}
	out := filepath.Join(t.TempDir(), "demo-json")
	var stdout, stderr bytes.Buffer
	code := run([]string{"demo", "fake", "--fixture", "control", "--format", "json", out}, &stdout, &stderr)
	if code != 4 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stored, err := os.ReadFile(filepath.Join(out, "report.json"))
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	if !bytes.Equal(stdout.Bytes(), stored) || bytes.HasSuffix(stdout.Bytes(), []byte("\n")) {
		t.Fatalf("JSON stdout contract violated")
	}
}

func TestDemoFakeOutputWriterFailure(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("demo creation is linux-only")
	}
	out := filepath.Join(t.TempDir(), "demo-short")
	var stderr bytes.Buffer
	code := run([]string{"demo", "fake", out}, &shortWriter{limit: 8}, &stderr)
	if code != 3 || !strings.Contains(stderr.String(), "output-failed") {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
}
