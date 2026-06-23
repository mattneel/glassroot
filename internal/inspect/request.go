package inspect

import (
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mattneel/glassroot/internal/model"
)

const TimeFormat = "2006-01-02T15:04:05Z"
const maxCLIPathBytes = 4096

type ManifestIntegrityMode string

const (
	ManifestIntegrityExpectedDigest          ManifestIntegrityMode = "expected-manifest-digest"
	ManifestIntegrityInternalConsistencyOnly ManifestIntegrityMode = "internal-consistency-only"
)

type VerificationMode string

const (
	VerificationModeExpectedManifestDigest  VerificationMode = "expected-manifest-digest"
	VerificationModeInternalConsistencyOnly VerificationMode = "internal-consistency-only"
)

type Request struct {
	BundleDir                    string
	GitDir                       string
	BaseCommitID                 string
	HeadCommitID                 string
	EvaluatedAt                  time.Time
	ManifestIntegrityMode        ManifestIntegrityMode
	ExpectedManifestDigest       model.Digest
	AllowInternalConsistencyOnly bool
}

type CLIRequest struct {
	Request Request
	Format  string
	Help    bool
}

func ParseEvaluatedAt(s string) (time.Time, error) {
	if len(s) != len(TimeFormat) || strings.TrimSpace(s) != s {
		return time.Time{}, errCode(CodeInvalidEvaluatedAt, "request", "evaluated-at must use YYYY-MM-DDTHH:MM:SSZ", nil)
	}
	t, err := time.Parse(TimeFormat, s)
	if err != nil || t.Format(TimeFormat) != s || t.Location() != time.UTC {
		return time.Time{}, errCode(CodeInvalidEvaluatedAt, "request", "evaluated-at must use exact UTC form", err)
	}
	return t.UTC().Round(0), nil
}

func ParseCLIArguments(args []string) (CLIRequest, error) {
	var out CLIRequest
	out.Format = "terminal"
	seen := map[string]struct{}{}
	positionals := []string{}
	if len(args) == 1 && args[0] == "--help" {
		out.Help = true
		return out, nil
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "" {
			return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "empty argument", nil)
		}
		if !strings.HasPrefix(arg, "--") {
			positionals = append(positionals, arg)
			continue
		}
		name := arg
		value := ""
		hasValue := false
		if idx := strings.IndexByte(arg, '='); idx >= 0 {
			name = arg[:idx]
			value = arg[idx+1:]
			hasValue = true
		}
		if name == "--allow-internal-consistency-only" {
			if hasValue {
				return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "allow-internal-consistency-only takes no value", nil)
			}
			if _, ok := seen[name]; ok {
				return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "duplicate flag", nil)
			}
			seen[name] = struct{}{}
			out.Request.ManifestIntegrityMode = ManifestIntegrityInternalConsistencyOnly
			out.Request.AllowInternalConsistencyOnly = true
			continue
		}
		if !isKnownValueFlag(name) {
			return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "unknown flag", nil)
		}
		if _, ok := seen[name]; ok {
			return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "duplicate flag", nil)
		}
		seen[name] = struct{}{}
		if !hasValue {
			i++
			if i >= len(args) {
				return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "missing flag value", nil)
			}
			value = args[i]
		}
		switch name {
		case "--git-dir":
			out.Request.GitDir = value
		case "--base-commit":
			out.Request.BaseCommitID = value
		case "--head-commit":
			out.Request.HeadCommitID = value
		case "--evaluated-at":
			t, err := ParseEvaluatedAt(value)
			if err != nil {
				return CLIRequest{}, err
			}
			out.Request.EvaluatedAt = t
		case "--expected-manifest-digest":
			out.Request.ManifestIntegrityMode = ManifestIntegrityExpectedDigest
			out.Request.ExpectedManifestDigest = model.Digest(value)
		case "--format":
			out.Format = value
		}
	}
	if len(positionals) != 1 {
		return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "exactly one evidence directory is required", nil)
	}
	out.Request.BundleDir = positionals[0]
	if out.Format != "terminal" && out.Format != "markdown" && out.Format != "json" {
		return CLIRequest{}, usageErr(CodeInvalidRequest, "cli", "format must be terminal, markdown, or json", nil)
	}
	if err := ValidateRequest(out.Request); err != nil {
		return CLIRequest{}, err
	}
	return out, nil
}

func isKnownValueFlag(name string) bool {
	switch name {
	case "--git-dir", "--base-commit", "--head-commit", "--evaluated-at", "--expected-manifest-digest", "--format":
		return true
	default:
		return false
	}
}

func ValidateRequest(req Request) error {
	if err := validateCleanAbsolutePath(req.BundleDir, CodeInvalidBundlePath, "bundle path"); err != nil {
		return err
	}
	if err := validateCleanAbsolutePath(req.GitDir, CodeInvalidGitDir, "git directory"); err != nil {
		return err
	}
	if err := validateCommitID(req.BaseCommitID, CodeInvalidBaseCommit, "base commit"); err != nil {
		return err
	}
	if err := validateCommitID(req.HeadCommitID, CodeInvalidHeadCommit, "head commit"); err != nil {
		return err
	}
	if err := validateEvaluatedAt(req.EvaluatedAt); err != nil {
		return err
	}
	hasExpected := req.ManifestIntegrityMode == ManifestIntegrityExpectedDigest || req.ExpectedManifestDigest != ""
	hasInternal := req.ManifestIntegrityMode == ManifestIntegrityInternalConsistencyOnly || req.AllowInternalConsistencyOnly
	if !hasExpected && !hasInternal {
		return usageErr(CodeIntegrityModeRequired, "request", "expected manifest digest or explicit internal-consistency-only mode is required", nil)
	}
	if hasExpected && hasInternal {
		return usageErr(CodeConflictingIntegrityMode, "request", "manifest integrity modes are mutually exclusive", nil)
	}
	switch req.ManifestIntegrityMode {
	case ManifestIntegrityExpectedDigest:
		if !validDigest(req.ExpectedManifestDigest) {
			return usageErr(CodeInvalidManifestDigest, "request", "expected manifest digest must be sha256 lowercase hex", nil)
		}
	case ManifestIntegrityInternalConsistencyOnly:
		if req.ExpectedManifestDigest != "" {
			return usageErr(CodeConflictingIntegrityMode, "request", "expected manifest digest conflicts with internal-consistency-only", nil)
		}
	default:
		return usageErr(CodeIntegrityModeRequired, "request", "unknown manifest integrity mode", nil)
	}
	return nil
}

func validateCleanAbsolutePath(p string, code ErrorCode, label string) error {
	if p == "" || len(p) > maxCLIPathBytes || !utf8.ValidString(p) || strings.ContainsRune(p, 0) || containsControl(p) {
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

func validateCommitID(id string, code ErrorCode, label string) error {
	if (len(id) != 40 && len(id) != 64) || !utf8.ValidString(id) || strings.ContainsRune(id, 0) || containsControl(id) {
		return usageErr(code, "request", label+" must be a full lowercase object id", nil)
	}
	for _, r := range id {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return usageErr(code, "request", label+" must be lowercase hexadecimal", nil)
	}
	return nil
}

func validateEvaluatedAt(t time.Time) error {
	if t.IsZero() || t.Location() != time.UTC || t.UTC().Round(0) != t {
		return usageErr(CodeInvalidEvaluatedAt, "request", "evaluated-at must be explicit UTC without monotonic data", nil)
	}
	return nil
}

func validDigest(d model.Digest) bool {
	s := string(d)
	if len(s) != len("sha256:")+64 || !strings.HasPrefix(s, "sha256:") || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s[7:] {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}

func containsControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
