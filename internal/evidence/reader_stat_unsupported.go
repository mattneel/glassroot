//go:build !linux

package evidence

import "os"

func fileInfoIdentityPlatform(info os.FileInfo) (statIdentity, error) {
	return statIdentity{}, errCode(CodeUnsupportedPlatform, "platform", "stat", "strict evidence verification is initially supported only on Linux", nil)
}
