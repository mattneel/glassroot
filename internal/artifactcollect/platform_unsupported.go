//go:build !linux

package artifactcollect

import (
	"context"
	"os"
)

func (c *Collector) BindWorkspace(ctx context.Context, rootPath string) (*BoundWorkspace, error) {
	return nil, errCode(CodeUnsupportedPlatform, "bind", "", "artifact collection is supported only on Linux", nil)
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
	return fileIdentity{}, errCode(CodeUnsupportedPlatform, "identity", "", "artifact collection is supported only on Linux", nil)
}
func sameFileIdentity(a, b fileIdentity) bool   { return false }
func sameStableIdentity(a, b fileIdentity) bool { return false }
