//go:build !linux

package githubbroker

import "net"

func checkPeer(*net.UnixConn) error {
	return errCode(CodeUnsupportedPlatform, "peer", "linux peer credentials required", nil)
}
