package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdditionalStrictParsingAndValidationCases(t *testing.T) {
	base := string(readFixture(t, "valid/pipeline.yaml"))
	cases := []struct {
		name string
		data string
		code Code
	}{
		{"unknown metadata key", strings.Replace(base, "  name: default", "  name: default\n  extra: nope", 1), CodeUnknownField},
		{"unknown environment key", strings.Replace(base, "    workdir: /workspace", "    workdir: /workspace\n    extra: nope", 1), CodeUnknownField},
		{"unknown scenario key", strings.Replace(base, "      timeout: 10m", "      timeout: 10m\n      extra: nope", 1), CodeUnknownField},
		{"null required object", strings.Replace(base, "metadata:\n  name: default", "metadata: null", 1), CodeMissingRequiredField},
		{"null required array", strings.Replace(base, "allow: []", "allow: null", 1), CodeMissingRequiredField},
		{"invalid identifier characters", strings.Replace(base, "id: test", "id: test$", 1), CodeInvalidValue},
		{"too many scenarios", replaceBlock(base, "  scenarios:", "  collect:", tooManyScenariosYAML(MaxScenarioCount+1)), CodeOutOfRange},
		{"overlong workdir", strings.Replace(base, "workdir: /workspace", "workdir: /"+strings.Repeat("a", MaxPathBytes), 1), CodeOutOfRange},
		{"root traversal segment", strings.Replace(base, "- /tmp", "- /tmp/..", 1), CodeInvalidPath},
		{"too many roots", replaceBlock(base, "      roots:", "      contents:", tooManyRootsYAML(MaxFilesystemRootCount+1)), CodeOutOfRange},
		{"too many artifacts", replaceBlock(base, "    artifacts:", "    logs:", tooManyArtifactsYAML(MaxArtifactCount+1)), CodeOutOfRange},
		{"too many ignore fields", replaceBlock(base, "    ignore:", "    repetitions:", tooManyIgnoreYAML(MaxCompareIgnoreCount+1)), CodeOutOfRange},
		{"small memory", strings.Replace(base, "memory: 2GiB", "memory: 15MiB", 1), CodeOutOfRange},
		{"large disk", strings.Replace(base, "disk: 4GiB", "disk: 17TiB", 1), CodeOutOfRange},
		{"short global timeout", strings.Replace(base, "timeout: 15m", "timeout: 99ms", 1), CodeOutOfRange},
		{"long global timeout", strings.Replace(base, "timeout: 15m", "timeout: 25h", 1), CodeOutOfRange},
		{"artifact too large", strings.Replace(base, "maxBytes: 50MiB", "maxBytes: 2GiB", 1), CodeOutOfRange},
		{"log too large", strings.Replace(base, "maxBytesPerStream: 10MiB", "maxBytesPerStream: 101MiB", 1), CodeOutOfRange},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseAndValidate([]byte(tc.data))
			assertDiagnosticCode(t, err, tc.code)
		})
	}
}

func TestParseDistinguishesOmittedFromExplicitZero(t *testing.T) {
	base := string(readFixture(t, "valid/pipeline.yaml"))
	zeroCPU, err := Parse([]byte(strings.Replace(base, "cpu: 2", "cpu: 0", 1)))
	if err != nil {
		t.Fatalf("Parse zero cpu: %v", err)
	}
	if !zeroCPU.Spec.Resources.CPU.Present || zeroCPU.Spec.Resources.CPU.Value != 0 {
		t.Fatalf("explicit zero not preserved: %#v", zeroCPU.Spec.Resources.CPU)
	}
	missingCPU, err := Parse([]byte(strings.Replace(base, "    cpu: 2\n", "", 1)))
	if err != nil {
		t.Fatalf("Parse missing cpu: %v", err)
	}
	if missingCPU.Spec.Resources.CPU.Present {
		t.Fatalf("omitted cpu marked present: %#v", missingCPU.Spec.Resources.CPU)
	}
}

func TestValidFixtureArraysAreNotNull(t *testing.T) {
	value := yamlFixtureAsJSONValue(t, readFixture(t, "valid/pipeline.yaml")).(map[string]any)
	checks := []struct {
		name  string
		value any
	}{
		{"network.allow", spec(value)["network"].(map[string]any)["allow"]},
		{"spec.scenarios", spec(value)["scenarios"]},
		{"filesystem.roots", spec(value)["collect"].(map[string]any)["filesystem"].(map[string]any)["roots"]},
		{"collect.artifacts", spec(value)["collect"].(map[string]any)["artifacts"]},
		{"compare.ignore", spec(value)["compare"].(map[string]any)["ignore"]},
	}
	for _, check := range checks {
		if check.value == nil {
			t.Fatalf("%s is null", check.name)
		}
	}
}

func TestSchemaObjectsSetAdditionalPropertiesFalse(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "api", "v1alpha1", "pipeline.schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("schema JSON: %v", err)
	}
	assertObjectSchemasClosed(t, "#", doc)
}

func assertObjectSchemasClosed(t *testing.T, path string, value any) {
	t.Helper()
	switch v := value.(type) {
	case map[string]any:
		if typ, ok := v["type"].(string); ok && typ == "object" {
			if closed, ok := v["additionalProperties"].(bool); !ok || closed {
				t.Fatalf("object schema %s missing additionalProperties:false", path)
			}
		}
		for key, child := range v {
			assertObjectSchemasClosed(t, path+"/"+key, child)
		}
	case []any:
		for i, child := range v {
			assertObjectSchemasClosed(t, path+"[]", child)
			_ = i
		}
	}
}

func tooManyScenariosYAML(n int) string {
	var b strings.Builder
	b.WriteString("  scenarios:\n")
	for i := 0; i < n; i++ {
		b.WriteString("    - id: s")
		b.WriteString(decimal(i))
		b.WriteString("\n      name: Scenario\n      shell: /bin/sh\n      run: echo ok\n      timeout: 1s\n")
	}
	return b.String()
}

func tooManyRootsYAML(n int) string {
	var b strings.Builder
	b.WriteString("      roots:\n")
	for i := 0; i < n; i++ {
		b.WriteString("        - /root")
		b.WriteString(decimal(i))
		b.WriteByte('\n')
	}
	return b.String()
}

func tooManyArtifactsYAML(n int) string {
	var b strings.Builder
	b.WriteString("    artifacts:\n")
	for i := 0; i < n; i++ {
		b.WriteString("      - path: /workspace/bin/")
		b.WriteString(decimal(i))
		b.WriteString("/**\n        maxBytes: 1MiB\n")
	}
	return b.String()
}

func tooManyIgnoreYAML(n int) string {
	var b strings.Builder
	b.WriteString("    ignore:\n")
	for i := 0; i < n; i++ {
		b.WriteString("      - field: event.timestamp\n")
	}
	return b.String()
}

func decimal(n int) string {
	if n == 0 {
		return "0"
	}
	digits := [20]byte{}
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[i:])
}
