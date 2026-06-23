package githubreceiver

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unicode/utf8"

	"github.com/mattneel/glassroot/internal/githubapp"
)

func LoadWebhookSecrets(currentPath, previousPath string, limits Limits) (githubapp.WebhookSecrets, error) {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if currentPath == "" {
		return githubapp.WebhookSecrets{}, errCode(CodeInvalidSecretPath, "secret", "current secret path required", nil)
	}
	if previousPath != "" && currentPath == previousPath {
		return githubapp.WebhookSecrets{}, errCode(CodeDuplicateSecret, "secret", "secret paths must differ", nil)
	}
	current, err := readSecretFile(currentPath, limits)
	if err != nil {
		return githubapp.WebhookSecrets{}, err
	}
	var previous []byte
	if previousPath != "" {
		previous, err = readSecretFile(previousPath, limits)
		if err != nil {
			zero(current)
			return githubapp.WebhookSecrets{}, err
		}
		if bytes.Equal(current, previous) {
			zero(current)
			zero(previous)
			return githubapp.WebhookSecrets{}, errCode(CodeDuplicateSecret, "secret", "current and previous secrets must differ", nil)
		}
	}
	return githubapp.WebhookSecrets{Current: current, Previous: previous}, nil
}

func readSecretFile(path string, limits Limits) ([]byte, error) {
	if err := validatePath(path, limits.MaxPathBytes); err != nil {
		return nil, errCode(CodeInvalidSecretPath, "secret", "secret path rejected", nil)
	}
	before, err := os.Lstat(path)
	if err != nil {
		return nil, wrap(CodeInvalidSecretPath, "secret", "secret file rejected", err)
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return nil, errCode(CodeSecretSymlink, "secret", "secret symlink rejected", nil)
	}
	if !before.Mode().IsRegular() {
		return nil, errCode(CodeInvalidSecretPath, "secret", "secret file rejected", nil)
	}
	if before.Mode().Perm() != 0o400 && before.Mode().Perm() != 0o600 {
		return nil, errCode(CodeSecretModeInvalid, "secret", "secret mode rejected", nil)
	}
	if before.Size() < int64(limits.GitHub.MinWebhookSecretBytes) || before.Size() > int64(limits.GitHub.MaxWebhookSecretBytes) {
		return nil, errCode(CodeSecretSizeInvalid, "secret", "secret size rejected", nil)
	}
	if runtime.GOOS == "linux" {
		if sys, ok := before.Sys().(*syscall.Stat_t); ok {
			if sys.Nlink != 1 {
				return nil, errCode(CodeInvalidSecretPath, "secret", "secret link count rejected", nil)
			}
			if sys.Uid != uint32(os.Geteuid()) {
				return nil, errCode(CodeSecretOwnerInvalid, "secret", "secret owner rejected", nil)
			}
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, wrap(CodeSecretReadFailed, "secret", "secret open failed", err)
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, wrap(CodeSecretReadFailed, "secret", "secret stat failed", err)
	}
	if !os.SameFile(before, opened) {
		return nil, errCode(CodeSecretReadFailed, "secret", "secret changed", nil)
	}
	buf := make([]byte, before.Size())
	if _, err := io.ReadFull(f, buf); err != nil {
		zero(buf)
		return nil, wrap(CodeSecretReadFailed, "secret", "secret read failed", err)
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || after.Size() != before.Size() || after.Mode() != before.Mode() {
		zero(buf)
		return nil, errCode(CodeSecretReadFailed, "secret", "secret changed", nil)
	}
	return append([]byte(nil), buf...), nil
}

func validatePath(path string, max int) error {
	if path == "" || len(path) > max || !utf8.ValidString(path) || hasControl(path) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errCode(CodeInvalidConfig, "path", "path rejected", nil)
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
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
func ZeroSecrets(s githubapp.WebhookSecrets) { zero(s.Current); zero(s.Previous) }
