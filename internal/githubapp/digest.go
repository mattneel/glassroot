package githubapp

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

func DigestRawBody(body []byte) string {
	s := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(s[:])
}

func domainHash(domain string, fields ...string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(domain))
	for _, f := range fields {
		var lenbuf [8]byte
		binary.BigEndian.PutUint64(lenbuf[:], uint64(len(f)))
		_, _ = h.Write(lenbuf[:])
		_, _ = h.Write([]byte(f))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func prefixedID(prefix, domain string, fields ...string) string {
	return fmt.Sprintf("%s-%s", prefix, domainHash(domain, fields...))
}

func validateDigest(s string) bool {
	if len(s) != len("sha256:")+64 || s[:len("sha256:")] != "sha256:" {
		return false
	}
	return isLowerHex(s[len("sha256:"):], 64)
}

func isLowerHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, r := range s {
		if r < '0' || (r > '9' && r < 'a') || r > 'f' {
			return false
		}
	}
	return true
}
