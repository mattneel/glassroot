package gitstore

import (
	"path"
	"strings"
	"unicode/utf8"
)

func ValidateGitTreePath(value string) error {
	if value == "" {
		return pathErr(CodeInvalidTreePath, "tree", "path", value, "path is empty", nil)
	}
	if len(value) > MaxTreePathBytes {
		return pathErr(CodeInvalidTreePath, "tree", "path", value, "path exceeds byte limit", nil)
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) || containsControl(value) {
		return pathErr(CodeInvalidTreePath, "tree", "path", value, "path must be valid UTF-8 without control characters", nil)
	}
	if strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return pathErr(CodeInvalidTreePath, "tree", "path", value, "path must be relative slash-separated form", nil)
	}
	if path.Clean(value) != value {
		return pathErr(CodeInvalidTreePath, "tree", "path", value, "path must already be clean", nil)
	}
	parts := strings.Split(value, "/")
	if len(parts) > MaxTreeDepth {
		return pathErr(CodeInvalidTreePath, "tree", "path", value, "path depth exceeds limit", nil)
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return pathErr(CodeInvalidTreePath, "tree", "path", value, "path contains invalid component", nil)
		}
		if len(part) > MaxTreePathComponentBytes {
			return pathErr(CodeInvalidTreePath, "tree", "path", value, "path component exceeds limit", nil)
		}
		if strings.EqualFold(part, ".git") {
			return pathErr(CodeInvalidTreePath, "tree", "path", value, ".git path component is unsupported", nil)
		}
	}
	return nil
}

func validateRequestedPath(value string) error { return ValidateGitTreePath(value) }
