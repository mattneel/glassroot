package localrun

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mattneel/glassroot/internal/runner/dockerdev"
)

const TimeFormat = "2006-01-02T15:04:05Z"

func ParseTime(s string, code ErrorCode) (time.Time, error) {
	if len(s) != len(TimeFormat) || strings.TrimSpace(s) != s {
		return time.Time{}, usageErr(code, "request", "timestamp must use YYYY-MM-DDTHH:MM:SSZ", nil)
	}
	t, err := time.Parse(TimeFormat, s)
	if err != nil || t.Format(TimeFormat) != s || t.Location() != time.UTC {
		return time.Time{}, usageErr(code, "request", "timestamp must use exact UTC form", err)
	}
	return t.UTC().Round(0), nil
}

func ParseCLIArgumentsSyntaxOnly(args []string) (CLIRequest, error) {
	out := CLIRequest{Format: "terminal"}
	if len(args) == 1 && args[0] == "--help" {
		out.Help = true
		return out, nil
	}
	seen := map[string]struct{}{}
	positionals := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "" {
			return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "empty argument", nil)
		}
		if !strings.HasPrefix(arg, "--") {
			positionals = append(positionals, arg)
			continue
		}
		name, val, has := arg, "", false
		if idx := strings.IndexByte(arg, '='); idx >= 0 {
			name, val, has = arg[:idx], arg[idx+1:], true
		}
		if !knownFlag(name) {
			return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "unknown flag", nil)
		}
		if _, ok := seen[name]; ok {
			return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "duplicate flag", nil)
		}
		seen[name] = struct{}{}
		if !has {
			i++
			if i >= len(args) {
				return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "missing flag value", nil)
			}
			val = args[i]
		}
		switch name {
		case "--git-dir":
			out.Request.GitDir = val
		case "--base-commit":
			out.Request.BaseCommitID = val
		case "--head-commit":
			out.Request.HeadCommitID = val
		case "--docker-socket":
			out.Request.DockerSocket = val
		case "--run-id":
			out.Request.RunID = val
		case "--created-at":
			t, err := ParseTime(val, CodeInvalidCreatedAt)
			if err != nil {
				return CLIRequest{}, err
			}
			out.Request.CreatedAt = t
		case "--evaluated-at":
			t, err := ParseTime(val, CodeInvalidEvaluatedAt)
			if err != nil {
				return CLIRequest{}, err
			}
			out.Request.EvaluatedAt = t
		case "--acknowledge-unsafe-development-runner":
			ack, err := dockerdev.AcknowledgeUnsafeDevelopmentRunner(val)
			if err != nil {
				return CLIRequest{}, usageErr(CodeAcknowledgementInvalid, "cli", "exact unsafe-development acknowledgement is required", err)
			}
			out.Request.Acknowledgement = ack
		case "--format":
			out.Format = val
		}
	}
	if len(positionals) != 1 {
		return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "exactly one output directory is required", nil)
	}
	out.Request.OutputDir = positionals[0]
	if out.Format != "terminal" && out.Format != "markdown" && out.Format != "json" {
		return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "format must be terminal, markdown, or json", nil)
	}
	if err := validatePathSyntax(out.Request.OutputDir, CodeInvalidOutputPath, "output path"); err != nil {
		return CLIRequest{}, err
	}
	if _, ok := seen["--acknowledge-unsafe-development-runner"]; !ok {
		return CLIRequest{}, usageErr(CodeAcknowledgementInvalid, "cli", "unsafe-development acknowledgement is required", nil)
	}
	if err := validatePathSyntax(out.Request.GitDir, CodeInvalidGitDir, "git directory"); err != nil {
		return CLIRequest{}, err
	}
	if err := validatePathSyntax(out.Request.DockerSocket, CodeInvalidDockerSocket, "docker socket"); err != nil {
		return CLIRequest{}, err
	}
	if err := validateCommitID(out.Request.BaseCommitID, CodeInvalidBaseCommit); err != nil {
		return CLIRequest{}, err
	}
	if err := validateCommitID(out.Request.HeadCommitID, CodeInvalidHeadCommit); err != nil {
		return CLIRequest{}, err
	}
	if err := validateRunID(out.Request.RunID); err != nil {
		return CLIRequest{}, err
	}
	if out.Request.CreatedAt.IsZero() {
		return CLIRequest{}, usageErr(CodeInvalidCreatedAt, "request", "created-at is required", nil)
	}
	if out.Request.EvaluatedAt.IsZero() {
		return CLIRequest{}, usageErr(CodeInvalidEvaluatedAt, "request", "evaluated-at is required", nil)
	}
	if out.Request.Acknowledgement == (dockerdev.UnsafeDevelopmentAcknowledgement{}) {
		return CLIRequest{}, usageErr(CodeAcknowledgementInvalid, "request", "unsafe-development acknowledgement is required", nil)
	}
	return out, nil
}

func ParseCLIArguments(args []string) (CLIRequest, error) {
	out, err := ParseCLIArgumentsSyntaxOnly(args)
	if err != nil || out.Help {
		return out, err
	}
	if err := ValidateRequest(out.Request); err != nil {
		return CLIRequest{}, err
	}
	return out, nil
}

func knownFlag(name string) bool {
	switch name {
	case "--git-dir", "--base-commit", "--head-commit", "--docker-socket", "--run-id", "--created-at", "--evaluated-at", "--acknowledge-unsafe-development-runner", "--format":
		return true
	default:
		return false
	}
}

func ValidateRequest(req Request) error {
	if err := validatePathSyntax(req.OutputDir, CodeInvalidOutputPath, "output path"); err != nil {
		return err
	}
	if err := validatePathSyntax(req.GitDir, CodeInvalidGitDir, "git directory"); err != nil {
		return err
	}
	if err := validatePathSyntax(req.DockerSocket, CodeInvalidDockerSocket, "docker socket"); err != nil {
		return err
	}
	if err := validateOutputPath(req.OutputDir); err != nil {
		return err
	}
	if overlaps(req.OutputDir, req.GitDir) || overlaps(req.GitDir, req.OutputDir) || overlaps(req.OutputDir, req.DockerSocket) || overlaps(req.DockerSocket, req.OutputDir) {
		return usageErr(CodeInvalidOutputPath, "request", "output path must not overlap trusted Git or Docker socket paths", nil)
	}
	if err := validateCommitID(req.BaseCommitID, CodeInvalidBaseCommit); err != nil {
		return err
	}
	if err := validateCommitID(req.HeadCommitID, CodeInvalidHeadCommit); err != nil {
		return err
	}
	if err := validateRunID(req.RunID); err != nil {
		return err
	}
	if req.CreatedAt.IsZero() || req.CreatedAt.Location() != time.UTC || req.CreatedAt.UTC().Round(0) != req.CreatedAt {
		return usageErr(CodeInvalidCreatedAt, "request", "created-at must be explicit UTC without monotonic data", nil)
	}
	if req.EvaluatedAt.IsZero() || req.EvaluatedAt.Location() != time.UTC || req.EvaluatedAt.UTC().Round(0) != req.EvaluatedAt {
		return usageErr(CodeInvalidEvaluatedAt, "request", "evaluated-at must be explicit UTC without monotonic data", nil)
	}
	if req.Acknowledgement == (dockerdev.UnsafeDevelopmentAcknowledgement{}) {
		return usageErr(CodeAcknowledgementInvalid, "request", "unsafe-development acknowledgement is required", nil)
	}
	return nil
}

func validatePathSyntax(p string, code ErrorCode, label string) error {
	if p == "" || len(p) > MaxOutputPathBytes || !utf8.ValidString(p) || strings.ContainsRune(p, 0) || containsControl(p) {
		return usageErr(code, "request", label+" is invalid", nil)
	}
	if !filepath.IsAbs(p) {
		return usageErr(code, "request", label+" must be absolute", nil)
	}
	if filepath.Clean(p) != p {
		return usageErr(code, "request", label+" must be lexically clean", nil)
	}
	return nil
}

func validateOutputPath(p string) error {
	if p == string(filepath.Separator) || filepath.Base(p) == "." || filepath.Base(p) == string(filepath.Separator) {
		return usageErr(CodeInvalidOutputPath, "request", "output path must name a new directory", nil)
	}
	if _, err := os.Lstat(p); err == nil {
		return usageErr(CodeOutputAlreadyExists, "request", "output path already exists", nil)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return usageErr(CodeInvalidOutputPath, "request", "cannot inspect output path", err)
	}
	parent := filepath.Dir(p)
	st, err := os.Lstat(parent)
	if err != nil {
		return usageErr(CodeOutputParentInvalid, "request", "output parent is invalid", err)
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
		return usageErr(CodeOutputParentInvalid, "request", "output parent must be a real directory", nil)
	}
	return nil
}

func validateCommitID(id string, code ErrorCode) error {
	if (len(id) != 40 && len(id) != 64) || !utf8.ValidString(id) || strings.ContainsRune(id, 0) || containsControl(id) {
		return usageErr(code, "request", "commit must be a full lowercase object id", nil)
	}
	for _, r := range id {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return usageErr(code, "request", "commit must be lowercase hexadecimal", nil)
	}
	return nil
}

func validateRunID(id string) error {
	if id == "" || len(id) > MaxRunIDBytes || !utf8.ValidString(id) || strings.ContainsRune(id, 0) || containsControl(id) {
		return usageErr(CodeInvalidRunID, "request", "run id is invalid", nil)
	}
	for i, r := range id {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || (i > 0 && (r == '.' || r == '_' || r == '-'))
		if !ok {
			return usageErr(CodeInvalidRunID, "request", "run id must match lowercase safe syntax", nil)
		}
	}
	return nil
}

func overlaps(parent, child string) bool {
	if parent == child {
		return true
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != "." && rel != "" && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

func containsControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
