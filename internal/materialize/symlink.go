package materialize

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"strings"
	"unicode/utf8"
)

type symlinkTargetMetadata struct {
	TargetDigest string
	ByteLength   int64
}

func validateSymlinkTarget(linkPath string, target []byte, limits Limits) (symlinkTargetMetadata, error) {
	if len(target) == 0 {
		return symlinkTargetMetadata{}, pathErr(CodeInvalidSymlinkTarget, "symlink", "target", linkPath, "symlink target is empty", nil)
	}
	if len(target) > limits.MaxSymlinkTargetBytes {
		return symlinkTargetMetadata{}, pathErr(CodeInvalidSymlinkTarget, "symlink", "target", linkPath, "symlink target exceeds byte limit", nil)
	}
	if !utf8.Valid(target) {
		return symlinkTargetMetadata{}, pathErr(CodeInvalidSymlinkTarget, "symlink", "target", linkPath, "symlink target is not valid UTF-8", nil)
	}
	t := string(target)
	if strings.ContainsRune(t, 0) || containsControl(t) || strings.Contains(t, "\\") {
		return symlinkTargetMetadata{}, pathErr(CodeInvalidSymlinkTarget, "symlink", "target", linkPath, "symlink target contains unsupported characters", nil)
	}
	if strings.HasPrefix(t, "/") {
		return symlinkTargetMetadata{}, pathErr(CodeInvalidSymlinkTarget, "symlink", "target", linkPath, "absolute symlink target is unsupported", nil)
	}
	if path.Clean(t) != t {
		return symlinkTargetMetadata{}, pathErr(CodeInvalidSymlinkTarget, "symlink", "target", linkPath, "symlink target must already be clean", nil)
	}
	resolved := path.Clean(path.Join(path.Dir(linkPath), t))
	if strings.HasPrefix(resolved, "/") || resolved == ".." || strings.HasPrefix(resolved, "../") || hasDotGitComponent(resolved) {
		return symlinkTargetMetadata{}, pathErr(CodeInvalidSymlinkTarget, "symlink", "target", linkPath, "symlink target escapes the workspace", nil)
	}
	if resolved == "." {
		return symlinkTargetMetadata{}, pathErr(CodeInvalidSymlinkTarget, "symlink", "target", linkPath, "symlink target resolves to workspace root", nil)
	}
	if err := validateMaterializationPath(resolved, limits); err != nil {
		return symlinkTargetMetadata{}, pathErr(CodeInvalidSymlinkTarget, "symlink", "target", linkPath, "resolved symlink target is invalid", err)
	}
	sum := sha256.Sum256(target)
	return symlinkTargetMetadata{TargetDigest: "sha256:" + hex.EncodeToString(sum[:]), ByteLength: int64(len(target))}, nil
}
