package evidence

import (
	"path"
	"strings"
	"unicode/utf8"
)

func ValidateEvidenceEntryPath(p string) error {
	if !validRelativePath(p, MaxPhysicalEntryPathBytes, false) {
		return pathErr(CodeInvalidEntryPath, "path", "validate", p, "invalid bundle entry path", nil)
	}
	return nil
}

func ValidateLogicalArtifactPath(p string) error {
	if p == "" || len(p) > MaxLogicalPathBytes || !utf8.ValidString(p) || strings.ContainsRune(p, 0) || containsControl(p) || strings.Contains(p, "\\") {
		return pathErr(CodeInvalidArtifact, "artifact", "logical-path", p, "invalid logical artifact path", nil)
	}
	clean := path.Clean(p)
	if clean != p {
		return pathErr(CodeInvalidArtifact, "artifact", "logical-path", p, "logical artifact path must be clean", nil)
	}
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return pathErr(CodeInvalidArtifact, "artifact", "logical-path", p, "invalid logical artifact path segment", nil)
		}
	}
	return nil
}

func validScenarioID(id string) bool {
	if id == "" || len(id) > 64 || !utf8.ValidString(id) {
		return false
	}
	for i, r := range id {
		if i == 0 {
			if r < 'a' || r > 'z' {
				return false
			}
			continue
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func validRelativePath(p string, max int, allowDot bool) bool {
	if p == "" || len(p) > max || !utf8.ValidString(p) || strings.HasPrefix(p, "/") || strings.ContainsRune(p, 0) || containsControl(p) || strings.Contains(p, "\\") {
		return false
	}
	if p == "." {
		return allowDot
	}
	if path.Clean(p) != p {
		return false
	}
	parts := strings.Split(p, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func attemptDir(key AttemptKey) string {
	return "attempts/" + string(key.Revision) + "/" + key.ScenarioID + "/" + repetitionDir(key.Repetition)
}
func repetitionDir(n uint32) string { return "repetition-" + zeroPad4(n) }
func zeroPad4(n uint32) string {
	if n > 9999 {
		return "repetition-overflow"
	}
	s := "0000" + uintToString(n)
	return s[len(s)-4:]
}
func uintToString(n uint32) string {
	if n == 0 {
		return "0"
	}
	var b [10]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func containsControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
