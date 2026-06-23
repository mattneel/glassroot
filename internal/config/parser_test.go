package config

import (
	"bytes"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestParseValidFixture(t *testing.T) {
	data := readFixture(t, "valid/pipeline.yaml")
	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if doc.APIVersion.Value != APIVersionV1Alpha1 || doc.Kind.Value != KindPipeline {
		t.Fatalf("identity mismatch: %#v %#v", doc.APIVersion, doc.Kind)
	}
	if got := doc.Spec.Scenarios[0].Run.Value; !strings.Contains(got, "\ngo test ./...\n") {
		t.Fatalf("run string was not preserved literally: %q", got)
	}
}

func TestParseRejectsStrictYAMLSubsetViolations(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		code Code
	}{
		{"empty input", nil, CodeYAMLSyntax},
		{"invalid utf8", []byte{0xff, 0xfe}, CodeInvalidUTF8},
		{"nul byte", []byte("apiVersion: glassroot.dev/v1alpha1\x00"), CodeNULByte},
		{"oversized", bytes.Repeat([]byte("a"), MaxPipelineBytes+1), CodeInputTooLarge},
		{"multiple documents", readFixture(t, "invalid/multiple-documents.yaml"), CodeMultipleDocuments},
		{"duplicate key", readFixture(t, "invalid/duplicate-key.yaml"), CodeDuplicateKey},
		{"unknown key", readFixture(t, "invalid/unknown-field.yaml"), CodeUnknownField},
		{"alias", readFixture(t, "invalid/alias.yaml"), CodeUnsupportedYAMLFeature},
		{"anchor", []byte("apiVersion: &v glassroot.dev/v1alpha1\nkind: Pipeline\n"), CodeUnsupportedYAMLFeature},
		{"merge key", []byte("apiVersion: glassroot.dev/v1alpha1\nkind: Pipeline\nmetadata:\n  <<: {name: default}\n"), CodeUnsupportedYAMLFeature},
		{"explicit tag", []byte("apiVersion: !custom glassroot.dev/v1alpha1\nkind: Pipeline\n"), CodeUnsupportedYAMLFeature},
		{"directive", []byte("%YAML 1.2\n---\napiVersion: glassroot.dev/v1alpha1\nkind: Pipeline\n"), CodeUnsupportedYAMLFeature},
		{"complex key", []byte("? [apiVersion]\n: glassroot.dev/v1alpha1\nkind: Pipeline\n"), CodeUnsupportedYAMLFeature},
		{"excessive depth", nestedYAML(MaxYAMLDepth + 2), CodeOutOfRange},
		{"excessive scalar", []byte("apiVersion: " + strings.Repeat("a", MaxGeneralStringBytes+1) + "\nkind: Pipeline\n"), CodeOutOfRange},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.data)
			assertDiagnosticCode(t, err, tc.code)
			assertBoundedSanitizedError(t, err)
		})
	}
}

func TestParseEnforcesNodeAndDiagnosticLimits(t *testing.T) {
	var b strings.Builder
	b.WriteString("apiVersion: glassroot.dev/v1alpha1\nkind: Pipeline\nmetadata:\n")
	for i := 0; i < MaxYAMLNodes+10; i++ {
		b.WriteString("  k")
		b.WriteString(strconv.Itoa(i))
		b.WriteString(": v\n")
	}
	_, err := Parse([]byte(b.String()))
	assertDiagnosticCode(t, err, CodeOutOfRange)

	diags := Diagnostics{}
	for i := 0; i < MaxDiagnostics+50; i++ {
		diags = append(diags, Diagnostic{Code: CodeInvalidValue, Message: "x"})
	}
	if got := capDiagnostics(diags); len(got) != MaxDiagnostics {
		t.Fatalf("capDiagnostics len = %d, want %d", len(got), MaxDiagnostics)
	}
}

func TestDiagnosticsSupportErrorsAsAndIs(t *testing.T) {
	_, err := Parse(readFixture(t, "invalid/unknown-field.yaml"))
	var diags Diagnostics
	if !errors.As(err, &diags) {
		t.Fatalf("errors.As did not expose Diagnostics: %v", err)
	}
	if !errors.Is(err, ErrInvalidPipeline) {
		t.Fatalf("errors.Is(err, ErrInvalidPipeline) = false")
	}
}

func readFixture(t testing.TB, rel string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + rel)
	if err != nil {
		t.Fatalf("read fixture %s: %v", rel, err)
	}
	return data
}

func assertDiagnosticCode(t *testing.T, err error, want Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected diagnostic code %q, got nil", want)
	}
	var diags Diagnostics
	if !errors.As(err, &diags) {
		t.Fatalf("error %T does not expose Diagnostics: %v", err, err)
	}
	for _, diag := range diags {
		if diag.Code == want {
			return
		}
	}
	t.Fatalf("diagnostics %v do not contain code %q", diags, want)
}

func assertBoundedSanitizedError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	msg := err.Error()
	if len(msg) > 8192 {
		t.Fatalf("diagnostic output too long: %d", len(msg))
	}
	for _, r := range msg {
		if r < 0x20 && r != '\n' && r != '\t' {
			t.Fatalf("diagnostic output contains raw control character %U in %q", r, msg)
		}
	}
}

func nestedYAML(depth int) []byte {
	var b strings.Builder
	for i := 0; i < depth; i++ {
		b.WriteString(strings.Repeat("  ", i))
		b.WriteString("k:\n")
	}
	b.WriteString(strings.Repeat("  ", depth))
	b.WriteString("v: x\n")
	return []byte(b.String())
}
