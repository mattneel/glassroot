//go:build linux

package evidence

import (
	"os"
	"syscall"
	"time"
)

func fileInfoIdentityPlatform(info os.FileInfo) (statIdentity, error) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return statIdentity{}, errCode(CodeUnexpectedEntryMode, "filesystem", "stat", "missing Linux stat identity", nil)
	}
	return statIdentity{dev: uint64(st.Dev), ino: uint64(st.Ino), nlink: uint64(st.Nlink), mode: info.Mode(), size: info.Size(), mtime: info.ModTime(), ctime: time.Unix(st.Ctim.Sec, st.Ctim.Nsec)}, nil
}
