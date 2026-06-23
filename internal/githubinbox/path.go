package githubinbox

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unicode/utf8"
)

func validateStateDir(path string, limits Limits) error {
	if path == "" || len(path) > limits.MaxStateDirBytes || !utf8.ValidString(path) || hasControl(path) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errCode(CodeInvalidStateDir, "state-dir", "state directory path rejected", nil)
	}
	st, err := os.Lstat(path)
	if err != nil {
		return wrap(CodeInvalidStateDir, "state-dir", "state directory rejected", err)
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
		return errCode(CodeInvalidStateDir, "state-dir", "state directory rejected", nil)
	}
	if runtime.GOOS == "linux" {
		if st.Mode().Perm() != 0o700 {
			return errCode(CodeInvalidStateDir, "state-dir", "state directory mode rejected", nil)
		}
		if sys, ok := st.Sys().(*syscall.Stat_t); ok && sys.Uid != uint32(os.Geteuid()) {
			return errCode(CodeInvalidStateDir, "state-dir", "state directory owner rejected", nil)
		}
	}
	return nil
}

func validateReceiverID(id string) error {
	if id == "" || len(id) > 64 || id[0] < 'a' || id[0] > 'z' || hasControl(id) {
		return errCode(CodeRecordInvalid, "receiver", "receiver id rejected", nil)
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return errCode(CodeRecordInvalid, "receiver", "receiver id rejected", nil)
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
