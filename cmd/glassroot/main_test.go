package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionCommandPrintsBuildMetadata(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"version"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run returned exit code %d, want 0; stderr=%q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	got := stdout.String()
	for _, want := range []string{
		"glassroot dev",
		"commit: unknown",
		"built: unknown",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("version output %q does not contain %q", got, want)
		}
	}
}

func TestValidateCommandExitCodesAndOutput(t *testing.T) {
	valid := filepath.Join("..", "..", "internal", "config", "testdata", "valid", "pipeline.yaml")
	invalid := filepath.Join("..", "..", "internal", "config", "testdata", "invalid", "invalid-unit.yaml")

	t.Run("valid file exits zero", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"validate", "--file", valid}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr.String())
		}
		if stdout.String() != "valid: "+valid+"\n" || stderr.Len() != 0 {
			t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
		}
	})

	t.Run("invalid file exits two", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"validate", "--file", invalid}, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("exit = %d, want 2; stderr=%q", code, stderr.String())
		}
		if stdout.Len() != 0 || !strings.Contains(stderr.String(), "invalid-unit") {
			t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
		}
	})

	t.Run("malformed usage exits two", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"validate", "--file", valid, "extra"}, &stdout, &stderr)
		if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "usage:") {
			t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	})

	t.Run("missing file exits two", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"validate", "--file", filepath.Join(t.TempDir(), "missing.yaml")}, &stdout, &stderr)
		if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "missing configuration") {
			t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	})
}

func TestValidateCommandUnexpectedReadFailureExitsThree(t *testing.T) {
	oldRead := readConfigFile
	readConfigFile = func(string) ([]byte, error) { return nil, errors.New("boom") }
	t.Cleanup(func() { readConfigFile = oldRead })

	var stdout, stderr bytes.Buffer
	code := run([]string{"validate", "--file", "anything.yaml"}, &stdout, &stderr)
	if code != 3 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "unexpected I/O") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestValidateDefaultPath(t *testing.T) {
	tmp := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmp, ".glassroot"), 0o755); err != nil {
		t.Fatal(err)
	}
	valid, err := os.ReadFile(filepath.Join("..", "..", "internal", "config", "testdata", "valid", "pipeline.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".glassroot", "pipeline.yaml"), valid, 0o644); err != nil {
		t.Fatal(err)
	}
	oldwd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })

	var stdout, stderr bytes.Buffer
	code := run([]string{"validate"}, &stdout, &stderr)
	if code != 0 || stdout.String() != "valid: .glassroot/pipeline.yaml\n" || stderr.Len() != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
