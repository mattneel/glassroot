package evidence

import (
	"io/fs"
	"os"
	"time"
)

type statIdentity struct {
	dev   uint64
	ino   uint64
	nlink uint64
	mode  fs.FileMode
	size  int64
	mtime time.Time
	ctime time.Time
}

func sameIdentity(a, b statIdentity) bool { return a.dev == b.dev && a.ino == b.ino }
func sameStableFile(a, b statIdentity) bool {
	return sameIdentity(a, b) && a.nlink == b.nlink && a.mode == b.mode && a.size == b.size && a.mtime.Equal(b.mtime) && a.ctime.Equal(b.ctime)
}

func fileInfoIdentity(info os.FileInfo) (statIdentity, error) { return fileInfoIdentityPlatform(info) }
