package githubsourcestore

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf8"
)

func LayoutPath(root, sourceStoreID string) (string, error) {
	if err := ValidateSourceStoreID(sourceStoreID); err != nil {
		return "", err
	}
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root || !utf8.ValidString(root) || hasControl(root) {
		return "", errCode(CodeInvalidSourceRoot, "layout", "source root rejected", nil)
	}
	hexPart := sourceStoreID[len("source-"):]
	return filepath.Join(root, "stores", "sha256", hexPart[:2], sourceStoreID), nil
}

func ValidateSourceRoot(root string, limits Limits) error {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := validateLimits(limits); err != nil {
		return err
	}
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root || !utf8.ValidString(root) || len(root) > limits.MaxSourceRootPathBytes || hasControl(root) {
		return errCode(CodeInvalidSourceRoot, "open", "source root rejected", nil)
	}
	st, err := os.Lstat(root)
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return wrap(CodeInvalidSourceRoot, "open", "source root rejected", err)
	}
	if runtime.GOOS == "linux" {
		if st.Mode().Perm() != 0o700 {
			return errCode(CodeSourceRootModeInvalid, "open", "source root mode rejected", nil)
		}
		if sys, ok := st.Sys().(*syscall.Stat_t); ok && sys.Uid != uint32(os.Geteuid()) {
			return errCode(CodeSourceRootModeInvalid, "open", "source root owner rejected", nil)
		}
	}
	return nil
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func RepositoryPath(storePath string) string { return filepath.Join(storePath, "repository.git") }
func MetadataPath(storePath string) string   { return filepath.Join(storePath, "source.json") }

func IsSafeRouteSegment(s string) bool {
	if s == "" || len(s) > 256 || s == "." || s == ".." || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == '/' || r == '\\' || r == '?' || r == '#' {
			return false
		}
	}
	return !strings.Contains(s, "..")
}
