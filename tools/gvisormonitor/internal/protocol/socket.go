package protocol

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func ValidateMonitorSocketPath(socketPath string, limits Limits) error {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if socketPath == "" || int64(len(socketPath)) > limits.MaxStringBytes || !utf8.ValidString(socketPath) || strings.ContainsRune(socketPath, '\x00') || filepath.Clean(socketPath) != socketPath || !filepath.IsAbs(socketPath) {
		return errCode(CodeInvalidSocketPath, "socket", "path", "monitor socket path must be absolute, clean, bounded UTF-8", nil)
	}
	for _, r := range socketPath {
		if r < 0x20 || r == 0x7f {
			return errCode(CodeInvalidSocketPath, "socket", "path", "monitor socket path contains controls", nil)
		}
	}
	if st, err := os.Lstat(socketPath); err == nil {
		if st.Mode()&os.ModeSymlink != 0 {
			return errCode(CodeSocketSymlink, "socket", "path", "monitor socket final component is a symlink", nil)
		}
		return errCode(CodeSocketExists, "socket", "path", "monitor socket path already exists", nil)
	} else if !errors.Is(err, os.ErrNotExist) {
		return errCode(CodeInvalidSocketPath, "socket", "path", "monitor socket path cannot be inspected", nil)
	}
	parent := filepath.Dir(socketPath)
	st, err := os.Lstat(parent)
	if err != nil {
		return errCode(CodeInvalidSocketPath, "socket", "parent", "monitor socket parent cannot be inspected", nil)
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm() != 0o700 {
		return errCode(CodeInvalidSocketPath, "socket", "parent", "monitor socket parent must be a private directory", nil)
	}
	return nil
}
