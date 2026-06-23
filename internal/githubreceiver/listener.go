package githubreceiver

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

type UnixListener struct {
	*net.UnixListener
	path string
	info os.FileInfo
}

func ListenUnix(path, stateDir string, limits Limits) (*UnixListener, error) {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := validatePath(path, limits.MaxPathBytes); err != nil {
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
	if runtime.GOOS == "linux" {
		if pst.Mode().Perm() != 0o700 {
			return nil, errCode(CodeListenerParentInvalid, "listener", "listener parent mode rejected", nil)
		}
		if sys, ok := pst.Sys().(*syscall.Stat_t); ok && sys.Uid != uint32(os.Geteuid()) {
			return nil, errCode(CodeListenerParentInvalid, "listener", "listener parent owner rejected", nil)
		}
	}
	if _, err := os.Lstat(path); err == nil {
		return nil, errCode(CodeListenerPathExists, "listener", "listener path exists", nil)
	} else if !os.IsNotExist(err) {
		return nil, wrap(CodeInvalidListenerPath, "listener", "listener path rejected", err)
	}
	if stateDir != "" {
		if overlap(path, stateDir) {
			return nil, errCode(CodeInvalidConfig, "listener", "state and socket paths overlap", nil)
		}
	}
	addr := &net.UnixAddr{Name: path, Net: "unix"}
	ln, err := net.ListenUnix("unix", addr)
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

func overlap(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	rel, err := filepath.Rel(b, a)
	if err == nil && rel != ".." && rel != "." && rel != "" && len(rel) >= 2 && rel[:2] != ".." {
		return true
	}
	rel, err = filepath.Rel(filepath.Dir(a), b)
	return err == nil && rel == "."
}

type limitedListener struct {
	net.Listener
	sem chan struct{}
}

func (l *limitedListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		select {
		case l.sem <- struct{}{}:
			return &limitedConn{Conn: c, release: func() { <-l.sem }}, nil
		default:
			_ = c.Close()
		}
	}
}

type limitedConn struct {
	net.Conn
	release func()
}

func (c *limitedConn) Close() error {
	err := c.Conn.Close()
	if c.release != nil {
		c.release()
		c.release = nil
	}
	return err
}
