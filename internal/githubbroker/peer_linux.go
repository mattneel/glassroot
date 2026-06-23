//go:build linux

package githubbroker

import (
	"net"
	"os"
	"syscall"
)

func checkPeer(c *net.UnixConn) error {
	raw, err := c.SyscallConn()
	if err != nil {
		return wrap(CodePeerCredentialUnavailable, "peer", "peer credentials unavailable", err)
	}
	var cred *syscall.Ucred
	var serr error
	err = raw.Control(func(fd uintptr) {
		cred, serr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})
	if err != nil {
		return wrap(CodePeerCredentialUnavailable, "peer", "peer credentials unavailable", err)
	}
	if serr != nil {
		return wrap(CodePeerCredentialUnavailable, "peer", "peer credentials unavailable", serr)
	}
	if cred == nil {
		return errCode(CodePeerCredentialUnavailable, "peer", "peer credentials unavailable", nil)
	}
	if cred.Uid != uint32(os.Geteuid()) {
		return errCode(CodePeerUIDRejected, "peer", "peer uid rejected", nil)
	}
	return nil
}
