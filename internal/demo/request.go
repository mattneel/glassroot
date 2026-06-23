package demo

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type CLIRequest struct {
	Request Request
	Format  string
	Help    bool
}

func ParseCLIArguments(args []string) (CLIRequest, error) {
	out := CLIRequest{Format: "terminal", Request: Request{Fixture: FixtureBehaviorChange}}
	if len(args) == 1 && args[0] == "--help" {
		out.Help = true
		return out, nil
	}
	seen := map[string]bool{}
	pos := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "" {
			return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "empty argument", nil)
		}
		if !strings.HasPrefix(a, "--") {
			pos = append(pos, a)
			continue
		}
		name, val, has := a, "", false
		if idx := strings.IndexByte(a, '='); idx >= 0 {
			name = a[:idx]
			val = a[idx+1:]
			has = true
		}
		if name != "--fixture" && name != "--format" {
			return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "unknown flag", nil)
		}
		if seen[name] {
			code := CodeInvalidRequest
			if name == "--fixture" {
				code = CodeInvalidFixture
			}
			return CLIRequest{}, usageErr(code, "cli", "duplicate flag", nil)
		}
		seen[name] = true
		if !has {
			i++
			if i >= len(args) {
				return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "missing flag value", nil)
			}
			val = args[i]
		}
		switch name {
		case "--fixture":
			out.Request.Fixture = Fixture(val)
		case "--format":
			out.Format = val
		}
	}
	if len(pos) != 1 {
		return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "exactly one output directory is required", nil)
	}
	out.Request.OutputDir = pos[0]
	if out.Format != "terminal" && out.Format != "markdown" && out.Format != "json" {
		return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "format must be terminal, markdown, or json", nil)
	}
	if out.Request.Fixture != FixtureBehaviorChange && out.Request.Fixture != FixtureControl {
		return CLIRequest{}, usageErr(CodeInvalidFixture, "cli", "fixture must be behavior-change or control", nil)
	}
	if err := ValidateOutputPathSyntax(out.Request.OutputDir); err != nil {
		return CLIRequest{}, err
	}
	return out, nil
}

func ValidateOutputPathSyntaxForTest(p string) error { return ValidateOutputPathSyntax(p) }
func ValidateOutputPathSyntax(p string) error {
	if p == "" || len(p) > MaxOutputPathBytes || !utf8.ValidString(p) || strings.ContainsRune(p, 0) || containsControl(p) {
		return usageErr(CodeInvalidOutputPath, "request", "output path is invalid", nil)
	}
	if !filepath.IsAbs(p) {
		return usageErr(CodeInvalidOutputPath, "request", "output path must be absolute", nil)
	}
	if filepath.Clean(p) != p {
		return usageErr(CodeInvalidOutputPath, "request", "output path must be lexically clean", nil)
	}
	if p == string(filepath.Separator) || filepath.Base(p) == "." || filepath.Base(p) == string(filepath.Separator) {
		return usageErr(CodeInvalidOutputPath, "request", "output path must name a new directory", nil)
	}
	return nil
}
func validateRequest(req Request) error {
	if req.Fixture != FixtureBehaviorChange && req.Fixture != FixtureControl {
		return usageErr(CodeInvalidFixture, "request", "fixture must be behavior-change or control", nil)
	}
	if err := ValidateOutputPathSyntax(req.OutputDir); err != nil {
		return err
	}
	if _, err := os.Lstat(req.OutputDir); err == nil {
		return usageErr(CodeOutputAlreadyExists, "request", "output path already exists", nil)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return usageErr(CodeInvalidOutputPath, "request", "cannot inspect output path", err)
	}
	parent := filepath.Dir(req.OutputDir)
	st, err := os.Lstat(parent)
	if err != nil {
		return usageErr(CodeOutputParentInvalid, "request", "output parent is invalid", err)
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
		return usageErr(CodeOutputParentInvalid, "request", "output parent must be a real directory", nil)
	}
	return nil
}
func containsControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
