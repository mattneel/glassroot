package materialize

import (
	"path"
	"strings"
	"unicode/utf8"
)

func validateMaterializationPath(value string, limits Limits) error {
	if value == "" {
		return pathErr(CodeInvalidTreeEntry, "preflight", "path", value, "path is empty", nil)
	}
	if len(value) > limits.MaxPathBytes {
		return pathErr(CodeInvalidTreeEntry, "preflight", "path", value, "path exceeds byte limit", nil)
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) || containsControl(value) {
		return pathErr(CodeInvalidTreeEntry, "preflight", "path", value, "path must be valid UTF-8 without control characters", nil)
	}
	if strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return pathErr(CodeInvalidTreeEntry, "preflight", "path", value, "path must be relative slash-separated form", nil)
	}
	if path.Clean(value) != value {
		return pathErr(CodeInvalidTreeEntry, "preflight", "path", value, "path must already be clean", nil)
	}
	parts := strings.Split(value, "/")
	if len(parts) > limits.MaxPathDepth {
		return pathErr(CodeInvalidTreeEntry, "preflight", "path", value, "path depth exceeds limit", nil)
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return pathErr(CodeInvalidTreeEntry, "preflight", "path", value, "path contains invalid component", nil)
		}
		if len(part) > limits.MaxPathComponentBytes {
			return pathErr(CodeInvalidTreeEntry, "preflight", "path", value, "path component exceeds limit", nil)
		}
		if strings.EqualFold(part, ".git") {
			return pathErr(CodeInvalidTreeEntry, "preflight", "path", value, ".git path component is unsupported", nil)
		}
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

func hasDotGitComponent(value string) bool {
	for _, part := range strings.Split(value, "/") {
		if strings.EqualFold(part, ".git") {
			return true
		}
	}
	return false
}

func pathDepth(value string) int {
	if value == "" {
		return 0
	}
	return len(strings.Split(value, "/"))
}
