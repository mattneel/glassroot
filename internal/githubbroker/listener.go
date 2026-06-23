package githubbroker

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unicode/utf8"
)

type UnixListener struct {
	*net.UnixListener
	path string
	info os.FileInfo
}

func ListenUnix(path string, limits Limits) (*UnixListener, error) {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := validateLimits(limits); err != nil {
		return nil, err
	}
	if err := validateSocketPath(path, limits.MaxPathBytes); err != nil {
		return nil, errCode(CodeInvalidListenerPath, "listener", "listener path rejected", nil)
	}
	parent := filepath.Dir(path)
	pst, err := os.Lstat(parent)
	if err != nil {
		return nil, wrap(CodeListenerParentInvalid, "listener", "listener parent rejected", err)
	}
	if pst.Mode()&os.ModeSymlink != 0 || !pst.IsDir() {
		return nil, errCode(CodeListenerParentInvalid, "listener", "listener parent rejected", nil)
	}
	if runtime.GOOS != "linux" {
		return nil, errCode(CodeUnsupportedPlatform, "listener", "linux required", nil)
	}
	if pst.Mode().Perm() != 0o700 {
		return nil, errCode(CodeListenerParentInvalid, "listener", "listener parent mode rejected", nil)
	}
	if sys, ok := pst.Sys().(*syscall.Stat_t); ok && sys.Uid != uint32(os.Geteuid()) {
		return nil, errCode(CodeListenerParentInvalid, "listener", "listener parent owner rejected", nil)
	}
	if _, err := os.Lstat(path); err == nil {
		return nil, errCode(CodeListenerPathExists, "listener", "listener path exists", nil)
	} else if !os.IsNotExist(err) {
		return nil, wrap(CodeInvalidListenerPath, "listener", "listener path rejected", err)
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, wrap(CodeListenerCreateFailed, "listener", "listener create failed", err)
	}
	ln.SetUnlinkOnClose(false)
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		_ = os.Remove(path)
		return nil, wrap(CodeListenerCreateFailed, "listener", "listener chmod failed", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		_ = ln.Close()
		_ = os.Remove(path)
		return nil, wrap(CodeListenerCreateFailed, "listener", "listener stat failed", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		_ = ln.Close()
		_ = os.Remove(path)
		return nil, errCode(CodeListenerCreateFailed, "listener", "listener is not socket", nil)
	}
	return &UnixListener{UnixListener: ln, path: path, info: info}, nil
}
func (l *UnixListener) CloseAndRemove() error {
	var first error
	if l == nil {
		return nil
	}
	if l.UnixListener != nil {
		if err := l.UnixListener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			first = wrap(CodeShutdownFailed, "listener", "listener close failed", err)
		}
	}
	if st, err := os.Lstat(l.path); err == nil {
		if os.SameFile(st, l.info) {
			if err := os.Remove(l.path); err != nil && first == nil {
				first = wrap(CodeCleanupFailed, "listener", "listener cleanup failed", err)
			}
		}
	} else if !os.IsNotExist(err) && first == nil {
		first = wrap(CodeCleanupFailed, "listener", "listener cleanup stat failed", err)
	}
	return first
}
func validateSocketPath(path string, max int) error {
	if path == "" || len(path) > max || !utf8.ValidString(path) || hasControl(path) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errCode(CodeInvalidListenerPath, "path", "path rejected", nil)
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
