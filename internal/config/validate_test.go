package config

import (
	"strings"
	"testing"
)

func TestValidateValidFixtureNormalizesValues(t *testing.T) {
	pipeline, err := ParseAndValidate(readFixture(t, "valid/pipeline.yaml"))
	if err != nil {
		t.Fatalf("ParseAndValidate() error = %v", err)
	}
	if pipeline.Name != "default" || pipeline.ImageDigest != strings.Repeat("", 0) && len(pipeline.ImageDigest) != 64 {
		t.Fatalf("unexpected identity/digest: %#v", pipeline)
	}
	if pipeline.Resources.MemoryBytes != 2*1024*1024*1024 || pipeline.Resources.TimeoutMillis != 15*60*1000 {
		t.Fatalf("resource normalization mismatch: %#v", pipeline.Resources)
	}
	if len(pipeline.Scenarios) != 2 || pipeline.Scenarios[0].TimeoutMillis != 10*60*1000 {
		t.Fatalf("scenario normalization mismatch: %#v", pipeline.Scenarios)
	}
	if !strings.Contains(pipeline.Scenarios[0].Run, "echo \"literal shell text stays data\"") {
		t.Fatalf("run string not preserved literally: %q", pipeline.Scenarios[0].Run)
	}
}

func TestValidateRejectsSemanticInvalidCases(t *testing.T) {
	base := string(readFixture(t, "valid/pipeline.yaml"))
	cases := []struct {
		name string
		mut  func(string) string
		code Code
	}{
		{"missing apiVersion", func(s string) string { return strings.Replace(s, "apiVersion: glassroot.dev/v1alpha1\n", "", 1) }, CodeMissingRequiredField},
		{"null required scalar", func(s string) string { return strings.Replace(s, "kind: Pipeline", "kind: null", 1) }, CodeMissingRequiredField},
		{"wrong apiVersion", func(s string) string { return strings.Replace(s, "glassroot.dev/v1alpha1", "glassroot.dev/v9", 1) }, CodeInvalidAPIVersion},
		{"wrong kind casing", func(s string) string { return strings.Replace(s, "kind: Pipeline", "kind: pipeline", 1) }, CodeInvalidKind},
		{"invalid metadata id uppercase", func(s string) string { return strings.Replace(s, "name: default", "name: Default", 1) }, CodeInvalidValue},
		{"duplicate scenario IDs", func(s string) string { return strings.Replace(s, "id: build", "id: test", 1) }, CodeDuplicateScenarioID},
		{"zero scenarios", func(s string) string {
			return replaceBlock(s, "  scenarios:", "  collect:", "  scenarios: []\n")
		}, CodeOutOfRange},
		{"image without digest", func(s string) string {
			return strings.Replace(s, "docker.io/library/golang:1.26@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "docker.io/library/golang:1.26", 1)
		}, CodeInvalidValue},
		{"uppercase digest", func(s string) string { return strings.Replace(s, "0123456789abcdef", "0123456789ABCDEF", 1) }, CodeInvalidValue},
		{"placeholder digest", func(s string) string {
			return strings.Replace(s, "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "sha256:REPLACE_WITH_REAL_DIGEST", 1)
		}, CodeInvalidValue},
		{"relative workdir", func(s string) string { return strings.Replace(s, "workdir: /workspace", "workdir: workspace", 1) }, CodeInvalidPath},
		{"traversal workdir", func(s string) string {
			return strings.Replace(s, "workdir: /workspace", "workdir: /workspace/../tmp", 1)
		}, CodeInvalidPath},
		{"backslash root", func(s string) string { return strings.Replace(s, "- /tmp", "- /tmp\\bad", 1) }, CodeInvalidPath},
		{"duplicate root", func(s string) string { return strings.Replace(s, "- /tmp", "- /workspace", 1) }, CodeInvalidPath},
		{"invalid artifact glob", func(s string) string { return strings.Replace(s, "/workspace/bin/**", "/workspace/bin/[", 1) }, CodeInvalidPath},
		{"duplicate artifact path", func(s string) string {
			return strings.Replace(s, "artifacts:\n      - path: /workspace/bin/**\n        maxBytes: 50MiB", "artifacts:\n      - path: /workspace/bin/**\n        maxBytes: 50MiB\n      - path: /workspace/bin/**\n        maxBytes: 1MiB", 1)
		}, CodeInvalidPath},
		{"invalid shell", func(s string) string { return strings.Replace(s, "shell: /bin/sh", "shell: /bin/zsh", 1) }, CodeInvalidValue},
		{"shell args", func(s string) string { return strings.Replace(s, "shell: /bin/sh", "shell: /bin/sh -c", 1) }, CodeInvalidValue},
		{"empty run", func(s string) string {
			return strings.Replace(s, "run: |\n        echo \"literal shell text stays data\"\n        go test ./...", "run: \"\"", 1)
		}, CodeInvalidValue},
		{"oversized run", func(s string) string {
			return strings.Replace(s, "go build ./cmd/glassroot", strings.Repeat("x", MaxRunBytes+1), 1)
		}, CodeOutOfRange},
		{"invalid byte unit", func(s string) string { return strings.Replace(s, "memory: 2GiB", "memory: 2GB", 1) }, CodeInvalidUnit},
		{"overflow byte unit", func(s string) string { return strings.Replace(s, "disk: 4GiB", "disk: 999999999999999999999TiB", 1) }, CodeInvalidUnit},
		{"invalid duration unit", func(s string) string { return strings.Replace(s, "timeout: 15m", "timeout: 1h30m", 1) }, CodeInvalidUnit},
		{"scenario timeout greater", func(s string) string { return strings.Replace(s, "timeout: 10m", "timeout: 16m", 1) }, CodeCrossFieldConstraint},
		{"zero cpu", func(s string) string { return strings.Replace(s, "cpu: 2", "cpu: 0", 1) }, CodeOutOfRange},
		{"excessive processes", func(s string) string { return strings.Replace(s, "processes: 256", "processes: 70000", 1) }, CodeOutOfRange},
		{"network allowlist", func(s string) string { return strings.Replace(s, "mode: deny", "mode: allowlist", 1) }, CodeInvalidValue},
		{"nonempty allow", func(s string) string { return strings.Replace(s, "allow: []", "allow:\n      - example.invalid", 1) }, CodeInvalidValue},
		{"null allow", func(s string) string { return strings.Replace(s, "allow: []", "allow: null", 1) }, CodeMissingRequiredField},
		{"unknown collection mode", func(s string) string { return strings.Replace(s, "metadata-and-digests", "contents", 1) }, CodeInvalidValue},
		{"duplicate ignore", func(s string) string {
			return strings.Replace(s, "- field: process.pid", "- field: event.timestamp", 1)
		}, CodeInvalidValue},
		{"unknown ignore", func(s string) string { return strings.Replace(s, "process.pid", "process.ppid", 1) }, CodeInvalidValue},
		{"bad repetitions", func(s string) string { return strings.Replace(s, "repetitions: 1", "repetitions: 0", 1) }, CodeOutOfRange},
		{"unknown policy", func(s string) string { return strings.Replace(s, "profile: strict", "profile: loose", 1) }, CodeInvalidValue},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseAndValidate([]byte(tc.mut(base)))
			assertDiagnosticCode(t, err, tc.code)
		})
	}
}

func TestUnitParsersNormalizeExactly(t *testing.T) {
	sizes := map[string]int64{"1B": 1, "16MiB": 16 * 1024 * 1024, "1TiB": 1024 * 1024 * 1024 * 1024}
	for in, want := range sizes {
		got, err := ParseSizeBytes(in)
		if err != nil || got != want {
			t.Fatalf("ParseSizeBytes(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"0B", "-1B", "1.5MiB", "1 MB", "1GB", "999999999999999999999TiB"} {
		if _, err := ParseSizeBytes(bad); err == nil {
			t.Fatalf("ParseSizeBytes(%q) succeeded", bad)
		}
	}

	durations := map[string]int64{"100ms": 100, "1s": 1000, "2m": 120000, "1h": 3600000}
	for in, want := range durations {
		got, err := ParseDurationMillis(in)
		if err != nil || got != want {
			t.Fatalf("ParseDurationMillis(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"0ms", "-1s", "1.5s", "1h30m", "1d", "999999999999999999999h"} {
		if _, err := ParseDurationMillis(bad); err == nil {
			t.Fatalf("ParseDurationMillis(%q) succeeded", bad)
		}
	}
}

func replaceBlock(s, start, end, replacement string) string {
	startIdx := strings.Index(s, start)
	if startIdx < 0 {
		return s
	}
	endIdx := strings.Index(s[startIdx:], end)
	if endIdx < 0 {
		return s
	}
	endIdx += startIdx
	return s[:startIdx] + replacement + s[endIdx:]
}
