package main

import (
	"bytes"
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
