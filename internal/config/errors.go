package config

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type Code string

const (
	CodeInputTooLarge          Code = "input-too-large"
	CodeInvalidUTF8            Code = "invalid-utf8"
	CodeNULByte                Code = "nul-byte"
	CodeYAMLSyntax             Code = "yaml-syntax"
	CodeMultipleDocuments      Code = "multiple-documents"
	CodeDuplicateKey           Code = "duplicate-key"
	CodeUnknownField           Code = "unknown-field"
	CodeUnsupportedYAMLFeature Code = "unsupported-yaml-feature"
	CodeMissingRequiredField   Code = "missing-required-field"
	CodeInvalidAPIVersion      Code = "invalid-api-version"
	CodeInvalidKind            Code = "invalid-kind"
	CodeInvalidValue           Code = "invalid-value"
	CodeInvalidUnit            Code = "invalid-unit"
	CodeOutOfRange             Code = "out-of-range"
	CodeInvalidPath            Code = "invalid-path"
	CodeDuplicateScenarioID    Code = "duplicate-scenario-id"
	CodeCrossFieldConstraint   Code = "cross-field-constraint"
)

var ErrInvalidPipeline = errors.New("invalid pipeline")

type Diagnostic struct {
	Code    Code
	Path    string
	Line    int
	Column  int
	Message string
}

type Diagnostics []Diagnostic

func (d Diagnostics) Error() string {
	if len(d) == 0 {
		return ErrInvalidPipeline.Error()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d diagnostic", ErrInvalidPipeline, len(d))
	if len(d) != 1 {
		b.WriteByte('s')
	}
	for i, diag := range d {
		if i >= MaxDiagnostics {
			break
		}
		b.WriteString("\n")
		b.WriteString(formatDiagnostic(diag))
	}
	return b.String()
}

func (d Diagnostics) Is(target error) bool {
	return target == ErrInvalidPipeline
}

func formatDiagnostic(diag Diagnostic) string {
	var b strings.Builder
	if diag.Path != "" {
		b.WriteString(diag.Path)
	}
	if diag.Line > 0 {
		if b.Len() > 0 {
			b.WriteByte(':')
		}
		fmt.Fprintf(&b, "%d", diag.Line)
		if diag.Column > 0 {
			fmt.Fprintf(&b, ":%d", diag.Column)
		}
	}
	if b.Len() > 0 {
		b.WriteString(": ")
	}
	b.WriteString(string(diag.Code))
	if diag.Message != "" {
		b.WriteString(": ")
		b.WriteString(sanitizeForDiagnostic(diag.Message, 512))
	}
	return b.String()
}

func capDiagnostics(diags Diagnostics) Diagnostics {
	if len(diags) <= MaxDiagnostics {
		return diags
	}
	out := make(Diagnostics, MaxDiagnostics)
	copy(out, diags[:MaxDiagnostics])
	return out
}

func newDiagnostic(code Code, path string, line, column int, msg string) Diagnostic {
	return Diagnostic{Code: code, Path: path, Line: line, Column: column, Message: sanitizeForDiagnostic(msg, 512)}
}

func sanitizeForDiagnostic(s string, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = 512
	}
	var b strings.Builder
	for len(s) > 0 && b.Len() < maxBytes {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			if b.Len()+6 > maxBytes {
				break
			}
			fmt.Fprintf(&b, "\\x%02x", s[0])
			s = s[1:]
			continue
		}
		s = s[size:]
		if r < 0x20 || r == 0x7f {
			if b.Len()+6 > maxBytes {
				break
			}
			fmt.Fprintf(&b, "\\u%04x", r)
			continue
		}
		if b.Len()+size > maxBytes {
			break
		}
		b.WriteRune(r)
	}
	if len(s) > 0 && b.Len()+1 <= maxBytes {
		b.WriteString("…")
	}
	return b.String()
}
