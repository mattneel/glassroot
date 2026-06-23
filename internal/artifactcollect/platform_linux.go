//go:build linux

package artifactcollect

import (
	"context"
	"os"
	"syscall"
)

func (c *Collector) BindWorkspace(ctx context.Context, rootPath string) (*BoundWorkspace, error) {
	if err := checkContext(ctx, "bind"); err != nil {
		return nil, err
	}
	if err := validateWorkspacePath(rootPath, c.limits); err != nil {
		return nil, err
	}
	preInfo, err := os.Lstat(rootPath)
	if err != nil {
		return nil, errCode(CodeWorkspaceOpenFailed, "bind", "", "lstat workspace", err)
	}
	if preInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errCode(CodeWorkspaceSymlink, "bind", "", "workspace final component is a symlink", nil)
	}
	if !preInfo.IsDir() {
		return nil, errCode(CodeWorkspaceNotDirectory, "bind", "", "workspace is not a directory", nil)
	}
	if preInfo.Mode().Perm() != 0o700 {
		return nil, errCode(CodeWorkspaceModeInvalid, "bind", "", "workspace mode must be 0700", nil)
	}
	preID, err := identityFromInfo(preInfo)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, errCode(CodeWorkspaceOpenFailed, "bind", "", "open workspace root", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = root.Close()
		}
	}()
	openedInfo, err := root.Stat(".")
	if err != nil {
		return nil, errCode(CodeWorkspaceOpenFailed, "bind", "", "stat opened workspace root", err)
	}
	openedID, err := identityFromInfo(openedInfo)
	if err != nil {
		return nil, err
	}
	if !sameFileIdentity(preID, openedID) || openedInfo.Mode().Perm() != 0o700 {
		return nil, errCode(CodeWorkspaceChanged, "bind", "", "workspace identity changed while opening", nil)
	}
	postInfo, err := os.Lstat(rootPath)
	if err != nil {
		return nil, errCode(CodeWorkspaceChanged, "bind", "", "lstat workspace after open", err)
	}
	postID, err := identityFromInfo(postInfo)
	if err != nil {
		return nil, err
	}
	if !sameFileIdentity(preID, postID) || postInfo.Mode().Perm() != 0o700 {
		return nil, errCode(CodeWorkspaceChanged, "bind", "", "workspace identity changed after opening", nil)
	}
	ok = true
	return &BoundWorkspace{root: root, path: rootPath, identity: openedID, limits: c.limits, hooks: c.hooks}, nil
}

type fileIdentity struct {
	Dev       uint64
	Ino       uint64
	Mode      os.FileMode
	Nlink     uint64
	Size      int64
	ModSec    int64
	ModNSec   int64
	CTimeSec  int64
	CTimeNSec int64
}

func identityFromInfo(info os.FileInfo) (fileIdentity, error) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return fileIdentity{}, errCode(CodeWorkspaceOpenFailed, "identity", "", "linux stat identity unavailable", nil)
	}
	return fileIdentity{Dev: uint64(st.Dev), Ino: uint64(st.Ino), Mode: info.Mode(), Nlink: uint64(st.Nlink), Size: info.Size(), ModSec: st.Mtim.Sec, ModNSec: st.Mtim.Nsec, CTimeSec: st.Ctim.Sec, CTimeNSec: st.Ctim.Nsec}, nil
}

func sameFileIdentity(a, b fileIdentity) bool { return a.Dev == b.Dev && a.Ino == b.Ino }

func sameStableIdentity(a, b fileIdentity) bool {
	return sameFileIdentity(a, b) && a.Mode == b.Mode && a.Nlink == b.Nlink && a.Size == b.Size && a.ModSec == b.ModSec && a.ModNSec == b.ModNSec && a.CTimeSec == b.CTimeSec && a.CTimeNSec == b.CTimeNSec
}
